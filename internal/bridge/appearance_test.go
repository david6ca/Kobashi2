package bridge

import (
	"encoding/json"
	"net/http"
	"os"
	"testing"
)

func TestAppearancePersistsExplicitAndAutomaticModes(t *testing.T) {
	a := testApp(t, nil)
	statusMode := func() string {
		t.Helper()
		var status object
		result := controlCall(a, "/api/status", nil)
		if result.Code != http.StatusOK || json.Unmarshal(result.Body.Bytes(), &status) != nil {
			t.Fatal("could not read appearance", result.Code)
		}
		return str(status["appearance"])
	}
	if statusMode() != "system" {
		t.Fatal("new installations must default to Auto")
	}
	for _, mode := range []string{"light", "dark", "system"} {
		result := controlCall(a, "/api/appearance", object{"appearance": mode})
		var value object
		if result.Code != http.StatusOK || json.Unmarshal(result.Body.Bytes(), &value) != nil || value["appearance"] != mode || statusMode() != mode {
			t.Fatal("appearance selection was not applied", mode, result.Code)
		}
		data, err := os.ReadFile(a.path("appearance.json"))
		var saved string
		if err != nil || json.Unmarshal(data, &saved) != nil || saved != mode {
			t.Fatal("appearance selection was not persisted", mode, err)
		}
		reloaded, err := New(a.options)
		if err != nil {
			t.Fatal(err)
		}
		if reloaded.appearance != mode {
			t.Error("saved appearance did not survive relaunch", mode, reloaded.appearance)
		}
		if err := reloaded.Stop(); err != nil {
			t.Fatal(err)
		}
	}
	for _, invalid := range []any{"auto", "invalid", true, nil} {
		if result := controlCall(a, "/api/appearance", object{"appearance": invalid}); result.Code != http.StatusBadRequest || statusMode() != "system" {
			t.Fatal("invalid appearance changed the saved selection", invalid, result.Code)
		}
	}
	if err := os.Remove(a.path("appearance.json")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(a.path("appearance.json"), 0700); err != nil {
		t.Fatal(err)
	}
	result := controlCall(a, "/api/appearance", object{"appearance": "dark"})
	if result.Code != http.StatusBadRequest || statusMode() != "system" {
		t.Fatal("failed persistence changed the confirmed mode", result.Code)
	}
}
