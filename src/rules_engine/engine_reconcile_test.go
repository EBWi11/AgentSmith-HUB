package rules_engine

import (
	"AgentSmith-HUB/common"
	"testing"
)

func TestRulesetStartReconcilesErrorRuntime(t *testing.T) {
	r := &Ruleset{
		RulesetID:  "rs-test",
		Status:     common.StatusError,
		UpStream:   make(map[string]*chan map[string]interface{}),
		DownStream: make(map[string]*chan map[string]interface{}),
		stopChan:   make(chan struct{}),
	}

	if err := r.Start(); err != nil {
		t.Fatalf("expected ruleset start to reconcile stale runtime, got error: %v", err)
	}
	defer func() { _ = r.Stop() }()

	if r.Status != common.StatusRunning {
		t.Fatalf("expected ruleset status %q, got %q", common.StatusRunning, r.Status)
	}
	if r.stopChan == nil {
		t.Fatal("expected ruleset stop channel to be reinitialized")
	}
}
