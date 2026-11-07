package bridge

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const localRuntimeVersion = "0.7.3"
const localWheel = "omlx-0.7.3-cp312-cp312-macosx_15_0_universal2.whl"
const localWheelHash = "ea314fd5f49e72a801a03f3775512a1bdf3e1bc5ba670f25f69cff27899efd40"
const localUVHash = "81b811b57e4624df7ed3aac148e0359100d9962f3d9dce00aec533dc2e086731"

//go:embed local_support.py
var localSupport string

func (l *localManager) environment() []string {
	env := []string{}
	for _, e := range os.Environ() {
		key, _, _ := strings.Cut(e, "=")
		if strings.HasPrefix(key, "PYTHON") || strings.HasPrefix(key, "UV_") || strings.HasPrefix(key, "HF_") || strings.HasPrefix(key, "OMLX_") || strings.HasPrefix(key, "ANTHROPIC_") || strings.HasPrefix(key, "OPENAI_") || strings.HasPrefix(key, "GITHUB_") || key == "GH_TOKEN" || key == "VIRTUAL_ENV" {
			continue
		}
		env = append(env, e)
	}
	return append(env, "PYTHONNOUSERSITE=1", "PYTHONUNBUFFERED=1", "HF_HUB_DISABLE_IMPLICIT_TOKEN=1", "HF_HUB_DISABLE_TELEMETRY=1", "DO_NOT_TRACK=1", "HF_HOME="+l.path("hf-cache"), "UV_CACHE_DIR="+l.path("package-cache"), "UV_PYTHON_INSTALL_DIR="+l.path("python"), "UV_PYTHON_PREFERENCE=only-managed", "UV_NO_CONFIG=1", "SETUPTOOLS_SCM_PRETEND_VERSION_FOR_MLX_LM=0.32.3.dev0+g0d4d3d04", "UV_PYTHON_BIN_DIR="+l.path("python-bin"))
}
func (l *localManager) command(ctx context.Context, name string, args ...string) *exec.Cmd {
	c := exec.CommandContext(ctx, name, args...)
	c.Env = l.environment()
	configureLocalCommand(c)
	return c
}
func (l *localManager) python() string {
	return l.path("runtime-"+localRuntimeVersion, "bin", "python3")
}
func (l *localManager) run(ctx context.Context, name string, args ...string) error {
	f, err := os.OpenFile(l.path("setup.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	c := l.command(ctx, name, args...)
	c.Stdout = f
	c.Stderr = f
	if err = c.Run(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("Local setup could not complete %s. Retry; details: %s", filepath.Base(name), l.path("setup.log"))
	}
	return nil
}
func (l *localManager) prepare(ctx context.Context, m localModel) error {
	if err := os.MkdirAll(l.path("downloads"), 0700); err != nil {
		return err
	}
	if err := l.installRuntime(ctx); err != nil {
		return err
	}
	if err := writeAtomic(l.path("support.py"), []byte(localSupport)); err != nil {
		return err
	}
	marker := l.path("models", m.ID, ".kobashi-ready")
	if b, _ := os.ReadFile(marker); strings.TrimSpace(string(b)) == m.Revision {
		return nil
	}
	if free, err := localFreeDisk(l.path()); err != nil {
		return err
	} else if free < m.Bytes+(2<<30) {
		return errors.New("Not enough disk space to download this model")
	}
	l.progress("Downloading", 0, m.Bytes)
	c := l.command(ctx, l.python(), l.path("support.py"), "download", m.Repo, m.Revision, l.path("models", m.ID))
	pipe, err := c.StdoutPipe()
	if err != nil {
		return err
	}
	log, err := os.OpenFile(l.path("setup.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer log.Close()
	c.Stderr = log
	if err = c.Start(); err != nil {
		return err
	}
	scan := bufio.NewScanner(pipe)
	for scan.Scan() {
		var p struct{ Done, Total int64 }
		if json.Unmarshal(scan.Bytes(), &p) == nil && p.Total > 0 {
			l.progress("Downloading", p.Done, p.Total)
		}
	}
	if err = c.Wait(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("Model download failed. Retry to continue the download")
	}
	return writeAtomic(marker, []byte(m.Revision))
}
func (l *localManager) installRuntime(ctx context.Context) error {
	marker := l.path("runtime-"+localRuntimeVersion, ".kobashi-ready")
	if b, _ := os.ReadFile(marker); strings.TrimSpace(string(b)) == localWheelHash {
		return nil
	}
	uv := l.path("uv")
	if _, err := os.Stat(uv); err != nil {
		archive := l.path("downloads", "uv.tar.gz")
		if err = l.download(ctx, "https://github.com/astral-sh/uv/releases/download/0.13.7/uv-aarch64-apple-darwin.tar.gz", localUVHash, archive, "Installing"); err != nil {
			return err
		}
		if err = extractLocalUV(archive, uv); err != nil {
			return err
		}
	}
	l.progress("Installing", 0, 0)
	if err := l.run(ctx, uv, "python", "install", "3.12.12"); err != nil {
		return err
	}
	if _, err := os.Stat(l.python()); err != nil {
		if err = l.run(ctx, uv, "venv", "--python", "3.12.12", l.path("runtime-"+localRuntimeVersion)); err != nil {
			return err
		}
	}
	wheel := l.path("downloads", localWheel)
	if err := l.download(ctx, "https://github.com/jundot/omlx/releases/download/v"+localRuntimeVersion+"/"+localWheel, localWheelHash, wheel, "Installing"); err != nil {
		return err
	}
	// Archive URLs avoid requiring git or Xcode's command-line tools on a fresh Mac.
	overrides := `mlx-lm @ https://github.com/ml-explore/mlx-lm/archive/0d4d3d04f8ab7ce4a51e7209e8615755a48ba94d.zip
mlx-embeddings @ https://github.com/Blaizzy/mlx-embeddings/archive/32981fa4e8064ed664b52071789dd18271fe4206.zip
mlx-vlm @ https://github.com/Blaizzy/mlx-vlm/archive/138d7617d39653b39a5cbc379b1b5907a4481d67.zip
mlx-audio @ https://github.com/Blaizzy/mlx-audio/archive/8c244a1faff44cfa69e3151c195f52a4c1c90be5.zip
`
	overrideFile := l.path("runtime-dependencies.txt")
	if err := writeAtomic(overrideFile, []byte(overrides)); err != nil {
		return err
	}
	l.progress("Installing", 0, 0)
	if err := l.run(ctx, uv, "pip", "install", "--python", l.python(), "--override", overrideFile, wheel); err != nil {
		return err
	}
	if err := l.run(ctx, l.python(), "-c", "from omlx.model_settings import ModelSettings; ModelSettings(turboquant_kv_enabled=True, turboquant_kv_scheme='turboquant', turboquant_kv_bits=4); import omlx.server"); err != nil {
		return err
	}
	return writeAtomic(marker, []byte(localWheelHash))
}
func extractLocalUV(archive, target string) error {
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err != nil {
			return fmt.Errorf("Invalid runtime archive: %w", err)
		}
		if h.Typeflag != tar.TypeReg || filepath.Base(h.Name) != "uv" {
			continue
		}
		if h.Size > 200<<20 {
			return errors.New("Runtime executable exceeds size limit")
		}
		out, err := os.OpenFile(target+".tmp", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0700)
		if err != nil {
			return err
		}
		_, err = io.Copy(out, tr)
		closeErr := out.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
		return os.Rename(target+".tmp", target)
	}
}
func fileSHA(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	h := sha256.New()
	if _, err = io.Copy(h, f); err != nil {
		return ""
	}
	return hex.EncodeToString(h.Sum(nil))
}
func (l *localManager) download(ctx context.Context, address, checksum, target, stage string) error {
	if fileSHA(target) == checksum {
		return nil
	}
	partial := target + ".part"
	if fileSHA(partial) == checksum {
		return os.Rename(partial, target)
	}
	offset := int64(0)
	if st, err := os.Stat(partial); err == nil {
		offset = st.Size()
	}
	req, err := http.NewRequestWithContext(ctx, "GET", address, nil)
	if err != nil {
		return err
	}
	if offset > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", offset))
	}
	response, err := l.app.client.Do(req)
	if err != nil {
		return fmt.Errorf("Download failed: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusRequestedRangeNotSatisfiable {
		_ = os.Remove(partial)
	}
	if response.StatusCode != 200 && response.StatusCode != 206 {
		return fmt.Errorf("Download failed (HTTP %d)", response.StatusCode)
	}
	if response.StatusCode == 200 {
		offset = 0
	}
	f, err := os.OpenFile(partial, os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	if err = f.Truncate(offset); err != nil {
		return err
	}
	if _, err = f.Seek(offset, 0); err != nil {
		return err
	}
	total := response.ContentLength + offset
	done := offset
	buf := make([]byte, 128<<10)
	last := time.Time{}
	for {
		n, e := response.Body.Read(buf)
		if n > 0 {
			if _, err = f.Write(buf[:n]); err != nil {
				return err
			}
			done += int64(n)
			if time.Since(last) > 250*time.Millisecond {
				l.progress(stage, done, total)
				last = time.Now()
			}
		}
		if e == io.EOF {
			break
		}
		if e != nil {
			return e
		}
	}
	if err = f.Close(); err != nil {
		return err
	}
	if fileSHA(partial) != checksum {
		_ = os.Remove(partial)
		return errors.New("Runtime download checksum mismatch. Retry download")
	}
	return os.Rename(partial, target)
}
func (l *localManager) startRuntime(ctx context.Context, id string) error {
	l.mu.Lock()
	same := l.runtimeModel == id && l.endpoint != ""
	l.mu.Unlock()
	if same {
		return nil
	}
	if _, err := os.Stat(l.python()); err != nil {
		return errors.New("Local runtime is not installed. Add the model to resume setup")
	}
	if _, err := os.Stat(l.path("models", id, ".kobashi-ready")); err != nil {
		return errors.New("Local model download is incomplete. Add the model to resume setup")
	}
	l.stopRuntime()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	secret := localSecret()
	if err = l.runtimeConfig(id, port, secret); err != nil {
		return err
	}
	runCtx, cancel := context.WithCancel(l.app.ctx)
	c := l.command(runCtx, l.python(), "-m", "omlx.cli", "serve", "--base-path", l.path("server", id))
	log, err := os.OpenFile(l.path("runtime.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		cancel()
		return err
	}
	c.Stdout = log
	c.Stderr = log
	if err = c.Start(); err != nil {
		cancel()
		log.Close()
		return err
	}
	done := make(chan struct{})
	endpoint := fmt.Sprintf("http://127.0.0.1:%d", port)
	l.mu.Lock()
	l.runtime = c
	l.runtimeCancel = cancel
	l.runtimeDone = done
	l.endpoint = endpoint
	l.secret = secret
	l.runtimeModel = id
	l.mu.Unlock()
	go func() {
		_ = c.Wait()
		log.Close()
		l.mu.Lock()
		unexpected := l.runtime == c && l.app.ctx.Err() == nil
		if l.runtime == c {
			l.runtimeModel = ""
			l.endpoint = ""
		}
		if unexpected && l.app.snapshot().LocalModel == id {
			l.setup.Stage = "Error"
			l.setup.Error = "Local model stopped. Select its account to reconnect"
		}
		l.mu.Unlock()
		if unexpected && l.app.snapshot().LocalModel == id {
			l.app.report(errors.New("Local model stopped. Select its account to reconnect"))
		}
		close(done)
	}()
	l.progress("Starting", 0, 0)
	bounded, stop := context.WithTimeout(ctx, 3*time.Minute)
	defer stop()
	for {
		req, _ := http.NewRequestWithContext(bounded, "GET", endpoint+"/v1/models", nil)
		req.Header.Set("Authorization", "Bearer "+secret)
		response, e := l.client.Do(req)
		if e == nil {
			data, readErr := readResponse(response, 1<<20)
			if readErr == nil && response.StatusCode == 200 {
				var obj object
				if json.Unmarshal(data, &obj) == nil {
					for _, m := range arr(obj["data"]) {
						if str(nested(m, "id")) == id {
							return nil
						}
					}
				}
			}
		}
		select {
		case <-bounded.Done():
			l.stopRuntime()
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return errors.New("Local model startup timed out. Retry setup")
		case <-done:
			l.stopRuntime()
			return errors.New("oMLX could not start. See the local runtime log")
		case <-time.After(500 * time.Millisecond):
		}
	}
}
