package agent

import (
	"AgentSmith-HUB/common"
	"context"
	"testing"
	"time"
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

func TestAgentActiveProcessTrackingAndCancel(t *testing.T) {
	a := &Agent{Id: "agent-active-test"}
	ctx, cancel := context.WithCancel(context.Background())

	untrack := a.trackActiveProcess(cancel)
	if got := a.GetRunningTaskCount(); got != 1 {
		t.Fatalf("expected one running task, got %d", got)
	}

	a.cancelActiveProcesses()
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("expected active process context to be cancelled")
	}

	untrack()
	if got := a.GetRunningTaskCount(); got != 0 {
		t.Fatalf("expected no running tasks, got %d", got)
	}
}
