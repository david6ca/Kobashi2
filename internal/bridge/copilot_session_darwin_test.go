package bridge

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestCopilotSessionQueriesReadOnly(t *testing.T) {
	a := testApp(t, nil)
	path := filepath.Join(a.options.Home, ".copilot", "data.db")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	query := `
CREATE TABLE app_state (key TEXT, value TEXT);
CREATE TABLE workspaces (id TEXT, session_id TEXT, host_id TEXT);
CREATE TABLE sessions (id TEXT, model TEXT, provider_id TEXT, is_running INTEGER,
 execution_location TEXT, reasoning_effort TEXT, context_tier TEXT,
 remote_connect_target TEXT, archived_at TEXT, provisional INTEGER DEFAULT 0);
INSERT INTO sessions VALUES
 ('original','native-model','github-provider',0,'local','high','long_context',NULL,NULL,0),
 ('bridged','bridge-model','` + copilotProviderID + `',0,'local','low','default',NULL,NULL,0),
 ('running','other-model','` + copilotProviderID + `',1,'local',NULL,NULL,NULL,NULL,0),
 ('remote','bridge-model','` + copilotProviderID + `',0,'local',NULL,NULL,'remote-target',NULL,0),
 ('archived','bridge-model','` + copilotProviderID + `',0,'local',NULL,NULL,NULL,'2026-10-01',0),
 ('warming','native-model',NULL,0,'local',NULL,NULL,NULL,NULL,1);
INSERT INTO workspaces VALUES ('workspace','original','local'),('remote-workspace','remote','remote-host');
INSERT INTO app_state VALUES ('activeWorkspaceId','workspace');`
	if data, err := exec.Command("/usr/bin/sqlite3", path, query).CombinedOutput(); err != nil {
		t.Fatal("create selection fixture", err, string(data))
	}
	if err := os.Chmod(path, 0400); err != nil {
		t.Fatal(err)
	}
	selected, err := a.selectedCopilotSession()
	if err != nil || selected.ID != "original" || selected.ProviderID != "github-provider" ||
		selected.ContextTier != "long_context" || selected.ReasoningEffort != "high" || selected.ready() != nil {
		t.Fatal("selected session lost restore settings", selected, err)
	}
	bridged, err := a.bridgedCopilotSessions()
	if err != nil || len(bridged) != 3 {
		t.Fatal("dependent sessions were not read", bridged, err)
	}
	for _, selected := range bridged {
		if selected.ProviderID != copilotProviderID || selected.ID == "archived" {
			t.Fatal("unrelated or archived session included", selected)
		}
		if selected.ID != "bridged" && selected.ready() == nil {
			t.Fatal("a running or remote session was treated as safe to restore", selected)
		}
	}
	local, err := a.localCopilotSessions()
	if err != nil || len(local) != 3 {
		t.Fatal("local session inspection included remote/archived sessions or lost running sessions", local, err)
	}
	for _, selected := range local {
		if selected.ID == "remote" || selected.ID == "archived" || selected.ID == "warming" || selected.Location != "local" {
			t.Fatal("ineligible session included in automatic routing", selected)
		}
		warmingData, err := copilotQuery(path, "SELECT json_object("+copilotSessionFieldsSQL+") FROM sessions s WHERE id='warming'")
		var warming copilotSelection
		if err != nil || json.Unmarshal(warmingData, &warming) != nil || warming.Provisional != 1 || warming.ready() == nil {
			t.Fatal("a provisional warmup session was considered ready to switch", string(warmingData), err)
		}
	}
	data, err := copilotQuery(path, "SELECT json_group_array(json_object("+copilotSessionFieldsSQL+")) FROM sessions s WHERE 0")
	if err != nil || json.Unmarshal(data, &bridged) != nil || len(bridged) != 0 {
		t.Fatal("empty dependencies did not produce a valid list", string(data), err)
	}
}
