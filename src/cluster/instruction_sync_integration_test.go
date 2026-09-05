package cluster

import (
	"AgentSmith-HUB/agent"
	"AgentSmith-HUB/common"
	"AgentSmith-HUB/input"
	"AgentSmith-HUB/output"
	"AgentSmith-HUB/project"
	"AgentSmith-HUB/rules_engine"
	"AgentSmith-HUB/skill"
	"fmt"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

const testInputRaw = "type: kafka\nkafka:\n  brokers:\n    - 127.0.0.1:9092\n  topic: test-topic\n  group: test-group\n"
const testOutputRaw = "type: kafka\nkafka:\n  brokers:\n    - 127.0.0.1:9092\n  topic: test-topic-out\n"
const testProjectRaw = "content: |\n  INPUT.demo -> OUTPUT.demo\n"
const testProjectRawV2 = "content: |\n  INPUT.demo -> OUTPUT.demo\n  # keep project materialization valid after compaction\n"

type fakeClusterRedis struct {
	mu      sync.Mutex
	data    map[string]string
	setHook func(key string, value string, expiration int) error
}

func newFakeClusterRedis() *fakeClusterRedis {
	return &fakeClusterRedis{
		data: make(map[string]string),
	}
}

func (f *fakeClusterRedis) get(key string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	val, ok := f.data[key]
	if !ok {
		return "", fmt.Errorf("key not found: %s", key)
	}
	return val, nil
}

func (f *fakeClusterRedis) set(key string, value interface{}, expiration int) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	stringValue := fmt.Sprint(value)
	if f.setHook != nil {
		if err := f.setHook(key, stringValue, expiration); err != nil {
			return "", err
		}
	}

	f.data[key] = stringValue
	return "OK", nil
}

func (f *fakeClusterRedis) del(key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	delete(f.data, key)
	return nil
}

func (f *fakeClusterRedis) keys(pattern string) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if !strings.HasSuffix(pattern, "*") {
		if _, ok := f.data[pattern]; ok {
			return []string{pattern}, nil
		}
		return nil, nil
	}

	prefix := strings.TrimSuffix(pattern, "*")
	keys := make([]string, 0)
	for key := range f.data {
		if strings.HasPrefix(key, prefix) {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys, nil
}

func (f *fakeClusterRedis) publish(channel string, message interface{}) error {
	return nil
}

func (f *fakeClusterRedis) install(t *testing.T) {
	t.Helper()

	clusterRedisGet = f.get
	clusterRedisSet = f.set
	clusterRedisDel = f.del
	clusterRedisKeys = f.keys
	clusterRedisPublish = f.publish
	clusterRetryWithExponentialBackoff = func(fn func() error, maxRetries int, baseDelay time.Duration) error {
		return fn()
	}
	clusterRecordInstruction = func(common.OperationType, string, string, string, string, string, string, map[string]interface{}) {}
	clusterRecordComponentAdd = func(string, string, string, string, string) {}
	clusterRecordComponentUpdate = func(string, string, string, string, string) {}
	clusterRecordLocalPush = func(string, string, string, string, string) {}
	clusterRecordChangePush = func(string, string, string, string, string, string, string) {}

	t.Cleanup(resetClusterRedisHooks)
}

func resetProjectStateForTest() {
	common.ClearAllRawConfigsForAllTypes()
	project.GlobalProject = &project.GlobalProjectInfo{
		Projects:    make(map[string]*project.Project),
		Inputs:      make(map[string]*input.Input),
		Outputs:     make(map[string]*output.Output),
		Rulesets:    make(map[string]*rules_engine.Ruleset),
		Agents:      make(map[string]*agent.Agent),
		Skills:      make(map[string]*skill.Skill),
		PNSOutputs:  make(map[string]*output.Output),
		PNSRulesets: make(map[string]*rules_engine.Ruleset),
		PNSAgents:   make(map[string]*agent.Agent),
		ProjectsNew: make(map[string]string),
		InputsNew:   make(map[string]string),
		OutputsNew:  make(map[string]string),
		RulesetsNew: make(map[string]string),
		AgentsNew:   make(map[string]string),
		SkillsNew:   make(map[string]string),
	}
}

func TestCompactionHistoryStillReplaysOnFollowerAfterProjectStateMaterializes(t *testing.T) {
	store := newFakeClusterRedis()
	store.install(t)
	resetProjectStateForTest()

	oldHandler := globalProjectCmdHandler
	oldSyncRetryDelay := syncRetryDelay
	restoreProjectConfigStore := project.SetProjectConfigStoreForTest(func(string, string) error { return nil })
	defer func() {
		globalProjectCmdHandler = oldHandler
		syncRetryDelay = oldSyncRetryDelay
		restoreProjectConfigStore()
		resetProjectStateForTest()
		common.SetNodeLeadership(false, "")
	}()

	common.SetNodeLeadership(true, "leader-test")
	im := &InstructionManager{
		currentVersion:        0,
		baseVersion:           "sess",
		maxInstructions:       4,
		lastCompactionVersion: 0,
	}

	if err := im.processInstructionInternal("demo", "input", testInputRaw, "add", nil, nil); err != nil {
		t.Fatalf("publish input add failed: %v", err)
	}
	if err := im.processInstructionInternal("demo", "output", testOutputRaw, "add", nil, nil); err != nil {
		t.Fatalf("publish output add failed: %v", err)
	}
	if err := im.processInstructionInternal("test", "project", testProjectRaw, "add", nil, nil); err != nil {
		t.Fatalf("publish add failed: %v", err)
	}
	if err := im.processInstructionInternal("test", "project", "", "restart", nil, nil); err != nil {
		t.Fatalf("publish restart failed: %v", err)
	}
	if err := im.processInstructionInternal("test", "project", testProjectRawV2, "push_change", []string{"test"}, map[string]interface{}{"source": "pending_changes", "affected_projects": []string{"test"}}); err != nil {
		t.Fatalf("publish push_change failed: %v", err)
	}

	if got, err := store.get("cluster:instruction:3"); err != nil || got != GetDeletedIntentionsString() {
		t.Fatalf("expected compacted tombstone at v3, got value=%q err=%v", got, err)
	}

	common.SetNodeLeadership(false, "follower-test")
	resetProjectStateForTest()
	handler := &fakeProjectCommandHandler{}
	globalProjectCmdHandler = handler
	syncRetryDelay = 0

	sl := &SyncListener{
		nodeID:           "follower-test",
		currentVersion:   0,
		baseVersion:      "sess",
		executionFlagTTL: 1,
	}

	if err := sl.SyncInstructions("sess.5"); err != nil {
		t.Fatalf("follower sync failed: %v", err)
	}
	if sl.currentVersion != 5 {
		t.Fatalf("expected follower currentVersion=5, got %d", sl.currentVersion)
	}
	if len(handler.calls) != 1 || handler.calls[0] != "restart:test" {
		t.Fatalf("unexpected deferred project command calls: %#v", handler.calls)
	}
	if _, exists := project.GetProject("test"); !exists {
		t.Fatal("expected project instance to exist after replaying push_change")
	}
}

func TestSyncInstructionsResetsFollowerOnMissingInstruction(t *testing.T) {
	store := newFakeClusterRedis()
	store.install(t)
	resetProjectStateForTest()

	oldSyncRetryDelay := syncRetryDelay
	defer func() {
		syncRetryDelay = oldSyncRetryDelay
		resetProjectStateForTest()
		common.SetNodeLeadership(false, "")
	}()

	common.SetNodeLeadership(false, "follower-test")
	syncRetryDelay = 0

	if _, err := store.set("cluster:leader_version", "sess.3", 0); err != nil {
		t.Fatalf("failed to set leader version: %v", err)
	}
	instruction1 := `{"version":1,"component_name":"demo","component_type":"input","content":"type: kafka","operation":"add","timestamp":1}`
	instruction3 := `{"version":3,"component_name":"demo","component_type":"output","content":"type: kafka","operation":"add","timestamp":3}`
	if _, err := store.set("cluster:instruction:1", instruction1, 0); err != nil {
		t.Fatalf("failed to set instruction 1: %v", err)
	}
	if _, err := store.set("cluster:instruction:3", instruction3, 0); err != nil {
		t.Fatalf("failed to set instruction 3: %v", err)
	}

	sl := &SyncListener{
		nodeID:           "follower-test",
		currentVersion:   0,
		baseVersion:      "sess",
		executionFlagTTL: 1,
	}

	err := sl.SyncInstructions("sess.3")
	if err == nil || !strings.Contains(err.Error(), "missing instructions") {
		t.Fatalf("expected missing instruction error, got %v", err)
	}
	if sl.currentVersion != 0 {
		t.Fatalf("expected follower currentVersion reset to 0, got %d", sl.currentVersion)
	}
	if got := sl.GetCurrentVersion(); got != "sess.0" {
		t.Fatalf("expected follower version sess.0 after reset, got %s", got)
	}
	if _, err := store.get(fmt.Sprintf("cluster:execution_flag:%s", sl.nodeID)); err == nil {
		t.Fatal("expected follower execution flag to be cleared after failed sync")
	}
}

func TestSyncInstructionsDoesNotAdvanceVersionWhenDeferredProjectCommandFails(t *testing.T) {
	store := newFakeClusterRedis()
	store.install(t)
	resetProjectStateForTest()

	oldHandler := globalProjectCmdHandler
	oldSyncRetryDelay := syncRetryDelay
	defer func() {
		globalProjectCmdHandler = oldHandler
		syncRetryDelay = oldSyncRetryDelay
		resetProjectStateForTest()
		common.SetNodeLeadership(false, "")
	}()

	common.SetNodeLeadership(false, "follower-test")
	globalProjectCmdHandler = &fakeProjectCommandHandler{}
	syncRetryDelay = 0

	instruction := `{"version":1,"component_name":"missing-project","component_type":"project","operation":"stop","timestamp":1}`
	if _, err := store.set("cluster:instruction:1", instruction, 0); err != nil {
		t.Fatalf("failed to seed project stop instruction: %v", err)
	}

	sl := &SyncListener{
		nodeID:           "follower-test",
		currentVersion:   0,
		baseVersion:      "sess",
		executionFlagTTL: 1,
	}
	err := sl.SyncInstructions("sess.1")
	if err == nil || !strings.Contains(err.Error(), "failed instructions") {
		t.Fatalf("expected deferred project command failure, got %v", err)
	}
	if sl.currentVersion != 0 {
		t.Fatalf("expected follower version to remain at zero, got %d", sl.currentVersion)
	}
	if got := sl.GetCurrentVersion(); got != "sess.0" {
		t.Fatalf("expected follower version sess.0 after failed execution, got %s", got)
	}
	if _, err := store.get("cluster:execution_flag:follower-test"); err == nil {
		t.Fatal("expected execution flag to be cleared after failed execution")
	}
}

func TestFollowerHeartbeatConsumesResyncFlagAndResetsForFullSync(t *testing.T) {
	store := newFakeClusterRedis()
	store.install(t)
	resetProjectStateForTest()

	oldSyncListener := GlobalSyncListener
	defer func() {
		GlobalSyncListener = oldSyncListener
		resetProjectStateForTest()
		common.SetNodeLeadership(false, "")
	}()

	common.SetNodeLeadership(false, "follower-test")
	GlobalSyncListener = &SyncListener{
		nodeID:         "follower-test",
		currentVersion: 9,
		baseVersion:    "sess",
		stopChan:       make(chan struct{}),
	}

	if _, err := store.set("cluster:resync_required:follower-test", "kicked_for_slow_sync", 60); err != nil {
		t.Fatalf("failed to set resync flag: %v", err)
	}

	hm := &HeartbeatManager{
		nodeID:   "follower-test",
		isLeader: false,
		stopChan: make(chan struct{}),
	}
	hm.sendHeartbeat()

	if GlobalSyncListener.currentVersion != 0 {
		t.Fatalf("expected follower currentVersion reset to 0, got %d", GlobalSyncListener.currentVersion)
	}
	if got := GlobalSyncListener.GetCurrentVersion(); got != "sess.0" {
		t.Fatalf("expected follower version sess.0 after reset, got %s", got)
	}
	if _, err := store.get("cluster:resync_required:follower-test"); err == nil {
		t.Fatal("expected resync flag to be cleared after heartbeat reset")
	}
}

func TestLoadAllInstructionsFailsOnUnreadableHistory(t *testing.T) {
	store := newFakeClusterRedis()
	store.install(t)

	tests := []struct {
		name    string
		seed    func(t *testing.T)
		wantErr string
	}{
		{
			name: "missing version",
			seed: func(t *testing.T) {
				t.Helper()
				if _, err := store.set("cluster:instruction:1", `{"version":1,"component_name":"demo","component_type":"input","content":"a","operation":"add","timestamp":1}`, 0); err != nil {
					t.Fatalf("seed instruction 1: %v", err)
				}
			},
			wantErr: "failed to load instruction v2",
		},
		{
			name: "invalid json",
			seed: func(t *testing.T) {
				t.Helper()
				store.mu.Lock()
				store.data["cluster:instruction:1"] = "not-json"
				store.mu.Unlock()
			},
			wantErr: "failed to unmarshal instruction v1",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store.mu.Lock()
			store.data = make(map[string]string)
			store.mu.Unlock()
			tc.seed(t)

			im := &InstructionManager{}
			_, err := im.loadAllInstructions(2)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestCompactionRollbackRestoresHistoryOnWriteFailure(t *testing.T) {
	store := newFakeClusterRedis()
	store.setHook = func(key string, value string, expiration int) error {
		if key == "cluster:instruction:3" {
			return fmt.Errorf("injected write failure")
		}
		return nil
	}
	store.install(t)

	defer common.SetNodeLeadership(false, "")
	common.SetNodeLeadership(true, "leader-test")

	im := &InstructionManager{
		currentVersion:        0,
		baseVersion:           "sess",
		maxInstructions:       2,
		lastCompactionVersion: 0,
	}

	if err := im.processInstructionInternal("demo", "input", "content-v1", "add", nil, nil); err != nil {
		t.Fatalf("publish v1 failed: %v", err)
	}
	if err := im.processInstructionInternal("demo", "input", "content-v2", "update", nil, nil); err != nil {
		t.Fatalf("publish v2 failed: %v", err)
	}

	before1, _ := store.get("cluster:instruction:1")
	before2, _ := store.get("cluster:instruction:2")

	err := im.processInstructionInternal("demo", "input", "content-v3", "push_change", nil, nil)
	if err == nil || !strings.Contains(err.Error(), "failed to store compacted instruction v3") {
		t.Fatalf("expected write failure from compaction, got %v", err)
	}

	after1, _ := store.get("cluster:instruction:1")
	after2, _ := store.get("cluster:instruction:2")
	if after1 != before1 {
		t.Fatalf("expected instruction 1 restored after rollback, got %q want %q", after1, before1)
	}
	if after2 != before2 {
		t.Fatalf("expected instruction 2 restored after rollback, got %q want %q", after2, before2)
	}
	if _, err := store.get("cluster:instruction:3"); err == nil {
		t.Fatal("expected instruction 3 to be absent after rollback")
	}
	if leaderVersion, err := store.get("cluster:leader_version"); err != nil || leaderVersion != "sess.2" {
		t.Fatalf("expected leader version restored to sess.2, got value=%q err=%v", leaderVersion, err)
	}
}

func TestCompactionRollbackRestoresHistoryOnVersionAdvanceFailure(t *testing.T) {
	store := newFakeClusterRedis()
	store.setHook = func(key string, value string, expiration int) error {
		if key == "cluster:leader_version" && value == "sess.3" {
			return fmt.Errorf("injected version update failure")
		}
		return nil
	}
	store.install(t)

	defer common.SetNodeLeadership(false, "")
	common.SetNodeLeadership(true, "leader-test")

	im := &InstructionManager{
		currentVersion:        0,
		baseVersion:           "sess",
		maxInstructions:       2,
		lastCompactionVersion: 0,
	}

	if err := im.processInstructionInternal("demo", "input", "content-v1", "add", nil, nil); err != nil {
		t.Fatalf("publish v1 failed: %v", err)
	}
	if err := im.processInstructionInternal("demo", "input", "content-v2", "update", nil, nil); err != nil {
		t.Fatalf("publish v2 failed: %v", err)
	}

	before1, _ := store.get("cluster:instruction:1")
	before2, _ := store.get("cluster:instruction:2")

	err := im.processInstructionInternal("demo", "input", "content-v3", "push_change", nil, nil)
	if err == nil || !strings.Contains(err.Error(), "failed to update version after compaction") {
		t.Fatalf("expected version update failure from compaction, got %v", err)
	}

	after1, _ := store.get("cluster:instruction:1")
	after2, _ := store.get("cluster:instruction:2")
	if after1 != before1 {
		t.Fatalf("expected instruction 1 restored after rollback, got %q want %q", after1, before1)
	}
	if after2 != before2 {
		t.Fatalf("expected instruction 2 restored after rollback, got %q want %q", after2, before2)
	}
	if _, err := store.get("cluster:instruction:3"); err == nil {
		t.Fatal("expected instruction 3 to be deleted after rollback")
	}
	if leaderVersion, err := store.get("cluster:leader_version"); err != nil || leaderVersion != "sess.2" {
		t.Fatalf("expected leader version restored to sess.2, got value=%q err=%v", leaderVersion, err)
	}
}
