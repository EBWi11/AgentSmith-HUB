package api

import (
	"AgentSmith-HUB/common"
	"AgentSmith-HUB/project"
	"AgentSmith-HUB/rules_engine"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/labstack/echo/v4"
)

const localRulesetTestRaw = `<root type="DETECTION"><rule id="r" name="r"><check type="EQU" field="event">test</check></rule></root>`

func setupLocalRulesetTest(t *testing.T, id, storedPath string) string {
	t.Helper()
	root := t.TempDir()
	previousConfig := common.Config
	common.Config = &common.HubConfig{ConfigRoot: root}
	t.Cleanup(func() { common.Config = previousConfig })

	formalPath := filepath.Join(root, "ruleset", "cwpp", id+".xml")
	if err := os.MkdirAll(filepath.Dir(formalPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(formalPath, []byte(localRulesetTestRaw), 0644); err != nil {
		t.Fatal(err)
	}
	if storedPath == "actual" {
		storedPath = formalPath
	} else if storedPath == "stale" {
		storedPath = filepath.Join(root, "ruleset", id+".xml")
	}
	previous, existed := project.GetRuleset(id)
	project.SetRuleset(id, &rules_engine.Ruleset{Path: storedPath, RawConfig: localRulesetTestRaw})
	t.Cleanup(func() {
		if existed {
			project.SetRuleset(id, previous)
		} else {
			project.DeleteRuleset(id)
		}
	})
	return formalPath
}

func localRulesetChanges(t *testing.T, id string) []map[string]interface{} {
	t.Helper()
	e := echo.New()
	rec := httptest.NewRecorder()
	c := e.NewContext(httptest.NewRequest(http.MethodGet, "/local-changes", nil), rec)
	if err := getLocalChanges(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var changes []map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &changes); err != nil {
		t.Fatal(err)
	}
	var matching []map[string]interface{}
	for _, change := range changes {
		if change["type"] == "ruleset" && change["id"] == id {
			matching = append(matching, change)
		}
	}
	return matching
}

func TestLocalChangesFindsNestedRulesetWithoutStoredPath(t *testing.T) {
	for _, storedPath := range []string{"", "stale", "actual"} {
		t.Run("path="+storedPath, func(t *testing.T) {
			const id = "local_nested_ruleset"
			formalPath := setupLocalRulesetTest(t, id, storedPath)
			if changes := localRulesetChanges(t, id); len(changes) != 0 {
				t.Fatalf("existing unchanged ruleset reported as changed: %#v", changes)
			}
			if err := os.WriteFile(formalPath, []byte(localRulesetTestRaw+"\n<!-- edit -->"), 0644); err != nil {
				t.Fatal(err)
			}
			changes := localRulesetChanges(t, id)
			if len(changes) != 1 || changes[0]["change_type"] != "modified" || changes[0]["file_path"] != formalPath {
				t.Fatalf("expected one nested-file modification, got %#v", changes)
			}
		})
	}
}

func TestLocalChangesReportsDeletedNestedRuleset(t *testing.T) {
	const id = "local_deleted_ruleset"
	formalPath := setupLocalRulesetTest(t, id, "actual")
	if err := os.Remove(formalPath); err != nil {
		t.Fatal(err)
	}
	changes := localRulesetChanges(t, id)
	if len(changes) != 1 || changes[0]["change_type"] != "deleted" || changes[0]["file_path"] != formalPath {
		t.Fatalf("expected actual file deletion, got %#v", changes)
	}
}

func TestRulesetContentReloadPreservesFolderPath(t *testing.T) {
	const id = "local_reload_ruleset"
	formalPath := setupLocalRulesetTest(t, id, "")
	if _, err := reloadComponentUnified(&ComponentReloadRequest{
		Type:       "ruleset",
		ID:         id,
		NewContent: localRulesetTestRaw,
		Source:     SourceClusterSync,
	}); err != nil {
		t.Fatal(err)
	}
	rs, ok := project.GetRuleset(id)
	if !ok || rs.Path != formalPath {
		t.Fatalf("reloaded ruleset lost its folder path: %#v", rs)
	}
	if changes := localRulesetChanges(t, id); len(changes) != 0 {
		t.Fatalf("reloaded ruleset reported as changed: %#v", changes)
	}
}
