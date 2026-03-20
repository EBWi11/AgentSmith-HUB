package agent

import (
	"AgentSmith-HUB/common"
	"AgentSmith-HUB/logger"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"
	"time"
)

// AgentTraceStep is one row in the persisted agent trace (Agent Logs / test API).
// type "llm" = one chat completion request/response (full input messages + model output).
// type "tool" = one executed tool with arguments and result string.
type AgentTraceStep struct {
	Type  string `json:"type"` // "llm" | "tool"
	Round int    `json:"round"`
	At    string `json:"at,omitempty"` // RFC3339Nano timestamp

	// --- type == "llm" ---
	Model            string              `json:"model,omitempty"`
	InputMessages    []Message           `json:"input_messages,omitempty"`
	OutputContent    string              `json:"output_content,omitempty"`
	OutputToolCalls  []ToolCallTraceItem `json:"output_tool_calls,omitempty"`
	ReasoningContent string              `json:"reasoning_content,omitempty"`
	ThinkingBlocks   json.RawMessage     `json:"thinking_blocks,omitempty"`
	Error            string              `json:"error,omitempty"`

	// --- type == "tool" ---
	ToolCallID string `json:"tool_call_id,omitempty"`
	ToolKind   string `json:"tool_kind,omitempty"` // "skill" | "plugin" | "tool"
	ToolName   string `json:"tool_name,omitempty"`
	Arguments  string `json:"arguments,omitempty"`
	Result     string `json:"result,omitempty"`

	// Legacy (older logs): role "assistant" | "tool" without type "llm"
	Role      string              `json:"role,omitempty"`
	Content   string              `json:"content,omitempty"`
	ToolCalls []ToolCallTraceItem `json:"tool_calls,omitempty"`
}

// ToolCallTraceItem is one tool call proposed by the model (in llm output or legacy assistant step).
type ToolCallTraceItem struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

func cloneMessagesForTrace(msgs []Message) []Message {
	if len(msgs) == 0 {
		return nil
	}
	// Keep trace concise: do not include system prompt in per-call input snapshot.
	filtered := make([]Message, 0, len(msgs))
	for _, m := range msgs {
		if strings.EqualFold(strings.TrimSpace(m.Role), "system") {
			continue
		}
		filtered = append(filtered, m)
	}
	data, err := json.Marshal(filtered)
	if err != nil {
		return nil
	}
	var out []Message
	if err := json.Unmarshal(data, &out); err != nil {
		return nil
	}
	return out
}

func toolKindFromName(name string) string {
	n := strings.TrimSpace(name)
	switch {
	case strings.HasPrefix(n, "skill__"):
		return "skill"
	case strings.HasPrefix(n, "tool_"):
		return "plugin"
	default:
		return "tool"
	}
}

func newLLMTraceStep(round int, model string, input []Message, resp *ChatResult, callErr error) AgentTraceStep {
	step := AgentTraceStep{
		Type:          "llm",
		Round:         round,
		At:            time.Now().UTC().Format(time.RFC3339Nano),
		Model:         model,
		InputMessages: input,
	}
	if callErr != nil {
		step.Error = callErr.Error()
		return step
	}
	if resp == nil {
		return step
	}
	step.OutputContent = resp.Content
	step.ReasoningContent = resp.ReasoningContent
	step.ThinkingBlocks = resp.ThinkingBlocks
	if len(resp.ToolCalls) > 0 {
		step.OutputToolCalls = make([]ToolCallTraceItem, 0, len(resp.ToolCalls))
		for _, c := range resp.ToolCalls {
			step.OutputToolCalls = append(step.OutputToolCalls, ToolCallTraceItem{
				ID:        c.ID,
				Name:      c.Function.Name,
				Arguments: c.Function.Arguments,
			})
		}
	}
	return step
}

// Start launches a goroutine for each upstream channel.
// Each message is treated as an independent event.
func (a *Agent) Start() error {
	if a.Status == common.StatusRunning {
		return nil
	}

	a.SetStatus(common.StatusStarting, nil)

	a.stopChan = make(chan struct{})
	a.sampler = common.GetSampler(a.Id)

	for pns, upCh := range a.UpStream {
		a.wg.Add(1)

		go func(pns string, ch *chan map[string]interface{}) {
			defer a.wg.Done()
			for {
				select {
				case <-a.stopChan:
					return
				case msg, ok := <-*ch:
					if !ok {
						return
					}
					a.processAndForward(msg)
				}
			}
		}(pns, upCh)
	}

	a.SetStatus(common.StatusRunning, nil)
	logger.Info("Agent started", "id", a.Id, "model", a.Config.Model,
		"skills", len(a.skills), "tools", len(a.toolDefs))
	return nil
}

func (a *Agent) processAndForward(msg map[string]interface{}) {
	if msg == nil {
		logger.Error("Agent received nil message, skipping", "agent", a.Id)
		return
	}

	// Snapshot original message for logging before it is mutated.
	origJSON := formatMessageAsJSON(msg)

	start := time.Now()
	result, trace := a.ProcessMessageWithTrace(msg)
	elapsedNs := time.Since(start).Nanoseconds()
	atomic.AddUint64(&a.processTotal, 1)
	atomic.AddUint64(&a.processLatencyNs, uint64(elapsedNs))
	// Record per-agent daily statistics in Redis for cluster-wide aggregation.
	_ = common.IncrementAgentDailyStats(a.Id, uint64(elapsedNs))

	// Attach processing time to this agent's LLM block if present.
	if topLlm, ok := result["llm"].(map[string]interface{}); ok {
		if agentLlm, ok := topLlm[a.Id].(map[string]interface{}); ok {
			agentLlm["processing_time_ms"] = float64(elapsedNs) / 1e6
		}
	}

	// Build high-level error string (if any) for logging purposes.
	var errStr string
	if topLlm, ok := result["llm"].(map[string]interface{}); ok {
		if agentLlm, ok := topLlm[a.Id].(map[string]interface{}); ok {
			if v, ok := agentLlm["error"].(string); ok && v != "" {
				errStr = v
			}
		}
	}

	// Serialize result and trace for logging.
	outJSON := formatMessageAsJSON(result)
	var traceJSON string
	if len(trace) > 0 {
		if data, err := json.Marshal(trace); err == nil {
			traceJSON = string(data)
		}
	}

	// Persist per-message Agent log into Redis with 7-day TTL.
	_ = common.WriteAgentLogToRedis(common.AgentLogEntry{
		ID:                  fmt.Sprintf("%d", time.Now().UnixNano()),
		Timestamp:           time.Now(),
		NodeID:              common.GetNodeID(),
		AgentID:             a.Id,
		ProjectID:           a.ProjectID,
		ProjectNodeSequence: a.ProjectNodeSequence,
		RawInput:            origJSON,
		RawOutput:           outJSON,
		Trace:               traceJSON,
		Error:               errStr,
	})

	// Optional control tags from agent output:
	// - _no_forward: do not send this message to any downstream components.
	// - _no_oridata: strip original data, only keep the merged llm block.
	//
	// These flags are expected to be set in the JSON object that the LLM
	// returns for this agent (i.e. inside msg["llm"][agentId]).
	var noForward, noOriData bool
	if topLlm, ok := result["llm"].(map[string]interface{}); ok {
		if agentLlm, ok := topLlm[a.Id].(map[string]interface{}); ok {
			if v, ok := agentLlm["_no_forward"].(bool); ok && v {
				noForward = true
			}
			if v, ok := agentLlm["_no_oridata"].(bool); ok && v {
				noOriData = true
			}
		}
	}

	// When _no_oridata is true, drop all original fields and only keep the
	// aggregated LLM results block for downstream components.
	if noOriData {
		if llmVal, ok := result["llm"]; ok {
			result = map[string]interface{}{
				"llm": llmVal,
			}
		} else {
			// No llm block; fall back to empty map to avoid leaking original data.
			result = map[string]interface{}{}
		}
	}

	if a.sampler != nil {
		a.sampler.Sample(result, a.ProjectNodeSequence)
	}

	// If _no_forward is set, stop after sampling and metrics.
	if noForward {
		return
	}

	for dsPNS, dsCh := range a.DownStream {
		if dsCh == nil {
			continue
		}

		func(pns string, ch *chan map[string]interface{}, msg map[string]interface{}) {
			defer func() {
				if r := recover(); r != nil {
					logger.Error("Agent downstream send failed (possibly closed channel)",
						"agent", a.Id,
						"downstream", pns,
						"panic", r)
				}
			}()

			select {
			case *ch <- msg:
			default:
				logger.Error("Agent downstream channel full, dropping message",
					"agent", a.Id, "downstream", pns)
			}
		}(dsPNS, dsCh, result)
	}
}

// Stop gracefully stops the agent. Safe to call even if Start was never called.
func (a *Agent) Stop() error {
	if a.Status != common.StatusRunning && a.Status != common.StatusStarting {
		return nil
	}

	a.SetStatus(common.StatusStopping, nil)
	if a.stopChan != nil {
		close(a.stopChan)
		a.stopChan = nil
	}
	a.wg.Wait()
	a.SetStatus(common.StatusStopped, nil)
	logger.Info("Agent stopped", "id", a.Id)
	return nil
}

func (a *Agent) processMessage(msg map[string]interface{}) map[string]interface{} {
	timeout, err := time.ParseDuration(a.Config.Timeout)
	if err != nil {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	conversation := []Message{
		{Role: "system", Content: a.buildEffectiveSystemPrompt()},
		{Role: "user", Content: formatMessageAsJSON(msg)},
	}
	toolDefs := a.buildAllToolDefinitions()

	for round := 0; round < a.Config.MaxRounds; round++ {
		if ctx.Err() != nil {
			logger.Error("Agent processing timed out, passing through original message",
				"agent", a.Id, "timeout", a.Config.Timeout)
			return a.attachLlmErrorAndNoForward(msg, fmt.Sprintf("agent timeout after %s", a.Config.Timeout))
		}

		resp, err := callChatWithTools(
			a.Config.Model, conversation, toolDefs,
			a.Config.MaxTokens, a.Config.Temperature,
			a.Config.ReasoningMode, a.Config.ReasoningBudgetTokens, ctx,
		)
		if err != nil {
			logger.Error("Agent LLM call failed", "agent", a.Id, "round", round, "error", err)
			return a.attachLlmErrorAndNoForward(msg, fmt.Sprintf("agent LLM call failed: %v", err))
		}

		if len(resp.ToolCalls) > 0 {
			conversation = append(conversation, resp.AssistantMessage())
			for _, call := range resp.ToolCalls {
				result := a.executeFunctionCall(call)
				conversation = append(conversation, ToolResultMessage(call.ID, result))
			}
			continue
		}

		output := parseOutputMessage(resp.Content)
		if output != nil {
			llmMap := make(map[string]interface{}, len(output)+2)
			for k, v := range output {
				llmMap[k] = v
			}
			llmMap["agent"] = a.Id
			if msg["llm"] == nil {
				msg["llm"] = make(map[string]interface{})
			}
			msg["llm"].(map[string]interface{})[a.Id] = llmMap
			return msg
		}
		// LLM 返回内容无法解析为有效 JSON，视为错误
		return a.attachLlmErrorAndNoForward(msg, "agent LLM response could not be parsed as JSON")
	}

	logger.Error("Agent max ReAct rounds exceeded, passing through original message",
		"agent", a.Id, "max_rounds", a.Config.MaxRounds)
	return a.attachLlmErrorAndNoForward(msg, "agent max ReAct rounds exceeded")
}

// ProcessMessageWithTrace runs one message through the agent and returns the result
// plus a full trace: every LLM call (input_messages + model output) and every tool
// execution (arguments + result). The final JSON-only LLM round is included.
func (a *Agent) ProcessMessageWithTrace(msg map[string]interface{}) (result map[string]interface{}, trace []AgentTraceStep) {
	timeout, err := time.ParseDuration(a.Config.Timeout)
	if err != nil {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	conversation := []Message{
		{Role: "system", Content: a.buildEffectiveSystemPrompt()},
		{Role: "user", Content: formatMessageAsJSON(msg)},
	}
	toolDefs := a.buildAllToolDefinitions()
	trace = make([]AgentTraceStep, 0)
	model := strings.TrimSpace(a.Config.Model)

	for round := 0; round < a.Config.MaxRounds; round++ {
		if ctx.Err() != nil {
			return a.attachLlmErrorAndNoForward(msg, fmt.Sprintf("agent timeout after %s", a.Config.Timeout)), trace
		}

		inputSnap := cloneMessagesForTrace(conversation)
		resp, err := callChatWithTools(
			a.Config.Model, conversation, toolDefs,
			a.Config.MaxTokens, a.Config.Temperature,
			a.Config.ReasoningMode, a.Config.ReasoningBudgetTokens, ctx,
		)
		trace = append(trace, newLLMTraceStep(round+1, model, inputSnap, resp, err))
		if err != nil {
			return a.attachLlmErrorAndNoForward(msg, fmt.Sprintf("agent LLM call failed: %v", err)), trace
		}

		if len(resp.ToolCalls) > 0 {
			conversation = append(conversation, resp.AssistantMessage())
			for _, call := range resp.ToolCalls {
				toolResult := a.executeFunctionCall(call)
				conversation = append(conversation, ToolResultMessage(call.ID, toolResult))
				trace = append(trace, AgentTraceStep{
					Type:       "tool",
					Round:      round + 1,
					At:         time.Now().UTC().Format(time.RFC3339Nano),
					ToolCallID: call.ID,
					ToolKind:   toolKindFromName(call.Function.Name),
					ToolName:   call.Function.Name,
					Arguments:  call.Function.Arguments,
					Result:     toolResult,
				})
			}
			continue
		}

		output := parseOutputMessage(resp.Content)
		if output != nil {
			llmMap := make(map[string]interface{}, len(output)+2)
			for k, v := range output {
				llmMap[k] = v
			}
			llmMap["agent"] = a.Id
			if msg["llm"] == nil {
				msg["llm"] = make(map[string]interface{})
			}
			msg["llm"].(map[string]interface{})[a.Id] = llmMap
			return msg, trace
		}
		return a.attachLlmErrorAndNoForward(msg, "agent LLM response could not be parsed as JSON"), trace
	}

	return a.attachLlmErrorAndNoForward(msg, "agent max ReAct rounds exceeded"), trace
}

// attachLlmErrorAndNoForward annotates the message with an llm error block for this agent
// and sets _no_forward=true so downstream components will not receive this message.
func (a *Agent) attachLlmErrorAndNoForward(msg map[string]interface{}, errMsg string) map[string]interface{} {
	// Ensure llm map exists
	llmAny, ok := msg["llm"]
	var llm map[string]interface{}
	if ok {
		if m, ok2 := llmAny.(map[string]interface{}); ok2 {
			llm = m
		}
	}
	if llm == nil {
		llm = make(map[string]interface{})
		msg["llm"] = llm
	}

	agentLlm := map[string]interface{}{
		"agent":       a.Id,
		"error":       errMsg,
		"_no_forward": true,
	}
	llm[a.Id] = agentLlm
	return msg
}

func formatMessageAsJSON(msg map[string]interface{}) string {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Sprintf("%v", msg)
	}
	return string(data)
}

// buildEffectiveSystemPrompt appends memory_notes to the system prompt so
// auto-generated memory can influence subsequent agent decisions.
func (a *Agent) buildEffectiveSystemPrompt() string {
	base := strings.TrimSpace(a.Config.SystemPrompt)
	mem := strings.TrimSpace(a.Config.MemoryNotes)
	if mem == "" {
		return base
	}
	return base + "\n\nMemory Notes (high priority guidance):\n" + mem
}

func parseOutputMessage(content string) map[string]interface{} {
	content = extractJSON(content)

	var single map[string]interface{}
	if err := json.Unmarshal([]byte(content), &single); err == nil {
		return single
	}

	var arr []map[string]interface{}
	if err := json.Unmarshal([]byte(content), &arr); err == nil && len(arr) > 0 {
		return arr[0]
	}

	return nil
}

// extractJSON tries to extract a JSON object or array from LLM output
// that may contain markdown fences or extra text.
func extractJSON(s string) string {
	start := -1
	for i, c := range s {
		if c == '{' || c == '[' {
			start = i
			break
		}
	}
	if start == -1 {
		return s
	}

	openChar := rune(s[start])
	var closeChar rune
	if openChar == '[' {
		closeChar = ']'
	} else {
		closeChar = '}'
	}

	depth := 0
	end := -1
	for i := start; i < len(s); i++ {
		ch := rune(s[i])
		if ch == openChar {
			depth++
		} else if ch == closeChar {
			depth--
			if depth == 0 {
				end = i + 1
				break
			}
		}
	}

	if end > start {
		return s[start:end]
	}
	return s
}
