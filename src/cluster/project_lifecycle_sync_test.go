package cluster

import (
	"AgentSmith-HUB/common"
	"AgentSmith-HUB/input"
	"AgentSmith-HUB/output"
	"AgentSmith-HUB/project"
	"AgentSmith-HUB/rules_engine"
	"fmt"
	"reflect"
	"testing"
)

type fakeProjectRuntimeRefresher struct {
	calls              []string
	rulesetReloadError error
	agentReloadError   error
	restartError       error
}

func (f *fakeProjectRuntimeRefresher) Restart(_ bool, triggeredBy string) error {
	f.calls = append(f.calls, "restart:"+triggeredBy)
	return f.restartError
}

func (f *fakeProjectRuntimeRefresher) HotReloadRuleset(rulesetID, triggeredBy string) error {
	f.calls = append(f.calls, "ruleset:"+rulesetID+":"+triggeredBy)
	return f.rulesetReloadError
}

func (f *fakeProjectRuntimeRefresher) HotReloadAgent(agentID, triggeredBy string) error {
	f.calls = append(f.calls, "agent:"+agentID+":"+triggeredBy)
	return f.agentReloadError
}

func TestFollowerProjectMaterializationCanonicalizesMergedInputPNS(t *testing.T) {
	const (
		projectID = "cluster-multi-input-project"
		inputAID  = "cluster-multi-input-a"
		inputBID  = "cluster-multi-input-b"
		rulesetID = "cluster-multi-input-ruleset"
		outputID  = "cluster-multi-input-output"
	)

	newInput := func(id string) *input.Input {
		in, err := input.NewInput("", `
type: kafka
kafka:
  brokers: ["localhost:9092"]
  group: cluster-test
  topic: cluster-test
`, id)
		if err != nil {
			t.Fatalf("create input %s: %v", id, err)
		}
		return in
	}
	rs, err := rules_engine.NewRuleset("", `
<root type="DETECTION" name="cluster-multi-input">
  <rule id="pass" name="pass">
    <check type="EQU" field="event">login</check>
  </rule>
</root>`, rulesetID)
	if err != nil {
		t.Fatalf("create ruleset: %v", err)
	}
	out, err := output.NewOutput("", `type: print`, outputID)
	if err != nil {
		t.Fatalf("create output: %v", err)
	}

	project.SetInput(inputAID, newInput(inputAID))
	project.SetInput(inputBID, newInput(inputBID))
	project.SetRuleset(rulesetID, rs)
	project.SetOutput(outputID, out)
	restoreStore := project.SetProjectConfigStoreForTest(func(string, string) error { return nil })
	t.Cleanup(func() {
		restoreStore()
		project.DeleteProject(projectID)
		project.DeleteInput(inputAID)
		project.DeleteInput(inputBID)
		project.DeleteRuleset(rulesetID)
		project.DeleteOutput(outputID)
		common.DeleteRawConfig("project", projectID)
	})

	content := "content: |\n" +
		"  INPUT." + inputAID + " -> RULESET." + rulesetID + "\n" +
		"  INPUT." + inputBID + " -> RULESET." + rulesetID + "\n" +
		"  RULESET." + rulesetID + " -> OUTPUT." + outputID + "\n"
	sl := &SyncListener{}
	if err := sl.createComponentInstance("project", projectID, content); err != nil {
		t.Fatalf("follower project materialization failed: %v", err)
	}

	proj, exists := project.GetProject(projectID)
	if !exists {
		t.Fatal("expected follower project to be registered")
	}
	var rulesetPNS string
	for _, node := range proj.FlowNodes {
		if node.ToType == "RULESET" && node.ToID == rulesetID {
			if rulesetPNS != "" && rulesetPNS != node.ToPNS {
				t.Fatalf("follower generated inconsistent merged PNS values %q and %q", rulesetPNS, node.ToPNS)
			}
			rulesetPNS = node.ToPNS
		}
		if node.FromType == "RULESET" && node.FromID == rulesetID {
			if rulesetPNS != "" && rulesetPNS != node.FromPNS {
				t.Fatalf("follower outbound PNS %q differs from inbound PNS %q", node.FromPNS, rulesetPNS)
			}
			rulesetPNS = node.FromPNS
		}
	}
	if rulesetPNS == "" {
		t.Fatal("expected follower to generate a ruleset PNS")
	}
}

func TestFollowerRefreshPlanCoalescesRulesetAndAgentUpdates(t *testing.T) {
	const projectID = "cluster-coalesced-refresh-project"
	instructions := []Instruction{
		{
			Version: 1, ComponentName: "ruleset-a", ComponentType: "ruleset",
			Operation: "push_change", Dependencies: []string{projectID},
		},
		{
			Version: 2, ComponentName: "agent-a", ComponentType: "agent",
			Operation: "push_change", Dependencies: []string{projectID},
		},
	}

	plans := buildProjectRefreshPlans(instructions)
	plan, exists := plans[projectID]
	if !exists {
		t.Fatal("expected follower refresh plan")
	}
	if plan.restart {
		t.Fatal("expected component hot reload plan before runtime sharing is evaluated")
	}
	if _, exists := plan.rulesets["ruleset-a"]; !exists {
		t.Fatal("expected ruleset update in coalesced plan")
	}
	if _, exists := plan.agents["agent-a"]; !exists {
		t.Fatal("expected agent update in coalesced plan")
	}
}

func TestFollowerRefreshPlanEscalatesInputDeleteToRestart(t *testing.T) {
	const projectID = "cluster-input-delete-project"
	tests := []struct {
		name         string
		instructions []Instruction
	}{
		{
			name: "restart instruction follows hot reload",
			instructions: []Instruction{
				{Version: 1, ComponentName: "ruleset-a", ComponentType: "ruleset", Operation: "push_change", Dependencies: []string{projectID}},
				{Version: 2, ComponentName: "input-a", ComponentType: "input", Operation: "delete", Dependencies: []string{projectID}},
			},
		},
		{
			name: "hot reload follows restart instruction",
			instructions: []Instruction{
				{Version: 1, ComponentName: "input-a", ComponentType: "input", Operation: "delete", Dependencies: []string{projectID}},
				{Version: 2, ComponentName: "agent-a", ComponentType: "agent", Operation: "push_change", Dependencies: []string{projectID}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan, exists := buildProjectRefreshPlans(tt.instructions)[projectID]
			if !exists {
				t.Fatal("expected follower refresh plan")
			}
			if !plan.restart {
				t.Fatal("expected input deletion to require a full project restart")
			}
			if len(plan.rulesets) != 0 || len(plan.agents) != 0 {
				t.Fatal("expected full restart to supersede component hot reloads")
			}
		})
	}
}

func TestFollowerDeferredCommandsKeepLatestProjectState(t *testing.T) {
	const projectID = "cluster-project-state-convergence"
	commands := buildDeferredProjectCommands([]Instruction{
		{Version: 1, ComponentName: projectID, ComponentType: "project", Operation: "start"},
		{Version: 2, ComponentName: projectID, ComponentType: "project", Operation: "stop"},
		{Version: 3, ComponentName: projectID, ComponentType: "project", Operation: "start"},
	})

	if len(commands) != 1 {
		t.Fatalf("expected one converged project command, got %#v", commands)
	}
	if commands[0].projectName != projectID || commands[0].operation != "start" || commands[0].version != 3 {
		t.Fatalf("unexpected converged project command: %#v", commands[0])
	}
}

func TestFollowerRulesetFallbackRestartSkipsRemainingHotReloads(t *testing.T) {
	runtime := &fakeProjectRuntimeRefresher{
		rulesetReloadError: fmt.Errorf("full restart required"),
	}
	plan := &projectRefreshPlan{
		projectName: "shared-project",
		source:      "cluster_change",
		rulesets: map[string]struct{}{
			"ruleset-a": {},
			"ruleset-b": {},
		},
		agents: map[string]struct{}{
			"agent-a": {},
		},
	}

	if err := executeProjectRefreshPlan(plan, runtime); err != nil {
		t.Fatalf("execute refresh plan: %v", err)
	}
	want := []string{
		"ruleset:ruleset-a:cluster_change",
		"restart:cluster_change_fallback",
	}
	if !reflect.DeepEqual(runtime.calls, want) {
		t.Fatalf("expected one fallback restart and no later hot reloads, got %#v want %#v", runtime.calls, want)
	}
}

func TestFollowerRefreshPlanReturnsFallbackRestartFailure(t *testing.T) {
	runtime := &fakeProjectRuntimeRefresher{
		agentReloadError: fmt.Errorf("reload failed"),
		restartError:     fmt.Errorf("restart failed"),
	}
	plan := &projectRefreshPlan{
		projectName: "failed-project",
		agents: map[string]struct{}{
			"agent-a": {},
		},
		rulesets: make(map[string]struct{}),
	}

	err := executeProjectRefreshPlan(plan, runtime)
	if err == nil {
		t.Fatal("expected fallback restart failure")
	}
	wantCalls := []string{
		"agent:agent-a:cluster_sync",
		"restart:cluster_sync_fallback",
	}
	if !reflect.DeepEqual(runtime.calls, wantCalls) {
		t.Fatalf("unexpected fallback call sequence: %#v", runtime.calls)
	}
}
