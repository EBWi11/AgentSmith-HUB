package project

import (
	"AgentSmith-HUB/common"
	"AgentSmith-HUB/input"
	"testing"
)

func TestProjectInputStopUsesBoundRuntimeInstance(t *testing.T) {
	inputID := "shared-input"
	pns := "INPUT.shared"
	downstreamID := "RULESET.next"

	prevInput, existed := GetInput(inputID)
	defer func() {
		if existed {
			SetInput(inputID, prevInput)
			return
		}
		DeleteInput(inputID)
	}()

	oldCh := make(chan map[string]interface{}, 1)
	newCh := make(chan map[string]interface{}, 1)

	oldInput := &input.Input{
		Id:         inputID,
		Status:     common.StatusRunning,
		DownStream: make(map[string]*chan map[string]interface{}),
	}
	oldInput.SetDownstream(downstreamID, &oldCh)

	newInput := &input.Input{
		Id:         inputID,
		Status:     common.StatusRunning,
		DownStream: make(map[string]*chan map[string]interface{}),
	}
	newInput.SetDownstream(downstreamID, &newCh)

	SetInput(inputID, newInput)

	proj := &Project{
		Id:      "reload-runtime-project",
		Testing: true,
		Inputs: map[string]*input.Input{
			pns: oldInput,
		},
		FlowNodes: []FlowNode{
			{
				FromType: "INPUT",
				FromID:   inputID,
				FromPNS:  pns,
				ToPNS:    downstreamID,
				FromInit: true,
			},
		},
		BackUpFlowNodes: []FlowNode{
			{
				FromType: "INPUT",
				FromID:   inputID,
				FromPNS:  pns,
				ToPNS:    downstreamID,
				FromInit: true,
			},
		},
	}

	gotInputs := proj.GetProjectInputs()
	if gotInputs[pns] != oldInput {
		t.Fatalf("expected project to use bound input runtime instance during stop")
	}

	proj.disconnectInputsFromDownstream()
	if oldInput.DownstreamCount() != 0 {
		t.Fatalf("expected bound input downstream to be disconnected, got %d entries", oldInput.DownstreamCount())
	}
	if newInput.DownstreamCount() != 1 {
		t.Fatalf("expected new global input downstream to remain untouched, got %d entries", newInput.DownstreamCount())
	}

	oldInput.SetDownstream("RULESET.other", &oldCh)
	if errs := proj.stopInputComponents(); len(errs) > 0 {
		t.Fatalf("expected stopInputComponents to succeed, got %v", errs)
	}
	if oldInput.DownstreamCount() != 0 {
		t.Fatalf("expected bound input to be stopped and cleared, got %d downstream entries", oldInput.DownstreamCount())
	}
	if newInput.DownstreamCount() != 1 {
		t.Fatalf("expected new global input to remain running, got %d downstream entries", newInput.DownstreamCount())
	}
}
