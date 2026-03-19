package common

import (
	"encoding/json"
	"fmt"
	"time"
)

// AgentLogEntry represents a single agent processing log entry stored in Redis.
// It captures, from the perspective of one Agent node in a project pipeline,
// how a single message was processed (input/output plus optional tool-call trace).
type AgentLogEntry struct {
	Timestamp           time.Time `json:"timestamp"`
	NodeID              string    `json:"node_id"`
	AgentID             string    `json:"agent_id"`
	ProjectNodeSequence string    `json:"project_node_sequence,omitempty"`

	// RawInput and RawOutput are JSON-encoded snapshots of the message
	// before and after agent processing (may be truncated upstream).
	RawInput  string `json:"raw_input,omitempty"`
	RawOutput string `json:"raw_output,omitempty"`

	// Trace is an optional JSON-encoded array describing the LLM/tool
	// interaction steps (structure defined in the agent package).
	Trace string `json:"trace,omitempty"`

	// Error is an optional high-level error string if the agent failed
	// to process this message successfully (e.g. timeout, LLM error).
	Error string `json:"error,omitempty"`
}

const (
	agentLogKeyPrefix     = "hub:agent_logs:"
	agentLogMaxEntries    = 5000
	agentLogRetentionDays = 7
)

// WriteAgentLogToRedis appends an agent log entry to a per-agent Redis list
// with a fixed maximum length and a TTL of 7 days.
func WriteAgentLogToRedis(entry AgentLogEntry) error {
	if rdb == nil {
		return fmt.Errorf("Redis client not initialized")
	}
	if entry.AgentID == "" {
		return fmt.Errorf("AgentID is empty")
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("failed to marshal agent log entry: %w", err)
	}

	key := fmt.Sprintf("%s%s", agentLogKeyPrefix, entry.AgentID)

	if err := RedisLPush(key, string(data), agentLogMaxEntries); err != nil {
		return fmt.Errorf("failed to push agent log to Redis: %w", err)
	}

	ttlSeconds := int(agentLogRetentionDays * 24 * time.Hour / time.Second)
	_ = RedisExpire(key, ttlSeconds)

	return nil
}

