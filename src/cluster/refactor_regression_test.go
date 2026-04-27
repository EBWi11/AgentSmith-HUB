package cluster

import (
	"AgentSmith-HUB/common"
	"errors"
	"reflect"
	"testing"
	"time"
)

type fakeProjectCommandHandler struct {
	calls []string
}

func (h *fakeProjectCommandHandler) ExecuteCommand(projectID, action string) error {
	return h.ExecuteCommandWithOptions(projectID, action, true)
}

func (h *fakeProjectCommandHandler) ExecuteCommandWithOptions(projectID, action string, recordOperation bool) error {
	h.calls = append(h.calls, action+":"+projectID)
	return nil
}

func TestLeaderLockerHandleLockLossDemotesNodeAndExits(t *testing.T) {
	oldExit := leaderLockLossExit
	defer func() {
		leaderLockLossExit = oldExit
		common.SetNodeLeadership(false, "node-test")
	}()

	exitCode := -1
	leaderLockLossExit = func(code int) {
		exitCode = code
	}

	common.SetNodeLeadership(true, "node-test")

	locker := &LeaderLocker{}
	locker.handleLockLoss(errors.New("lost lease"))

	if common.IsCurrentNodeLeader() {
		t.Fatal("expected node to be demoted after leader lock loss")
	}
	if common.IsLeader {
		t.Fatal("expected legacy leader flag to be cleared after leader lock loss")
	}
	if exitCode != 1 {
		t.Fatalf("expected process exit code 1, got %d", exitCode)
	}
}

func TestInstructionManagerShouldCompactOnlyAfterThreshold(t *testing.T) {
	im := &InstructionManager{
		currentVersion:        1999,
		lastCompactionVersion: 0,
		maxInstructions:       2000,
	}
	if im.shouldCompactLocked() {
		t.Fatal("should not compact before reaching threshold")
	}

	im.currentVersion = 2000
	if !im.shouldCompactLocked() {
		t.Fatal("expected compaction once threshold is reached")
	}

	im.lastCompactionVersion = 2000
	im.currentVersion = 3999
	if im.shouldCompactLocked() {
		t.Fatal("should not compact again until another full threshold of writes accumulates")
	}
}

func TestBuildDeferredProjectCommandsKeepsLatestCommandAndDropsPreDeleteIntent(t *testing.T) {
	instructions := []Instruction{
		{Version: 1, ComponentName: "test", ComponentType: "project", Operation: "start"},
		{Version: 2, ComponentName: "test", ComponentType: "project", Operation: "delete"},
		{Version: 3, ComponentName: "test", ComponentType: "project", Operation: "add"},
		{Version: 4, ComponentName: "other", ComponentType: "project", Operation: "start"},
		{Version: 5, ComponentName: "other", ComponentType: "project", Operation: "restart"},
	}

	got := buildDeferredProjectCommands(instructions)
	want := []deferredProjectCommand{
		{projectName: "other", operation: "restart", version: 5},
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected deferred project commands: got %#v want %#v", got, want)
	}
}

func TestBuildProjectPlansDeferExplicitProjectCommandAfterProjectStateSync(t *testing.T) {
	instructions := []Instruction{
		{Version: 2, ComponentName: "test", ComponentType: "project", Operation: "restart"},
		{Version: 3, ComponentName: "test", ComponentType: "project", Operation: "push_change", Dependencies: []string{"test"}},
	}

	refreshPlans := buildProjectRefreshPlans(instructions)
	if _, exists := refreshPlans["test"]; exists {
		t.Fatal("expected explicit project command to suppress coalesced refresh for the same project")
	}

	commands := buildDeferredProjectCommands(instructions)
	want := []deferredProjectCommand{
		{projectName: "test", operation: "restart", version: 2},
	}
	if !reflect.DeepEqual(commands, want) {
		t.Fatalf("unexpected deferred project commands: got %#v want %#v", commands, want)
	}
}

func TestExecuteDeferredProjectCommandsFailsForMissingProjects(t *testing.T) {
	oldHandler := globalProjectCmdHandler
	defer func() {
		globalProjectCmdHandler = oldHandler
	}()

	handler := &fakeProjectCommandHandler{}
	globalProjectCmdHandler = handler

	sl := &SyncListener{}
	err := sl.executeDeferredProjectCommands([]deferredProjectCommand{
		{projectName: "missing-project", operation: "restart", version: 7},
	})
	if err == nil {
		t.Fatal("expected missing retained project command to fail the sync")
	}
	if len(handler.calls) != 0 {
		t.Fatalf("expected no project command calls for missing project, got %#v", handler.calls)
	}
}

func TestPublishInstructionBlocksWhenQueueIsFull(t *testing.T) {
	store := newFakeClusterRedis()
	store.install(t)

	defer common.SetNodeLeadership(false, "")
	common.SetNodeLeadership(true, "leader-test")

	im := &InstructionManager{
		baseVersion:   "sess",
		queue:         make(chan *PendingInstruction, 1),
		workerStopped: make(chan struct{}),
	}

	dummyResult := make(chan error, 1)
	im.queue <- &PendingInstruction{
		ComponentName: "already-buffered",
		ComponentType: "input",
		Operation:     "add",
		ResultChan:    dummyResult,
	}

	done := make(chan error, 1)
	go func() {
		done <- im.PublishInstruction("demo", "input", "content", "add", nil, nil)
	}()

	select {
	case err := <-done:
		t.Fatalf("expected publish to block behind the full queue, got %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	<-im.queue

	var pending *PendingInstruction
	select {
	case pending = <-im.queue:
	case <-time.After(time.Second):
		t.Fatal("expected publish to enqueue once queue capacity was freed")
	}
	pending.ResultChan <- nil

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("expected publish to complete successfully, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("publish did not complete after queue drain")
	}
}
