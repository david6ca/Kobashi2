package bridge

import (
	"archive/tar"
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
	"slices"
	"strings"
	"time"
)

//go:embed native-runtime-LICENSE.txt
var nativeRuntimeLicense []byte

//go:embed native-runtime.json
var nativeRuntimeJSON []byte

var nativeRuntime = func() struct {
	Version, Archive, SHA256 string
	Files                    []string
} {
	var value struct {
		Version, Archive, SHA256 string
		Files                    []string
	}
	if err := json.Unmarshal(nativeRuntimeJSON, &value); err != nil {
		panic(err)
	}
	return value
}()

func (l *localManager) environment() []string {
	env := []string{}
	for _, e := range os.Environ() {
		key, _, _ := strings.Cut(e, "=")
		if key == "HOME" || strings.HasPrefix(key, "PIP_") || strings.HasPrefix(key, "PYTHON") || strings.HasPrefix(key, "UV_") || strings.HasPrefix(key, "HF_") || strings.HasPrefix(key, "OMLX_") || strings.HasPrefix(key, "LLAMA_") || strings.HasPrefix(key, "GGML_") || strings.HasPrefix(key, "DYLD_") || strings.HasPrefix(key, "ANTHROPIC_") || strings.HasPrefix(key, "OPENAI_") || strings.HasPrefix(key, "GITHUB_") || key == "GH_TOKEN" || key == "VIRTUAL_ENV" {
			continue
		}
		env = append(env, e)
	}
	return append(env, "HOME="+l.app.options.Home)
}
func (l *localManager) command(ctx context.Context, name string, args ...string) *exec.Cmd {
	c := exec.CommandContext(ctx, name, args...)
	c.Env = l.environment()
	configureLocalCommand(c)
	return c
}
func bundledNativeRuntime() string {
	executable, err := os.Executable()
	if err != nil {
		return ""
	}
	return filepath.Join(filepath.Dir(filepath.Dir(executable)), "Runtime")
}
func (l *localManager) runtimeDirectory() string {
	bundle := bundledNativeRuntime()
	if appStoreBuild {
		return bundle
	}
	if _, err := os.Stat(filepath.Join(bundle, "llama-server")); err == nil {
		return bundle
	}
	return l.path("llama-" + nativeRuntime.Version)
}
func (l *localManager) runtimeExecutable() string {
	return filepath.Join(l.runtimeDirectory(), "llama-server")
}
func (l *localManager) installRuntime(ctx context.Context) error {
	root := l.runtimeDirectory()
	if _, err := os.Stat(filepath.Join(root, "llama-server")); err == nil {
		for _, name := range nativeRuntime.Files {
			if _, err := os.Stat(filepath.Join(root, name)); err != nil {
				return errors.New("Local model runtime is incomplete. Reinstall ModelBridge")
			}
		}
		return nil
	}
	if appStoreBuild {
		return errors.New("Bundled model runtime is unavailable")
	}
	if err := os.MkdirAll(l.path("downloads"), 0700); err != nil {
		return err
	}
	l.progress("Preparing", 0, 0)
	archive := l.path("downloads", nativeRuntime.Archive)
	if err := l.download(ctx, "https://github.com/ggml-org/llama.cpp/releases/download/"+nativeRuntime.Version+"/"+nativeRuntime.Archive, nativeRuntime.SHA256, archive, "Preparing"); err != nil {
		return err
	}
	return extractNativeRuntime(archive, root)
}
func extractNativeRuntime(archive, target string) error {
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
	if err = os.MkdirAll(filepath.Dir(target), 0700); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(filepath.Dir(target), "llama-stage-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	reader := tar.NewReader(gz)
	for {
		h, e := reader.Next()
		if e == io.EOF {
			break
		}
		if e != nil {
			return e
		}
		name := filepath.Base(h.Name)
		if h.Typeflag == tar.TypeReg && slices.Contains(nativeRuntime.Files, name) {
			if h.Size > 100<<20 {
				return errors.New("Runtime component exceeds size limit")
			}
			out, e := os.OpenFile(filepath.Join(stage, name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0700)
			if e != nil {
				return e
			}
			_, e = io.Copy(out, reader)
			closeErr := out.Close()
			if e != nil {
				return e
			}
			if closeErr != nil {
				return closeErr
			}
		} else if h.Typeflag == tar.TypeSymlink && filepath.Base(h.Linkname) == h.Linkname && slices.Contains(nativeRuntime.Files, h.Linkname) {
			if name == "." || name == ".." {
				return errors.New("Invalid runtime link")
			}
			if err = os.Symlink(h.Linkname, filepath.Join(stage, name)); err != nil {
				return err
			}
		}
	}
	for _, name := range nativeRuntime.Files {
		if _, err = os.Stat(filepath.Join(stage, name)); err != nil {
			return fmt.Errorf("Missing runtime component %s", name)
		}
	}
	if err = os.WriteFile(filepath.Join(stage, "LICENSE.txt"), nativeRuntimeLicense, 0600); err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(stage, "runtime-manifest.json"), nativeRuntimeJSON, 0600); err != nil {
		return err
	}
	if _, err = os.Stat(target); err == nil {
		return errors.New("Runtime destination already exists")
	}
	return os.Rename(stage, target)
}
func (l *localManager) prepare(ctx context.Context, m localModel) error {
	if err := l.installRuntime(ctx); err != nil {
		return err
	}
	directory := l.path("models", m.ID)
	if err := os.MkdirAll(directory, 0700); err != nil {
		return err
	}
	target := filepath.Join(directory, m.File)
	marker := filepath.Join(directory, ".kobashi-ready")
	// The completion marker identifies the GGUF content, not the former MLX revision.
	if b, _ := os.ReadFile(marker); strings.TrimSpace(string(b)) == m.SHA256 {
		if st, err := os.Stat(target); err == nil && st.Size() == m.Bytes {
			return nil
		}
	}
	remaining := m.Bytes
	if st, err := os.Stat(target + ".part"); err == nil {
		remaining = max(0, remaining-st.Size())
	}
	if st, err := os.Stat(target); err == nil && st.Size() == m.Bytes {
		remaining = 0
	}
	if free, err := localFreeDisk(directory); err != nil {
		return err
	} else if free < remaining+(512<<20) {
		return errors.New("Not enough disk space to download this model")
	}
	if err := l.download(ctx, "https://huggingface.co/"+m.Repo+"/resolve/"+m.Revision+"/"+m.File, m.SHA256, target, "Downloading"); err != nil {
		return err
	}
	return writeAtomic(marker, []byte(m.SHA256))
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
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
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
	} else if !strings.HasPrefix(response.Header.Get("Content-Range"), fmt.Sprintf("bytes %d-", offset)) {
		return errors.New("Download returned an invalid resume range")
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
	buffer := make([]byte, 128<<10)
	last := time.Time{}
	for {
		n, e := response.Body.Read(buffer)
		if n > 0 {
			if _, err = f.Write(buffer[:n]); err != nil {
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
		return errors.New("Download checksum mismatch. Retry download")
	}
	if err = os.Rename(partial, target); err != nil {
		return err
	}
	l.progress(stage, done, done)
	return nil
}
func (l *localManager) runtimeArguments(id string, port, window int, secret string) ([]string, error) {
	m, ok := findLocalModel(id)
	if !ok {
		return nil, errors.New("Choose a local model")
	}
	keyfile := l.path("server", id, "api-key")
	if err := writeAtomic(keyfile, []byte(secret)); err != nil {
		return nil, err
	}
	return []string{"--model", l.path("models", id, m.File), "--alias", id, "--host", "127.0.0.1", "--port", fmt.Sprint(port), "--api-key-file", keyfile, "--ctx-size", fmt.Sprint(window), "--parallel", "1", "--n-gpu-layers", "all", "--flash-attn", "on", "--cache-type-k", "q4_0", "--cache-type-v", "q4_0", "--jinja", "--chat-template-kwargs", `{"enable_thinking":false}`, "--offline", "--no-webui", "--no-slots", "--no-warmup"}, nil
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
	if _, err := os.Stat(l.runtimeExecutable()); err != nil {
		return errors.New("Local runtime is unavailable. Reinstall ModelBridge")
	}
	model, ok := findLocalModel(id)
	if !ok {
		return errors.New("Choose a local model")
	}
	marker, _ := os.ReadFile(l.path("models", id, ".kobashi-ready"))
	if strings.TrimSpace(string(marker)) != model.SHA256 {
		return errors.New("Local model download is incomplete. Add the model to resume setup")
	}
	if st, err := os.Stat(l.path("models", id, model.File)); err != nil || st.Size() != model.Bytes {
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
	args, err := l.runtimeArguments(id, port, targetContext, secret)
	if err != nil {
		return err
	}
	runCtx, cancel := context.WithCancel(l.app.ctx)
	c := l.command(runCtx, l.runtimeExecutable(), args...)
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
		req, _ := http.NewRequestWithContext(bounded, http.MethodGet, endpoint+"/health", nil)
		req.Header.Set("Authorization", "Bearer "+secret)
		if response, e := l.client.Do(req); e == nil {
			_, readErr := readResponse(response, 1<<20)
			if readErr == nil && response.StatusCode == 200 {
				return nil
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
			return errors.New("llama.cpp could not start. See the local runtime log")
		case <-time.After(500 * time.Millisecond):
		}
	}
}
