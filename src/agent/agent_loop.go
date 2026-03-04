package agent

import (
	"AgentSmith-HUB/common"
	"AgentSmith-HUB/logger"
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"time"
)

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
	start := time.Now()
	result := a.processMessage(msg)
	elapsedNs := time.Since(start).Nanoseconds()
	atomic.AddUint64(&a.processTotal, 1)
	atomic.AddUint64(&a.processLatencyNs, uint64(elapsedNs))
	a.RecordDailyStats(uint64(elapsedNs))

	if topLlm, ok := result["llm"].(map[string]interface{}); ok {
		if agentLlm, ok := topLlm[a.Id].(map[string]interface{}); ok {
			agentLlm["processing_time_ms"] = float64(elapsedNs) / 1e6
		}
	}

	if a.sampler != nil {
		a.sampler.Sample(result, a.ProjectNodeSequence)
	}
	for dsPNS, dsCh := range a.DownStream {
		select {
		case *dsCh <- result:
		default:
			logger.Error("Agent downstream channel full, dropping message",
				"agent", a.Id, "downstream", dsPNS)
		}
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
		{Role: "system", Content: a.Config.SystemPrompt},
		{Role: "user", Content: formatMessageAsJSON(msg)},
	}
	toolDefs := a.buildAllToolDefinitions()

	for round := 0; round < a.Config.MaxRounds; round++ {
		if ctx.Err() != nil {
			logger.Error("Agent processing timed out, passing through original message",
				"agent", a.Id, "timeout", a.Config.Timeout)
			return msg
		}

		resp, err := callChatWithTools(
			a.Config.Model, conversation, toolDefs,
			a.Config.MaxTokens, a.Config.Temperature, ctx,
		)
		if err != nil {
			logger.Error("Agent LLM call failed", "agent", a.Id, "round", round, "error", err)
			return msg
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
		return msg
	}

	logger.Error("Agent max ReAct rounds exceeded, passing through original message",
		"agent", a.Id, "max_rounds", a.Config.MaxRounds)
	return msg
}

func formatMessageAsJSON(msg map[string]interface{}) string {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Sprintf("%v", msg)
	}
	return string(data)
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
