package cluster

import (
	"AgentSmith-HUB/plugin"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestDuplicateClusterPluginPushInitializesOnce(t *testing.T) {
	const pluginName = "cluster_push_init_once"
	marker := filepath.Join(t.TempDir(), "init-count.txt")
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
	previous, existed := plugin.Plugins[pluginName]
	delete(plugin.Plugins, pluginName)
	plugin.PluginsMu.Unlock()
	t.Cleanup(func() {
		plugin.PluginsMu.Lock()
		defer plugin.PluginsMu.Unlock()
		if existed {
			plugin.Plugins[pluginName] = previous
		} else {
			delete(plugin.Plugins, pluginName)
		}
	})

	listener := &SyncListener{}
	if err := listener.createComponentInstance("plugin", pluginName, raw); err != nil {
		t.Fatalf("first cluster push failed: %v", err)
	}
	if err := listener.createComponentInstance("plugin", pluginName, raw); err != nil {
		t.Fatalf("duplicate cluster push failed: %v", err)
	}

	content, err := os.ReadFile(marker)
	if err != nil {
		t.Fatalf("read init marker: %v", err)
	}
	if got := string(content); got != "x" {
		t.Fatalf("duplicate cluster push initialized plugin %d times, marker=%q", len(got), got)
	}
}

func TestPluginLifecycleInstructionsKeepVersionOrder(t *testing.T) {
	instructions := []Instruction{
		{Version: 7, ComponentType: "plugin", ComponentName: "demo", Operation: "delete"},
		{Version: 8, ComponentType: "plugin", ComponentName: "demo", Operation: "add"},
	}

	sortInstructionsForExecution(instructions)

	if instructions[0].Version != 7 || instructions[1].Version != 8 {
		t.Fatalf("same-plugin lifecycle was reordered: %+v", instructions)
	}
}
