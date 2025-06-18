package rules_engine

import (
	"AgentSmith-HUB/common"
	"AgentSmith-HUB/logger"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	regexp "github.com/BurntSushi/rure-go"
	"github.com/panjf2000/ants/v2"
)

const HitRuleIdFieldName = "_HUB_HIT_RULE_ID"

// Start the ruleset engine, consuming data from upstream and writing checked data to downstream.
func (r *Ruleset) Start() error {
	if r.stopChan != nil {
		return fmt.Errorf("already started: %v", r.RulesetID)
	}
	r.stopChan = make(chan struct{})

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
						results := r.EngineCheck(data)
						for _, res := range results {
							// Sample the result with source data
							sampleData := map[string]interface{}{
								"source": data,
								"result": res,
							}
							if r.sampler != nil {
								r.sampler.Sample(sampleData, "rule_check", r.ProjectNodeSequence)
							}

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
	close(r.stopChan)

	// Wait for all upstream channels to be consumed.
waitUpstream:
	for {
		allEmpty := true
		for _, upCh := range r.UpStream {
			if len(*upCh) > 0 {
				allEmpty = false
				break
			}
		}
		if allEmpty {
			break waitUpstream
		}
		time.Sleep(50 * time.Millisecond)
	}

	// Wait for all downstream channels to be consumed.
waitDownstream:
	for {
		allEmpty := true
		for _, downCh := range r.DownStream {
			if len(*downCh) > 0 {
				allEmpty = false
				break
			}
		}
		if allEmpty {
			break waitDownstream
		}
		time.Sleep(50 * time.Millisecond)
	}

	if r.antsPool != nil {
		r.antsPool.Release()
		r.antsPool = nil
	}
	r.stopChan = nil

	if r.Cache != nil {
		r.Cache.Close()
	}

	if r.CacheForClassify != nil {
		r.CacheForClassify.Close()
	}

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
					res, ok := tmpAppend.Plugin.FuncEvalOther(args...)
					if ok {
						dataCopy[tmpAppend.FieldName] = res
					}
				}
			}

			// Plugin process
			for i := range rule.Plugins {
				p := rule.Plugins[i]
				args := GetPluginRealArgs(p.PluginArgs, dataCopy, ruleCache)

				res, ok := p.Plugin.FuncEvalOther(args...)
				if ok {
					if dataCopy, ok = res.(map[string]interface{}); !ok {
						logger.Error("Plugin execution error: expected map[string]interface{}, got", fmt.Sprintf("%T", res), "Plugin:", p.Plugin.Name, "RuleID:", rule.ID, "RuleSetID:", r.RulesetID)
					}
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
		return checkNode.Plugin.FuncEvalCheckNode(args...)

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
