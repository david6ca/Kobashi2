//go:build appstore && darwin

package bridge

import (
	"context"
	"errors"
	"path/filepath"
)

const appStoreBuild = true

func prepareStoreOptions(options *Options) {
	// AppKit launches the helper with HOME set to the app's container.
	// Client folders are separate security-scoped bookmarks, not the user's home.
	options.CodexHome = filepath.Join(options.Home, ".codex")
	for _, client := range []string{"codex", "claude", "copilot"} {
		_ = acquireClientAccess(options.Home, client)
	}
	options.ReadEnvironment = func(string) (*string, error) { return nil, nil }
	options.WriteEnvironment = func(string, *string) error { return nil }
	options.Startup = func(bool) error { return nil } // SMAppService is owned by AppKit.
	options.NativeCatalog = func(context.Context, string) ([]object, error) { return nil, nil }
}

func storeClientFolderError() error {
	return errors.New("Choose client configuration folder")
}
