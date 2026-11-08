package bridge

import (
	"archive/tar"
	"bufio"
	"bytes"
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
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const localRuntimeVersion = "0.7.3"
const localWheel = "omlx-0.7.3-cp312-cp312-macosx_15_0_universal2.whl"
const localWheelHash = "ea314fd5f49e72a801a03f3775512a1bdf3e1bc5ba670f25f69cff27899efd40"
const localUVHash = "81b811b57e4624df7ed3aac148e0359100d9962f3d9dce00aec533dc2e086731"
const localPackageIndex = "https://packagefeedproxy.microsoft.io/pypi/simple/"

//go:embed local_support.py
var localSupport string

func (l *localManager) environment() []string {
	env := []string{}
	for _, e := range os.Environ() {
		key, _, _ := strings.Cut(e, "=")
		if strings.HasPrefix(key, "PIP_") || strings.HasPrefix(key, "PYTHON") || strings.HasPrefix(key, "UV_") || strings.HasPrefix(key, "HF_") || strings.HasPrefix(key, "OMLX_") || strings.HasPrefix(key, "ANTHROPIC_") || strings.HasPrefix(key, "OPENAI_") || strings.HasPrefix(key, "GITHUB_") || key == "GH_TOKEN" || key == "VIRTUAL_ENV" {
			continue
		}
		env = append(env, e)
	}
	return append(env, "PYTHONNOUSERSITE=1", "PYTHONUNBUFFERED=1", "HF_HUB_DISABLE_IMPLICIT_TOKEN=1", "HF_HUB_DISABLE_TELEMETRY=1", "DO_NOT_TRACK=1", "HF_HOME="+l.path("hf-cache"), "UV_CACHE_DIR="+l.path("package-cache"), "UV_PYTHON_INSTALL_DIR="+l.path("python"), "UV_PYTHON_PREFERENCE=managed", "UV_NO_CONFIG=1", "UV_DEFAULT_INDEX="+localPackageIndex, "PIP_INDEX_URL="+localPackageIndex, "PIP_CONFIG_FILE="+os.DevNull, "SETUPTOOLS_SCM_PRETEND_VERSION_FOR_MLX_LM=0.32.3.dev0+g0d4d3d04", "UV_PYTHON_BIN_DIR="+l.path("python-bin"))
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
	proxy, err := configuredProxy()
	if err != nil {
		return err
	}
	hasProxy := false
	for _, key := range []string{"HTTPS_PROXY", "https_proxy", "HTTP_PROXY", "http_proxy", "ALL_PROXY", "all_proxy"} {
		hasProxy = hasProxy || os.Getenv(key) != ""
	}
	if proxy != nil && !hasProxy {
		c.Env = append(c.Env, "HTTPS_PROXY="+proxy.String(), "HTTP_PROXY="+proxy.String(), "ALL_PROXY="+proxy.String())
	}
	for i, arg := range args {
		if arg == "--default-index" && i+1 < len(args) {
			if index, err := url.Parse(args[i+1]); err == nil && index.Hostname() == "127.0.0.1" {
				bypass := os.Getenv("NO_PROXY") + "," + os.Getenv("no_proxy") + ",127.0.0.1,localhost"
				c.Env = append(c.Env, "NO_PROXY="+bypass, "no_proxy="+bypass)
			}
		}
	}
	output := &localInstallOutput{manager: l}
	c.Stdout = io.MultiWriter(f, output)
	c.Stderr = c.Stdout
	err = c.Run()
	output.flush()
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		detail := l.status().Detail
		if detail == "" {
			detail = filepath.Base(name)
		}
		if output.networkFailure {
			host := output.host
			if host == "127.0.0.1" {
				host = "packagefeedproxy.microsoft.io"
			}
			return fmt.Errorf("%s failed: could not download dependencies. Check network/proxy access to %s, then retry. Details: %s", detail, host, l.path("setup.log"))
		}
		return fmt.Errorf("%s failed. Retry; details: %s", detail, l.path("setup.log"))
	}
	return nil
}

type localInstallOutput struct {
	mu             sync.Mutex
	manager        *localManager
	pending        string
	networkFailure bool
	host           string
}

func (o *localInstallOutput) Write(p []byte) (int, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.pending += string(p)
	for {
		line, rest, ok := strings.Cut(o.pending, "\n")
		if !ok {
			break
		}
		o.pending = rest
		o.line(strings.TrimSpace(line))
	}
	if len(o.pending) > 8192 {
		o.pending = o.pending[len(o.pending)-8192:]
	}
	return len(p), nil
}
func (o *localInstallOutput) flush() {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.line(strings.TrimSpace(o.pending))
	o.pending = ""
}
func (o *localInstallOutput) line(line string) {
	if strings.HasPrefix(line, "Downloaded ") {
		o.manager.mu.Lock()
		o.manager.setup.Packages++
		o.manager.mu.Unlock()
	}
	if _, address, ok := strings.Cut(line, "Failed to fetch: "); ok {
		if u, err := url.Parse(strings.TrimSpace(address)); err == nil && u.Hostname() != "" {
			o.networkFailure, o.host = true, u.Hostname()
		}
	}
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

const localPythonCheck = "import sys; assert sys.version_info[:2] == (3, 12), 'Python 3.12 is required'"

func (l *localManager) checkInstallation(ctx context.Context, python, check string) (bool, error) {
	if _, err := os.Stat(python); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	log, err := os.OpenFile(l.path("setup.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return false, err
	}
	defer log.Close()
	c := l.command(ctx, python, "-c", check)
	c.Stdout, c.Stderr = log, log
	if err := c.Run(); err != nil {
		if ctx.Err() != nil {
			return false, ctx.Err()
		}
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return false, nil
		}
		return false, fmt.Errorf("Could not check existing Python installation: %w", err)
	}
	return true, nil
}

func (l *localManager) findPython(ctx context.Context, uv string) (string, error) {
	log, err := os.OpenFile(l.path("setup.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return "", err
	}
	defer log.Close()
	c := l.command(ctx, uv, "python", "find", "--offline", "--no-python-downloads", "--no-project", "--system", "3.12")
	var stderr strings.Builder
	c.Stderr = io.MultiWriter(log, &stderr)
	output, err := c.Output()
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		var exit *exec.ExitError
		if errors.As(err, &exit) && strings.Contains(stderr.String(), "No interpreter found") {
			return "", nil
		}
		return "", fmt.Errorf("Could not discover existing Python installations. Details: %s: %w", l.path("setup.log"), err)
	}
	python := strings.TrimSpace(string(output))
	if !filepath.IsAbs(python) {
		return "", errors.New("Python discovery returned an invalid installation path")
	}
	return python, nil
}

func (l *localManager) preparePython(ctx context.Context, uv string) error {
	l.installProgress(1, "Checking Python")
	if _, err := os.Stat(l.python()); err == nil {
		ready, err := l.checkInstallation(ctx, l.python(), localPythonCheck)
		if err != nil {
			return err
		}
		if !ready {
			return fmt.Errorf("Existing local Python environment is not compatible with Python 3.12. Details: %s", l.path("setup.log"))
		}
		l.installProgress(2, "Reusing Python setup")
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	python, err := l.findPython(ctx, uv)
	if err != nil {
		return err
	}
	if python == "" {
		l.installProgress(1, "Installing Python")
		if err := l.run(ctx, uv, "python", "install", "3.12.12"); err != nil {
			return err
		}
		l.installProgress(1, "Checking Python")
		python, err = l.findPython(ctx, uv)
		if err != nil {
			return err
		}
	}
	ready, err := l.checkInstallation(ctx, python, localPythonCheck)
	if err != nil {
		return err
	}
	if !ready {
		return fmt.Errorf("No working Python 3.12 installation was found. Details: %s", l.path("setup.log"))
	}
	l.installProgress(2, "Setting up Python")
	if err := l.run(ctx, uv, "venv", "--no-python-downloads", "--python", python, l.path("runtime-"+localRuntimeVersion)); err != nil {
		return err
	}
	ready, err = l.checkInstallation(ctx, l.python(), localPythonCheck)
	if err != nil {
		return err
	}
	if !ready {
		return fmt.Errorf("Local Python environment verification failed. Details: %s", l.path("setup.log"))
	}
	return nil
}

func (l *localManager) installRuntime(ctx context.Context) error {
	marker := l.path("runtime-"+localRuntimeVersion, ".kobashi-ready")
	l.installProgress(0, "Checking installed tools")
	check := localPythonCheck + fmt.Sprintf("; from importlib.metadata import version; assert version('omlx') == %q; from omlx.model_settings import ModelSettings; ModelSettings(turboquant_kv_enabled=True, turboquant_kv_scheme='turboquant', turboquant_kv_bits=4); import omlx.server", localRuntimeVersion)
	ready, err := l.checkInstallation(ctx, l.python(), check)
	if err != nil {
		return err
	}
	if ready {
		if err := writeAtomic(marker, []byte(localWheelHash)); err != nil {
			return err
		}
		l.installProgress(6, "Model tools ready")
		return nil
	}
	if err := os.Remove(marker); err != nil && !os.IsNotExist(err) {
		return err
	}
	uv := l.path("uv")
	l.installProgress(0, "Checking installer")
	if _, err := os.Stat(uv); os.IsNotExist(err) {
		if existing, err := exec.LookPath("uv"); err == nil {
			uv = existing
		} else if !errors.Is(err, exec.ErrNotFound) {
			return err
		}
	} else if err != nil {
		return err
	}
	if _, err := os.Stat(uv); os.IsNotExist(err) {
		archive := l.path("downloads", "uv.tar.gz")
		l.installProgress(0, "Downloading installer")
		if err = l.download(ctx, "https://github.com/astral-sh/uv/releases/download/0.13.7/uv-aarch64-apple-darwin.tar.gz", localUVHash, archive, "Installing"); err != nil {
			return err
		}
		l.installProgress(0, "Preparing installer")
		if err = extractLocalUV(archive, uv); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	l.installProgress(0, "Checking installer")
	if err := l.run(ctx, uv, "--version"); err != nil {
		return err
	}
	if err := l.preparePython(ctx, uv); err != nil {
		return err
	}
	wheel := l.path("downloads", localWheel)
	l.installProgress(3, "Reusing model engine")
	if fileSHA(wheel) != localWheelHash && fileSHA(wheel+".part") != localWheelHash {
		l.installProgress(3, "Downloading model engine")
	}
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
	l.installProgress(4, "Installing model tools")
	index, closeIndex, err := l.packageIndex(ctx)
	if err != nil {
		return err
	}
	defer closeIndex()
	if err := l.run(ctx, uv, "pip", "install", "--default-index", index, "--python", l.python(), "--override", overrideFile, wheel); err != nil {
		return err
	}
	l.installProgress(5, "Checking model tools")
	if err := l.run(ctx, l.python(), "-c", check); err != nil {
		return err
	}
	if err := writeAtomic(marker, []byte(localWheelHash)); err != nil {
		return err
	}
	l.installProgress(6, "Model tools ready")
	return nil
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
	if response.ContentLength < 0 {
		total = 0
	}
	done := offset
	l.progress(stage, done, total)
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
	if err = os.Rename(partial, target); err != nil {
		return err
	}
	l.progress(stage, done, done)
	return nil
}
func (l *localManager) updateRuntimeContext(ctx context.Context, endpoint, secret, id string, window int) error {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, endpoint+"/admin/api/models/"+id+"/settings",
		bytes.NewReader(jsonBytes(object{"max_context_window": window})))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+secret)
	req.Header.Set("Content-Type", "application/json")
	response, err := l.client.Do(req)
	if err != nil {
		return err
	}
	data, err := readResponse(response, 1<<20)
	if err != nil {
		return err
	}
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("Local runtime rejected context settings (HTTP %d)", response.StatusCode)
	}
	var result object
	if err := json.Unmarshal(data, &result); err != nil {
		return fmt.Errorf("Invalid runtime context confirmation: %w", err)
	}
	reload, confirmed := result["requires_reload"].(bool)
	if result["success"] != true || str(result["model_id"]) != id ||
		number(nested(result, "settings", "max_context_window")) != float64(window) || !confirmed || reload {
		return errors.New("Local runtime did not confirm an in-place context update")
	}
	req, err = http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"/v1/models/status", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+secret)
	response, err = l.client.Do(req)
	if err != nil {
		return err
	}
	data, err = readResponse(response, 1<<20)
	if err != nil {
		return err
	}
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("Could not verify runtime context (HTTP %d)", response.StatusCode)
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return fmt.Errorf("Invalid runtime context status: %w", err)
	}
	for _, entry := range arr(result["models"]) {
		model := obj(entry)
		if str(model["id"]) == id && number(model["max_context_window"]) == float64(window) {
			return nil
		}
	}
	return errors.New("Local runtime is not using the requested context window")
}

func (l *localManager) startRuntime(ctx context.Context, id string) error {
	return l.startRuntimeAtContext(ctx, id, l.configuredContextSize(id))
}
func (l *localManager) startRuntimeAtContext(ctx context.Context, id string, targetContext int) error {
	l.mu.Lock()
	same := l.runtimeModel == id && l.endpoint != "" && l.runtimeContext == targetContext
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
	if err = l.runtimeConfig(id, port, secret, targetContext); err != nil {
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
	l.runtimeContext = targetContext
	l.mu.Unlock()
	go func() {
		_ = c.Wait()
		log.Close()
		l.mu.Lock()
		unexpected := l.runtime == c && l.app.ctx.Err() == nil
		if l.runtime == c {
			l.runtimeModel = ""
			l.runtimeContext = 0
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
