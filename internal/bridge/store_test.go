//go:build appstore && darwin

package bridge

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func TestAppStoreDoesNotImportOrdinaryLegacyAccounts(t *testing.T) {
	a := testApp(t, nil)
	legacy := filepath.Join(a.options.Home, ".codex", "ccb-session.json")
	if err := os.MkdirAll(filepath.Dir(legacy), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacy, []byte(`{"github_token":"synthetic-legacy","username":"legacy"}`), 0600); err != nil {
		t.Fatal(err)
	}
	a.initialize()
	if a.snapshot().active() || a.snapshot().Username != "" {
		t.Fatal("ordinary account was imported")
	}
	if _, err := os.Stat(legacy); err != nil {
		t.Fatal("ordinary data was modified", err)
	}
}

func TestAppStoreRequiresBookmarkBeforeEnablingClient(t *testing.T) {
	a := testApp(t, nil)
	a.state = session{Token: "synthetic-account", Username: "fixture"}
	result := controlCall(a, "/api/toggle-codex", object{})
	if result.Code != http.StatusBadRequest || a.snapshot().Codex {
		t.Fatal("unapproved client access was enabled", result.Code)
	}
	if _, err := os.Stat(filepath.Join(a.options.CodexHome, "config.toml")); !os.IsNotExist(err) {
		t.Fatal("unapproved client configuration was written", err)
	}
}

func TestAppStoreMissingBundledRuntimeNeverDownloadsCode(t *testing.T) {
	calls := 0
	a := testApp(t, func(*http.Request) (*http.Response, error) { calls++; return nil, nil })
	if err := a.local.installRuntime(context.Background()); err == nil {
		t.Fatal("missing bundle accepted")
	}
	if calls != 0 {
		t.Fatal("runtime download attempted", calls)
	}
	if _, err := os.Stat(a.local.path("downloads")); !os.IsNotExist(err) {
		t.Fatal("installer output was created", err)
	}
}
