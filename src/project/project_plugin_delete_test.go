package project

import (
	"AgentSmith-HUB/common"
	pluginpkg "AgentSmith-HUB/plugin"
	"AgentSmith-HUB/rules_engine"
	"strings"
	"testing"
)

func TestSafeDeletePluginComponentBlocksReferencedRulesets(t *testing.T) {
	pluginID := "delete-guard-plugin"
	rulesetID := "plugin-guard-ruleset"

	prevPlugin, pluginExists := pluginpkg.Plugins[pluginID]
	prevRuleset, rulesetExists := GetRuleset(rulesetID)
	prevRaw, rawExists := common.GetRawConfig("plugin", pluginID)

	defer func() {
		if pluginExists {
			pluginpkg.Plugins[pluginID] = prevPlugin
		} else {
			delete(pluginpkg.Plugins, pluginID)
		}
		if rulesetExists {
			SetRuleset(rulesetID, prevRuleset)
		} else {
			DeleteRuleset(rulesetID)
		}
		if rawExists {
			common.SetRawConfig("plugin", pluginID, prevRaw)
		} else {
			common.DeleteRawConfig("plugin", pluginID)
		}
	}()

	pluginpkg.Plugins[pluginID] = &pluginpkg.Plugin{Name: pluginID, Status: common.StatusStopped}
	common.SetRawConfig("plugin", pluginID, "package plugin\n")

	SetRuleset(rulesetID, &rules_engine.Ruleset{
		RulesetID: rulesetID,
		Rules: []rules_engine.Rule{
			{
				PluginMap: map[int]rules_engine.Plugin{
					0: {
						Plugin: &pluginpkg.Plugin{Name: pluginID},
					},
				},
			},
		},
	})

	_, err := SafeDeletePluginComponent(pluginID)
	if err == nil {
		t.Fatal("expected referenced plugin deletion to fail")
	}
	if !strings.Contains(err.Error(), rulesetID) {
		t.Fatalf("expected error to mention referring ruleset %q, got %v", rulesetID, err)
	}
	if _, exists := pluginpkg.Plugins[pluginID]; !exists {
		t.Fatal("expected plugin to remain registered after blocked deletion")
	}
	if _, exists := common.GetRawConfig("plugin", pluginID); !exists {
		t.Fatal("expected plugin raw config to remain after blocked deletion")
	}
}
