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

	// Load today's accumulated message count from Redis to resume counting from correct value
	// This ensures that component restarts don't reset daily message count to 0
	if common.GlobalDailyStatsManager != nil {
		today := time.Now().Format("2006-01-02")
		dailyStats := common.GlobalDailyStatsManager.GetDailyStats(today, "", "")

		for _, statsData := range dailyStats {
			// Find matching component data
			if statsData.ComponentID == r.RulesetID &&
				statsData.ComponentType == "ruleset" &&
				statsData.ProjectNodeSequence == r.ProjectNodeSequence {
				// Set the starting count to today's accumulated total
				atomic.StoreUint64(&r.processTotal, statsData.TotalMessages)
				logger.Info("Loaded historical message count for ruleset",
					"ruleset", r.RulesetID,
					"historical_total", statsData.TotalMessages,
					"date", today)
				break
			}
		}
	}

	// Start metric collection goroutine
	r.metricStop = make(chan struct{})
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		r.metricLoop()
	}()

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
							// Optimization: only increment total count, QPS is calculated by metricLoop
							atomic.AddUint64(&r.processTotal, 1)

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
						// Send results to downstream channels
						for _, res := range results {
							for _, downCh := range r.DownStream {
								*downCh <- res
							}
						}
					}
					_ = r.antsPool.Submit(task)
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

	// Stop metrics collection
	if r.metricStop != nil {
		close(r.metricStop)
		r.metricStop = nil
	}

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

	if r.antsPool != nil {
		r.antsPool.Release()
		r.antsPool = nil
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

	return nil
}

// StopForTesting stops the ruleset quickly for testing purposes without waiting for channel drainage
func (r *Ruleset) StopForTesting() error {
	if r.stopChan == nil {
		return fmt.Errorf("not started")
	}

	logger.Info("Quick stopping test ruleset", "ruleset", r.RulesetID)
	close(r.stopChan)

	// Stop metrics collection quickly
	if r.metricStop != nil {
		close(r.metricStop)
		r.metricStop = nil
	}

	// Quick cleanup without waiting
	if r.antsPool != nil {
		r.antsPool.Release()
		r.antsPool = nil
	}
	r.stopChan = nil

	// Wait for metric goroutine to finish quickly
	r.wg.Wait()

	if r.Cache != nil {
		r.Cache.Close()
	}

	if r.CacheForClassify != nil {
		r.CacheForClassify.Close()
	}

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

	needCheckData, _ := common.GetCheckData(data, checkNode.FieldList)

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

// metricLoop calculates QPS and can be extended for more metrics.
func (r *Ruleset) metricLoop() {
	var lastTotal uint64
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-r.metricStop:
			return
		case <-ticker.C:
			cur := atomic.LoadUint64(&r.processTotal)

			var qps uint64
			// Safe handling: if current value is less than last value, set QPS to 0
			if cur < lastTotal {
				logger.Warn("Counter decreased, possibly due to overflow or restart",
					"ruleset", r.RulesetID,
					"lastTotal", lastTotal,
					"currentTotal", cur)
				qps = 0         // Set QPS to 0 to avoid underflow
				lastTotal = cur // Reset lastTotal to current value
			} else {
				qps = cur - lastTotal
				lastTotal = cur
			}

			atomic.StoreUint64(&r.processQPS, qps)

			// Note: Redis persistence is now handled by QPS Manager via daily_stats system
			// This eliminates duplicate data writes and TTL conflicts
		}
	}
}

// GetProcessQPS returns the latest processing QPS.
func (r *Ruleset) GetProcessQPS() uint64 {
	return atomic.LoadUint64(&r.processQPS)
}

// GetProcessTotal returns the total processed message count.
func (r *Ruleset) GetProcessTotal() uint64 {
	return atomic.LoadUint64(&r.processTotal)
}
