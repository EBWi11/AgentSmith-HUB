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
	ID                  string    `json:"id"`
	Timestamp           time.Time `json:"timestamp"`
	NodeID              string    `json:"node_id"`
	AgentID             string    `json:"agent_id"`
	ProjectID           string    `json:"project_id,omitempty"`
	ProjectNodeSequence string    `json:"project_node_sequence,omitempty"`
	IsTest              bool      `json:"is_test,omitempty"`

	// RawInput and RawOutput are JSON-encoded snapshots of the message
	// before and after agent processing.
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

// AgentLogComment represents a single user comment on an agent log entry.
type AgentLogComment struct {
	Type             string    `json:"type,omitempty"` // "user_comment" | "memory_summary"
	Author           string    `json:"author"`
	Comment          string    `json:"comment"`
	Tag              string    `json:"tag,omitempty"`
	Status           string    `json:"status,omitempty"` // for memory_summary: committed | failed
	LinkedCommentIDs []string  `json:"linked_comment_ids,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
}

const agentLogCommentKeyPrefix = "hub:agent_log_comments:"

// AppendAgentLogComment appends a comment for a given log ID.
func AppendAgentLogComment(logID string, comment AgentLogComment) error {
	if rdb == nil {
		return fmt.Errorf("Redis client not initialized")
	}
	if logID == "" {
		return fmt.Errorf("logID is empty")
	}

	data, err := json.Marshal(comment)
	if err != nil {
		return fmt.Errorf("failed to marshal agent log comment: %w", err)
	}

	key := agentLogCommentKeyPrefix + logID
	if err := RedisLPush(key, string(data), 100); err != nil {
		return fmt.Errorf("failed to push agent log comment to Redis: %w", err)
	}

	ttlSeconds := int(agentLogRetentionDays * 24 * time.Hour / time.Second)
	_ = RedisExpire(key, ttlSeconds)
	return nil
}

// GetAgentLogComments returns comments for a given log ID (most recent first).
func GetAgentLogComments(logID string, limit int64) ([]AgentLogComment, error) {
	if rdb == nil {
		return nil, fmt.Errorf("Redis client not initialized")
	}
	if logID == "" {
		return nil, fmt.Errorf("logID is empty")
	}
	if limit <= 0 {
		limit = 50
	}

	key := agentLogCommentKeyPrefix + logID
	raw, err := RedisLRange(key, 0, limit-1)
	if err != nil {
		return nil, err
	}

	comments := make([]AgentLogComment, 0, len(raw))
	for _, s := range raw {
		var c AgentLogComment
		if err := json.Unmarshal([]byte(s), &c); err == nil {
			comments = append(comments, c)
		}
	}
	return comments, nil
}

// AgentLogMemoryCommitted reports whether this log already has a successfully
// committed memory summary (user flow is one-shot: no further comments after).
func AgentLogMemoryCommitted(logID string) (bool, error) {
	comments, err := GetAgentLogComments(logID, 200)
	if err != nil {
		return false, err
	}
	for _, c := range comments {
		if c.Type == "memory_summary" && c.Status == "committed" {
			return true, nil
		}
	}
	return false, nil
}

// FindAgentLogByID locates an agent log by ID from the agent's Redis list.
func FindAgentLogByID(agentID, logID string) (*AgentLogEntry, error) {
	if rdb == nil {
		return nil, fmt.Errorf("Redis client not initialized")
	}
	if agentID == "" || logID == "" {
		return nil, fmt.Errorf("agentID/logID is empty")
	}

	key := agentLogKeyPrefix + agentID
	rawEntries, err := RedisLRange(key, 0, int64(agentLogMaxEntries-1))
	if err != nil {
		return nil, err
	}

	for _, raw := range rawEntries {
		var entry AgentLogEntry
		if err := json.Unmarshal([]byte(raw), &entry); err != nil {
			continue
		}
		if entry.ID == logID {
			return &entry, nil
		}
	}
	return nil, nil
}

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
