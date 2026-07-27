package project

import (
	"AgentSmith-HUB/agent"
	"AgentSmith-HUB/common"
	pluginpkg "AgentSmith-HUB/plugin"
	"AgentSmith-HUB/rules_engine"
	"strings"
	"testing"
)

func TestSafeDeletePluginComponentBlocksReferencedRulesets(t *testing.T) {
	pluginID := "delete_guard_plugin"
	rulesetID := "plugin-guard-ruleset"

	prevPlugin, pluginExists := pluginpkg.GetPlugin(pluginID)
	prevRuleset, rulesetExists := GetRuleset(rulesetID)
	prevRaw, rawExists := common.GetRawConfig("plugin", pluginID)

	defer func() {
		if pluginExists {
			pluginpkg.PluginsMu.Lock()
			pluginpkg.Plugins[pluginID] = prevPlugin
			pluginpkg.PluginsMu.Unlock()
		} else {
			pluginpkg.PluginsMu.Lock()
			delete(pluginpkg.Plugins, pluginID)
			pluginpkg.PluginsMu.Unlock()
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

	pluginpkg.PluginsMu.Lock()
	pluginpkg.Plugins[pluginID] = &pluginpkg.Plugin{Name: pluginID, Type: pluginpkg.YAEGI_PLUGIN, Status: common.StatusStopped}
	pluginpkg.PluginsMu.Unlock()
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
	if _, exists := pluginpkg.GetPlugin(pluginID); !exists {
		t.Fatal("expected plugin to remain registered after blocked deletion")
	}
	if _, exists := common.GetRawConfig("plugin", pluginID); !exists {
		t.Fatal("expected plugin raw config to remain after blocked deletion")
	}
}

func TestPluginChangeIncludesProjectsUsingAgentTools(t *testing.T) {
	pluginID := "agent_tool_plugin"
	agentID := "agent_using_plugin"
	projectID := "project_using_agent"

	prevAgent, agentExists := GetAgent(agentID)
	prevProject, projectExists := GetProject(projectID)
	defer func() {
		if agentExists {
			SetAgent(agentID, prevAgent)
		} else {
			DeleteAgent(agentID)
		}
		if projectExists {
			SetProject(projectID, prevProject)
		} else {
			DeleteProject(projectID)
		}
	}()

	SetAgent(agentID, &agent.Agent{
		Id:     agentID,
		Config: &agent.AgentConfig{Tools: []string{pluginID}},
	})
	SetProject(projectID, &Project{
		Id:     projectID,
		Status: common.StatusRunning,
		BackUpFlowNodes: []FlowNode{{
			FromType: "AGENT",
			FromID:   agentID,
		}},
	})

	agentIDs := getAgentsUsingPlugin(pluginID, true)
	affected := getProjectsUsingAgents(agentIDs, func(string, *Project) bool { return true })
	if len(affected) != 1 || affected[0] != projectID {
		t.Fatalf("expected project %q to be affected, got %v", projectID, affected)
	}
}
