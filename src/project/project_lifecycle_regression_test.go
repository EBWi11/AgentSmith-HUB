package project

import (
	"AgentSmith-HUB/agent"
	"AgentSmith-HUB/common"
	"AgentSmith-HUB/input"
	"AgentSmith-HUB/output"
	"AgentSmith-HUB/rules_engine"
	"strings"
	"testing"
	"time"
)

func TestProjectStartReturnsErrorAfterRecoveredPanic(t *testing.T) {
	p := &Project{
		Id:          "panic-start-regression",
		Status:      common.StatusStopped,
		Inputs:      make(map[string]*input.Input),
		Rulesets:    make(map[string]*rules_engine.Ruleset),
		MsgChannels: make(map[string]*chan map[string]interface{}),
	}

	if err := p.Start(false); err == nil {
		t.Fatal("expected Start to return an error after recovering a panic")
	}
	if p.Status != common.StatusError {
		t.Fatalf("expected project status error, got %s", p.Status)
	}
}

func TestProjectStopReturnsErrorAfterRecoveredPanic(t *testing.T) {
	const pns = "INPUT.panic.RULESET.stop"
	previousRuleset, hadRuleset := GetPNSRuleset(pns)
	SetPNSRuleset(pns, nil)
	t.Cleanup(func() {
		if hadRuleset {
			SetPNSRuleset(pns, previousRuleset)
		} else {
			DeletePNSRuleset(pns)
		}
	})

	p := &Project{
		Id:     "panic-stop-regression",
		Status: common.StatusRunning,
		FlowNodes: []FlowNode{{
			FromType: "INPUT", FromID: "panic", FromPNS: "INPUT.panic",
			ToType: "RULESET", ToID: "stop", ToPNS: pns, ToInit: true,
		}},
		Inputs:      make(map[string]*input.Input),
		Rulesets:    make(map[string]*rules_engine.Ruleset),
		MsgChannels: make(map[string]*chan map[string]interface{}),
	}

	err := p.Stop(false)
	if err == nil || !strings.Contains(err.Error(), "panic during stop") {
		t.Fatalf("expected Stop to return recovered panic error, got %v", err)
	}
	if p.Status != common.StatusError {
		t.Fatalf("expected project status error, got %s", p.Status)
	}
}

func TestClusterStopCommandIsIdempotentAfterProjectReplacement(t *testing.T) {
	const projectID = "cluster-stop-convergence-regression"
	previousProject, hadProject := GetProject(projectID)
	previousConfig := common.Config
	common.Config = &common.HubConfig{LocalIP: "cluster-test-node"}
	SetProject(projectID, &Project{Id: projectID, Status: common.StatusStopped})
	t.Cleanup(func() {
		common.Config = previousConfig
		if hadProject {
			SetProject(projectID, previousProject)
		} else {
			DeleteProject(projectID)
		}
	})

	handler := &projectCommandHandler{}
	if err := handler.ExecuteCommandWithOptions(projectID, "stop", false); err != nil {
		t.Fatalf("expected replayed cluster stop to converge, got %v", err)
	}
}

func TestSafeDeleteRulesetBlocksReferencedProject(t *testing.T) {
	const (
		projectID = "delete-ruleset-regression-project"
		rulesetID = "delete-ruleset-regression"
	)

	previousRuleset, hadRuleset := GetRuleset(rulesetID)
	previousProject, hadProject := GetProject(projectID)
	SetRuleset(rulesetID, &rules_engine.Ruleset{RulesetID: rulesetID})
	SetProject(projectID, &Project{
		Id:     projectID,
		Status: common.StatusRunning,
		BackUpFlowNodes: []FlowNode{{
			ToType: "RULESET",
			ToID:   rulesetID,
		}},
	})
	t.Cleanup(func() {
		if hadRuleset {
			SetRuleset(rulesetID, previousRuleset)
		} else {
			DeleteRuleset(rulesetID)
		}
		if hadProject {
			SetProject(projectID, previousProject)
		} else {
			DeleteProject(projectID)
		}
	})

	if _, err := SafeDeleteRuleset(rulesetID); err == nil {
		t.Fatal("expected referenced ruleset deletion to be blocked")
	}
	if _, exists := GetRuleset(rulesetID); !exists {
		t.Fatal("expected blocked ruleset to remain registered")
	}
}

func TestSafeDeleteComponentsBlockStoppedProjectReferences(t *testing.T) {
	const (
		projectID = "stopped-project-delete-regression"
		inputID   = "stopped-project-input"
		rulesetID = "stopped-project-ruleset"
		agentID   = "stopped-project-agent"
		outputID  = "stopped-project-output"
	)

	SetInput(inputID, &input.Input{Id: inputID})
	SetRuleset(rulesetID, &rules_engine.Ruleset{RulesetID: rulesetID})
	SetAgent(agentID, &agent.Agent{Id: agentID})
	SetOutput(outputID, &output.Output{Id: outputID})
	SetProject(projectID, &Project{
		Id:     projectID,
		Status: common.StatusStopped,
		BackUpFlowNodes: []FlowNode{
			{FromType: "INPUT", FromID: inputID, ToType: "RULESET", ToID: rulesetID},
			{FromType: "RULESET", FromID: rulesetID, ToType: "AGENT", ToID: agentID},
			{FromType: "AGENT", FromID: agentID, ToType: "OUTPUT", ToID: outputID},
		},
	})
	t.Cleanup(func() {
		DeleteProject(projectID)
		DeleteInput(inputID)
		DeleteRuleset(rulesetID)
		DeleteAgent(agentID)
		DeleteOutput(outputID)
	})

	deletions := []struct {
		name string
		run  func() ([]string, error)
	}{
		{name: "input", run: func() ([]string, error) { return SafeDeleteInput(inputID) }},
		{name: "ruleset", run: func() ([]string, error) { return SafeDeleteRuleset(rulesetID) }},
		{name: "agent", run: func() ([]string, error) { return SafeDeleteAgentComponent(agentID) }},
		{name: "output", run: func() ([]string, error) { return SafeDeleteOutput(outputID) }},
	}
	for _, deletion := range deletions {
		t.Run(deletion.name, func(t *testing.T) {
			if _, err := deletion.run(); err == nil {
				t.Fatalf("expected %s deletion to be blocked", deletion.name)
			}
		})
	}
}

func TestStoppingProjectKeepsSharedPNSRuntime(t *testing.T) {
	const (
		projectAID = "shared-pns-regression-a"
		projectBID = "shared-pns-regression-b"
		rulesetID  = "shared-pns-regression-ruleset"
		pns        = "INPUT.shared.RULESET.shared"
	)

	previousPNSRuleset, hadPNSRuleset := GetPNSRuleset(pns)
	previousProjectA, hadProjectA := GetProject(projectAID)
	previousProjectB, hadProjectB := GetProject(projectBID)

	sharedChannel := make(chan map[string]interface{}, 1)
	inbound := FlowNode{ToType: "RULESET", ToID: rulesetID, ToPNS: pns, ToInit: true}
	outputA := FlowNode{FromType: "RULESET", FromID: rulesetID, FromPNS: pns, ToType: "OUTPUT", ToID: "a", ToPNS: pns + ".OUTPUT.a"}
	outputB := FlowNode{FromType: "RULESET", FromID: rulesetID, FromPNS: pns, ToType: "OUTPUT", ToID: "b", ToPNS: pns + ".OUTPUT.b"}
	downstreamA := make(chan map[string]interface{}, 1)
	downstreamB := make(chan map[string]interface{}, 1)
	shared := &rules_engine.Ruleset{
		RulesetID: rulesetID, ProjectNodeSequence: pns,
		DownStream: map[string]*chan map[string]interface{}{
			outputA.ToPNS: &downstreamA,
			outputB.ToPNS: &downstreamB,
		},
	}
	SetPNSRuleset(pns, shared)

	projectA := &Project{
		Id: projectAID, Status: common.StatusRunning,
		FlowNodes: []FlowNode{inbound, outputA}, BackUpFlowNodes: []FlowNode{inbound, outputA},
		Inputs: make(map[string]*input.Input), Rulesets: map[string]*rules_engine.Ruleset{pns: shared},
		MsgChannels: map[string]*chan map[string]interface{}{pns: &sharedChannel},
	}
	projectB := &Project{
		Id: projectBID, Status: common.StatusRunning,
		FlowNodes: []FlowNode{inbound, outputB}, BackUpFlowNodes: []FlowNode{inbound, outputB},
		Inputs: make(map[string]*input.Input), Rulesets: map[string]*rules_engine.Ruleset{pns: shared},
		MsgChannels: make(map[string]*chan map[string]interface{}),
	}
	SetProject(projectAID, projectA)
	SetProject(projectBID, projectB)
	t.Cleanup(func() {
		if hadPNSRuleset {
			SetPNSRuleset(pns, previousPNSRuleset)
		} else {
			DeletePNSRuleset(pns)
		}
		if hadProjectA {
			SetProject(projectAID, previousProjectA)
		} else {
			DeleteProject(projectAID)
		}
		if hadProjectB {
			SetProject(projectBID, previousProjectB)
		} else {
			DeleteProject(projectBID)
		}
		close(sharedChannel)
		close(downstreamA)
		close(downstreamB)
	})

	if err := projectA.stopComponentsInternalWithTimeout(time.Millisecond); err != nil {
		t.Fatalf("stop components: %v", err)
	}
	if got, exists := GetPNSRuleset(pns); !exists || got != shared {
		t.Fatal("expected shared PNS ruleset to remain registered")
	}
	downstreams := shared.CopyDownstream()
	if _, exists := downstreams[outputA.ToPNS]; exists {
		t.Fatal("expected stopped project's ruleset route to be removed")
	}
	if _, exists := downstreams[outputB.ToPNS]; !exists {
		t.Fatal("expected running project's ruleset route to remain")
	}

	select {
	case sharedChannel <- map[string]interface{}{"message": "still open"}:
	default:
		t.Fatal("expected shared project channel to remain open and writable")
	}
}

func TestStoppingLastProjectReleasesPNSComponents(t *testing.T) {
	const (
		rulesetPNS = "INPUT.last.RULESET.release"
		agentPNS   = rulesetPNS + ".AGENT.release"
		outputPNS  = agentPNS + ".OUTPUT.release"
	)

	rs := &rules_engine.Ruleset{RulesetID: "last-release-ruleset", Status: common.StatusStopped}
	a := &agent.Agent{Id: "last-release-agent", Status: common.StatusStopped}
	out := &output.Output{Id: "last-release-output", Status: common.StatusStopped}
	SetPNSRuleset(rulesetPNS, rs)
	SetPNSAgent(agentPNS, a)
	SetPNSOutput(outputPNS, out)
	t.Cleanup(func() {
		DeletePNSRuleset(rulesetPNS)
		DeletePNSAgent(agentPNS)
		DeletePNSOutput(outputPNS)
	})

	p := &Project{
		Id: "last-release-project", Status: common.StatusStopping, Testing: true,
		FlowNodes: []FlowNode{
			{
				FromType: "INPUT", FromID: "last", FromPNS: "INPUT.last",
				ToType: "RULESET", ToID: rs.RulesetID, ToPNS: rulesetPNS, ToInit: true,
			},
			{
				FromType: "RULESET", FromID: rs.RulesetID, FromPNS: rulesetPNS, FromInit: true,
				ToType: "AGENT", ToID: a.Id, ToPNS: agentPNS, ToInit: true,
			},
			{
				FromType: "AGENT", FromID: a.Id, FromPNS: agentPNS, FromInit: true,
				ToType: "OUTPUT", ToID: out.Id, ToPNS: outputPNS, ToInit: true,
			},
		},
		Inputs:      make(map[string]*input.Input),
		MsgChannels: make(map[string]*chan map[string]interface{}),
	}

	if err := p.stopComponentsInternalWithTimeout(time.Nanosecond); err != nil {
		t.Fatalf("stop last project components: %v", err)
	}
	if _, exists := GetPNSRuleset(rulesetPNS); exists {
		t.Fatal("expected last ruleset PNS reference to be released")
	}
	if _, exists := GetPNSAgent(agentPNS); exists {
		t.Fatal("expected last agent PNS reference to be released")
	}
	if _, exists := GetPNSOutput(outputPNS); exists {
		t.Fatal("expected last output PNS reference to be released")
	}
}

func TestStoppingProjectKeepsSharedInputRoute(t *testing.T) {
	const (
		fromPNS    = "INPUT.shared-route"
		downstream = "INPUT.shared-route.RULESET.shared"
	)

	ch := make(chan map[string]interface{}, 1)
	sharedInput := &input.Input{
		Id:         "shared-input-route-regression",
		Status:     common.StatusRunning,
		DownStream: make(map[string]*chan map[string]interface{}),
	}
	sharedInput.SetDownstream(downstream, &ch)

	node := FlowNode{
		FromType: "INPUT", FromID: sharedInput.Id, FromPNS: fromPNS,
		ToType: "RULESET", ToID: "shared", ToPNS: downstream, FromInit: true,
	}
	projectA := &Project{
		Id: "shared-route-regression-a", Status: common.StatusStopping,
		Inputs: map[string]*input.Input{fromPNS: sharedInput}, FlowNodes: []FlowNode{node},
	}
	projectB := &Project{
		Id: "shared-route-regression-b", Status: common.StatusRunning,
		Inputs: map[string]*input.Input{fromPNS: sharedInput}, FlowNodes: []FlowNode{node},
	}
	SetProject(projectA.Id, projectA)
	SetProject(projectB.Id, projectB)
	t.Cleanup(func() {
		DeleteProject(projectA.Id)
		DeleteProject(projectB.Id)
	})

	projectA.disconnectInputsFromDownstream()
	if got := sharedInput.DownstreamCount(); got != 1 {
		t.Fatalf("expected shared input route to remain, got %d downstreams", got)
	}
}

func TestCalculateEdgeRefCountUsesOnlyOtherRunningProjects(t *testing.T) {
	const (
		fromPNS = "INPUT.edge-count"
		toPNS   = "INPUT.edge-count.RULESET.shared"
	)
	node := FlowNode{FromPNS: fromPNS, ToPNS: toPNS}
	projectA := &Project{Id: "edge-count-a", Status: common.StatusStopping, FlowNodes: []FlowNode{node}}
	projectB := &Project{Id: "edge-count-b", Status: common.StatusRunning, FlowNodes: []FlowNode{node}}
	projectC := &Project{Id: "edge-count-c", Status: common.StatusStopped, FlowNodes: []FlowNode{node}}
	SetProject(projectA.Id, projectA)
	SetProject(projectB.Id, projectB)
	SetProject(projectC.Id, projectC)
	t.Cleanup(func() {
		DeleteProject(projectA.Id)
		DeleteProject(projectB.Id)
		DeleteProject(projectC.Id)
	})

	if got := CalculateEdgeRefCount(fromPNS, toPNS, projectA.Id); got != 1 {
		t.Fatalf("expected one other running edge reference, got %d", got)
	}
	projectB.Status = common.StatusStopping
	if got := CalculateEdgeRefCount(fromPNS, toPNS, projectA.Id); got != 0 {
		t.Fatalf("expected stopping and stopped projects to be ignored, got %d", got)
	}
}

func TestStoppingProjectRemovesUnsharedInputRoute(t *testing.T) {
	const (
		fromPNS    = "INPUT.unshared-route"
		downstream = "INPUT.unshared-route.RULESET.only"
	)

	ch := make(chan map[string]interface{}, 1)
	in := &input.Input{
		Id:         "unshared-input-route-regression",
		Status:     common.StatusRunning,
		DownStream: make(map[string]*chan map[string]interface{}),
	}
	in.SetDownstream(downstream, &ch)

	projectA := &Project{
		Id: "unshared-route-regression", Status: common.StatusStopping,
		Inputs: map[string]*input.Input{fromPNS: in},
		FlowNodes: []FlowNode{{
			FromType: "INPUT", FromID: in.Id, FromPNS: fromPNS,
			ToType: "RULESET", ToID: "only", ToPNS: downstream, FromInit: true,
		}},
	}
	SetProject(projectA.Id, projectA)
	t.Cleanup(func() { DeleteProject(projectA.Id) })

	projectA.disconnectInputsFromDownstream()
	if got := in.DownstreamCount(); got != 0 {
		t.Fatalf("expected unshared input route to be removed, got %d downstreams", got)
	}
}

func TestStoppingProjectKeepsSharedRulesetAndAgentEdges(t *testing.T) {
	const (
		projectAID = "shared-component-edge-regression-a"
		projectBID = "shared-component-edge-regression-b"
		rulesetPNS = "INPUT.shared.RULESET.edge"
		agentPNS   = rulesetPNS + ".AGENT.edge"
		outputPNS  = agentPNS + ".OUTPUT.shared"
	)

	rulesetToAgent := make(chan map[string]interface{}, 1)
	agentToOutput := make(chan map[string]interface{}, 1)
	sharedRuleset := &rules_engine.Ruleset{
		RulesetID: "shared-component-edge-ruleset",
		DownStream: map[string]*chan map[string]interface{}{
			agentPNS: &rulesetToAgent,
		},
	}
	sharedAgent := &agent.Agent{
		Id: "shared-component-edge-agent",
		DownStream: map[string]*chan map[string]interface{}{
			outputPNS: &agentToOutput,
		},
	}

	previousRuleset, hadRuleset := GetPNSRuleset(rulesetPNS)
	previousAgent, hadAgent := GetPNSAgent(agentPNS)
	SetPNSRuleset(rulesetPNS, sharedRuleset)
	SetPNSAgent(agentPNS, sharedAgent)

	nodes := []FlowNode{
		{
			FromType: "RULESET", FromID: sharedRuleset.RulesetID, FromPNS: rulesetPNS,
			ToType: "AGENT", ToID: sharedAgent.Id, ToPNS: agentPNS,
		},
		{
			FromType: "AGENT", FromID: sharedAgent.Id, FromPNS: agentPNS,
			ToType: "OUTPUT", ToID: "shared", ToPNS: outputPNS,
		},
	}
	projectA := &Project{Id: projectAID, Status: common.StatusStopping, FlowNodes: nodes}
	projectB := &Project{Id: projectBID, Status: common.StatusRunning, FlowNodes: nodes}
	SetProject(projectAID, projectA)
	SetProject(projectBID, projectB)
	t.Cleanup(func() {
		DeleteProject(projectAID)
		DeleteProject(projectBID)
		if hadRuleset {
			SetPNSRuleset(rulesetPNS, previousRuleset)
		} else {
			DeletePNSRuleset(rulesetPNS)
		}
		if hadAgent {
			SetPNSAgent(agentPNS, previousAgent)
		} else {
			DeletePNSAgent(agentPNS)
		}
	})

	projectA.cleanupRulesetChannel()
	if _, exists := sharedRuleset.CopyDownstream()[agentPNS]; !exists {
		t.Fatal("expected exact ruleset edge shared by running project to remain")
	}
	if _, exists := sharedAgent.CopyDownstream()[outputPNS]; !exists {
		t.Fatal("expected exact agent edge shared by running project to remain")
	}
}

func TestSharedPNSRequiresFullRestartInsteadOfHotReload(t *testing.T) {
	const pns = "INPUT.shared.RULESET.reload"
	projectA := &Project{
		Id: "shared-reload-regression-a",
		FlowNodes: []FlowNode{{
			ToPNS: pns,
		}},
	}
	projectB := &Project{
		Id:     "shared-reload-regression-b",
		Status: common.StatusRunning,
		FlowNodes: []FlowNode{{
			ToPNS: pns,
		}},
	}
	SetProject(projectA.Id, projectA)
	SetProject(projectB.Id, projectB)
	t.Cleanup(func() {
		DeleteProject(projectA.Id)
		DeleteProject(projectB.Id)
	})

	if !projectA.hasSharedPNS(map[string]struct{}{pns: {}}) {
		t.Fatal("expected shared PNS to require a full project restart")
	}
}

func TestSharedPNSHotReloadReturnsFallbackSignal(t *testing.T) {
	const (
		projectAID = "shared-hot-reload-regression-a"
		projectBID = "shared-hot-reload-regression-b"
		rulesetID  = "shared-hot-reload-ruleset"
		agentID    = "shared-hot-reload-agent"
		rulesetPNS = "INPUT.shared.RULESET.reload"
		agentPNS   = rulesetPNS + ".AGENT.reload"
	)

	SetRuleset(rulesetID, &rules_engine.Ruleset{RulesetID: rulesetID})
	SetAgent(agentID, &agent.Agent{Id: agentID})
	projectA := &Project{
		Id: projectAID, Status: common.StatusRunning,
		FlowNodes: []FlowNode{
			{ToType: "RULESET", ToID: rulesetID, ToPNS: rulesetPNS, ToInit: true},
			{ToType: "AGENT", ToID: agentID, ToPNS: agentPNS, ToInit: true},
		},
	}
	projectB := &Project{
		Id: projectBID, Status: common.StatusRunning,
		FlowNodes: []FlowNode{
			{ToPNS: rulesetPNS},
			{ToPNS: agentPNS},
		},
	}
	SetProject(projectAID, projectA)
	SetProject(projectBID, projectB)
	t.Cleanup(func() {
		DeleteProject(projectAID)
		DeleteProject(projectBID)
		DeleteRuleset(rulesetID)
		DeleteAgent(agentID)
	})

	if err := projectA.HotReloadRuleset(rulesetID, "cluster_sync"); err == nil || !strings.Contains(err.Error(), "full restart required") {
		t.Fatalf("expected shared ruleset hot reload to request fallback restart, got %v", err)
	}
	if err := projectA.HotReloadAgent(agentID, "cluster_sync"); err == nil || !strings.Contains(err.Error(), "full restart required") {
		t.Fatalf("expected shared agent hot reload to request fallback restart, got %v", err)
	}
}
