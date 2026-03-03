package agent

import (
	"AgentSmith-HUB/common"
	"AgentSmith-HUB/logger"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// BatchBuffer accumulates messages and flushes them as a batch.
type BatchBuffer struct {
	messages []map[string]interface{}
	size     int
	timeout  time.Duration
	mu       sync.Mutex
	timer    *time.Timer
	flushCh  chan []map[string]interface{}
	stopCh   chan struct{}
}

func newBatchBuffer(size int, timeout time.Duration) *BatchBuffer {
	return &BatchBuffer{
		size:    size,
		timeout: timeout,
		flushCh: make(chan []map[string]interface{}, 4),
		stopCh:  make(chan struct{}),
	}
}

func (b *BatchBuffer) add(msg map[string]interface{}) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.messages = append(b.messages, msg)

	if len(b.messages) >= b.size {
		b.flushLocked()
		return
	}

	if b.timer == nil {
		b.timer = time.AfterFunc(b.timeout, func() {
			b.mu.Lock()
			defer b.mu.Unlock()
			if len(b.messages) > 0 {
				b.flushLocked()
			}
		})
	}
}

func (b *BatchBuffer) flushLocked() {
	batch := b.messages
	b.messages = nil
	if b.timer != nil {
		b.timer.Stop()
		b.timer = nil
	}
	select {
	case b.flushCh <- batch:
	case <-b.stopCh:
	}
}

func (b *BatchBuffer) flushRemaining() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.messages) > 0 {
		batch := b.messages
		b.messages = nil
		if b.timer != nil {
			b.timer.Stop()
			b.timer = nil
		}
		b.flushCh <- batch
	}
}

func (b *BatchBuffer) stop() {
	close(b.stopCh)
	b.mu.Lock()
	if b.timer != nil {
		b.timer.Stop()
		b.timer = nil
	}
	b.mu.Unlock()
}

// Start launches goroutines for each upstream channel.
func (a *Agent) Start() error {
	if a.Status == common.StatusRunning {
		return nil
	}

	// In leader_only mode, skip starting on non-leader nodes
	if a.Config.Distributed.Mode == "leader_only" && !common.IsLeader {
		logger.Info("Agent skipped on non-leader node", "id", a.Id, "mode", "leader_only")
		a.SetStatus(common.StatusStopped, nil)
		return nil
	}

	a.SetStatus(common.StatusStarting, nil)

	batchTimeout, err := time.ParseDuration(a.Config.Batch.Timeout)
	if err != nil {
		batchTimeout = 30 * time.Second
	}

	a.stopChan = make(chan struct{})
	a.sampler = common.GetSampler(a.Id)

	// Set up per-instance rate limiter if configured
	if a.Config.Distributed.RateLimitRPS > 0 {
		a.rateLimitInterval = time.Duration(float64(time.Second) / a.Config.Distributed.RateLimitRPS)
	}

	for pns, upCh := range a.UpStream {
		buf := newBatchBuffer(a.Config.Batch.Size, batchTimeout)

		a.wg.Add(2)

		// Reader goroutine: reads from upstream and adds to buffer
		go func(pns string, ch *chan map[string]interface{}, buffer *BatchBuffer) {
			defer a.wg.Done()
			defer func() {
				buffer.stop()
				buffer.flushRemaining()
				close(buffer.flushCh)
			}()
			for {
				select {
				case <-a.stopChan:
					return
				case msg, ok := <-*ch:
					if !ok {
						return
					}
					buffer.add(msg)
				}
			}
		}(pns, upCh, buf)

		// Processor goroutine: reads batches and runs ReAct loop
		go func(pns string, buffer *BatchBuffer) {
			defer a.wg.Done()
			for batch := range buffer.flushCh {
				if len(batch) == 0 {
					continue
				}
				a.processAndForward(batch)
			}
		}(pns, buf)
	}

	a.SetStatus(common.StatusRunning, nil)
	logger.Info("Agent started", "id", a.Id, "model", a.Config.Model,
		"skills", len(a.skills), "tools", len(a.toolDefs))
	return nil
}

func (a *Agent) processAndForward(batch []map[string]interface{}) {
	results := a.processBatch(batch)
	atomic.AddUint64(&a.processTotal, uint64(len(batch)))

	for _, result := range results {
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
}

// Stop gracefully stops the agent.
func (a *Agent) Stop() error {
	if a.Status != common.StatusRunning && a.Status != common.StatusStarting {
		return nil
	}

	a.SetStatus(common.StatusStopping, nil)
	close(a.stopChan)
	a.wg.Wait()
	a.SetStatus(common.StatusStopped, nil)
	logger.Info("Agent stopped", "id", a.Id)
	return nil
}

func (a *Agent) processBatch(batch []map[string]interface{}) []map[string]interface{} {
	if len(batch) == 0 {
		return nil
	}

	conversation := []Message{
		{Role: "system", Content: a.Config.SystemPrompt},
		{Role: "user", Content: formatBatchAsJSON(batch)},
	}
	toolDefs := a.buildAllToolDefinitions()

	for round := 0; round < a.Config.Batch.MaxRounds; round++ {
		if a.rateLimitInterval > 0 {
			time.Sleep(a.rateLimitInterval)
		}

		resp, err := callChatWithTools(
			a.Config.Model, conversation, toolDefs,
			a.Config.MaxTokens, a.Config.Temperature,
		)
		if err != nil {
			logger.Error("Agent LLM call failed", "agent", a.Id, "round", round, "error", err)
			return batch // pass through original on error
		}

		if len(resp.ToolCalls) > 0 {
			conversation = append(conversation, resp.AssistantMessage())
			for _, call := range resp.ToolCalls {
				result := a.executeFunctionCall(call)
				conversation = append(conversation, ToolResultMessage(call.ID, result))
			}
			continue
		}

		output := parseOutputMessages(resp.Content)
		if len(output) > 0 {
			return output
		}
		return batch
	}

	logger.Error("Agent max ReAct rounds exceeded, passing through original batch",
		"agent", a.Id, "max_rounds", a.Config.Batch.MaxRounds)
	return batch
}

func formatBatchAsJSON(batch []map[string]interface{}) string {
	data, err := json.Marshal(batch)
	if err != nil {
		return fmt.Sprintf("[%d messages]", len(batch))
	}
	return string(data)
}

func parseOutputMessages(content string) []map[string]interface{} {
	content = extractJSONArray(content)

	var messages []map[string]interface{}
	if err := json.Unmarshal([]byte(content), &messages); err != nil {
		// Try wrapping in a single-element array
		var single map[string]interface{}
		if err2 := json.Unmarshal([]byte(content), &single); err2 == nil {
			return []map[string]interface{}{single}
		}
		return nil
	}
	return messages
}

// extractJSONArray tries to extract a JSON array from LLM output
// that may contain markdown fences or extra text.
func extractJSONArray(s string) string {
	// Try to find JSON array directly
	start := -1
	for i, c := range s {
		if c == '[' {
			start = i
			break
		}
	}
	if start == -1 {
		// Try finding JSON object
		for i, c := range s {
			if c == '{' {
				start = i
				break
			}
		}
		if start == -1 {
			return s
		}
	}

	// Find matching close bracket
	depth := 0
	end := -1
	openChar := rune(s[start])
	var closeChar rune
	if openChar == '[' {
		closeChar = ']'
	} else {
		closeChar = '}'
	}

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
