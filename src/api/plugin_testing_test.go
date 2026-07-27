package api

import (
	"AgentSmith-HUB/common"
	"AgentSmith-HUB/plugin"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

type pluginTestResponse struct {
	Success bool        `json:"success"`
	Error   string      `json:"error"`
	Result  interface{} `json:"result"`
}

func callTestPluginHandler(t *testing.T, id string, body map[string]interface{}) pluginTestResponse {
	t.Helper()

	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	e := echo.New()
	target := "/test-plugin-content"
	if id != "" {
		target = "/test-plugin/" + id
	}
	req := httptest.NewRequest(http.MethodPost, target, bytes.NewReader(payload))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	if id != "" {
		c.SetParamNames("id")
		c.SetParamValues(id)
	}

	if err := testPlugin(c); err != nil {
		t.Fatalf("testPlugin returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var resp pluginTestResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v body=%s", err, rec.Body.String())
	}
	return resp
}

func TestPluginContentTestRejectsSparseArgumentIndexes(t *testing.T) {
	raw := `package plugin

func Eval(args ...interface{}) (bool, error) {
	return true, nil
}`

	resp := callTestPluginHandler(t, "", map[string]interface{}{
		"content": raw,
		"data": map[string]interface{}{
			"2": "sparse",
		},
	})

	if resp.Success {
		t.Fatalf("expected sparse indexes to fail, got %#v", resp)
	}
	if !strings.Contains(resp.Error, "contiguous") {
		t.Fatalf("expected contiguous index error, got %#v", resp)
	}
}

func TestPluginContentTestLoadsTemporaryPluginOnce(t *testing.T) {
	buildRaw := func(marker string) string {
		return fmt.Sprintf(`package plugin

import "os"

var _ = func() int {
	f, err := os.OpenFile(%q, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err == nil {
		_, _ = f.WriteString("x")
		_ = f.Close()
	}
	return 0
}()

func Eval() (bool, error) {
	return true, nil
}`, marker)
	}

	baselineMarker := filepath.Join(t.TempDir(), "baseline-load-count.txt")
	if _, err := plugin.NewTestPlugin("", buildRaw(baselineMarker), "api_plugin_test_load_baseline", plugin.YAEGI_PLUGIN); err != nil {
		t.Fatalf("create baseline plugin: %v", err)
	}
	baselineContent, err := os.ReadFile(baselineMarker)
	if err != nil {
		t.Fatalf("read baseline marker: %v", err)
	}
	if got := string(baselineContent); got != "x" {
		t.Fatalf("expected NewTestPlugin to initialize exactly once, got marker %q", got)
	}

	marker := filepath.Join(t.TempDir(), "handler-load-count.txt")
	resp := callTestPluginHandler(t, "", map[string]interface{}{
		"content": buildRaw(marker),
		"data":    map[string]interface{}{},
	})
	if !resp.Success {
		t.Fatalf("expected plugin test success, got %#v", resp)
	}

	content, err := os.ReadFile(marker)
	if err != nil {
		t.Fatalf("read marker: %v", err)
	}
	if got, want := string(content), string(baselineContent); got != want {
		t.Fatalf("expected handler load side effects to match one NewTestPlugin load, got %q want %q", got, want)
	}
}

func TestProductionPluginReloadInitializesOnce(t *testing.T) {
	const id = "api_plugin_reload_init_once"
	marker := filepath.Join(t.TempDir(), "production-reload-init-count.txt")
	raw := fmt.Sprintf(`package plugin

import "os"

func init() {
	f, err := os.OpenFile(%q, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err == nil {
		_, _ = f.WriteString("x")
		_ = f.Close()
	}
}

func Eval() (bool, error) {
	return true, nil
}`, marker)

	plugin.PluginsMu.Lock()
	previous, existed := plugin.Plugins[id]
	delete(plugin.Plugins, id)
	plugin.PluginsMu.Unlock()
	t.Cleanup(func() {
		plugin.PluginsMu.Lock()
		defer plugin.PluginsMu.Unlock()
		if existed {
			plugin.Plugins[id] = previous
		} else {
			delete(plugin.Plugins, id)
		}
	})

	if _, err := reloadComponentUnified(&ComponentReloadRequest{
		Type:        "plugin",
		ID:          id,
		NewContent:  raw,
		Source:      SourceClusterSync,
		SkipVerify:  false,
		WriteToFile: false,
	}); err != nil {
		t.Fatalf("reload production plugin: %v", err)
	}

	content, err := os.ReadFile(marker)
	if err != nil {
		t.Fatalf("read production reload marker: %v", err)
	}
	if got := string(content); got != "x" {
		t.Fatalf("expected production reload to initialize exactly once, got marker %q", got)
	}
}

func TestPluginReloadFailurePreservesActiveInstanceAndFile(t *testing.T) {
	const id = "api_plugin_transaction"
	oldRaw := `package plugin

func Eval() (bool, error) {
	return true, nil
}`
	badRaw := `package plugin

func init() {
	panic("init failed")
}

func Eval() (bool, error) {
	return false, nil
}`

	configRoot := t.TempDir()
	pluginDir := filepath.Join(configRoot, "plugin")
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		t.Fatalf("create plugin directory: %v", err)
	}
	formalPath := filepath.Join(pluginDir, id+".go")
	if err := os.WriteFile(formalPath, []byte(oldRaw), 0644); err != nil {
		t.Fatalf("write old plugin: %v", err)
	}

	previousConfig := common.Config
	common.Config = &common.HubConfig{ConfigRoot: configRoot}
	t.Cleanup(func() { common.Config = previousConfig })

	plugin.PluginsMu.Lock()
	previous, existed := plugin.Plugins[id]
	delete(plugin.Plugins, id)
	plugin.PluginsMu.Unlock()
	t.Cleanup(func() {
		plugin.PluginsMu.Lock()
		defer plugin.PluginsMu.Unlock()
		if existed {
			plugin.Plugins[id] = previous
		} else {
			delete(plugin.Plugins, id)
		}
	})

	if err := plugin.NewPlugin(formalPath, "", id, plugin.YAEGI_PLUGIN); err != nil {
		t.Fatalf("load old plugin: %v", err)
	}
	activeBefore, _ := plugin.GetPlugin(id)

	if _, err := reloadComponentUnified(&ComponentReloadRequest{
		Type:        "plugin",
		ID:          id,
		NewContent:  badRaw,
		Source:      SourceClusterSync,
		SkipVerify:  false,
		WriteToFile: true,
	}); err == nil {
		t.Fatal("expected plugin reload to fail")
	}

	activeAfter, _ := plugin.GetPlugin(id)
	if activeAfter != activeBefore {
		t.Fatal("failed reload replaced the active plugin instance")
	}
	onDisk, err := os.ReadFile(formalPath)
	if err != nil {
		t.Fatalf("read formal plugin: %v", err)
	}
	if string(onDisk) != oldRaw {
		t.Fatalf("failed reload changed formal plugin file: %q", string(onDisk))
	}
}

func TestSavedYaegiPluginTestDoesNotIncrementLiveStats(t *testing.T) {
	id := "api_plugin_test_stats"
	raw := `package plugin

func Eval() (bool, error) {
	return true, nil
}`

	plugin.PluginsMu.Lock()
	previous, hadPrevious := plugin.Plugins[id]
	delete(plugin.Plugins, id)
	plugin.PluginsMu.Unlock()
	t.Cleanup(func() {
		plugin.PluginsMu.Lock()
		defer plugin.PluginsMu.Unlock()
		if hadPrevious {
			plugin.Plugins[id] = previous
		} else {
			delete(plugin.Plugins, id)
		}
	})

	if err := plugin.NewPlugin("", raw, id, plugin.YAEGI_PLUGIN); err != nil {
		t.Fatalf("create plugin: %v", err)
	}

	plugin.PluginsMu.RLock()
	livePlugin := plugin.Plugins[id]
	plugin.PluginsMu.RUnlock()
	if livePlugin == nil {
		t.Fatal("expected live plugin to be registered")
	}
	_ = livePlugin.GetSuccessIncrementAndUpdate()

	resp := callTestPluginHandler(t, id, map[string]interface{}{
		"data": map[string]interface{}{},
	})
	if !resp.Success {
		t.Fatalf("expected plugin test success, got %#v", resp)
	}
	if got := livePlugin.GetSuccessIncrementAndUpdate(); got != 0 {
		t.Fatalf("expected manual test not to increment live success stats, got %d", got)
	}
}
