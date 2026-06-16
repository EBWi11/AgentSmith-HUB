package rules_engine

import (
	"AgentSmith-HUB/common"
	"AgentSmith-HUB/logger"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bytedance/sonic"
	"github.com/panjf2000/ants/v2"
)

const HitRuleIdFieldName = "_hub_hit_rule_id"
const HitFieldsFieldName = "_hub_hit_fields"

// parseProjectInfoFromPNS parses project information from ProjectNodeSequence
// Format: "INPUT.api_sec.RULESET.test.OUTPUT.print_demo" -> project: "api_sec", ruleset: "test"
// Also handles test mode: "TEST.INPUT.api_sec.RULESET.test.OUTPUT.print_demo"
func parseProjectInfoFromPNS(pns string) (projectID, rulesetID string) {
	if pns == "" {
		return "", ""
	}

	parts := strings.Split(pns, ".")
	if len(parts) < 4 {
		return "", ""
	}

	// Handle test mode prefix
	startIndex := 0
	if len(parts) > 0 && strings.ToUpper(parts[0]) == "TEST" {
		startIndex = 1
	}

	// Find INPUT type and extract project ID
	for i := startIndex; i < len(parts)-1; i++ {
		if strings.ToUpper(parts[i]) == "INPUT" {
			projectID = parts[i+1]
			break
		}
	}

	// Find RULESET type and extract ruleset ID
	for i := startIndex; i < len(parts)-1; i++ {
		if strings.ToUpper(parts[i]) == "RULESET" {
			rulesetID = parts[i+1]
			break
		}
	}

	return projectID, rulesetID
}

// SIMD statistics variables
var (
	simdEnabled bool = false // SIMD enable flag, will be set from config
)

// ruleCachePool reuses map objects to reduce allocations
var ruleCachePool = sync.Pool{
	New: func() interface{} { return make(map[string]common.CheckCoreCache, 8) },
}

// stringBuilderPool reuses strings.Builder objects to reduce allocations
var stringBuilderPool = sync.Pool{
	New: func() interface{} {
		sb := &strings.Builder{}
		sb.Grow(64) // Pre-allocate 64 bytes capacity
		return sb
	},
}

// slicePool reuses small slices to reduce allocations for delimiter operations
var slicePool = sync.Pool{
	New: func() interface{} {
		s := make([]string, 0, 8)
		return &s
	},
}

// Optimized prefix checking - avoid strings.HasPrefix overhead
func hasFromRawPrefix(s string) bool {
	return len(s) >= 2 && s[0] == '_' && (s[1] == '$' || s[1] == '@')
}

// InitSIMDConfig initializes SIMD configuration from global config
func InitSIMDConfig() {
	if common.Config != nil {
		simdEnabled = common.Config.SIMDEnabled
		logger.Info("SIMD configuration initialized", "enabled", simdEnabled)
	} else {
		simdEnabled = false
		logger.Info("SIMD configuration not found, defaulting to disabled")
	}
}

// Start the ruleset engine, consuming data from upstream and writing checked data to downstream.
func (r *Ruleset) Start() error {
	// Initialize SIMD configuration
	InitSIMDConfig()

	// Add panic recovery for critical state changes
	defer func() {
		if panicErr := recover(); panicErr != nil {
			logger.Error("Panic during ruleset start", "ruleset", r.RulesetID, "panic", panicErr)
			// Ensure cleanup and proper status setting on panic
			r.cleanup()
			r.SetStatus(common.StatusError, fmt.Errorf("panic during start: %v", panicErr))
		}
	}()

	if err := r.reconcileBeforeStart(); err != nil {
		return err
	}
	if r.Status == common.StatusRunning {
		logger.Info("Ruleset already running; start request is a no-op", "ruleset", r.RulesetID)
		return nil
	}

	// Allow restart from stopped state or from error state
	if r.Status != common.StatusStopped && r.Status != common.StatusError {
		return fmt.Errorf("cannot start ruleset engine, current status: %s", r.Status)
	}

	// Clear error state when restarting
	r.Err = nil
	r.SetStatus(common.StatusStarting, nil)

	// Initialize regex result cache if not already initialized
	if r.RegexResultCache == nil {
		r.RegexResultCache = NewRegexResultCache(1000) // Default capacity: 1000 entries
	}

	r.ResetProcessTotal()
	if r.stopChan != nil {
		r.SetStatus(common.StatusError, fmt.Errorf("already started: %v", r.RulesetID))
		return fmt.Errorf("already started: %v", r.RulesetID)
	}
	r.stopChan = make(chan struct{})

	var err error
	minPoolSize := getMinPoolSize()
	r.antsPool, err = ants.NewPool(minPoolSize)
	if err != nil {
		r.SetStatus(common.StatusError, fmt.Errorf("failed to create ants pool: %v", err))
		return fmt.Errorf("failed to create ants pool: %v", err)
	}

	// Auto-scaling goroutine
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		ticker := time.NewTicker(20 * time.Second)
		defer ticker.Stop()
		minPoolSize := getMinPoolSize()
		maxPoolSize := getMaxPoolSize()
		for {
			select {
			case <-r.stopChan:
				return
			case <-ticker.C:
				totalBacklog := 0
				for _, upCh := range r.UpStream {
					totalBacklog += len(*upCh)
				}
				// Calculate linear scaling between min and max pool size
				// 4 levels: min -> level1 -> level2 -> max
				level1 := minPoolSize + (maxPoolSize-minPoolSize)/3
				level2 := minPoolSize + (maxPoolSize-minPoolSize)*2/3

				targetSize := minPoolSize
				switch {
				case totalBacklog > 256:
					targetSize = maxPoolSize
				case totalBacklog > 128:
					targetSize = level2
				case totalBacklog > 64:
					targetSize = level1
				case totalBacklog > 32:
					targetSize = minPoolSize + (level1-minPoolSize)/2
				default:
					targetSize = minPoolSize
				}

				// Ensure target size is within bounds
				if targetSize < minPoolSize {
					targetSize = minPoolSize
				}
				if targetSize > maxPoolSize {
					targetSize = maxPoolSize
				}

				if r.antsPool != nil {
					if r.antsPool.Cap() != targetSize {
						r.antsPool.Tune(targetSize)
					}
				}
			}
		}
	}()

	for upID, upCh := range r.UpStream {
		r.wg.Add(1)
		go func(id string, ch *chan map[string]interface{}) {
			defer r.wg.Done()
			defer func() {
				if panicErr := recover(); panicErr != nil {
					logger.Error("Panic in ruleset processing goroutine", "ruleset", r.RulesetID, "upstream", id, "panic", panicErr)
					// Set ruleset status to error on panic
					r.SetStatus(common.StatusError, fmt.Errorf("processing goroutine panic: %v", panicErr))
				}
			}()

			for {
				select {
				case <-r.stopChan:
					return
				case data, ok := <-*ch:
					if !ok {
						return
					}

					task := func() {
						// Only count and sample in production mode (not test mode)
						// Test mode flag is pre-computed during ruleset initialization for performance
						if !r.isTestMode {
							atomic.AddUint64(&r.processTotal, 1)
							if r.sampler != nil {
								_ = r.sampler.Sample(data, r.ProjectNodeSequence)
							}
						}

						// Now perform rule checking on the input data
						results := r.EngineCheck(data)
						// Send results to downstream channels - blocking to ensure no data loss
						for _, res := range results {
							r.sendToDownstream(res)
						}
					}

					// PERFORMANCE FIX: Improved task submission with backpressure handling
					select {
					case <-r.stopChan:
						// Ruleset is stopping, execute synchronously to not lose the message
						logger.Info("Ruleset stopping, executing final task synchronously",
							"ruleset", r.RulesetID)
						task()
						return
					default:
						err := r.antsPool.Submit(task)
						if err != nil {
							// Pool is full - execute synchronously to maintain throughput
							// This prevents the busy-wait loop that was causing CPU waste
							task()
						}
					}
				}
			}
		}(upID, upCh)
	}

	// Start absence scanner goroutine if any rule has sequences with absence stages
	if r.hasAbsenceSequences && r.seqStateManager != nil {
		r.wg.Add(1)
		go r.absenceScannerLoop()
	}

	r.SetStatus(common.StatusRunning, nil)
	return nil
}

func (r *Ruleset) hasRuntimeState() bool {
	return r.stopChan != nil || r.antsPool != nil
}

func (r *Ruleset) resetToStoppedForRestart() {
	r.cleanup()
	r.Err = nil
	r.Status = common.StatusStopped
}

func (r *Ruleset) reconcileBeforeStart() error {
	switch r.Status {
	case common.StatusRunning:
		if r.hasRuntimeState() {
			return nil
		}
		logger.Error("Ruleset marked running without active runtime; resetting before start", "ruleset", r.RulesetID)
		r.resetToStoppedForRestart()
	case common.StatusStarting, common.StatusStopping, common.StatusError:
		logger.Info("Reconciling ruleset runtime before start", "ruleset", r.RulesetID, "status", r.Status)
		if err := r.Stop(); err != nil {
			logger.Error("Ruleset stop during start reconciliation returned error; forcing cleanup",
				"ruleset", r.RulesetID,
				"error", err)
			r.resetToStoppedForRestart()
		}
	case common.StatusStopped:
		if r.hasRuntimeState() {
			logger.Info("Ruleset has stale runtime state while stopped; cleaning up before start", "ruleset", r.RulesetID)
			r.resetToStoppedForRestart()
		}
	default:
		return fmt.Errorf("cannot start ruleset engine, current status: %s", r.Status)
	}

	return nil
}

// absenceScannerLoop periodically checks for expired absence sequences and triggers alerts.
func (r *Ruleset) absenceScannerLoop() {
	defer r.wg.Done()
	defer func() {
		if panicErr := recover(); panicErr != nil {
			logger.Error("Panic in absence scanner, restarting", "ruleset", r.RulesetID, "panic", panicErr)
			// Check if we're shutting down before restarting
			select {
			case <-r.stopChan:
				return // Don't restart during shutdown
			default:
			}
			time.Sleep(2 * time.Second)
			r.wg.Add(1)
			go r.absenceScannerLoop()
		}
	}()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.stopChan:
			return
		case <-ticker.C:
			r.scanAbsenceTimeouts()
		}
	}
}

// scanAbsenceTimeouts checks for expired absence sequences and triggers them.
func (r *Ruleset) scanAbsenceTimeouts() {
	nowMs := time.Now().UnixMilli()
	expiredKeys := r.seqStateManager.GetExpiredAbsenceKeys(nowMs)

	for key, info := range expiredKeys {
		// Lock the state key to prevent concurrent modification
		r.seqStateManager.LockKey(key)

		state := r.seqStateManager.GetState(key)
		if state == nil {
			logger.Error("Absence scanner: state already evicted from cache",
				"rulesetID", r.RulesetID, "ruleID", info.RuleID, "key", key)
			r.seqStateManager.UnlockKey(key)
			r.seqStateManager.CleanupKeyLock(key)
			continue // Already cleaned up
		}

		// Find the rule and sequence
		rule := r.ruleByID[info.RuleID]
		if rule == nil {
			r.cleanupStateValueRefs(state)
			r.seqStateManager.DeleteState(key)
			r.seqStateManager.UnlockKey(key)
			r.seqStateManager.CleanupKeyLock(key)
			continue
		}

		seq, exists := rule.SequenceMap[info.SeqID]
		if !exists {
			r.cleanupStateValueRefs(state)
			r.seqStateManager.DeleteState(key)
			r.seqStateManager.UnlockKey(key)
			r.seqStateManager.CleanupKeyLock(key)
			continue
		}

		// Check if the absence timeout triggers the sequence
		absencePath := seq.Condition.FindAbsenceCompletionPath(state, nowMs)
		if absencePath != nil {
			// Build result data from the stored stage match snapshots
			resultData := r.buildAbsenceResult(state, &seq, absencePath)
			if resultData != nil {
				// Execute post-sequence operations (append/plugin/del/modify)
				// that follow the sequence in the rule's operation queue
				resultData = r.executePostSequenceOps(rule, info.SeqID, resultData)

				// Add hit rule ID
				sb := stringBuilderPool.Get().(*strings.Builder)
				sb.Reset()
				sb.WriteString(r.RulesetID)
				sb.WriteString(".")
				sb.WriteString(rule.ID)
				addHitRuleID(resultData, sb.String())
				stringBuilderPool.Put(sb)

				// Send to downstream
				r.sendToDownstream(resultData)
			}
		}

		// Clean up: unlock first, then remove the lock entry
		r.cleanupStateValueRefs(state)
		r.seqStateManager.DeleteState(key)
		r.seqStateManager.UnlockKey(key)
		r.seqStateManager.CleanupKeyLock(key)
	}
}

// executePostSequenceOps executes operations that follow a sequence in the rule's queue.
// This is needed for absence-triggered sequences which bypass the normal rule execution pipeline.
// Only executes data-modifying operations: append, modify, del, plugin.
func (r *Ruleset) executePostSequenceOps(rule *Rule, seqOpID int, data map[string]interface{}) map[string]interface{} {
	if rule.Queue == nil {
		return data
	}

	ruleCache := make(map[string]common.CheckCoreCache)
	foundSequence := false

	for _, op := range *rule.Queue {
		if op.Type == T_Sequence && op.ID == seqOpID {
			foundSequence = true
			continue
		}
		if !foundSequence {
			continue
		}

		// Only execute data-modifying operations after the sequence
		switch op.Type {
		case T_Append:
			if modified := r.executeAppend(rule, op.ID, true, data, ruleCache); modified != nil {
				data = modified
			}
		case T_Modify:
			if modified := r.executeModify(rule, op.ID, true, data, ruleCache); modified != nil {
				data = modified
			}
		case T_Del:
			if modified := r.executeDel(rule, op.ID, true, data); modified != nil {
				data = modified
			}
		case T_Plugin:
			r.executePlugin(rule, op.ID, data, ruleCache)
		}
	}

	return data
}

// buildAbsenceResult constructs the result data for a sequence triggered by absence timeout.
func (r *Ruleset) buildAbsenceResult(state *SequenceState, seq *Sequence, path *CompletionPath) map[string]interface{} {
	// Find the last non-absence stage match data to use as base
	var baseData map[string]interface{}
	for i := len(seq.Condition.Stages) - 1; i >= 0; i-- {
		stage := seq.Condition.Stages[i]
		if stage.IsAbsent {
			continue
		}
		matches, exists := state.StageMatches[i]
		if exists && len(matches) > 0 {
			// Use the match from the completion path when available.
			matchIdx := 0
			if path != nil {
				if idx, ok := path.MatchIndices[i]; ok && idx < len(matches) {
					matchIdx = idx
				}
			}
			if resolved := r.resolveStageMatchData(matches[matchIdx]); resolved != nil {
				baseData = resolved
			}
			break
		}
	}

	if baseData == nil {
		return nil
	}

	result := common.MapDeepCopy(baseData)
	r.enrichSequenceResultData(result, state, seq, path)
	return result
}

// Stop the ruleset engine, waiting for all upstream and downstream data to be processed before shutdown.
func (r *Ruleset) Stop() error {
	// Add panic recovery for critical state changes
	defer func() {
		if panicErr := recover(); panicErr != nil {
			logger.Error("Panic during ruleset stop", "ruleset", r.RulesetID, "panic", panicErr)
			// Ensure cleanup and proper status setting on panic
			r.cleanup()
			r.SetStatus(common.StatusError, fmt.Errorf("panic during stop: %v", panicErr))
		}
	}()

	if r.Status != common.StatusRunning && r.Status != common.StatusError {
		// Allow stopping from any state for cleanup purposes, but only do actual work if needed
		if r.Status == common.StatusStopped {
			logger.Debug("Ruleset already stopped, skipping stop operation", "ruleset", r.RulesetID)
			return nil
		}
		// For other states (e.g., StatusStarting), proceed with stop to ensure cleanup
		logger.Debug("Stopping ruleset from non-running state", "ruleset", r.RulesetID, "current_status", r.Status)
	}
	r.SetStatus(common.StatusStopping, nil)

	// Keep workers alive while draining channels; stop signal is sent after drain stage.
	// If stopChan is already closed, skip drain waits because workers are already exiting.
	stopSignalAlreadyClosed := false
	if r.stopChan != nil {
		select {
		case <-r.stopChan:
			stopSignalAlreadyClosed = true
		default:
		}
	}

	// Overall timeout for ruleset stop (drain stage only)
	// Keep this larger than per-stage drain timeout to avoid premature cutoff.
	stopBudget := 360 * time.Second
	stopStart := time.Now()
	overallTimeout := time.After(stopBudget)
	stopCompleted := make(chan struct{})
	var stopError error
	var stopErrorMu sync.Mutex
	setStopError := func(err error) {
		if err == nil {
			return
		}
		stopErrorMu.Lock()
		if stopError == nil {
			stopError = err
		}
		stopErrorMu.Unlock()
	}
	getStopError := func() error {
		stopErrorMu.Lock()
		defer stopErrorMu.Unlock()
		return stopError
	}
	remainingBudget := func() time.Duration {
		elapsed := time.Since(stopStart)
		remaining := stopBudget - elapsed
		if remaining <= 0 {
			return time.Millisecond
		}
		return remaining
	}

	go func() {
		defer close(stopCompleted)
		if stopSignalAlreadyClosed {
			return
		}

		// Wait for all upstream channels to be consumed.
		logger.Info("Waiting for upstream channels to empty", "ruleset", r.RulesetID)
		upstreamTimeout := time.After(180 * time.Second)
		waitCount := 0

	waitUpstream:
		for {
			select {
			case <-upstreamTimeout:
				logger.Error("Timeout waiting for upstream channels, forcing shutdown", "ruleset", r.RulesetID)
				setStopError(fmt.Errorf("timeout waiting for upstream channels to drain"))
				break waitUpstream
			default:
				allEmpty := true
				totalMessages := 0
				for _, upCh := range r.UpStream {
					chLen := len(*upCh)
					if chLen > 0 {
						allEmpty = false
						totalMessages += chLen
					}
				}
				if allEmpty {
					break waitUpstream
				}
				waitCount++
				time.Sleep(50 * time.Millisecond)
			}
		}

		downstreamTimeout := time.After(180 * time.Second)
		waitCount = 0

	waitDownstream:
		for {
			select {
			case <-downstreamTimeout:
				// Downstream consumers may be restarting or temporarily detached during
				// hot reload/restart flows. Treat drain timeout as a soft failure and
				// continue shutdown instead of blocking control-plane recovery.
				logger.Warn("Timeout waiting for downstream channels, forcing ruleset shutdown", "ruleset", r.RulesetID)
				break waitDownstream
			default:
				allEmpty := true
				totalMessages := 0
				for _, downCh := range r.downstreamSnapshot() {
					chLen := len(*downCh)
					if chLen > 0 {
						allEmpty = false
						totalMessages += chLen
					}
				}
				if allEmpty {
					break waitDownstream
				}
				waitCount++
				time.Sleep(50 * time.Millisecond)
			}
		}
	}()

	select {
	case <-stopCompleted:
		logger.Info("Ruleset channels drained successfully", "ruleset", r.RulesetID)
	case <-overallTimeout:
		logger.Error("Ruleset stop timeout exceeded, forcing shutdown", "ruleset", r.RulesetID)
		setStopError(fmt.Errorf("overall stop operation timeout"))
	}

	// Safely close stopChan after drain wait, allowing processing workers to exit.
	if r.stopChan != nil {
		select {
		case <-r.stopChan:
			// Already closed
		default:
			close(r.stopChan)
		}
	}

	// Wait for goroutines to finish with timeout
	logger.Info("Waiting for ruleset goroutines to finish", "ruleset", r.RulesetID)
	waitDone := make(chan struct{})
	go func() {
		r.wg.Wait()
		close(waitDone)
	}()

	select {
	case <-waitDone:
		logger.Info("Ruleset stopped gracefully", "ruleset", r.RulesetID)
	case <-time.After(minDuration(20*time.Second, remainingBudget())):
		logger.Error("Timeout waiting for ruleset goroutines, forcing cleanup", "ruleset", r.RulesetID)
		setStopError(fmt.Errorf("timeout waiting for goroutines to finish"))
	}

	// Wait for thread pool to finish with timeout
	if r.antsPool != nil {
		logger.Info("Waiting for thread pool tasks to complete", "ruleset", r.RulesetID)
		poolWaitTimeout := time.After(minDuration(30*time.Second, remainingBudget()))
	poolWait:
		for {
			select {
			case <-poolWaitTimeout:
				logger.Error("Thread pool timeout, forcing cleanup", "ruleset", r.RulesetID)
				setStopError(fmt.Errorf("timeout waiting for thread pool to finish"))
				break poolWait
			default:
				if r.antsPool.Running() == 0 {
					break poolWait
				}
				time.Sleep(50 * time.Millisecond)
			}
		}
	}

	// Use cleanup to ensure all resources are properly released
	r.cleanup()

	// Set final status based on whether there were any errors during stop
	if finalStopErr := getStopError(); finalStopErr != nil {
		r.SetStatus(common.StatusError, fmt.Errorf("stop operation failed: %w", finalStopErr))
		return finalStopErr
	} else {
		r.SetStatus(common.StatusStopped, nil)
		return nil
	}
}

func (r *Ruleset) StopForTesting(timeout time.Duration) error {
	if r.Status == common.StatusStopped && r.stopChan == nil {
		return nil
	}
	if timeout <= 0 {
		timeout = 2 * time.Second
	}

	r.SetStatus(common.StatusStopping, nil)
	if r.stopChan != nil {
		select {
		case <-r.stopChan:
		default:
			close(r.stopChan)
		}
	}

	waitDone := make(chan struct{})
	go func() {
		r.wg.Wait()
		close(waitDone)
	}()

	var stopErr error
	select {
	case <-waitDone:
	case <-time.After(timeout):
		stopErr = fmt.Errorf("timeout waiting for test ruleset goroutines to finish")
		logger.Error("Timeout waiting for test ruleset goroutines, forcing cleanup", "ruleset", r.RulesetID)
	}

	if r.antsPool != nil {
		deadline := time.Now().Add(timeout)
		for r.antsPool.Running() > 0 && time.Now().Before(deadline) {
			time.Sleep(50 * time.Millisecond)
		}
		if r.antsPool.Running() > 0 && stopErr == nil {
			stopErr = fmt.Errorf("timeout waiting for test ruleset tasks to finish")
			logger.Error("Timeout waiting for test ruleset tasks, forcing cleanup", "ruleset", r.RulesetID)
		}
	}

	r.cleanup()
	if stopErr != nil {
		r.SetStatus(common.StatusError, stopErr)
		return stopErr
	}
	r.SetStatus(common.StatusStopped, nil)
	return nil
}

func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}

// EngineCheck executes all rules in the ruleset on the provided data using the new flexible syntax.
func (r *Ruleset) EngineCheck(data map[string]interface{}) []map[string]interface{} {
	// Pre-allocate result slice with better capacity estimation
	var initialCap int
	if r.IsDetection {
		// For detection rules, estimate that 10-20% of rules might hit
		initialCap = len(r.Rules) / 5
		if initialCap < 1 {
			initialCap = 1
		}
	} else {
		// For exclude rules, usually only 1 result
		initialCap = 1
	}
	finalRes := make([]map[string]interface{}, 0, initialCap)
	ruleCache := ruleCachePool.Get().(map[string]common.CheckCoreCache)

	// More efficient cache clearing - only clear if not empty
	if len(ruleCache) > 0 {
		// Faster map clearing for Go 1.11+
		for k := range ruleCache {
			delete(ruleCache, k)
		}
	}

	// For exclude, keep track of the last modified data
	var lastModifiedData map[string]interface{}

	// For empty exclude, data should pass through
	if !r.IsDetection && len(r.Rules) == 0 {
		// Empty exclude means all data passes through
		ruleCachePool.Put(ruleCache)
		// Reuse the same slice pattern for consistency
		result := make([]map[string]interface{}, 1)
		result[0] = data
		return result
	}

	// Process each rule in the ruleset
	for ruleIndex := range r.Rules {
		rule := &r.Rules[ruleIndex] // Use pointer to avoid copying

		// Execute all operations in the order specified by the Queue
		ruleCheckRes, copied, modifiedData := r.executeRuleOperations(rule, data, ruleCache)

		// Handle rule result based on ruleset type
		if r.IsDetection {
			// For detection rules, if rule passes, add to results
			if ruleCheckRes {
				if !copied {
					modifiedData = mapDeepCopyWithExtraCapacity(data, 1)
				}
				// Keep internal sequence helper fields out of output payloads.
				// Non-sequence rules never write "#" keys, so skip the scan entirely.
				if len(rule.SequenceMap) > 0 {
					sanitizeOutputData(modifiedData)
				}
				// Add rule info
				// Build hit rule ID efficiently using string builder pool
				sb := stringBuilderPool.Get().(*strings.Builder)
				sb.Reset()
				sb.WriteString(r.RulesetID)
				sb.WriteString(".")
				sb.WriteString(rule.ID)
				addHitRuleID(modifiedData, sb.String())
				stringBuilderPool.Put(sb)
				if len(rule.HitFields) > 0 {
					modifiedData[HitFieldsFieldName] = append([]string(nil), rule.HitFields...)
				}
				// Add to final result
				finalRes = append(finalRes, modifiedData)
			}
		} else {
			// For exclude rules
			// Always update lastModifiedData with the result of rule execution
			if modifiedData == nil {
				lastModifiedData = data
			} else {
				lastModifiedData = modifiedData
			}

			if ruleCheckRes {
				// If exclude rule passes, data is excluded (filtered) - don't pass forward (return empty)
				ruleCachePool.Put(ruleCache)
				return make([]map[string]interface{}, 0)
			}
		}
	}

	// For exclude: if no rule passed, data needs processing - pass forward the last modified data
	if !r.IsDetection && len(finalRes) == 0 && lastModifiedData != nil {
		sanitizeOutputData(lastModifiedData)
		finalRes = append(finalRes, lastModifiedData)
	}

	// put back to pool
	ruleCachePool.Put(ruleCache)
	ruleCache = nil

	return finalRes
}

// sanitizeOutputData removes internal helper fields that are only needed during rule execution.
func sanitizeOutputData(data map[string]interface{}) {
	for key := range data {
		if strings.HasPrefix(key, "#") {
			delete(data, key)
		}
	}
}

// executeRuleOperations executes all operations in a rule according to the Queue order
func (r *Ruleset) executeRuleOperations(rule *Rule, data map[string]interface{}, ruleCache map[string]common.CheckCoreCache) (bool, bool, map[string]interface{}) {
	copied := false

	if rule.Queue == nil || len(*rule.Queue) == 0 {
		// No operations to execute
		// For detection rules, empty rule means no match (false)
		// For exclude rules, empty rule also means no match (false), allowing data to pass
		return false, copied, nil
	}
	ruleResult := true
	// Execute operations in the exact order specified by the Queue
	for _, op := range *rule.Queue {
		var modifiedRes map[string]interface{}
		switch op.Type {
		case T_CheckList:
			checkResult := r.executeCheckList(rule, op.ID, data, ruleCache)
			if !checkResult {
				ruleResult = false
				// For detection rules, if check fails, stop execution
				if r.IsDetection {
					return false, copied, data
				}
				// For exclude rules, continue executing other operations
			}
		case T_Check:
			checkResult := r.executeCheck(rule, op.ID, data, ruleCache)
			if !checkResult {
				ruleResult = false
				// For detection rules, if check fails, stop execution
				if r.IsDetection {
					return false, copied, data
				}
				// For exclude rules, continue executing other operations
			}
		case T_Threshold:
			thresholdResult := r.executeThreshold(rule, op.ID, data, ruleCache)
			if !thresholdResult {
				ruleResult = false
				// For detection rules, if threshold fails, stop execution
				if r.IsDetection {
					return false, copied, data
				}
				// For exclude rules, continue executing other operations
			}
		case T_Iterator:
			iteratorResult := r.executeIterator(rule, op.ID, data, ruleCache)
			if !iteratorResult {
				ruleResult = false
				// For detection rules, if iterator fails, stop execution
				if r.IsDetection {
					return false, copied, data
				}
				// For exclude rules, continue executing other operations
			}
		case T_Append:
			// Execute append operation according to user-defined order
			modifiedRes = r.executeAppend(rule, op.ID, copied, data, ruleCache)

		case T_Modify:
			// Execute modify operation according to user-defined order
			modifiedRes = r.executeModify(rule, op.ID, copied, data, ruleCache)
		case T_Del:
			// Execute del operation according to user-defined order
			modifiedRes = r.executeDel(rule, op.ID, copied, data)
		case T_Plugin:
			// Execute plugin operation according to user-defined order
			r.executePlugin(rule, op.ID, data, ruleCache)
		case T_Sequence:
			seqResult, seqData := r.executeSequence(rule, op.ID, data, ruleCache)
			if !seqResult {
				ruleResult = false
				if r.IsDetection {
					return false, copied, data
				}
			} else if seqData != nil {
				// Sequence completed: use the enriched data
				copied = true
				data = seqData
			}
		}
		if modifiedRes != nil {
			copied = true
			data = modifiedRes
		}
	}

	return ruleResult, copied, data
}

// executeCheckList executes a checklist operation
func (r *Ruleset) executeCheckList(rule *Rule, operationID int, data map[string]interface{}, ruleCache map[string]common.CheckCoreCache) bool {
	checklist, exists := rule.ChecklistMap[operationID]
	if !exists {
		return true
	}

	// Pre-allocate conditionMap only if needed
	var conditionMap map[string]bool
	if checklist.ConditionFlag {
		conditionMap = make(map[string]bool, len(checklist.CheckNodes)+len(checklist.ThresholdNodes))
	}

	// Execute each check node in the checklist
	for _, checkNode := range checklist.CheckNodes {
		checkResult := r.executeCheckNode(&checkNode, data, ruleCache)

		if checklist.ConditionFlag {
			conditionMap[checkNode.ID] = checkResult
		} else {
			// Simple AND logic for non-condition checklists
			if !checkResult {
				return false
			}
		}
	}

	// Execute each threshold node in the checklist
	for i := range checklist.ThresholdNodes {
		threshold := &checklist.ThresholdNodes[i]
		thresholdID := threshold.ID
		if thresholdID == "" {
			thresholdID = fmt.Sprintf("threshold_%d", i)
		}

		thresholdResult := r.executeThresholdNode(threshold, rule.ID, data, ruleCache)

		if checklist.ConditionFlag {
			conditionMap[thresholdID] = thresholdResult
		} else {
			if !thresholdResult {
				return false
			}
		}
	}

	// If using condition expression, evaluate it
	if checklist.ConditionFlag {
		result := checklist.ConditionAST.ExprASTResult(checklist.ConditionAST.ExprAST, conditionMap)
		return result
	}

	return true
}

// executeCheck executes a standalone check operation
func (r *Ruleset) executeCheck(rule *Rule, operationID int, data map[string]interface{}, ruleCache map[string]common.CheckCoreCache) bool {
	checkNode, exists := rule.CheckMap[operationID]
	if !exists {
		return true
	}

	return r.executeCheckNode(&checkNode, data, ruleCache)
}

// executeCheckNode executes a single check node
func (r *Ruleset) executeCheckNode(checkNode *CheckNodes, data map[string]interface{}, ruleCache map[string]common.CheckCoreCache) bool {
	var checkNodeValue string
	var checkNodeValueFromRaw bool

	switch checkNode.Logic {
	case "":
		if hasFromRawPrefix(checkNode.Value) {
			checkNodeValue = GetRuleValueFromRawFromCache(ruleCache, checkNode.Value, data)
			checkNodeValueFromRaw = true
		} else {
			checkNodeValue = checkNode.Value
		}
		return checkNodeLogic(r, checkNode, data, checkNodeValue, checkNodeValueFromRaw, ruleCache, r.RegexResultCache)
	case "AND":
		for _, v := range checkNode.DelimiterFieldList {
			if hasFromRawPrefix(v) {
				checkNodeValue = GetRuleValueFromRawFromCache(ruleCache, v, data)
				checkNodeValueFromRaw = true
			} else {
				checkNodeValue = v
				checkNodeValueFromRaw = false
			}
			if !checkNodeLogic(r, checkNode, data, checkNodeValue, checkNodeValueFromRaw, ruleCache, r.RegexResultCache) {
				return false
			}
		}
		return true
	case "OR":
		for _, v := range checkNode.DelimiterFieldList {
			if hasFromRawPrefix(v) {
				checkNodeValue = GetRuleValueFromRawFromCache(ruleCache, v, data)
				checkNodeValueFromRaw = true
			} else {
				checkNodeValue = v
				checkNodeValueFromRaw = false
			}
			if checkNodeLogic(r, checkNode, data, checkNodeValue, checkNodeValueFromRaw, ruleCache, r.RegexResultCache) {
				return true
			}
		}
		return false
	}

	return false
}

// executeThreshold executes a threshold operation
func (r *Ruleset) executeThreshold(rule *Rule, operationID int, data map[string]interface{}, ruleCache map[string]common.CheckCoreCache) bool {
	threshold, exists := rule.ThresholdMap[operationID]
	if !exists {
		return true
	}

	// Build a deterministic group-by key in configured field order.
	// If any group_by field is missing/empty, skip threshold counting for this event.
	sb := stringBuilderPool.Get().(*strings.Builder)
	sb.Reset()
	sb.WriteString(threshold.GroupByID)
	sb.WriteString("|")
	groupByFields := orderedThresholdGroupByFields(&threshold)
	for i := range groupByFields {
		field := groupByFields[i]
		fieldPath, ok := threshold.GroupByList[field]
		if !ok || len(fieldPath) == 0 {
			fieldPath = common.StringToList(field)
		}
		tmpData, exists := GetCheckDataFromCache(ruleCache, field, data, fieldPath)
		if !exists || strings.TrimSpace(tmpData) == "" {
			stringBuilderPool.Put(sb)
			return false
		}
		sb.WriteString(tmpData)
		sb.WriteString("|")
	}
	groupByKey := common.XXHash64(sb.String())
	stringBuilderPool.Put(sb)

	var ruleCheckRes bool
	var err error

	switch threshold.CountType {
	case "":
		// Use builder pool for prefix concatenation
		sb := stringBuilderPool.Get().(*strings.Builder)
		sb.Reset()
		sb.WriteString("F_")
		sb.WriteString(groupByKey)
		prefixedKey := sb.String()
		stringBuilderPool.Put(sb)

		if threshold.LocalCache {
			ruleCheckRes, err = r.LocalCacheFRQSum(prefixedKey, 1, threshold.RangeInt, threshold.Value)
		} else {
			ruleCheckRes, err = RedisFRQSum(prefixedKey, 1, threshold.RangeInt, threshold.Value)
		}

	case "SUM":
		// Use builder pool for prefix concatenation
		sb := stringBuilderPool.Get().(*strings.Builder)
		sb.Reset()
		sb.WriteString("FS_")
		sb.WriteString(groupByKey)
		prefixedKey := sb.String()
		stringBuilderPool.Put(sb)

		sumDataStr, ok := GetCheckDataFromCache(ruleCache, threshold.CountField, data, threshold.CountFieldList)
		if !ok {
			return false
		}

		sumData, err := strconv.Atoi(sumDataStr)
		if err != nil {
			return false
		}

		if threshold.LocalCache {
			ruleCheckRes, err = r.LocalCacheFRQSum(prefixedKey, sumData, threshold.RangeInt, threshold.Value)
		} else {
			ruleCheckRes, err = RedisFRQSum(prefixedKey, sumData, threshold.RangeInt, threshold.Value)
		}

	case "CLASSIFY":
		// Use builder pool for prefix concatenation
		sb := stringBuilderPool.Get().(*strings.Builder)
		sb.Reset()
		sb.WriteString("FC_")
		sb.WriteString(groupByKey)
		prefixedKey := sb.String()

		classifyData, ok := GetCheckDataFromCache(ruleCache, threshold.CountField, data, threshold.CountFieldList)
		if !ok {
			stringBuilderPool.Put(sb)
			return false
		}

		// Continue building the final key
		sb.WriteString("_")
		sb.WriteString(common.XXHash64(classifyData))
		tmpKey := sb.String()
		stringBuilderPool.Put(sb)

		if threshold.LocalCache {
			ruleCheckRes, err = r.LocalCacheFRQClassify(tmpKey, prefixedKey, threshold.RangeInt, threshold.Value)
		} else {
			ruleCheckRes, err = RedisFRQClassify(tmpKey, prefixedKey, threshold.RangeInt, threshold.Value)
		}
	}

	if err != nil {
		logger.Error("Threshold check error:", err, "GroupByKey:", groupByKey, "RuleID:", rule.ID, "RuleSetID:", r.RulesetID)
		return false
	}

	return ruleCheckRes
}

// executeThresholdNode executes a threshold directly from a *Threshold pointer,
// avoiding the temporary map[int]Threshold + *Rule allocation required by executeThreshold.
// Used by executeCheckList and evaluateEventDef.
func (r *Ruleset) executeThresholdNode(threshold *Threshold, ruleID string, data map[string]interface{}, ruleCache map[string]common.CheckCoreCache) bool {
	sb := stringBuilderPool.Get().(*strings.Builder)
	sb.Reset()
	sb.WriteString(threshold.GroupByID)
	sb.WriteString("|")
	groupByFields := orderedThresholdGroupByFields(threshold)
	for i := range groupByFields {
		field := groupByFields[i]
		fieldPath, ok := threshold.GroupByList[field]
		if !ok || len(fieldPath) == 0 {
			fieldPath = common.StringToList(field)
		}
		tmpData, exists := GetCheckDataFromCache(ruleCache, field, data, fieldPath)
		if !exists || strings.TrimSpace(tmpData) == "" {
			stringBuilderPool.Put(sb)
			return false
		}
		sb.WriteString(tmpData)
		sb.WriteString("|")
	}
	groupByKey := common.XXHash64(sb.String())
	stringBuilderPool.Put(sb)

	var ruleCheckRes bool
	var err error

	switch threshold.CountType {
	case "":
		sb := stringBuilderPool.Get().(*strings.Builder)
		sb.Reset()
		sb.WriteString("F_")
		sb.WriteString(groupByKey)
		prefixedKey := sb.String()
		stringBuilderPool.Put(sb)

		if threshold.LocalCache {
			ruleCheckRes, err = r.LocalCacheFRQSum(prefixedKey, 1, threshold.RangeInt, threshold.Value)
		} else {
			ruleCheckRes, err = RedisFRQSum(prefixedKey, 1, threshold.RangeInt, threshold.Value)
		}

	case "SUM":
		sb := stringBuilderPool.Get().(*strings.Builder)
		sb.Reset()
		sb.WriteString("FS_")
		sb.WriteString(groupByKey)
		prefixedKey := sb.String()
		stringBuilderPool.Put(sb)

		sumDataStr, ok := GetCheckDataFromCache(ruleCache, threshold.CountField, data, threshold.CountFieldList)
		if !ok {
			return false
		}
		sumData, err2 := strconv.Atoi(sumDataStr)
		if err2 != nil {
			return false
		}
		if threshold.LocalCache {
			ruleCheckRes, err = r.LocalCacheFRQSum(prefixedKey, sumData, threshold.RangeInt, threshold.Value)
		} else {
			ruleCheckRes, err = RedisFRQSum(prefixedKey, sumData, threshold.RangeInt, threshold.Value)
		}

	case "CLASSIFY":
		sb := stringBuilderPool.Get().(*strings.Builder)
		sb.Reset()
		sb.WriteString("FC_")
		sb.WriteString(groupByKey)
		prefixedKey := sb.String()

		classifyData, ok := GetCheckDataFromCache(ruleCache, threshold.CountField, data, threshold.CountFieldList)
		if !ok {
			stringBuilderPool.Put(sb)
			return false
		}
		sb.WriteString("_")
		sb.WriteString(common.XXHash64(classifyData))
		tmpKey := sb.String()
		stringBuilderPool.Put(sb)

		if threshold.LocalCache {
			ruleCheckRes, err = r.LocalCacheFRQClassify(tmpKey, prefixedKey, threshold.RangeInt, threshold.Value)
		} else {
			ruleCheckRes, err = RedisFRQClassify(tmpKey, prefixedKey, threshold.RangeInt, threshold.Value)
		}
	}

	if err != nil {
		logger.Error("Threshold check error:", err, "GroupByKey:", groupByKey, "RuleID:", ruleID, "RuleSetID:", r.RulesetID)
		return false
	}
	return ruleCheckRes
}

// Returns (completed bool, enrichedData map) where enrichedData is non-nil when the sequence completes.
func (r *Ruleset) executeSequence(rule *Rule, operationID int, data map[string]interface{}, ruleCache map[string]common.CheckCoreCache) (bool, map[string]interface{}) {
	seq, exists := rule.SequenceMap[operationID]
	if !exists {
		return true, nil
	}

	if r.seqStateManager == nil {
		logger.Error("CEP state manager not initialized", "ruleID", rule.ID, "rulesetID", r.RulesetID)
		return false, nil
	}

	// Step 1: Match incoming event against all event definitions (checks only).
	// Thresholds are skipped in the first pass to avoid double-counting side effects;
	// the full evaluation (including thresholds) runs in the second pass after
	// sequence context (_@) is loaded.
	matchMap := make(map[string]bool, len(seq.Events))
	matchedEventIDs := make([]string, 0, 2) // Track which events matched (for group_by extraction)

	for _, eventID := range seq.EventOrder {
		eventDef := seq.Events[eventID]
		matched := r.evaluateEventDefChecksOnly(eventDef, data, ruleCache)
		matchMap[eventID] = matched
		if matched {
			matchedEventIDs = append(matchedEventIDs, eventID)
		}
	}

	// Step 2: Stage evaluation - determine which stages this event satisfies
	matchedStages := seq.Condition.EvaluateEvent(matchMap)

	// Step 3: Compute correlation key for state lookup.
	// Use deterministic group_by extraction from sequence/event configuration
	// (independent of stage match outcome) so "_@"-dependent stages can still load state.
	// If first-pass matchedEventIDs is empty (e.g., first stage depends on _@), we need
	// to try sequence-level group_by first before falling back to per-event group_by.
	correlateValues := r.extractCorrelateValuesForStateLookup(&seq, matchedEventIDs, data, ruleCache)

	// If correlation key is empty and we have group_by configured, try sequence-level first
	// before giving up. This handles the case where first-pass matchedEventIDs is empty
	// (due to _@ dependency) but sequence-level group_by might still work.
	if correlateValues == "" && len(seq.GroupByList) == 0 && r.hasEventGroupBy(&seq) {
		// Try sequence-level group_by as fallback when per-event group_by failed
		// (e.g., first-pass matchedEventIDs empty, or per-event fields not in current data)
		correlateValues = r.extractCorrelateValuesForStateLookupWithSequenceFallback(&seq, data, ruleCache)
	}

	if correlateValues == "" && (len(seq.GroupByList) > 0 || r.hasEventGroupBy(&seq)) {
		// group_by is configured but no values could be extracted
		logger.Error("Sequence group_by extraction yielded empty key",
			"rulesetID", r.RulesetID,
			"ruleID", rule.ID,
			"sequenceOpID", operationID)
		return false, nil
	}
	stateKey := BuildStateKey(seq.GroupByID, correlateValues)

	// Lock the state key to prevent concurrent modification from multiple upstream goroutines.
	// This protects the entire read-modify-write cycle on SequenceState.
	r.seqStateManager.LockKey(stateKey)
	sequenceCompleted := false
	defer func() {
		r.seqStateManager.UnlockKey(stateKey)
		if sequenceCompleted {
			r.seqStateManager.CleanupKeyLock(stateKey)
		}
	}()

	// Step 4: Get or create sequence state
	state := r.seqStateManager.GetOrCreateState(stateKey, seq.WithinMs, seq.WithinSec)

	// Build sequence-scoped evaluation data with context exposed as "#ctx".
	// This enables "_@foo.bar" references by rewriting them to "_$#ctx.foo.bar".
	seqEvalData := data
	if state.Context == nil {
		state.Context = make(map[string]interface{})
	}
	seqEvalDataCopy := make(map[string]interface{}, len(data)+1)
	for k, v := range data {
		seqEvalDataCopy[k] = v
	}
	seqEvalDataCopy["#ctx"] = state.Context
	seqEvalData = seqEvalDataCopy

	// Clear cached dynamic references for sequence context before re-evaluation.
	// First-pass checks run without "#ctx" and may cache empty values.
	for k := range ruleCache {
		if strings.HasPrefix(k, FromRawSymbol+"#ctx") || strings.HasPrefix(k, SequenceCtxSymbol) {
			delete(ruleCache, k)
		}
	}

	// Re-evaluate event matching with sequence context available.
	// (When "_@" is not used, behavior is equivalent to the first evaluation pass.)
	matchMap = make(map[string]bool, len(seq.Events))
	matchedEventIDs = matchedEventIDs[:0]
	for _, eventID := range seq.EventOrder {
		eventDef := seq.Events[eventID]
		matched := r.evaluateEventDef(eventDef, seqEvalData, ruleCache)
		matchMap[eventID] = matched
		if matched {
			matchedEventIDs = append(matchedEventIDs, eventID)
		}
	}
	matchedStages = seq.Condition.EvaluateEvent(matchMap)
	if len(matchedStages) == 0 {
		return false, nil
	}
	stageBindings := seq.Condition.EvaluateEventBindings(matchMap)
	selectedMatchedEventIDs := buildSelectedMatchedEventIDs(seq, matchedStages, stageBindings)

	// Apply per-event append side effects for sequence context ("_@...") once
	// matched event definitions are finalized for this input event.
	r.applySequenceEventAppends(&seq, selectedMatchedEventIDs, seqEvalData, state, ruleCache)

	// Step 5: Extract event timestamp
	eventTimestamp := r.extractEventTimestamp(&seq, selectedMatchedEventIDs, data, ruleCache)

	// Step 6: Record stage matches
	// For absence stages, recording a match means the absent event WAS observed,
	// which causes CheckComplete to return false for that stage.
	// For normal stages, recording advances the sequence towards completion.
	dataSnapshot := r.snapshotEventData(data, matchedEventIDs, &seq)
	var valueRef string
	if seq.LocalCache && r.cepValueStore != nil {
		// Add grace period so the absence scanner can still read the snapshot
		// after the sequence window expires (same reason as ristretto TTL grace).
		expiresAtMs := state.ExpiresAt + int64(localCacheTTLGracePeriod)*1000
		expiresAtNs := expiresAtMs * int64(time.Millisecond)
		if ref, err := r.cepValueStore.PutSnapshot(dataSnapshot, expiresAtNs); err == nil {
			valueRef = ref
			// Keep only pointer in memory for local_cache mode.
			dataSnapshot = nil
		}
	}
	for _, stageIdx := range matchedStages {
		matchedIDs, bindingExists := stageBindings[stageIdx]
		if !bindingExists {
			matchedIDs = seq.Condition.Stages[stageIdx].EventIDs
		}
		state.AddMatch(stageIdx, StageMatch{
			Timestamp:          eventTimestamp,
			Data:               dataSnapshot,
			ValueRef:           valueRef,
			MatchedEventIDsSet: bindingExists,
			MatchedEventIDs:    matchedIDs,
		})
	}

	// Step 7: Check if the sequence is now complete
	if seq.Condition.CheckComplete(state) {
		// Sequence completed - find the actual match path and build enriched data
		path := seq.Condition.FindCompletionPath(state)
		enrichedData := r.buildSequenceResult(state, &seq, data, path)
		r.cleanupStateValueRefs(state)
		r.seqStateManager.DeleteState(stateKey)
		sequenceCompleted = true
		return true, enrichedData
	}

	// Step 8: Persist updated state
	r.seqStateManager.UpdateState(stateKey, state, seq.WithinSec)

	// Step 9: Track for absence scanning if needed
	if seq.Condition.HasAbsenceStages() {
		r.seqStateManager.TrackAbsenceKey(stateKey, absenceKeyInfo{
			ExpiresAt: state.ExpiresAt,
			RuleID:    rule.ID,
			SeqID:     operationID,
		})
	}

	return false, nil
}

func setSequenceContextValue(ctx map[string]interface{}, path []string, value interface{}) {
	if len(path) == 0 {
		return
	}
	cur := ctx
	for i := 0; i < len(path)-1; i++ {
		k := path[i]
		next, ok := cur[k]
		if !ok {
			nm := make(map[string]interface{})
			cur[k] = nm
			cur = nm
			continue
		}
		if nm, ok := next.(map[string]interface{}); ok {
			cur = nm
			continue
		}
		nm := make(map[string]interface{})
		cur[k] = nm
		cur = nm
	}
	cur[path[len(path)-1]] = value
}

func buildSelectedMatchedEventIDs(seq Sequence, matchedStages []int, stageBindings map[int][]string) []string {
	if len(matchedStages) == 0 {
		return nil
	}

	selectedSet := make(map[string]struct{}, len(matchedStages))
	for _, stageIdx := range matchedStages {
		if ids, ok := stageBindings[stageIdx]; ok {
			for _, id := range ids {
				selectedSet[id] = struct{}{}
			}
			continue
		}

		// Defensive fallback: if bindings are unavailable, keep previous behavior.
		stage := seq.Condition.Stages[stageIdx]
		for _, id := range stage.EventIDs {
			selectedSet[id] = struct{}{}
		}
	}

	if len(selectedSet) == 0 {
		return nil
	}

	ordered := make([]string, 0, len(selectedSet))
	for _, eventID := range seq.EventOrder {
		if _, ok := selectedSet[eventID]; ok {
			ordered = append(ordered, eventID)
		}
	}
	return ordered
}

func (r *Ruleset) applySequenceEventAppends(seq *Sequence, matchedEventIDs []string, data map[string]interface{}, state *SequenceState, ruleCache map[string]common.CheckCoreCache) {
	if state == nil {
		return
	}
	if state.Context == nil {
		state.Context = make(map[string]interface{})
	}

	for _, eventID := range matchedEventIDs {
		eventDef := seq.Events[eventID]
		if eventDef == nil || len(eventDef.Appends) == 0 {
			continue
		}
		for _, appendOp := range eventDef.Appends {
			if !strings.HasPrefix(appendOp.FieldName, SequenceCtxSymbol) {
				continue
			}
			pathRaw := strings.TrimSpace(appendOp.FieldName[SequenceCtxSymbolLen:])
			if pathRaw == "" {
				continue
			}
			path := common.StringToList(pathRaw)
			if len(path) == 0 {
				continue
			}

			if strings.TrimSpace(appendOp.Type) == "" {
				val := replaceFromRawPlaceholders(ruleCache, appendOp.Value, data)
				setSequenceContextValue(state.Context, path, val)
				continue
			}

			args := GetPluginRealArgs(appendOp.PluginArgs, data, ruleCache)
			if appendOp.Plugin == nil {
				continue
			}
			if appendOp.Plugin.ReturnType == "bool" {
				boolResult, err := appendOp.Plugin.FuncEvalCheckNode(args...)
				if err == nil {
					setSequenceContextValue(state.Context, path, boolResult)
				}
				continue
			}
			res, ok, err := appendOp.Plugin.FuncEvalOther(args...)
			if err == nil && ok {
				setSequenceContextValue(state.Context, path, res)
			}
		}
	}
}

// cleanupStateValueRefs deletes external snapshot references associated with a sequence state.
func (r *Ruleset) cleanupStateValueRefs(state *SequenceState) {
	if r.cepValueStore == nil || state == nil {
		return
	}
	for _, matches := range state.StageMatches {
		for _, m := range matches {
			if m.ValueRef != "" {
				_ = r.cepValueStore.DeleteSnapshot(m.ValueRef)
			}
		}
	}
}

// evaluateEventDefChecksOnly evaluates an event definition's checks and checklists
// only, WITHOUT evaluating thresholds. Used in the first-pass correlation key
// extraction to avoid double-counting threshold side effects when the second
// pass (with sequence context) re-evaluates the same event definitions.
func (r *Ruleset) evaluateEventDefChecksOnly(eventDef *EventDef, data map[string]interface{}, ruleCache map[string]common.CheckCoreCache) bool {
	for i := range eventDef.CheckNodes {
		if !r.executeCheckNode(&eventDef.CheckNodes[i], data, ruleCache) {
			return false
		}
	}
	for i := range eventDef.Checklists {
		cl := &eventDef.Checklists[i]
		if !r.evaluateEventChecklist(cl, data, ruleCache) {
			return false
		}
	}
	// Thresholds are intentionally skipped; the full evaluation (including
	// thresholds) happens in the second pass after sequence context is loaded.
	return true
}

// evaluateEventDef checks if the incoming data matches a single event definition.
// All checks within an event must pass (AND logic).
func (r *Ruleset) evaluateEventDef(eventDef *EventDef, data map[string]interface{}, ruleCache map[string]common.CheckCoreCache) bool {
	// Evaluate standalone check nodes (AND logic)
	for i := range eventDef.CheckNodes {
		if !r.executeCheckNode(&eventDef.CheckNodes[i], data, ruleCache) {
			return false
		}
	}

	// Evaluate checklists
	for i := range eventDef.Checklists {
		cl := &eventDef.Checklists[i]
		if !r.evaluateEventChecklist(cl, data, ruleCache) {
			return false
		}
	}

	// Evaluate thresholds
	for i := range eventDef.Thresholds {
		if !r.executeThresholdNode(&eventDef.Thresholds[i], fmt.Sprintf("cep_%s_th%d", eventDef.ID, i), data, ruleCache) {
			return false
		}
	}

	return true
}

// evaluateEventChecklist evaluates a checklist within an event definition.
func (r *Ruleset) evaluateEventChecklist(cl *Checklist, data map[string]interface{}, ruleCache map[string]common.CheckCoreCache) bool {
	var conditionMap map[string]bool
	if cl.ConditionFlag {
		conditionMap = make(map[string]bool, len(cl.CheckNodes)+len(cl.ThresholdNodes))
	}

	for _, checkNode := range cl.CheckNodes {
		result := r.executeCheckNode(&checkNode, data, ruleCache)
		if cl.ConditionFlag {
			conditionMap[checkNode.ID] = result
		} else if !result {
			return false
		}
	}

	for i, thresholdNode := range cl.ThresholdNodes {
		thresholdID := thresholdNode.ID
		if thresholdID == "" {
			thresholdID = fmt.Sprintf("threshold_%d", i)
		}
		tempThresholdMap := map[int]Threshold{1: thresholdNode}
		tempRule := &Rule{
			ID:           "cep_cl_th_" + thresholdID,
			ThresholdMap: tempThresholdMap,
		}
		result := r.executeThreshold(tempRule, 1, data, ruleCache)
		if cl.ConditionFlag {
			conditionMap[thresholdID] = result
		} else if !result {
			return false
		}
	}

	if cl.ConditionFlag {
		return cl.ConditionAST.ExprASTResult(cl.ConditionAST.ExprAST, conditionMap)
	}
	return true
}

// extractCorrelateValues extracts correlation values from the event data based on group_by configuration.
func (r *Ruleset) extractCorrelateValues(seq *Sequence, matchedEventIDs []string, data map[string]interface{}, ruleCache map[string]common.CheckCoreCache) string {
	sb := stringBuilderPool.Get().(*strings.Builder)
	sb.Reset()
	defer stringBuilderPool.Put(sb)

	// Try per-event group_by first (from the first matched event)
	for _, eventID := range matchedEventIDs {
		eventDef := seq.Events[eventID]
		groupByFields := eventDef.GroupByList
		if len(groupByFields) == 0 {
			groupByFields = seq.GroupByList
		}
		if len(groupByFields) == 0 {
			continue
		}

		var fieldPaths [][]string
		if len(eventDef.GroupByFieldPaths) > 0 {
			fieldPaths = eventDef.GroupByFieldPaths
		} else {
			fieldPaths = seq.GroupByFieldPaths
		}

		for idx, field := range groupByFields {
			fieldList := common.StringToList(field)
			if idx < len(fieldPaths) {
				fieldList = fieldPaths[idx]
			}
			val, ok := GetCheckDataFromCache(ruleCache, field, data, fieldList)
			if ok {
				sb.WriteString(val)
				sb.WriteString("|")
			}
		}
		break // Use the first matched event's group_by values
	}

	return sb.String()
}

// extractCorrelateValuesForStateLookup extracts group_by values without relying on
// stage match results, so sequence state can be loaded before "_@"-dependent checks.
func (r *Ruleset) extractCorrelateValuesForStateLookup(seq *Sequence, matchedEventIDs []string, data map[string]interface{}, ruleCache map[string]common.CheckCoreCache) string {
	sb := stringBuilderPool.Get().(*strings.Builder)
	sb.Reset()
	defer stringBuilderPool.Put(sb)

	appendGroupBy := func(groupByFields []string, fieldPaths [][]string) int {
		before := sb.Len()
		for idx, field := range groupByFields {
			fieldList := common.StringToList(field)
			if idx < len(fieldPaths) && len(fieldPaths[idx]) > 0 {
				fieldList = fieldPaths[idx]
			}
			if val, ok := GetCheckDataFromCache(ruleCache, field, data, fieldList); ok {
				sb.WriteString(val)
				sb.WriteString("|")
			}
		}
		return sb.Len() - before
	}

	if len(seq.GroupByList) > 0 {
		appendGroupBy(seq.GroupByList, seq.GroupByFieldPaths)
		return sb.String()
	}

	// Prefer event-level group_by from event definitions that matched this input.
	// This supports multi-source sequences where each event uses different field names
	// (e.g. src_ip on stage 1, client_ip on stage 2) while preserving a stable key.
	for _, eventID := range matchedEventIDs {
		eventDef := seq.Events[eventID]
		if eventDef == nil || len(eventDef.GroupByList) == 0 {
			continue
		}
		if appendGroupBy(eventDef.GroupByList, eventDef.GroupByFieldPaths) > 0 {
			return sb.String()
		}
		sb.Reset()
	}

	// Per-event fallback: use first event definition that has group_by configured.
	for _, eventID := range seq.EventOrder {
		eventDef := seq.Events[eventID]
		if eventDef == nil || len(eventDef.GroupByList) == 0 {
			continue
		}
		if appendGroupBy(eventDef.GroupByList, eventDef.GroupByFieldPaths) > 0 {
			break
		}
		sb.Reset()
	}

	return sb.String()
}

// extractCorrelateValuesForStateLookupWithSequenceFallback attempts to extract group_by
// using sequence-level configuration when per-event extraction failed. This is used as
// a fallback when first-pass matchedEventIDs is empty (e.g., _@-dependent first stage).
func (r *Ruleset) extractCorrelateValuesForStateLookupWithSequenceFallback(seq *Sequence, data map[string]interface{}, ruleCache map[string]common.CheckCoreCache) string {
	// This function is only called when sequence-level group_by is not set and
	// per-event extraction failed. We try to find any event's group_by that can
	// extract values from current data, preferring events that are more likely
	// to match (earlier in EventOrder for first stage, later for subsequent stages).
	// However, since we don't know which stage this is, we try all events in order.
	sb := stringBuilderPool.Get().(*strings.Builder)
	sb.Reset()
	defer stringBuilderPool.Put(sb)

	for _, eventID := range seq.EventOrder {
		eventDef := seq.Events[eventID]
		if eventDef == nil || len(eventDef.GroupByList) == 0 {
			continue
		}

		// Try to extract group_by values for this event
		extracted := false
		for idx, field := range eventDef.GroupByList {
			fieldList := common.StringToList(field)
			if idx < len(eventDef.GroupByFieldPaths) && len(eventDef.GroupByFieldPaths[idx]) > 0 {
				fieldList = eventDef.GroupByFieldPaths[idx]
			}
			if val, ok := GetCheckDataFromCache(ruleCache, field, data, fieldList); ok && val != "" {
				sb.WriteString(val)
				sb.WriteString("|")
				extracted = true
			}
		}

		// If we successfully extracted at least one value, return it
		if extracted {
			return sb.String()
		}
	}

	return ""
}

// hasEventGroupBy checks if any event in the sequence has a per-event group_by.
func (r *Ruleset) hasEventGroupBy(seq *Sequence) bool {
	for _, eventDef := range seq.Events {
		if len(eventDef.GroupByList) > 0 {
			return true
		}
	}
	return false
}

// extractEventTimestamp extracts the event timestamp from data based on event_time configuration.
// Returns unix nanoseconds. When event_time is not specified on the matched event(s), or the
// field is missing or unparseable, uses the engine processing time (time at which the event is seen).
func (r *Ruleset) extractEventTimestamp(seq *Sequence, matchedEventIDs []string, data map[string]interface{}, ruleCache map[string]common.CheckCoreCache) int64 {
	for _, eventID := range matchedEventIDs {
		eventDef := seq.Events[eventID]
		if eventDef.EventTime == "" {
			continue
		}

		fieldList := eventDef.EventTimeFieldPath
		if len(fieldList) == 0 {
			fieldList = common.StringToList(eventDef.EventTime)
		}
		val, ok := GetCheckDataFromCache(ruleCache, eventDef.EventTime, data, fieldList)
		if !ok || val == "" {
			continue
		}

		ts := parseTimestampToNs(val)
		if ts > 0 {
			return ts
		}
	}

	// No event_time set or parseable: use engine processing time (when the engine sees the event)
	return time.Now().UnixNano()
}

// parseTimestampToNs attempts to parse a timestamp string into unix nanoseconds.
// Supports: Unix seconds, Unix milliseconds, Unix microseconds, Unix nanoseconds, ISO 8601, RFC 3339.
func parseTimestampToNs(val string) int64 {
	// Try numeric (unix epoch)
	if n, err := strconv.ParseInt(val, 10, 64); err == nil {
		if n > 1e18 {
			// Already in nanoseconds
			return n
		} else if n > 1e15 {
			// Microseconds -> nanoseconds
			return n * 1000
		} else if n > 1e12 {
			// Milliseconds -> nanoseconds
			return n * int64(time.Millisecond)
		} else if n > 1e9 {
			// Seconds
			return n * int64(time.Second)
		}
		// Very small number, treat as seconds
		return n * int64(time.Second)
	}

	// Try float (e.g., "1700000000.123")
	if f, err := strconv.ParseFloat(val, 64); err == nil {
		return int64(f * float64(time.Second))
	}

	// Try common time formats
	formats := []string{
		time.RFC3339,
		time.RFC3339Nano,
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
	}
	for _, format := range formats {
		if t, err := time.Parse(format, val); err == nil {
			return t.UnixNano()
		}
	}

	return 0
}

// snapshotEventData creates a deep copy snapshot of event data for cross-event reference.
// Deep copy is essential because the original data may be modified by subsequent operations
// (append/modify/del), and we need the snapshot to remain unchanged in stored state.
func (r *Ruleset) snapshotEventData(data map[string]interface{}, matchedEventIDs []string, seq *Sequence) map[string]interface{} {
	return common.MapDeepCopy(data)
}

// buildSequenceResult builds the final enriched data when a sequence completes.
// The current event provides the base data, enriched with:
//   - "#<event_id>" keys: per-event snapshots for cross-event field references (_$#event_id.field)
//   - "_sequence_events" key: a map[event_id] -> raw data for all events in the sequence,
//     providing downstream consumers a single structured view of the entire sequence
func (r *Ruleset) buildSequenceResult(state *SequenceState, seq *Sequence, currentData map[string]interface{}, path *CompletionPath) map[string]interface{} {
	result := common.MapDeepCopy(currentData)
	r.enrichSequenceResultData(result, state, seq, path)
	return result
}

func (r *Ruleset) resolveStageMatchData(match StageMatch) map[string]interface{} {
	if match.Data != nil {
		return match.Data
	}
	if match.ValueRef == "" || r.cepValueStore == nil {
		return nil
	}
	data, err := r.cepValueStore.GetSnapshot(match.ValueRef)
	if err != nil {
		return nil
	}
	return data
}

// enrichSequenceResultData adds sequence metadata to the result.
// Internal "#<event_id>" fields are kept during rule execution for cross-event references
// and removed before data is emitted downstream by sanitizeOutputData.
func (r *Ruleset) enrichSequenceResultData(result map[string]interface{}, state *SequenceState, seq *Sequence, path *CompletionPath) {
	sequenceEvents := make(map[string]interface{})

	for stageIdx, stage := range seq.Condition.Stages {
		matches := state.StageMatches[stageIdx]

		if stage.IsAbsent || len(matches) == 0 {
			continue
		}

		// Use the match that was actually part of the completion path (Bug 2 fix).
		// Falls back to matches[0] when path is nil (e.g. legacy / absence timeout).
		matchIdx := 0
		if path != nil {
			if idx, ok := path.MatchIndices[stageIdx]; ok && idx < len(matches) {
				matchIdx = idx
			}
		}

		matchData := r.resolveStageMatchData(matches[matchIdx])
		if matchData == nil {
			continue
		}

		boundEventIDs := matches[matchIdx].MatchedEventIDs
		if !matches[matchIdx].MatchedEventIDsSet && len(boundEventIDs) == 0 {
			boundEventIDs = stage.EventIDs
		}
		// Keep match under "#<event_id>" for internal cross-event references,
		// and add to _sequence_events for structured output consumption.
		for _, eventID := range boundEventIDs {
			result["#"+eventID] = matchData
			sequenceEvents[eventID] = matchData
		}
	}

	if len(sequenceEvents) > 0 {
		result["_sequence_events"] = sequenceEvents
	}

	result["_sequence_condition"] = map[string]interface{}{
		"content": seq.ConditionExpr,
	}
}

// executeAppend executes an append operation
func (r *Ruleset) executeAppend(rule *Rule, operationID int, copied bool, data map[string]interface{}, ruleCache map[string]common.CheckCoreCache) (modifiedData map[string]interface{}) {
	appendOp, exists := rule.AppendsMap[operationID]
	if !exists {
		return
	}
	if !copied {
		modifiedData = common.MapDeepCopy(data)
	} else {
		modifiedData = data
	}
	if appendOp.Type == "" {
		appendData := appendOp.Value
		// Support inline interpolation of multiple _$placeholders in static text
		// Covers both full replacement (value starts with "_$") and mixed templates
		appendData = replaceFromRawPlaceholders(ruleCache, appendData, data)

		modifiedData[appendOp.FieldName] = appendData
	} else {
		// Plugin
		args := GetPluginRealArgs(appendOp.PluginArgs, modifiedData, ruleCache)

		// Check plugin return type to determine which evaluation method to use
		if appendOp.Plugin.ReturnType == "bool" {
			// For check-type plugins (bool return type), use FuncEvalCheckNode and get the boolean result
			boolResult, err := appendOp.Plugin.FuncEvalCheckNode(args...)
			if err == nil {
				modifiedData[appendOp.FieldName] = boolResult
			} else {
				r.addRuntimeError(fmt.Sprintf("Append plugin %s failed in rule %s: %v", appendOp.Plugin.Name, rule.ID, err))
				// Log error with full context to Redis (plugin executor only logs to local file)
				projectID, rulesetID := parseProjectInfoFromPNS(r.ProjectNodeSequence)
				logger.PluginErrorWithContext("Check-type plugin evaluation failed in append",
					"plugin", appendOp.Plugin.Name,
					"project", projectID,
					"ruleset", rulesetID,
					"ruleID", rule.ID,
					"error", err)
			}
		} else {
			// For interface{} type plugins, use the original FuncEvalOther logic
			res, ok, err := appendOp.Plugin.FuncEvalOther(args...)
			if err == nil && ok {
				if appendOp.FieldName == PluginArgFromRawSymbol {
					if rmap, ok := res.(map[string]interface{}); ok {
						res = rmap
					} else {
						r.addRuntimeError(fmt.Sprintf("Append plugin %s in rule %s returned non-map result for _$ORIDATA", appendOp.Plugin.Name, rule.ID))
						projectID, rulesetID := parseProjectInfoFromPNS(r.ProjectNodeSequence)
						logger.PluginErrorWithContext("Plugin result is not a map",
							"plugin", appendOp.Plugin.Name,
							"project", projectID,
							"ruleset", rulesetID,
							"ruleID", rule.ID,
							"result", res)
						res = nil
					}
				}

				modifiedData[appendOp.FieldName] = res
			} else if err != nil {
				r.addRuntimeError(fmt.Sprintf("Append plugin %s failed in rule %s: %v", appendOp.Plugin.Name, rule.ID, err))
				// Log error with full context to Redis (plugin executor only logs to local file)
				projectID, rulesetID := parseProjectInfoFromPNS(r.ProjectNodeSequence)
				logger.PluginErrorWithContext("Interface-type plugin evaluation failed in append",
					"plugin", appendOp.Plugin.Name,
					"project", projectID,
					"ruleset", rulesetID,
					"ruleID", rule.ID,
					"error", err)
			}
		}
	}
	return
}

// executeModify executes a modify operation
func (r *Ruleset) executeModify(rule *Rule, operationID int, copied bool, data map[string]interface{}, ruleCache map[string]common.CheckCoreCache) (modifiedData map[string]interface{}) {
	modifyOp, exists := rule.ModifyMap[operationID]
	if !exists {
		return
	}
	if !copied {
		modifiedData = common.MapDeepCopy(data)
	} else {
		modifiedData = data
	}
	// Handle by type
	if strings.TrimSpace(modifyOp.Type) == "" {
		// Literal assignment mode; field must be present (enforced in build/validation)
		modifiedData[modifyOp.FieldName] = modifyOp.Value
		return
	}

	// Plugin mode
	args := GetPluginRealArgs(modifyOp.PluginArgs, modifiedData, ruleCache)

	// Check plugin return type to determine which evaluation method to use
	if modifyOp.Plugin.ReturnType == "bool" {
		boolResult, err := modifyOp.Plugin.FuncEvalCheckNode(args...)
		if err != nil {
			r.addRuntimeError(fmt.Sprintf("Modify plugin %s failed in rule %s: %v", modifyOp.Plugin.Name, rule.ID, err))
			// Log error with full context to Redis (plugin executor only logs to local file)
			projectID, rulesetID := parseProjectInfoFromPNS(r.ProjectNodeSequence)
			logger.PluginErrorWithContext("Check-type plugin evaluation failed in modify",
				"plugin", modifyOp.Plugin.Name,
				"project", projectID,
				"ruleset", rulesetID,
				"ruleID", rule.ID,
				"error", err)
			return
		}
		if modifyOp.FieldName != "" {
			modifiedData[modifyOp.FieldName] = boolResult
			return
		} else {
			r.addRuntimeError(fmt.Sprintf("Modify plugin %s in rule %s requires a field when return type is bool", modifyOp.Plugin.Name, rule.ID))
			projectID, rulesetID := parseProjectInfoFromPNS(r.ProjectNodeSequence)
			logger.PluginErrorWithContext("Modify without field requires map result; got bool",
				"plugin", modifyOp.Plugin.Name,
				"project", projectID,
				"ruleset", rulesetID,
				"ruleID", rule.ID)
			return
		}
	}

	// For interface{} type plugins, use FuncEvalOther
	res, ok, err := modifyOp.Plugin.FuncEvalOther(args...)
	if err != nil || !ok {
		if err != nil {
			r.addRuntimeError(fmt.Sprintf("Modify plugin %s failed in rule %s: %v", modifyOp.Plugin.Name, rule.ID, err))
			// Log error with full context to Redis (plugin executor only logs to local file)
			projectID, rulesetID := parseProjectInfoFromPNS(r.ProjectNodeSequence)
			logger.PluginErrorWithContext("Interface-type plugin evaluation failed in modify",
				"plugin", modifyOp.Plugin.Name,
				"project", projectID,
				"ruleset", rulesetID,
				"ruleID", rule.ID,
				"error", err)
		}
		return
	}

	if modifyOp.FieldName != "" {
		if modifyOp.FieldName == PluginArgFromRawSymbol {
			if rmap, ok := res.(map[string]interface{}); ok {
				modifiedData = rmap
				return
			} else {
				r.addRuntimeError(fmt.Sprintf("Modify plugin %s in rule %s returned non-map result for _$ORIDATA", modifyOp.Plugin.Name, rule.ID))
				projectID, rulesetID := parseProjectInfoFromPNS(r.ProjectNodeSequence)
				logger.PluginErrorWithContext("Plugin result is not a map",
					"plugin", modifyOp.Plugin.Name,
					"project", projectID,
					"ruleset", rulesetID,
					"ruleID", rule.ID,
					"result", res)
				return
			}
		}
		modifiedData[modifyOp.FieldName] = res
		return
	}

	// rmap from plugin's result without any race condition
	if rmap, ok := res.(map[string]interface{}); ok {
		modifiedData = rmap
		return
	} else {
		r.addRuntimeError(fmt.Sprintf("Modify plugin %s in rule %s returned non-map result", modifyOp.Plugin.Name, rule.ID))
		projectID, rulesetID := parseProjectInfoFromPNS(r.ProjectNodeSequence)
		logger.PluginErrorWithContext("Modify without field expects map result to replace data",
			"plugin", modifyOp.Plugin.Name,
			"project", projectID,
			"ruleset", rulesetID,
			"ruleID", rule.ID,
			"result", res)
		return
	}
}

// executeDel executes a delete operation
func (r *Ruleset) executeDel(rule *Rule, operationID int, copied bool, data map[string]interface{}) (modifiedData map[string]interface{}) {
	delFields, exists := rule.DelMap[operationID]
	if !exists {
		return
	}
	if !copied {
		modifiedData = common.MapDeepCopy(data)
	} else {
		modifiedData = data
	}
	for _, fieldPath := range delFields {
		common.MapDel(modifiedData, fieldPath)
	}
	return modifiedData
}

// executePlugin executes a plugin operation
func (r *Ruleset) executePlugin(rule *Rule, operationID int, dataCopy map[string]interface{}, ruleCache map[string]common.CheckCoreCache) {
	pluginOp, exists := rule.PluginMap[operationID]
	if !exists {
		return
	}
	args := GetPluginRealArgs(pluginOp.PluginArgs, dataCopy, ruleCache)

	// Check plugin return type to determine which evaluation method to use
	if pluginOp.Plugin.ReturnType == "bool" {
		// For check-type plugins (bool return type), use FuncEvalCheckNode
		ok, err := pluginOp.Plugin.FuncEvalCheckNode(args...)
		if err != nil {
			r.addRuntimeError(fmt.Sprintf("Plugin %s failed in rule %s: %v", pluginOp.Plugin.Name, rule.ID, err))
			// Log error with full context to Redis (plugin executor only logs to local file)
			projectID, rulesetID := parseProjectInfoFromPNS(r.ProjectNodeSequence)
			logger.PluginErrorWithContext("Check-type plugin evaluation failed",
				"plugin", pluginOp.Plugin.Name,
				"project", projectID,
				"ruleset", rulesetID,
				"ruleID", rule.ID,
				"error", err)
		}

		if !ok {
			logger.Info("Check-type plugin check failed", "plugin", pluginOp.Plugin.Name, "ruleID", rule.ID, "rulesetID", r.RulesetID)
		}
	} else {
		// For interface{} type plugins, use FuncEvalOther (for side effects, result is ignored)
		_, ok, err := pluginOp.Plugin.FuncEvalOther(args...)
		if err != nil {
			r.addRuntimeError(fmt.Sprintf("Plugin %s failed in rule %s: %v", pluginOp.Plugin.Name, rule.ID, err))
			// Log error with full context to Redis (plugin executor only logs to local file)
			projectID, rulesetID := parseProjectInfoFromPNS(r.ProjectNodeSequence)
			logger.PluginErrorWithContext("Interface-type plugin evaluation failed",
				"plugin", pluginOp.Plugin.Name,
				"project", projectID,
				"ruleset", rulesetID,
				"ruleID", rule.ID,
				"error", err)
		}

		if !ok {
			logger.Info("Interface-type plugin execution failed", "plugin", pluginOp.Plugin.Name, "ruleID", rule.ID, "rulesetID", r.RulesetID)
		}
	}
}

// executeIterator executes an iterator operation
func (r *Ruleset) executeIterator(rule *Rule, operationID int, data map[string]interface{}, ruleCache map[string]common.CheckCoreCache) bool {
	iterator, exists := rule.IteratorMap[operationID]
	if !exists {
		return true
	}

	// Get the array/slice to iterate over
	iterateData, exist := common.GetCheckDataWithType(data, iterator.FieldList)
	if !exist {
		return false
	}

	// Convert to slice of interface{} for iteration
	var iterateSlice []interface{}
	switch v := iterateData.(type) {
	case []interface{}:
		iterateSlice = v
	case []string:
		iterateSlice = make([]interface{}, len(v))
		for i, item := range v {
			iterateSlice[i] = item
		}
	case []map[string]interface{}:
		iterateSlice = make([]interface{}, len(v))
		for i, item := range v {
			iterateSlice[i] = item
		}
	case string:
		// If it's a string, try to parse it as JSON array
		var jsonArray []interface{}
		if err := sonic.Unmarshal([]byte(v), &jsonArray); err == nil {
			iterateSlice = jsonArray
		} else {
			return false
		}
	default:
		// Try to convert using reflection if possible
		return false
	}

	if len(iterateSlice) == 0 {
		return false
	}

	successCount := 0
	totalCount := len(iterateSlice)

	// Iterate over each item in the array
	for _, item := range iterateSlice {
		iterationContext := map[string]interface{}{iterator.Variable: item}

		// Execute all check nodes and threshold nodes for this item
		itemResult := true

		// Execute check nodes
		for i := range iterator.CheckNodes {
			checkNode := &iterator.CheckNodes[i]
			checkResult := r.executeCheckNode(checkNode, iterationContext, ruleCache)
			if !checkResult {
				itemResult = false
				break // Early exit for this item if any check fails
			}
		}

		// Execute threshold nodes only if check nodes passed
		if itemResult && len(iterator.ThresholdNodes) > 0 {
			for _, thresholdNode := range iterator.ThresholdNodes {

				tempRule := &Rule{
					ID:           rule.ID, // Use the original rule ID
					ThresholdMap: map[int]Threshold{1: thresholdNode},
				}

				// Use iteration context so group_by/count_field can reference the iterator variable
				thresholdResult := r.executeThreshold(tempRule, 1, iterationContext, ruleCache)
				if !thresholdResult {
					itemResult = false
					break
				}
			}
		}

		// Execute checklists inside iterator
		if itemResult && len(iterator.Checklists) > 0 {
			for _, checklist := range iterator.Checklists {
				tempRule := &Rule{
					ID:           rule.ID, // Use the original rule ID
					ChecklistMap: map[int]Checklist{1: checklist},
				}

				// Use iteration context so inner checks/thresholds evaluate against iterator variable only
				checklistResult := r.executeCheckList(tempRule, 1, iterationContext, ruleCache)
				if !checklistResult {
					itemResult = false
					break
				}
			}
		}

		if itemResult {
			successCount++
		}

		// Early exit optimization
		if iterator.Type == "ANY" && successCount > 0 {
			return true // Found at least one match for ANY
		}
		if iterator.Type == "ALL" && !itemResult {
			return false // Found a failure for ALL
		}
	}

	// Final result based on iterator type
	switch iterator.Type {
	case "ANY":
		return successCount > 0
	case "ALL":
		return successCount == totalCount
	default:
		return false
	}
}

// executeIteratorThreshold executes a threshold check within an iterator context
func (r *Ruleset) executeIteratorThreshold(threshold *Threshold, data map[string]interface{}, ruleCache map[string]common.CheckCoreCache) bool {
	// This is similar to executeThreshold but operates within iterator context
	// For simplicity, we'll use a basic implementation that checks if the threshold conditions are met
	// In a full implementation, you might want to handle iterator-specific threshold logic

	// Get group by data
	groupByKey := ""
	for _, groupByField := range orderedThresholdGroupByFields(threshold) {
		fieldData, exist := common.GetCheckData(data, threshold.GroupByList[groupByField])
		if exist {
			groupByKey += fmt.Sprintf("%v", fieldData) + "_"
		}
	}

	if groupByKey == "" {
		return false
	}

	// For iterator thresholds, we use a simplified approach
	// In practice, you might want to implement more sophisticated threshold logic
	// that accumulates across iterator iterations

	// Get count value based on count type
	countValue := 1 // Default count
	if threshold.CountType == "SUM" && threshold.CountFieldList != nil {
		if fieldData, exist := common.GetCheckDataWithType(data, threshold.CountFieldList); exist {
			if val, ok := fieldData.(int); ok {
				countValue = val
			} else if val, ok := fieldData.(float64); ok {
				countValue = int(val)
			}
		}
	}

	// Simple threshold check - in practice, this would accumulate over time/iterations
	return countValue >= threshold.Value
}

func orderedThresholdGroupByFields(threshold *Threshold) []string {
	if threshold == nil {
		return nil
	}

	if threshold.group_by != "" {
		parts := strings.Split(threshold.group_by, ",")
		fields := make([]string, 0, len(parts))
		for _, field := range parts {
			field = strings.TrimSpace(field)
			if field != "" {
				fields = append(fields, field)
			}
		}
		return fields
	}

	if len(threshold.GroupByList) == 0 {
		return nil
	}

	fields := make([]string, 0, len(threshold.GroupByList))
	for field := range threshold.GroupByList {
		field = strings.TrimSpace(field)
		if field != "" {
			fields = append(fields, field)
		}
	}
	sort.Strings(fields)
	return fields
}

// checkNodeLogic executes the check logic for a single check node.
func checkNodeLogic(r *Ruleset, checkNode *CheckNodes, data map[string]interface{}, checkNodeValue string, checkNodeValueFromRaw bool, ruleCache map[string]common.CheckCoreCache, regexResultCache *RegexResultCache) bool {
	var checkListFlag = false

	needCheckData, exist := common.GetCheckData(data, checkNode.FieldList)

	// CRITICAL FIX: Handle field existence properly for ISNULL and NOTNULL checks
	if checkNode.Type == "ISNULL" {
		// For ISNULL: field doesn't exist OR field exists but is empty (including whitespace-only)
		if !exist || strings.TrimSpace(needCheckData) == "" {
			return true
		} else {
			return false
		}
	}

	if checkNode.Type == "NOTNULL" {
		// For NOTNULL: field must exist AND not be empty (including whitespace-only)
		if !exist || strings.TrimSpace(needCheckData) == "" {
			return false
		} else {
			return true
		}
	}

	// For other check types, if field doesn't exist, the check should fail
	if !exist && checkNode.Type != "PLUGIN" {
		return false
	}

	switch checkNode.Type {
	case "REGEX":
		if !checkNodeValueFromRaw {
			// Static regex value - use result cache with pre-compiled regex for better performance
			// This maintains the same behavior as original: REGEX(needCheckData, checkNode.Regex)
			checkListFlag = CachedRegexMatchWithPrecompiled(regexResultCache, checkNode.Regex, checkNodeValue, needCheckData)
		} else {
			// Dynamic regex from raw data - use compiled regex cache (no result caching)
			// This maintains the same behavior as original
			regex, err := GetCompiledRegex(checkNodeValue)
			if err != nil {
				break
			}
			checkListFlag, _ = REGEX(needCheckData, regex)
		}
	case "PLUGIN":
		args := GetPluginRealArgs(checkNode.PluginArgs, data, ruleCache)
		result, err := checkNode.Plugin.FuncEvalCheckNode(args...)
		if err != nil {
			r.addRuntimeError(fmt.Sprintf("Check plugin %s failed in rule node %s: %v", checkNode.Plugin.Name, checkNode.ID, err))
			return false
		}

		// Check if plugin function should be negated (starts with !)
		if checkNode.IsNegated {
			return !result
		}

		return result

	default:
		// SIMD optimization path: intelligently choose whether to use SIMD
		if shouldUseSIMD(checkNode.Type, needCheckData, checkNodeValue) {
			switch checkNode.Type {
			case "INCL":
				checkListFlag, _ = SIMDEnhancedINCL(needCheckData, checkNodeValue)
			case "NCS_INCL":
				checkListFlag, _ = SIMDEnhancedNCS_INCL(needCheckData, checkNodeValue)
			case "START":
				checkListFlag, _ = SIMDEnhancedSTART(needCheckData, checkNodeValue)
			case "NCS_START":
				checkListFlag, _ = SIMDEnhancedNCS_START(needCheckData, checkNodeValue)
			case "END":
				checkListFlag, _ = SIMDEnhancedEND(needCheckData, checkNodeValue)
			case "NCS_END":
				checkListFlag, _ = SIMDEnhancedNCS_END(needCheckData, checkNodeValue)
			default:
				// Fallback to standard implementation
				checkListFlag, _ = checkNode.CheckFunc(needCheckData, checkNodeValue)
			}
		} else {
			// Use standard implementation
			checkListFlag, _ = checkNode.CheckFunc(needCheckData, checkNodeValue)
		}
	}

	return checkListFlag
}

// mapDeepCopyWithExtraCapacity performs a deep copy with additional capacity for rule operations
// extraCap: additional capacity for fields that will be added (hit_rule_id, append)
// This function only adds extra capacity at the top level; nested structures use standard deep copy
func mapDeepCopyWithExtraCapacity(m map[string]interface{}, extraCap int) map[string]interface{} {
	if m == nil {
		return nil
	}

	result := make(map[string]interface{}, len(m)+extraCap)
	for k, v := range m {
		// Use common.MapDeepCopyAction for recursive deep copy of all nested structures
		// This ensures correct handling of nested maps, slices, and any combination
		result[k] = common.MapDeepCopyAction(v)
	}
	return result
}

// addHitRuleID appends the hit rule ID to the data map.
func addHitRuleID(data map[string]interface{}, ruleID string) {
	// data is guaranteed to be non-nil when called from EngineCheck
	if existingID, ok := data[HitRuleIdFieldName]; !ok {
		data[HitRuleIdFieldName] = ruleID
	} else {
		// Check if this is the same rule ID to avoid duplication
		existingStr := existingID.(string)
		if existingStr == ruleID {
			// Same rule ID, don't duplicate
			return
		}
		// Use strings.Builder pool for efficient string concatenation
		sb := stringBuilderPool.Get().(*strings.Builder)
		sb.Reset()
		sb.WriteString(existingStr)
		sb.WriteString(",")
		sb.WriteString(ruleID)
		data[HitRuleIdFieldName] = sb.String()
		stringBuilderPool.Put(sb)
	}
}

// collectHitFields walks through all operators in a rule and collects field names
// that are relevant to the rule's matching logic (check, threshold, iterator, sequence).
// Called once during parsing; the result is stored in Rule.HitFields for zero-cost
// injection at runtime.
func collectHitFields(rule *Rule) []string {
	seen := make(map[string]struct{}, 8)
	fields := make([]string, 0, 8)
	add := func(field string) {
		if field == "" {
			return
		}
		if _, ok := seen[field]; !ok {
			seen[field] = struct{}{}
			fields = append(fields, field)
		}
	}

	addFromCheckNodes := func(nodes []CheckNodes) {
		for i := range nodes {
			add(nodes[i].Field)
		}
	}

	addFromThresholds := func(thresholds []Threshold) {
		for i := range thresholds {
			for _, f := range orderedThresholdGroupByFields(&thresholds[i]) {
				add(f)
			}
			add(thresholds[i].CountField)
		}
	}

	addFromChecklist := func(cl Checklist) {
		addFromCheckNodes(cl.CheckNodes)
		addFromThresholds(cl.ThresholdNodes)
	}

	addFromIterator := func(iter Iterator) {
		add(iter.Field)
		addFromCheckNodes(iter.CheckNodes)
		addFromThresholds(iter.ThresholdNodes)
		for _, cl := range iter.Checklists {
			addFromChecklist(cl)
		}
	}

	addFromSequence := func(seq Sequence) {
		for _, f := range seq.GroupByList {
			add(f)
		}
		for _, eventID := range seq.EventOrder {
			ev := seq.Events[eventID]
			if ev == nil {
				continue
			}
			for _, f := range ev.GroupByList {
				add(f)
			}
			addFromCheckNodes(ev.CheckNodes)
			addFromThresholds(ev.Thresholds)
			for _, cl := range ev.Checklists {
				addFromChecklist(cl)
			}
		}
	}

	if rule.Queue != nil {
		for _, op := range *rule.Queue {
			switch op.Type {
			case T_CheckList:
				if cl, ok := rule.ChecklistMap[op.ID]; ok {
					addFromChecklist(cl)
				}
			case T_Check:
				if cn, ok := rule.CheckMap[op.ID]; ok {
					add(cn.Field)
				}
			case T_Threshold:
				if th, ok := rule.ThresholdMap[op.ID]; ok {
					addFromThresholds([]Threshold{th})
				}
			case T_Iterator:
				if iter, ok := rule.IteratorMap[op.ID]; ok {
					addFromIterator(iter)
				}
			case T_Sequence:
				if seq, ok := rule.SequenceMap[op.ID]; ok {
					addFromSequence(seq)
				}
			}
		}
	} else {
		for _, cn := range rule.CheckMap {
			add(cn.Field)
		}
		for _, cl := range rule.ChecklistMap {
			addFromChecklist(cl)
		}
		for _, th := range rule.ThresholdMap {
			addFromThresholds([]Threshold{th})
		}
		for _, iter := range rule.IteratorMap {
			addFromIterator(iter)
		}
		for _, seq := range rule.SequenceMap {
			addFromSequence(seq)
		}
	}

	if len(seen) == 0 {
		return nil
	}
	return fields
}

// GetProcessTotal returns the total processed message count.
func (r *Ruleset) GetProcessTotal() uint64 {
	return atomic.LoadUint64(&r.processTotal)
}

// ResetProcessTotal resets the total processed count to zero.
// This should only be called during component cleanup or forced restart.
func (r *Ruleset) ResetProcessTotal() uint64 {
	atomic.StoreUint64(&r.lastReportedTotal, 0)
	return atomic.SwapUint64(&r.processTotal, 0)
}

// GetIncrementAndUpdate returns the increment since last call and updates the baseline.
// This method is thread-safe and designed for statistics collection.
// Uses CAS operation to ensure atomicity.
func (r *Ruleset) GetIncrementAndUpdate() uint64 {
	current := atomic.LoadUint64(&r.processTotal)
	last := atomic.LoadUint64(&r.lastReportedTotal)

	// Use CAS to atomically update lastReportedTotal
	// If CAS fails, we simply return 0 - one missed stat collection is not critical
	if atomic.CompareAndSwapUint64(&r.lastReportedTotal, last, current) {
		return current - last
	}

	return 0
}

// GetRunningTaskCount returns the number of currently running tasks in the thread pool
// Returns 0 if the thread pool is not initialized
func (r *Ruleset) GetRunningTaskCount() int {
	if r.antsPool != nil {
		return r.antsPool.Running()
	}
	return 0
}

// shouldUseSIMD determines whether to use SIMD optimization based on operation type and data characteristics
func shouldUseSIMD(operationType, data, pattern string) bool {
	// First check if SIMD is globally enabled
	if !simdEnabled {
		return false
	}

	// Only enable SIMD for supported operation types
	switch operationType {
	case "INCL", "NCS_INCL", "START", "NCS_START", "END", "NCS_END":
		// Intelligent thresholds based on data and pattern length
		dataLen := len(data)
		patternLen := len(pattern)

		// Empty data or empty pattern not suitable for SIMD
		if dataLen == 0 || patternLen == 0 {
			return false
		}

		var useSIMD bool
		// For contains operations, data length should be at least twice the pattern length and >=16 bytes
		if operationType == "INCL" || operationType == "NCS_INCL" {
			useSIMD = dataLen >= 16 && dataLen >= patternLen*2
		} else {
			// For prefix/suffix operations, data length >=16 bytes is sufficient
			useSIMD = dataLen >= 16
		}

		return useSIMD

	default:
		return false
	}
}
