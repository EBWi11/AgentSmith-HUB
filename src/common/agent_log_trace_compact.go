package common

import (
	"encoding/json"
	"strings"
)

// TraceInputMessagesPlaceholder is stored in persisted agent logs instead of full LLM
// input_messages (verbose duplicate of conversation). Full inbound JSON stays in RawInput.
// Use an explicit token — not a bullet — so it is not mistaken for punctuation or corruption.
const TraceInputMessagesPlaceholder = "[omitted]"

// CompactAgentLogTraceJSON rewrites a trace JSON array: each llm step's input_messages
// becomes TraceInputMessagesPlaceholder. Tool rows and LLM outputs are unchanged.
func CompactAgentLogTraceJSON(trace string) string {
	trace = strings.TrimSpace(trace)
	if trace == "" {
		return ""
	}
	var steps []map[string]interface{}
	if err := json.Unmarshal([]byte(trace), &steps); err != nil {
		return trace
	}
	ph := TraceInputMessagesPlaceholder
	for _, step := range steps {
		if step == nil {
			continue
		}
		if typ, _ := step["type"].(string); typ == "llm" {
			step["input_messages"] = ph
		}
	}
	b, err := json.Marshal(steps)
	if err != nil {
		return trace
	}
	return string(b)
}
