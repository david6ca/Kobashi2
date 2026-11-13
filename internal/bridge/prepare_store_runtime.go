package bridge

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// PrepareAppStoreRuntime installs pinned build dependencies in a disposable home.
// It never starts an inference server, downloads weights or accesses saved accounts.
func PrepareAppStoreRuntime(ctx context.Context, home string) (string, error) {
	a, err := New(Options{Home: home})
	if err != nil {
		return "", err
	}
	defer a.cancel()
	l := a.local
	if err = os.MkdirAll(l.path("downloads"), 0700); err != nil {
		return "", err
	}
	uv, err := exec.LookPath("uv")
	if err != nil {
		return "", err
	}
	if _, err = os.Stat(l.python()); os.IsNotExist(err) {
		if err = l.run(ctx, uv, "python", "install", "3.12.12"); err != nil {
			return "", err
		}
		paths, err := filepath.Glob(l.path("python", "cpython-3.12.12-*-none", "bin", "python3.12"))
		if err != nil || len(paths) != 1 {
			return "", fmt.Errorf("expected one staged standalone Python 3.12.12 installation")
		}
		if err = l.run(ctx, uv, "venv", "--no-python-downloads", "--python", paths[0], l.path("runtime-"+localRuntimeVersion)); err != nil {
			return "", err
		}
	}
	if err = l.installRuntime(ctx); err != nil {
		return "", err
	}
	return l.path("runtime-" + localRuntimeVersion), nil
}
