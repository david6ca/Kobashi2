package bridge

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestNativeRuntimeEnvironmentDoesNotInheritCredentialsOrEngineOverrides(t *testing.T) {
	for _, key := range []string{"PYTHONPATH", "UV_INDEX", "HF_TOKEN", "LLAMA_ARG_HOST", "LLAMA_ARG_API_KEY", "GGML_METAL_PATH_RESOURCES", "DYLD_LIBRARY_PATH", "OPENAI_API_KEY", "GITHUB_TOKEN"} {
		t.Setenv(key, "untrusted")
	}
	a := localTestApp(t)
	for _, entry := range a.local.environment() {
		if strings.Contains(entry, "untrusted") {
			t.Fatal("inherited an unsafe runtime override", entry)
		}
	}
}
func TestNativeRuntimeArguments(t *testing.T) {
	a := localTestApp(t)
	args, err := a.local.runtimeArguments("qwen3-4b", 12345, 8192, "synthetic")
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Join(args, " ")
	for _, part := range []string{"--host 127.0.0.1", "--cache-type-k q4_0", "--cache-type-v q4_0", "--ctx-size 8192", "--parallel 1", "--alias qwen3-4b", "--jinja", "--no-webui"} {
		if !strings.Contains(text, part) {
			t.Fatal("missing required native setting", part)
		}
	}
	if strings.Contains(text, "python") || strings.Contains(text, "omlx") {
		t.Fatal("Python runtime retained")
	}
}
func TestNativeContextChangeWaitsForRequests(t *testing.T) {
	a := localTestApp(t)
	a.local.runtimeModel = "qwen3-4b"
	a.local.endpoint = "http://127.0.0.1:1"
	a.local.runtimeContext = 8192
	a.routes.RLock()
	_, err := a.local.setContext("qwen3-4b", 16384)
	a.routes.RUnlock()
	if !errors.Is(err, errLocalBusy) || a.local.contextSize("qwen3-4b") != 8192 {
		t.Fatal("context change interrupted request", err)
	}
	if _, err = os.Stat(a.path("local-context.json")); !os.IsNotExist(err) {
		t.Fatal("busy context was persisted")
	}
}
func TestNativeRuntimeExtractionKeepsOnlyPinnedComponents(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "native.tar.gz")
	f, err := os.Create(archive)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	for _, name := range append(slices.Clone(nativeRuntime.Files), "unused-tool", "support.py") {
		data := []byte("fixture")
		if err = tw.WriteHeader(&tar.Header{Name: "bundle/" + name, Mode: 0700, Size: int64(len(data)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err = tw.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	_ = tw.WriteHeader(&tar.Header{Name: "bundle/libggml.1.dylib", Linkname: "libggml.1.0.0.dylib", Typeflag: tar.TypeSymlink})
	_ = tw.WriteHeader(&tar.Header{Name: "bundle/untrusted.dylib", Linkname: "../../outside", Typeflag: tar.TypeSymlink})
	if err = tw.Close(); err != nil {
		t.Fatal(err)
	}
	_ = gz.Close()
	_ = f.Close()
	target := filepath.Join(dir, "runtime")
	if err = extractNativeRuntime(archive, target); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"unused-tool", "support.py", "untrusted.dylib"} {
		if _, err = os.Stat(filepath.Join(target, name)); !os.IsNotExist(err) {
			t.Fatal("unexpected runtime file", name)
		}
	}
	if _, err = os.Stat(filepath.Join(target, "libggml.1.dylib")); err != nil {
		t.Fatal("required local dylib alias missing", err)
	}
}
func TestNativeModelCompletionDoesNotAcceptMLXMarker(t *testing.T) {
	a := localTestApp(t)
	m, _ := findLocalModel("qwen3-4b")
	for _, name := range nativeRuntime.Files {
		if err := writeAtomic(filepath.Join(a.local.runtimeDirectory(), name), []byte("fixture")); err != nil {
			t.Fatal(err)
		}
	}
	if err := writeAtomic(a.local.path("models", m.ID, ".kobashi-ready"), []byte("old-mlx-revision")); err != nil {
		t.Fatal(err)
	}
	if err := a.local.startRuntime(context.Background(), m.ID); err == nil || !strings.Contains(err.Error(), "download is incomplete") {
		t.Fatal("old weights accepted as GGUF")
	}
}
