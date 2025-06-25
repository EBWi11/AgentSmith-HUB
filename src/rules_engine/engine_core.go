package rules_engine

import (
	"AgentSmith-HUB/common"
	"AgentSmith-HUB/logger"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	regexp "github.com/BurntSushi/rure-go"
	"github.com/panjf2000/ants/v2"
)

const HitRuleIdFieldName = "_hub_hit_rule_id"

// Start the ruleset engine, consuming data from upstream and writing checked data to downstream.
func (r *Ruleset) Start() error {
	if r.stopChan != nil {
		return fmt.Errorf("already started: %v", r.RulesetID)
	}
	r.stopChan = make(chan struct{})

	// Start metric collection goroutine
	r.metricStop = make(chan struct{})
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		r.metricLoop()
	}()

	var err error
	r.antsPool, err = ants.NewPool(MinPoolSize)
	if err != nil {
		return fmt.Errorf("failed to create ants pool: %v", err)
	}

	// Auto-scaling goroutine
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-r.stopChan:
				return
			case <-ticker.C:
				totalBacklog := 0
				for _, upCh := range r.UpStream {
					totalBacklog += len(*upCh)
				}
				targetSize := MinPoolSize
				switch {
				case totalBacklog > 1024:
					targetSize = MaxPoolSize
				case totalBacklog > 256:
					targetSize = 128
				case totalBacklog > 64:
					targetSize = 96
				case totalBacklog > 16:
					targetSize = 64
				default:
					targetSize = MinPoolSize
				}
				if r.antsPool != nil {
					r.antsPool.Tune(targetSize)
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
						// Optimization: only increment total count, QPS is calculated by metricLoop
						atomic.AddUint64(&r.processTotal, 1)

						// IMPORTANT: Sample the input data BEFORE rule checking starts
						// This ensures we capture the raw data entering the ruleset for analysis
						if r.sampler != nil {
							success := r.sampler.Sample(data, "rule_input", r.ProjectNodeSequence)
							logger.Info("Ruleset input sampler call",
								"rulesetID", r.RulesetID,
								"projectNodeSequence", r.ProjectNodeSequence,
								"success", success)
						}

						// Now perform rule checking on the input data
						results := r.EngineCheck(data)
						logger.Info("Ruleset processing",
							"rulesetID", r.RulesetID,
							"projectNodeSequence", r.ProjectNodeSequence,
							"resultsCount", len(results),
							"samplerExists", r.sampler != nil)

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

func (r *Ruleset) HotUpdate(raw string, id string) (*Ruleset, error) {
	newR, err := NewRuleset("", raw, id)
	if err != nil {
		return nil, errors.New("new ruleset parse error: " + err.Error())
	}

	err = r.Stop()
	if err != nil {
		return nil, errors.New("Hot update stop ruleset error: " + err.Error())
	}

	// init ruleset
	r.Rules = make([]Rule, 0)
	r.RulesByFilter = make(map[string]*RulesByFilter)
	r.RawConfig = ""

	for i := range r.DownStream {
		newR.DownStream[i] = r.DownStream[i]
	}

	for i := range r.UpStream {
		newR.UpStream[i] = r.UpStream[i]
	}

	err = newR.Start()
	if err != nil {
		return newR, errors.New("Hot update stop ruleset error: " + err.Error())
	}
	return newR, nil
}

// EngineCheck executes all rules in the ruleset on the provided data.
func (r *Ruleset) EngineCheck(data map[string]interface{}) []map[string]interface{} {
	finalRes := make([]map[string]interface{}, 0)
	ruleCache := make(map[string]common.CheckCoreCache, 8)

	for _, rf := range r.RulesByFilter {
		// Filter check process
		if rf.Filter.Check {
			checkData, exist := GetCheckDataFromCache(ruleCache, rf.Filter.Field, data, rf.Filter.FieldList)
			if exist {
				filterValue := rf.Filter.Value
				if strings.HasPrefix(rf.Filter.Value, FromRawSymbol) {
					filterValue = GetRuleValueFromRawFromCache(ruleCache, rf.Filter.Value, data)
				}

				filterCheckRes, _ := INCL(checkData, filterValue)
				if !filterCheckRes {
					continue
				}
			}
		}

		for i := range rf.Rules {
			rule := rf.Rules[i]

			// Checklist process
			checkListRes := false
			ruleCheckRes := false

			var conditionMap map[string]bool

			if rule.Checklist.ConditionFlag {
				conditionMap = make(map[string]bool, len(rule.Checklist.CheckNodes))
			}

			for _, checkNode := range rule.Checklist.CheckNodes {
				var checkNodeValue = checkNode.Value
				var checkNodeValueFromRaw = false

				switch checkNode.Logic {
				case "":
					if strings.HasPrefix(checkNode.Value, FromRawSymbol) {
						checkNodeValue = GetRuleValueFromRawFromCache(ruleCache, checkNode.Value, data)
						checkNodeValueFromRaw = true
					}
					checkListRes = checkNodeLogic(&checkNode, data, checkNodeValue, checkNodeValueFromRaw, ruleCache)
				case "AND":
					for _, v := range checkNode.DelimiterFieldList {
						if strings.HasPrefix(v, FromRawSymbol) {
							checkNodeValue = GetRuleValueFromRawFromCache(ruleCache, v, data)
							checkNodeValueFromRaw = true
						}
						if checkListRes = checkNodeLogic(&checkNode, data, v, checkNodeValueFromRaw, ruleCache); !checkListRes {
							break
						}
					}
				case "OR":
					for _, v := range checkNode.DelimiterFieldList {
						if strings.HasPrefix(v, FromRawSymbol) {
							checkNodeValue = GetRuleValueFromRawFromCache(ruleCache, v, data)
							checkNodeValueFromRaw = true
						}

						if checkListRes = checkNodeLogic(&checkNode, data, v, checkNodeValueFromRaw, ruleCache); checkListRes {
							break
						}
					}
				}

				if rule.Checklist.ConditionFlag {
					conditionMap[checkNode.ID] = checkListRes
				} else {
					if !checkListRes {
						break
					}
				}
			}

			if rule.ChecklistLen == 0 {
				ruleCheckRes = true
			}

			if rule.Checklist.ConditionFlag {
				ruleCheckRes = rule.Checklist.ConditionAST.ExprASTResult(rule.Checklist.ConditionAST.ExprAST, rule.Checklist.ConditionMap)
			} else {
				if r.IsDetection && checkListRes {
					ruleCheckRes = true
				} else if !r.IsDetection && !checkListRes {
					ruleCheckRes = true
				}
			}

			if !ruleCheckRes {
				continue
			}

			// Threshold process
			if rule.ThresholdCheck {
				var err error
				ruleCheckRes = false

				// Isolate by ruleset ID and rule ID
				var groupByKey = rule.Threshold.GroupByID
				for k, v := range rule.Threshold.GroupByList {
					tmpData, _ := GetCheckDataFromCache(ruleCache, k, data, v)
					groupByKey = groupByKey + tmpData
				}
				groupByKey = common.XXHash64(groupByKey)

				switch rule.Threshold.CountType {
				case "":
					groupByKey = "F_" + groupByKey

					if rule.Threshold.LocalCache {
						ruleCheckRes, err = r.LocalCacheFRQSum(groupByKey, 1, rule.Threshold.RangeInt, rule.Threshold.Value)
					} else {
						ruleCheckRes, err = RedisFRQSum(groupByKey, 1, rule.Threshold.RangeInt, rule.Threshold.Value)
					}

				case "SUM":
					groupByKey = "FS_" + groupByKey

					sumDataStr, ok := GetCheckDataFromCache(ruleCache, rule.Threshold.CountField, data, rule.Threshold.CountFieldList)
					if !ok {
						break
					}

					sumData, err := strconv.Atoi(sumDataStr)
					if err != nil {
						break
					}

					if rule.Threshold.LocalCache {
						ruleCheckRes, err = r.LocalCacheFRQSum(groupByKey, sumData, rule.Threshold.RangeInt, rule.Threshold.Value)
					} else {
						ruleCheckRes, err = RedisFRQSum(groupByKey, sumData, rule.Threshold.RangeInt, rule.Threshold.Value)
					}

				case "CLASSIFY":
					groupByKey = "FC_" + groupByKey
					classifyData, ok := GetCheckDataFromCache(ruleCache, rule.Threshold.CountField, data, rule.Threshold.CountFieldList)
					if !ok {
						break
					}

					tmpKey := groupByKey + "_" + common.XXHash64(classifyData)

					if rule.Threshold.LocalCache {
						ruleCheckRes, err = r.LocalCacheFRQClassify(tmpKey, groupByKey, rule.Threshold.RangeInt, rule.Threshold.Value)
					} else {
						ruleCheckRes, err = RedisFRQClassify(tmpKey, groupByKey, rule.Threshold.RangeInt, rule.Threshold.Value)
					}
				}

				if err != nil {
					logger.Error("Threshold check error:", err, "GroupByKey:", groupByKey, "RuleID:", rule.ID, "RuleSetID:", r.RulesetID)
				}
			}

			if !ruleCheckRes {
				continue
			}

			dataCopy := common.MapDeepCopy(data)

			// Add rule info
			addHitRuleID(dataCopy, r.RulesetID+"."+rule.ID)

			// Append process
			for i := range rule.Appends {
				tmpAppend := rule.Appends[i]
				if tmpAppend.Type == "" {
					appendData := tmpAppend.Value
					if strings.HasPrefix(tmpAppend.Value, FromRawSymbol) {
						appendData = GetRuleValueFromRawFromCache(ruleCache, tmpAppend.Value, dataCopy)
					}

					dataCopy[tmpAppend.FieldName] = appendData
				} else {
					// Plugin
					args := GetPluginRealArgs(tmpAppend.PluginArgs, dataCopy, ruleCache)
					res, ok, err := tmpAppend.Plugin.FuncEvalOther(args...)
					if err == nil && ok {
						if tmpAppend.FieldName == PluginArgFromRawSymbol {
							if r, ok := res.(map[string]interface{}); ok {
								res = common.MapDeepCopy(r)
							} else {
								logger.Error("Plugin result is not a map", "plugin", tmpAppend.Plugin.Name, "result", res)
								res = nil
							}
						}

						dataCopy[tmpAppend.FieldName] = res
					}
				}
			}

			// Plugin process
			for i := range rule.Plugins {
				p := rule.Plugins[i]
				args := GetPluginRealArgs(p.PluginArgs, dataCopy, ruleCache)

				ok, err := p.Plugin.FuncEvalCheckNode(args...)
				if err != nil {
					logger.Error("Plugin evaluation error", "plugin", p.Plugin.Name, "error", err)
				}

				if !ok {
					logger.Info("Plugin check failed", "plugin", p.Plugin.Name, "ruleID", rule.ID, "rulesetID", r.RulesetID)
				}
			}

			// Delete process
			for i := range rule.DelList {
				common.MapDel(dataCopy, rule.DelList[i])
			}

			// Add to final result
			finalRes = append(finalRes, dataCopy)
		}
	}

	ruleCache = nil
	return finalRes
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
				fmt.Println("REGEX compile error", err)
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

	if _, ok := data[HitRuleIdFieldName]; !ok {
		data[HitRuleIdFieldName] = ruleID
	} else {
		data[HitRuleIdFieldName] = data[HitRuleIdFieldName].(string) + "," + ruleID
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
