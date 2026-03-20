package common

import (
	"encoding/json"
	"testing"
)

func TestCompactAgentLogTraceJSON(t *testing.T) {
	in := `[{"type":"llm","round":1,"input_messages":[{"role":"user","content":"secret"}],"output_content":"ok"},{"type":"tool","round":1,"arguments":"{}","result":"x"}]`
	out := CompactAgentLogTraceJSON(in)
	var steps []map[string]interface{}
	if err := json.Unmarshal([]byte(out), &steps); err != nil {
		t.Fatal(err)
	}
	if steps[0]["input_messages"] != TraceInputMessagesPlaceholder {
		t.Fatalf("llm input_messages: %v", steps[0]["input_messages"])
	}
	if steps[0]["output_content"] != "ok" {
		t.Fatal("output_content should remain")
	}
	if steps[1]["arguments"] != "{}" {
		t.Fatal("tool arguments should remain")
	}
}

func TestCompactAgentLogTraceJSON_InvalidJSON(t *testing.T) {
	s := "not-json"
	if CompactAgentLogTraceJSON(s) != s {
		t.Fatal("should return original")
	}
}
