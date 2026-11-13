package bridge

import (
	"encoding/json"
	"net/http"
	"os"
	"testing"

	"github.com/david6ca/Kobashi2/internal/localization"
)

func TestLanguagePersistsWithoutChangingAccountsOrSettings(t *testing.T) {
	a := testApp(t, nil)
	if a.language != "auto" {
		t.Fatal("language must default to system")
	}
	a.state = session{Token: "private-fixture", Username: "fixture", Codex: true, Claude: true}
	before, settings := a.snapshot(), a.Settings()
	for _, language := range append([]localization.Language{{Code: "auto"}}, localization.Languages()...) {
		r := controlCall(a, "/api/language", object{"language": language.Code})
		if r.Code != http.StatusOK {
			t.Fatal(language.Code, r.Code, r.Body.String())
		}
		var status object
		if r := controlCall(a, "/api/status", nil); json.Unmarshal(r.Body.Bytes(), &status) != nil || status["language"] != language.Code {
			t.Fatal("status did not apply choice", language.Code)
		}
		next, err := New(a.options)
		if err != nil {
			t.Fatal(err)
		}
		if next.language != language.Code {
			t.Fatal("language lost on relaunch", language.Code)
		}
		next.Stop()
		if a.snapshot().Token != before.Token || a.snapshot().Username != before.Username || a.snapshot().Codex != before.Codex || a.snapshot().Claude != before.Claude || a.Settings() != settings {
			t.Fatal("language selection altered account/client settings")
		}
	}
}

func TestLanguageRejectsInvalidAndFailedWrites(t *testing.T) {
	a := testApp(t, nil)
	for _, invalid := range []any{"", "pt-PT", "xx", true, nil} {
		if r := controlCall(a, "/api/language", object{"language": invalid}); r.Code != http.StatusBadRequest || a.language != "auto" {
			t.Fatal("invalid choice changed language", invalid, r.Code)
		}
	}
	if err := os.MkdirAll(a.path("language.json"), 0700); err != nil {
		t.Fatal(err)
	}
	if r := controlCall(a, "/api/language", object{"language": "ar"}); r.Code != http.StatusBadRequest || a.language != "auto" {
		t.Fatal("failed save changed the confirmed choice", r.Code)
	}
}
