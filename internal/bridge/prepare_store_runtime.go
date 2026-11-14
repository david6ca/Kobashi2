package bridge

import (
	"context"
	"path/filepath"
)

// PrepareNativeRuntime stages the pinned native engine in a disposable build home.
// It never starts inference, downloads weights or accesses saved accounts.
func PrepareNativeRuntime(ctx context.Context, home string) (string, error) {
	a, err := New(Options{Home: home})
	if err != nil {
		return "", err
	}
	defer a.cancel()
	if err = a.local.installRuntime(ctx); err != nil {
		return "", err
	}
	return filepath.Abs(a.local.runtimeDirectory())
}
