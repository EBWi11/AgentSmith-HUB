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

	regexp "github.com/BurntSushi/rure-go"
	"github.com/panjf2000/ants/v2"
)

const HitRuleIdFieldName = "_hub_hit_rule_id"

// ruleCachePool reuses map objects to reduce allocations
var ruleCachePool = sync.Pool{
	New: func() interface{} { return make(map[string]common.CheckCoreCache, 8) },
}

// Start the ruleset engine, consuming data from upstream and writing checked data to downstream.
func (r *Ruleset) Start() error {
	if r.stopChan != nil {
		return fmt.Errorf("already started: %v", r.RulesetID)
	}
	r.stopChan = make(chan struct{})

	var err error
	minPoolSize := getMinPoolSize()
	r.antsPool, err = ants.NewPool(minPoolSize)
	if err != nil {
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
				targetSize := minPoolSize
				switch {
				case totalBacklog > 1000:
					targetSize = maxPoolSize
				case totalBacklog > 512:
					targetSize = maxPoolSize * 3 / 4
				case totalBacklog > 256:
					targetSize = maxPoolSize / 2
				case totalBacklog > 32:
					targetSize = maxPoolSize / 4
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
						// Test mode is identified by ProjectNodeSequence starting with "TEST."
						isTestMode := strings.HasPrefix(r.ProjectNodeSequence, "TEST.")
						if !isTestMode {
							// IMPORTANT: Move counting AFTER successful task execution
							// This ensures we only count messages that are actually processed
							defer func() {
								atomic.AddUint64(&r.processTotal, 1)
							}()

							// IMPORTANT: Sample the input data BEFORE rule checking starts
							// This ensures we capture the raw data entering the ruleset for analysis
							if r.sampler != nil {
								pid := ""
								if len(r.OwnerProjects) > 0 {
									pid = r.OwnerProjects[0]
								}
								_ = r.sampler.Sample(data, r.ProjectNodeSequence, pid)
							}
						}

						// Now perform rule checking on the input data
						results := r.EngineCheck(data)
						// Send results to downstream channels with blocking writes to ensure no data loss
						for _, res := range results {
							for i, downCh := range r.DownStream {
								// Check if downstream channel is getting full
								chLen := len(*downCh)
								chCap := cap(*downCh)
								if chLen > chCap*3/4 {
									logger.Debug("Downstream channel getting full, but waiting to ensure no data loss",
										"ruleset", r.RulesetID,
										"channel_index", i,
										"channel_length", chLen,
										"channel_capacity", chCap)
								}

								// Blocking write to ensure data is never lost
								// If downstream is slow, we wait - data integrity is more important than speed
								*downCh <- res
							}
						}
					}

					// Handle task submission with retry mechanism to ensure no data loss
					for {
						err := r.antsPool.Submit(task)
						if err == nil {
							// Successfully submitted
							break
						}

						// If submission failed, log and retry after a short delay
						logger.Debug("Thread pool submit failed, retrying",
							"ruleset", r.RulesetID,
							"error", err,
							"pool_running", r.antsPool.Running(),
							"pool_capacity", r.antsPool.Cap())

						// Check if we should stop retrying (ruleset is shutting down)
						select {
						case <-r.stopChan:
							// Ruleset is stopping, execute synchronously to not lose the message
							logger.Info("Ruleset stopping, executing final task synchronously",
								"ruleset", r.RulesetID)
							task()
							return
						default:
							// Wait a bit and retry - prioritize data integrity over latency
							time.Sleep(10 * time.Millisecond)
						}
					}
				}
			}
		}(upID, upCh)
	}
	return nil
}

// Stop the ruleset engine, waiting for all upstream and downstream data to be processed before shutdown.
func (r *Ruleset) Stop() error {
	if r.stopChan == nil {
		return fmt.Errorf("not started")
	}

	logger.Info("Stopping ruleset", "ruleset", r.RulesetID, "upstream_count", len(r.UpStream), "downstream_count", len(r.DownStream))
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
				for i, upCh := range r.UpStream {
					chLen := len(*upCh)
					if chLen > 0 {
						allEmpty = false
						totalMessages += chLen
						if waitCount%20 == 0 { // Log every second (20 * 50ms)
							logger.Info("Upstream channel not empty", "ruleset", r.RulesetID, "channel", i, "length", chLen)
						}
					}
				}
				if allEmpty {
					logger.Info("All upstream channels empty", "ruleset", r.RulesetID)
					break waitUpstream
				}
				waitCount++
				if waitCount%20 == 0 { // Log every second (20 * 50ms)
					logger.Info("Still waiting for upstream channels", "ruleset", r.RulesetID, "total_messages", totalMessages, "wait_cycles", waitCount)
				}
				time.Sleep(50 * time.Millisecond)
			}
		}

		// Wait for all downstream channels to be consumed.
		logger.Info("Waiting for downstream channels to empty", "ruleset", r.RulesetID)
		downstreamTimeout := time.After(10 * time.Second) // 10 second timeout for downstream
		waitCount = 0

	waitDownstream:
		for {
			select {
			case <-downstreamTimeout:
				logger.Warn("Timeout waiting for downstream channels, forcing shutdown", "ruleset", r.RulesetID)
				break waitDownstream
			default:
				allEmpty := true
				totalMessages := 0
				for i, downCh := range r.DownStream {
					chLen := len(*downCh)
					if chLen > 0 {
						allEmpty = false
						totalMessages += chLen
						if waitCount%20 == 0 { // Log every second (20 * 50ms)
							logger.Info("Downstream channel not empty", "ruleset", r.RulesetID, "channel", i, "length", chLen)
						}
					}
				}
				if allEmpty {
					logger.Info("All downstream channels empty", "ruleset", r.RulesetID)
					break waitDownstream
				}
				waitCount++
				if waitCount%20 == 0 { // Log every second (20 * 50ms)
					logger.Info("Still waiting for downstream channels", "ruleset", r.RulesetID, "total_messages", totalMessages, "wait_cycles", waitCount)
				}
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

	// Wait for all tasks in thread pool to complete before releasing
	if r.antsPool != nil {
		logger.Info("Waiting for thread pool tasks to complete", "ruleset", r.RulesetID)

		// Wait for all running tasks to complete with timeout
		poolWaitTimeout := time.After(15 * time.Second)
		for {
			select {
			case <-poolWaitTimeout:
				logger.Warn("Timeout waiting for thread pool tasks, forcing release",
					"ruleset", r.RulesetID,
					"running_tasks", r.antsPool.Running())
				goto releasePool
			default:
				if r.antsPool.Running() == 0 {
					logger.Info("All thread pool tasks completed", "ruleset", r.RulesetID)
					goto releasePool
				}
				logger.Debug("Still waiting for thread pool tasks",
					"ruleset", r.RulesetID,
					"running_tasks", r.antsPool.Running())
				time.Sleep(100 * time.Millisecond)
			}
		}

	releasePool:
		r.antsPool.Release()
		r.antsPool = nil
		logger.Info("Thread pool released", "ruleset", r.RulesetID)
	}
	r.stopChan = nil

	// Wait for metric goroutine to finish
	r.wg.Wait()

	if r.Cache != nil {
		r.Cache.Close()
	}

	if r.CacheForClassify != nil {
		r.CacheForClassify.Close()
	}

	// Reset atomic counter for restart
	previousTotal := atomic.LoadUint64(&r.processTotal)
	atomic.StoreUint64(&r.processTotal, 0)
	logger.Debug("Reset atomic counter for ruleset component", "ruleset", r.RulesetID, "previous_total", previousTotal)

	// Note: ResetDiffCounter no longer needed - component manages its own increments

	return nil
}

// StopForTesting stops the ruleset quickly for testing purposes without waiting for channel drainage
func (r *Ruleset) StopForTesting() error {
	if r.stopChan == nil {
		return fmt.Errorf("not started")
	}

	logger.Info("Quick stopping test ruleset", "ruleset", r.RulesetID)
	close(r.stopChan)

	// Quick cleanup without waiting
	if r.antsPool != nil {
		r.antsPool.Release()
		r.antsPool = nil
	}
	r.stopChan = nil

	// Wait for any remaining goroutines to finish
	r.wg.Wait()

	if r.Cache != nil {
		r.Cache.Close()
	}

	if r.CacheForClassify != nil {
		r.CacheForClassify.Close()
	}

	// Reset atomic counter for testing cleanup
	previousTotal := atomic.LoadUint64(&r.processTotal)
	atomic.StoreUint64(&r.processTotal, 0)
	atomic.StoreUint64(&r.lastReportedTotal, 0)
	logger.Debug("Reset atomic counter for test ruleset component", "ruleset", r.RulesetID, "previous_total", previousTotal)

	logger.Info("Test ruleset stopped", "ruleset", r.RulesetID)
	return nil
}

// EngineCheck executes all rules in the ruleset on the provided data using the new flexible syntax.
func (r *Ruleset) EngineCheck(data map[string]interface{}) []map[string]interface{} {
	finalRes := make([]map[string]interface{}, 0)
	ruleCache := ruleCachePool.Get().(map[string]common.CheckCoreCache)
	// clean previous entries
	for k := range ruleCache {
		delete(ruleCache, k)
	}

	// For whitelist, keep track of the last modified data
	var lastModifiedData map[string]interface{}

	// For empty whitelist, data should pass through
	if !r.IsDetection && len(r.Rules) == 0 {
		// Empty whitelist means all data passes through
		ruleCachePool.Put(ruleCache)
		return []map[string]interface{}{data}
	}

	// Process each rule in the ruleset
	for _, rule := range r.Rules {
		// Create data copy for this rule execution
		dataCopy := common.MapDeepCopy(data)

		// Execute all operations in the order specified by the Queue
		ruleCheckRes := r.executeRuleOperations(&rule, dataCopy, ruleCache)

		// Handle rule result based on ruleset type
		if r.IsDetection {
			// For detection rules, if rule passes, add to results
			if ruleCheckRes {
				// Add rule info
				// Build hit rule ID efficiently
				var hitRuleIDSb strings.Builder
				hitRuleIDSb.WriteString(r.RulesetID)
				hitRuleIDSb.WriteString(".")
				hitRuleIDSb.WriteString(rule.ID)
				addHitRuleID(dataCopy, hitRuleIDSb.String())
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
	return finalRes
}

// executeRuleOperations executes all operations in a rule according to the Queue order
func (r *Ruleset) executeRuleOperations(rule *Rule, data map[string]interface{}, ruleCache map[string]common.CheckCoreCache) bool {
	if rule.Queue == nil || len(*rule.Queue) == 0 {
		// No operations to execute
		// For detection rules, empty rule means no match (false)
		// For whitelist rules, empty rule also means no match (false), allowing data to pass
		return false
	}

	// Check if the rule has any check operations (check, checklist, threshold)
	hasCheckOperations := false
	for _, op := range *rule.Queue {
		if op.Type == T_CheckList || op.Type == T_Check || op.Type == T_Threshold {
			hasCheckOperations = true
			break
		}
	}

	// If no check operations, the rule should not match
	if !hasCheckOperations {
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
	var checkNodeValue = checkNode.Value
	var checkNodeValueFromRaw = false

	switch checkNode.Logic {
	case "":
		if strings.HasPrefix(checkNode.Value, FromRawSymbol) {
			checkNodeValue = GetRuleValueFromRawFromCache(ruleCache, checkNode.Value, data)
			checkNodeValueFromRaw = true
		}
		return checkNodeLogic(checkNode, data, checkNodeValue, checkNodeValueFromRaw, ruleCache)
	case "AND":
		for _, v := range checkNode.DelimiterFieldList {
			if strings.HasPrefix(v, FromRawSymbol) {
				checkNodeValue = GetRuleValueFromRawFromCache(ruleCache, v, data)
				checkNodeValueFromRaw = true
			}
			if !checkNodeLogic(checkNode, data, v, checkNodeValueFromRaw, ruleCache) {
				return false
			}
		}
		return true
	case "OR":
		for _, v := range checkNode.DelimiterFieldList {
			if strings.HasPrefix(v, FromRawSymbol) {
				checkNodeValue = GetRuleValueFromRawFromCache(ruleCache, v, data)
				checkNodeValueFromRaw = true
			}
			if checkNodeLogic(checkNode, data, v, checkNodeValueFromRaw, ruleCache) {
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
	// Use strings.Builder for better performance
	var sb strings.Builder
	sb.WriteString(threshold.GroupByID)

	for k, v := range threshold.GroupByList {
		tmpData, _ := GetCheckDataFromCache(ruleCache, k, data, v)
		sb.WriteString(tmpData)
	}
	groupByKey := common.XXHash64(sb.String())

	var ruleCheckRes bool
	var err error

	switch threshold.CountType {
	case "":
		groupByKey = "F_" + groupByKey

		if threshold.LocalCache {
			ruleCheckRes, err = r.LocalCacheFRQSum(groupByKey, 1, threshold.RangeInt, threshold.Value)
		} else {
			ruleCheckRes, err = RedisFRQSum(groupByKey, 1, threshold.RangeInt, threshold.Value)
		}

	case "SUM":
		groupByKey = "FS_" + groupByKey

		sumDataStr, ok := GetCheckDataFromCache(ruleCache, threshold.CountField, data, threshold.CountFieldList)
		if !ok {
			return false
		}

		sumData, err := strconv.Atoi(sumDataStr)
		if err != nil {
			return false
		}

		if threshold.LocalCache {
			ruleCheckRes, err = r.LocalCacheFRQSum(groupByKey, sumData, threshold.RangeInt, threshold.Value)
		} else {
			ruleCheckRes, err = RedisFRQSum(groupByKey, sumData, threshold.RangeInt, threshold.Value)
		}

	case "CLASSIFY":
		groupByKey = "FC_" + groupByKey
		classifyData, ok := GetCheckDataFromCache(ruleCache, threshold.CountField, data, threshold.CountFieldList)
		if !ok {
			return false
		}

		// Use strings.Builder for consistency
		var tmpKeySb strings.Builder
		tmpKeySb.WriteString(groupByKey)
		tmpKeySb.WriteString("_")
		tmpKeySb.WriteString(common.XXHash64(classifyData))
		tmpKey := tmpKeySb.String()

		if threshold.LocalCache {
			ruleCheckRes, err = r.LocalCacheFRQClassify(tmpKey, groupByKey, threshold.RangeInt, threshold.Value)
		} else {
			ruleCheckRes, err = RedisFRQClassify(tmpKey, groupByKey, threshold.RangeInt, threshold.Value)
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
		if strings.HasPrefix(appendOp.Value, FromRawSymbol) {
			appendData = GetRuleValueFromRawFromCache(ruleCache, appendOp.Value, dataCopy)
		}

		dataCopy[appendOp.FieldName] = appendData
	} else {
		// Plugin
		args := GetPluginRealArgs(appendOp.PluginArgs, dataCopy, ruleCache)
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

	ok, err := pluginOp.Plugin.FuncEvalCheckNode(args...)
	if err != nil {
		logger.PluginError("Plugin evaluation error", "plugin", pluginOp.Plugin.Name, "error", err)
	}

	if !ok {
		logger.Info("Plugin check failed", "plugin", pluginOp.Plugin.Name, "ruleID", rule.ID, "rulesetID", r.RulesetID)
	}
}

// checkNodeLogic executes the check logic for a single check node.
func checkNodeLogic(checkNode *CheckNodes, data map[string]interface{}, checkNodeValue string, checkNodeValueFromRaw bool, ruleCache map[string]common.CheckCoreCache) bool {
	var checkListFlag = false

	needCheckData, exist := common.GetCheckData(data, checkNode.FieldList)

	// CRITICAL FIX: Handle field existence properly for ISNULL and NOTNULL checks
	if checkNode.Type == "ISNULL" {
		// For ISNULL: field doesn't exist OR field exists but is empty
		if !exist || strings.TrimSpace(needCheckData) == "" {
			return true
		} else {
			return false
		}
	}

	if checkNode.Type == "NOTNULL" {
		// For NOTNULL: field must exist AND not be empty
		if !exist || strings.TrimSpace(needCheckData) == "" {
			return false
		} else {
			return true
		}
	}

	// For other check types, if field doesn't exist, the check should fail
	if !exist {
		return false
	}

	switch checkNode.Type {
	case "REGEX":
		if !checkNodeValueFromRaw {
			checkListFlag, _ = REGEX(needCheckData, checkNode.Regex)
		} else {
			regex, err := regexp.Compile(checkNodeValue)
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
		if checkNode.Plugin.IsNegated {
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
	if data == nil {
		data = make(map[string]interface{})
	}

	if existingID, ok := data[HitRuleIdFieldName]; !ok {
		data[HitRuleIdFieldName] = ruleID
	} else {
		// Use strings.Builder for efficient string concatenation
		var sb strings.Builder
		sb.WriteString(existingID.(string))
		sb.WriteString(",")
		sb.WriteString(ruleID)
		data[HitRuleIdFieldName] = sb.String()
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
// This method is thread-safe and designed for 10-second statistics collection.
func (r *Ruleset) GetIncrementAndUpdate() uint64 {
	current := atomic.LoadUint64(&r.processTotal)
	last := atomic.SwapUint64(&r.lastReportedTotal, current)

	// Handle potential overflow (though practically impossible with uint64)
	if current >= last {
		return current - last
	} else {
		// Overflow case: component restarted, return current value as increment
		return current
	}
}

// GetRunningTaskCount returns the number of currently running tasks in the thread pool
// Returns 0 if the thread pool is not initialized
func (r *Ruleset) GetRunningTaskCount() int {
	if r.antsPool != nil {
		return r.antsPool.Running()
	}
	return 0
}
