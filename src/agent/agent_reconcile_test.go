package agent

import (
	"AgentSmith-HUB/common"
	"testing"
)

func TestAgentStartReconcilesErrorRuntime(t *testing.T) {
	a := &Agent{
		Id:         "agent-test",
		Status:     common.StatusError,
		Config:     &AgentConfig{Model: "test", Timeout: "1s"},
		UpStream:   make(map[string]*chan map[string]interface{}),
		DownStream: make(map[string]*chan map[string]interface{}),
		stopChan:   make(chan struct{}),
	}

	if err := a.Start(); err != nil {
		t.Fatalf("expected agent start to reconcile stale runtime, got error: %v", err)
	}
	defer func() { _ = a.Stop() }()

	if a.Status != common.StatusRunning {
		t.Fatalf("expected agent status %q, got %q", common.StatusRunning, a.Status)
	}
	if a.stopChan == nil {
		t.Fatal("expected agent stop channel to be reinitialized")
	}
}
