package api

import (
	"AgentSmith-HUB/project"
	"fmt"
	"sync"
)

// Per-agent mutex serializes memory_notes read-modify-write so concurrent API
// calls for the same agent cannot interleave. generate-from-log uses a
// two-phase pattern (snapshot / LLM / commit) so the lock is not held during
// the LLM call; commit re-checks memory_notes content against the snapshot.
var agentMemoryWriteLocks sync.Map // agentID -> *sync.Mutex

func acquireAgentMemoryWriteLock(agentID string) func() {
	v, _ := agentMemoryWriteLocks.LoadOrStore(agentID, &sync.Mutex{})
	mu := v.(*sync.Mutex)
	mu.Lock()
	return func() { mu.Unlock() }
}

// loadAgentYAMLForMemoryUpdates returns the YAML string used for agent memory
// edits. Prefers the pending .new file when present, matching other agent
// update paths.
func loadAgentYAMLForMemoryUpdates(agentID string) (raw string, err error) {
	tempPath, tempExists := GetComponentPath("agent", agentID, true)
	if tempExists {
		if content, e := ReadComponent(tempPath); e == nil && content != "" {
			raw = content
		}
	}
	if raw == "" {
		if v, ok := project.GetAgentNew(agentID); ok {
			raw = v
		} else if a, exists := project.GetAgent(agentID); exists {
			raw = a.RawConfig
		}
	}
	if raw == "" {
		return "", fmt.Errorf("agent not found: %s", agentID)
	}
	return raw, nil
}
