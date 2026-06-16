package rules_engine

import (
	pluginpkg "AgentSmith-HUB/plugin"
	"os"
	"path/filepath"
	"testing"
)

func TestNewFromExistingRebindsPluginPointers(t *testing.T) {
	const pluginName = "hotSwapRebindPlugin"

	previousPlugin, hadPlugin := pluginpkg.Plugins[pluginName]
	previousTemp, hadTemp := pluginpkg.PluginsNew[pluginName]
	t.Cleanup(func() {
		if hadPlugin {
			pluginpkg.Plugins[pluginName] = previousPlugin
		} else {
			delete(pluginpkg.Plugins, pluginName)
		}
		if hadTemp {
			pluginpkg.PluginsNew[pluginName] = previousTemp
		} else {
			delete(pluginpkg.PluginsNew, pluginName)
		}
	})

	oldPlugin := &pluginpkg.Plugin{Name: pluginName, ReturnType: "bool"}
	pluginpkg.Plugins[pluginName] = oldPlugin

	raw := `<root type="DETECTION" name="plugin-reload">
  <rule id="r1" name="r1">
    <check type="PLUGIN">hotSwapRebindPlugin()</check>
    <append type="PLUGIN" field="append_result">hotSwapRebindPlugin()</append>
    <modify type="PLUGIN" field="modify_result">hotSwapRebindPlugin()</modify>
    <plugin>hotSwapRebindPlugin()</plugin>
  </rule>
</root>`

	existing, err := NewRuleset("", raw, "reload-rebind")
	if err != nil {
		t.Fatalf("NewRuleset failed: %v", err)
	}
	t.Cleanup(existing.cleanup)
	assertRulePluginPointers(t, existing.Rules[0], oldPlugin)

	newPlugin := &pluginpkg.Plugin{Name: pluginName, ReturnType: "bool"}
	pluginpkg.Plugins[pluginName] = newPlugin

	instance, err := NewFromExisting(existing, "TEST.project.reload")
	if err != nil {
		t.Fatalf("NewFromExisting failed: %v", err)
	}
	t.Cleanup(instance.cleanup)

	if len(instance.Rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(instance.Rules))
	}
	assertRulePluginPointers(t, instance.Rules[0], newPlugin)
}

func assertRulePluginPointers(t *testing.T, rule Rule, expected *pluginpkg.Plugin) {
	t.Helper()

	if len(rule.CheckMap) != 1 {
		t.Fatalf("expected 1 check plugin, got %d", len(rule.CheckMap))
	}
	for _, checkNode := range rule.CheckMap {
		if checkNode.Plugin != expected {
			t.Fatalf("check plugin pointer = %p, want %p", checkNode.Plugin, expected)
		}
	}

	if len(rule.AppendsMap) != 1 {
		t.Fatalf("expected 1 append plugin, got %d", len(rule.AppendsMap))
	}
	for _, appendNode := range rule.AppendsMap {
		if appendNode.Plugin != expected {
			t.Fatalf("append plugin pointer = %p, want %p", appendNode.Plugin, expected)
		}
	}

	if len(rule.ModifyMap) != 1 {
		t.Fatalf("expected 1 modify plugin, got %d", len(rule.ModifyMap))
	}
	for _, modifyNode := range rule.ModifyMap {
		if modifyNode.Plugin != expected {
			t.Fatalf("modify plugin pointer = %p, want %p", modifyNode.Plugin, expected)
		}
	}

	if len(rule.PluginMap) != 1 {
		t.Fatalf("expected 1 plugin node, got %d", len(rule.PluginMap))
	}
	for _, pluginNode := range rule.PluginMap {
		if pluginNode.Plugin != expected {
			t.Fatalf("plugin node pointer = %p, want %p", pluginNode.Plugin, expected)
		}
	}
}

func TestNewFromExistingFallsBackToPathWhenRawConfigMissing(t *testing.T) {
	raw := `<root type="DETECTION" name="path-fallback">
  <rule id="r1" name="r1">
    <check type="EQU" field="event">login</check>
  </rule>
</root>`

	path := filepath.Join(t.TempDir(), "path-fallback.xml")
	if err := os.WriteFile(path, []byte(raw), 0644); err != nil {
		t.Fatalf("failed to write ruleset file: %v", err)
	}

	existing := &Ruleset{
		RulesetID: "path-fallback",
		Path:      path,
	}

	instance, err := NewFromExisting(existing, "TEST.project.path")
	if err != nil {
		t.Fatalf("NewFromExisting failed: %v", err)
	}
	t.Cleanup(instance.cleanup)

	if instance.Path != path {
		t.Fatalf("expected path %q, got %q", path, instance.Path)
	}
	if instance.RawConfig != raw {
		t.Fatalf("expected RawConfig from path, got %q", instance.RawConfig)
	}
	if instance.ProjectNodeSequence != "TEST.project.path" {
		t.Fatalf("expected ProjectNodeSequence to be set, got %q", instance.ProjectNodeSequence)
	}
	if len(instance.Rules) != 1 {
		t.Fatalf("expected 1 parsed rule, got %d", len(instance.Rules))
	}
}
