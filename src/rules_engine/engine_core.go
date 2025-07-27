package rules_engine

import (
	"AgentSmith-HUB/common"
	"AgentSmith-HUB/logger"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/panjf2000/ants/v2"
)

const HitRuleIdFieldName = "_hub_hit_rule_id"

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
	return len(s) >= 2 && s[0] == '_' && s[1] == '$'
}

// Start the ruleset engine, consuming data from upstream and writing checked data to downstream.
func (r *Ruleset) Start() error {
	// Add panic recovery for critical state changes
	defer func() {
		if panicErr := recover(); panicErr != nil {
			logger.Error("Panic during ruleset start", "ruleset", r.RulesetID, "panic", panicErr)
			// Ensure cleanup and proper status setting on panic
			r.cleanup()
			r.SetStatus(common.StatusError, fmt.Errorf("panic during start: %v", panicErr))
		}
	}()

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
	go func() {
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
				case totalBacklog > 2048:
					targetSize = maxPoolSize
				case totalBacklog > 1024:
					targetSize = level2
				case totalBacklog > 512:
					targetSize = level1
				case totalBacklog > 128:
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
		go func(id string, ch *chan map[string]interface{}) {
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
							for _, downCh := range r.DownStream {
								*downCh <- res // Blocking write to ensure data integrity
							}
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

	r.SetStatus(common.StatusRunning, nil)
	return nil
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
		return fmt.Errorf("cannot stop ruleset engine, current status: %s", r.Status)
	}
	r.SetStatus(common.StatusStopping, nil)
	close(r.stopChan)

	// Overall timeout for ruleset stop
	overallTimeout := time.After(30 * time.Second) // Reduced from 60s to 30s
	stopCompleted := make(chan struct{})

	go func() {
		defer close(stopCompleted)

		// Wait for all upstream channels to be consumed.
		logger.Info("Waiting for upstream channels to empty", "ruleset", r.RulesetID)
		upstreamTimeout := time.After(10 * time.Second) // 10 second timeout for upstream
		waitCount := 0

	waitUpstream:
		for {
			select {
			case <-upstreamTimeout:
				logger.Warn("Timeout waiting for upstream channels, forcing shutdown", "ruleset", r.RulesetID)
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

		downstreamTimeout := time.After(10 * time.Second) // 10 second timeout for downstream
		waitCount = 0

	waitDownstream:
		for {
			select {
			case <-downstreamTimeout:
				break waitDownstream
			default:
				allEmpty := true
				totalMessages := 0
				for _, downCh := range r.DownStream {
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
		logger.Warn("Ruleset stop timeout exceeded, forcing shutdown", "ruleset", r.RulesetID)
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
	case <-time.After(10 * time.Second):
		logger.Warn("Timeout waiting for ruleset goroutines, forcing cleanup", "ruleset", r.RulesetID)
	}

	// Wait for thread pool to finish with timeout
	if r.antsPool != nil {
		logger.Info("Waiting for thread pool tasks to complete", "ruleset", r.RulesetID)
		poolWaitTimeout := time.After(15 * time.Second)
	poolWait:
		for {
			select {
			case <-poolWaitTimeout:
				logger.Warn("Thread pool timeout, forcing cleanup", "ruleset", r.RulesetID)
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

	r.SetStatus(common.StatusStopped, nil)
	return nil
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
		// For whitelist rules, usually only 1 result
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

	// For whitelist, keep track of the last modified data
	var lastModifiedData map[string]interface{}

	// For empty whitelist, data should pass through
	if !r.IsDetection && len(r.Rules) == 0 {
		// Empty whitelist means all data passes through
		ruleCachePool.Put(ruleCache)
		// Reuse the same slice pattern for consistency
		result := make([]map[string]interface{}, 1)
		result[0] = data
		return result
	}

	// Process each rule in the ruleset
	for ruleIndex := range r.Rules {
		rule := &r.Rules[ruleIndex] // Use pointer to avoid copying

		// Create data copy for this rule execution only if rule modifies data
		var dataCopy map[string]interface{}
		if r.ruleModifiesData(rule) {
			dataCopy = common.MapDeepCopy(data)
		} else {
			dataCopy = data // Use original data if rule doesn't modify it
		}

		// Execute all operations in the order specified by the Queue
		ruleCheckRes := r.executeRuleOperations(rule, dataCopy, ruleCache)

		// Handle rule result based on ruleset type
		if r.IsDetection {
			// For detection rules, if rule passes, add to results
			if ruleCheckRes {
				// Add rule info
				// Build hit rule ID efficiently using string builder pool
				sb := stringBuilderPool.Get().(*strings.Builder)
				sb.Reset()
				sb.WriteString(r.RulesetID)
				sb.WriteString(".")
				sb.WriteString(rule.ID)
				addHitRuleID(dataCopy, sb.String())
				stringBuilderPool.Put(sb)
				// Add to final result
				finalRes = append(finalRes, dataCopy)
			}
		} else {
			// For whitelist rules
			// Always update lastModifiedData with the result of rule execution
			lastModifiedData = dataCopy

			if ruleCheckRes {
				// If whitelist rule passes, data is whitelisted (allowed) - don't pass forward (return empty)
				ruleCachePool.Put(ruleCache)
				return make([]map[string]interface{}, 0)
			}
		}
	}

	// For whitelist: if no rule passed, data needs processing - pass forward the last modified data
	if !r.IsDetection && len(finalRes) == 0 && lastModifiedData != nil {
		finalRes = append(finalRes, lastModifiedData)
	}

	// put back to pool
	ruleCachePool.Put(ruleCache)
	ruleCache = nil

	// Create a copy of the result to return, since we're using a pooled slice
	result := make([]map[string]interface{}, len(finalRes))
	copy(result, finalRes)
	return result
}

// executeRuleOperations executes all operations in a rule according to the Queue order
func (r *Ruleset) executeRuleOperations(rule *Rule, data map[string]interface{}, ruleCache map[string]common.CheckCoreCache) bool {
	if rule.Queue == nil || len(*rule.Queue) == 0 {
		// No operations to execute
		// For detection rules, empty rule means no match (false)
		// For whitelist rules, empty rule also means no match (false), allowing data to pass
		return false
	}

	ruleResult := true
	// Execute operations in the exact order specified by the Queue
	for _, op := range *rule.Queue {
		switch op.Type {
		case T_CheckList:
			checkResult := r.executeCheckList(rule, op.ID, data, ruleCache)
			if !checkResult {
				ruleResult = false
				// For detection rules, if check fails, stop execution
				if r.IsDetection {
					return false
				}
				// For whitelist rules, continue executing other operations
			}
		case T_Check:
			checkResult := r.executeCheck(rule, op.ID, data, ruleCache)
			if !checkResult {
				ruleResult = false
				// For detection rules, if check fails, stop execution
				if r.IsDetection {
					return false
				}
				// For whitelist rules, continue executing other operations
			}
		case T_Threshold:
			thresholdResult := r.executeThreshold(rule, op.ID, data, ruleCache)
			if !thresholdResult {
				ruleResult = false
				// For detection rules, if threshold fails, stop execution
				if r.IsDetection {
					return false
				}
				// For whitelist rules, continue executing other operations
			}
		case T_Append:
			// Execute append operation according to user-defined order
			r.executeAppend(rule, op.ID, data, ruleCache)
		case T_Del:
			// Execute del operation according to user-defined order
			r.executeDel(rule, op.ID, data)
		case T_Plugin:
			// Execute plugin operation according to user-defined order
			r.executePlugin(rule, op.ID, data, ruleCache)
		}
	}

	return ruleResult
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
		conditionMap = make(map[string]bool, len(checklist.CheckNodes))
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

	// If using condition expression, evaluate it
	if checklist.ConditionFlag {
		return checklist.ConditionAST.ExprASTResult(checklist.ConditionAST.ExprAST, conditionMap)
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
		return checkNodeLogic(checkNode, data, checkNodeValue, checkNodeValueFromRaw, ruleCache, r.RegexResultCache)
	case "AND":
		for _, v := range checkNode.DelimiterFieldList {
			if hasFromRawPrefix(v) {
				checkNodeValue = GetRuleValueFromRawFromCache(ruleCache, v, data)
				checkNodeValueFromRaw = true
			} else {
				checkNodeValue = v
				checkNodeValueFromRaw = false
			}
			if !checkNodeLogic(checkNode, data, checkNodeValue, checkNodeValueFromRaw, ruleCache, r.RegexResultCache) {
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
			if checkNodeLogic(checkNode, data, checkNodeValue, checkNodeValueFromRaw, ruleCache, r.RegexResultCache) {
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

	// Isolate by ruleset ID and rule ID
	// Use strings.Builder pool for better performance
	sb := stringBuilderPool.Get().(*strings.Builder)
	sb.Reset()
	sb.WriteString(threshold.GroupByID)

	for k, v := range threshold.GroupByList {
		tmpData, _ := GetCheckDataFromCache(ruleCache, k, data, v)
		sb.WriteString(tmpData)
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

// executeAppend executes an append operation
func (r *Ruleset) executeAppend(rule *Rule, operationID int, dataCopy map[string]interface{}, ruleCache map[string]common.CheckCoreCache) {
	appendOp, exists := rule.AppendsMap[operationID]
	if !exists {
		return
	}

	if appendOp.Type == "" {
		appendData := appendOp.Value
		if hasFromRawPrefix(appendOp.Value) {
			appendData = GetRuleValueFromRawFromCache(ruleCache, appendOp.Value, dataCopy)
		}

		dataCopy[appendOp.FieldName] = appendData
	} else {
		// Plugin
		args := GetPluginRealArgs(appendOp.PluginArgs, dataCopy, ruleCache)

		// Check plugin return type to determine which evaluation method to use
		if appendOp.Plugin.ReturnType == "bool" {
			// For check-type plugins (bool return type), use FuncEvalCheckNode and get the boolean result
			boolResult, err := appendOp.Plugin.FuncEvalCheckNode(args...)
			if err == nil {
				dataCopy[appendOp.FieldName] = boolResult
			} else {
				logger.PluginError("Check-type plugin evaluation error in append", "plugin", appendOp.Plugin.Name, "error", err)
			}
		} else {
			// For interface{} type plugins, use the original FuncEvalOther logic
			res, ok, err := appendOp.Plugin.FuncEvalOther(args...)
			if err == nil && ok {
				if appendOp.FieldName == PluginArgFromRawSymbol {
					if r, ok := res.(map[string]interface{}); ok {
						res = common.MapDeepCopy(r)
					} else {
						logger.PluginError("Plugin result is not a map", "plugin", appendOp.Plugin.Name, "result", res)
						res = nil
					}
				}

				dataCopy[appendOp.FieldName] = res
			} else if err != nil {
				logger.PluginError("Interface-type plugin evaluation error in append", "plugin", appendOp.Plugin.Name, "error", err)
			}
		}
	}
}

// executeDel executes a delete operation
func (r *Ruleset) executeDel(rule *Rule, operationID int, dataCopy map[string]interface{}) {
	delFields, exists := rule.DelMap[operationID]
	if !exists {
		return
	}

	for _, fieldPath := range delFields {
		common.MapDel(dataCopy, fieldPath)
	}
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
			logger.PluginError("Check-type plugin evaluation error", "plugin", pluginOp.Plugin.Name, "error", err)
		}

		if !ok {
			logger.Info("Check-type plugin check failed", "plugin", pluginOp.Plugin.Name, "ruleID", rule.ID, "rulesetID", r.RulesetID)
		}
	} else {
		// For interface{} type plugins, use FuncEvalOther (for side effects, result is ignored)
		_, ok, err := pluginOp.Plugin.FuncEvalOther(args...)
		if err != nil {
			logger.PluginError("Interface-type plugin evaluation error", "plugin", pluginOp.Plugin.Name, "error", err)
		}

		if !ok {
			logger.Info("Interface-type plugin execution failed", "plugin", pluginOp.Plugin.Name, "ruleID", rule.ID, "rulesetID", r.RulesetID)
		}
	}
}

// checkNodeLogic executes the check logic for a single check node.
func checkNodeLogic(checkNode *CheckNodes, data map[string]interface{}, checkNodeValue string, checkNodeValueFromRaw bool, ruleCache map[string]common.CheckCoreCache, regexResultCache *RegexResultCache) bool {
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
			return false
		}

		// Check if plugin function should be negated (starts with !)
		if checkNode.IsNegated {
			return !result
		}

		return result

	default:
		checkListFlag, _ = checkNode.CheckFunc(needCheckData, checkNodeValue)
	}

	return checkListFlag
}

// addHitRuleID appends the hit rule ID to the data map.
func addHitRuleID(data map[string]interface{}, ruleID string) {
	// data is guaranteed to be non-nil when called from EngineCheck
	if existingID, ok := data[HitRuleIdFieldName]; !ok {
		data[HitRuleIdFieldName] = ruleID
	} else {
		// Use strings.Builder pool for efficient string concatenation
		sb := stringBuilderPool.Get().(*strings.Builder)
		sb.Reset()
		sb.WriteString(existingID.(string))
		sb.WriteString(",")
		sb.WriteString(ruleID)
		data[HitRuleIdFieldName] = sb.String()
		stringBuilderPool.Put(sb)
	}
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

// ruleModifiesData checks if a rule contains operations that modify the input data
func (r *Ruleset) ruleModifiesData(rule *Rule) bool {
	if rule.Queue == nil {
		return false
	}

	for _, op := range *rule.Queue {
		switch op.Type {
		case T_Append, T_Del, T_Plugin:
			return true // These operations modify data
		}
	}
	return false
}

// GetRunningTaskCount returns the number of currently running tasks in the thread pool
// Returns 0 if the thread pool is not initialized
func (r *Ruleset) GetRunningTaskCount() int {
	if r.antsPool != nil {
		return r.antsPool.Running()
	}
	return 0
}
