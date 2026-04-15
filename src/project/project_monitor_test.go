package project

import (
	"AgentSmith-HUB/agent"
	"AgentSmith-HUB/common"
	"AgentSmith-HUB/input"
	"AgentSmith-HUB/output"
	"AgentSmith-HUB/rules_engine"
	"errors"
	"testing"
)

func TestSetProjectErrorStatusStopsProjectAndLeavesErrorState(t *testing.T) {
	projectID := "monitor-test-project"
	prevProject, existed := GetProject(projectID)
	defer func() {
		if existed {
			SetProject(projectID, prevProject)
			return
		}
		DeleteProject(projectID)
	}()

	msgChan := make(chan map[string]interface{}, 1)
	proj := &Project{
		Id:          projectID,
		Status:      common.StatusRunning,
		Testing:     true,
		Inputs:      map[string]*input.Input{},
		Outputs:     map[string]*output.Output{},
		Rulesets:    map[string]*rules_engine.Ruleset{},
		Agents:      map[string]*agent.Agent{},
		MsgChannels: map[string]*chan map[string]interface{}{"test": &msgChan},
		FlowNodes: []FlowNode{
			{FromType: "INPUT", FromID: "in1", FromInit: true},
		},
		BackUpFlowNodes: []FlowNode{
			{FromType: "INPUT", FromID: "in1", FromInit: true},
		},
		stopChan: make(chan struct{}),
	}
	SetProject(projectID, proj)

	SetProjectErrorStatus(projectID, []common.ProjectComponentError{
		{
			ProjectID:   projectID,
			ComponentID: "in1",
			Type:        "input",
			Status:      common.StatusError,
			Error:       errors.New("boom"),
		},
	})

	if proj.Status != common.StatusError {
		t.Fatalf("expected project status %q, got %q", common.StatusError, proj.Status)
	}
	if proj.Err == nil || proj.Err.Error() == "" {
		t.Fatal("expected project error to be populated")
	}
	if proj.stopChan != nil {
		t.Fatal("expected stop channel to be cleared by project cleanup")
	}
	if len(proj.FlowNodes) != 0 {
		t.Fatalf("expected flow nodes to be cleared during cleanup, got %d", len(proj.FlowNodes))
	}
	if len(proj.MsgChannels) != 0 {
		t.Fatalf("expected msg channels to be cleared during cleanup, got %d", len(proj.MsgChannels))
	}
}
