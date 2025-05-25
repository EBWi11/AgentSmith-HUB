package rules_engine

import (
	"AgentSmith-HUB/common"
	"fmt"
	regexp "github.com/BurntSushi/rure-go"
	"strconv"
	"strings"
)

const HitRuleIdFieldName = "_HUB_HIT_RULE_ID"

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
		res := checkNode.Plugin.FuncEval(args)
		if res[0].(bool) {
			return true
		} else {
			return false
		}

	default:
		checkListFlag, _ = checkNode.CheckFunc(needCheckData, checkNodeValue)
	}

	return checkListFlag
}

// EngineCheck executes all rules in the ruleset on the provided data.
func (r *Ruleset) EngineCheck(data map[string]interface{}) []map[string]interface{} {
	finalRes := make([]map[string]interface{}, 0)

	ruleCache := make(map[string]common.CheckCoreCache, 10)
	for _, rule := range r.Rules {
		//filter check process
		if len(rule.Filter.FieldList) > 0 {
			checkData, exist := GetCheckDataFromCache(ruleCache, rule.Filter.Field, data, rule.Filter.FieldList)
			if exist {
				filterValue := rule.Filter.Value
				if strings.HasPrefix(rule.Filter.Value, FromRawSymbol) {
					filterValue = GetRuleValueFromRawFromCache(ruleCache, rule.Filter.Value, data)
				}

				filterCheckRes, _ := INCL(checkData, filterValue)
				if !filterCheckRes {
					continue
				}
			}
		}

		//checklist process
		checkListRes := false
		ruleCheckRes := false
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
				rule.Checklist.ConditionMap[checkNode.ID] = checkListRes
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

		//threshold process
		if rule.ThresholdCheck {
			ruleCheckRes = false

			var groupByKey = ""
			for k, v := range rule.Threshold.GroupByList {
				tmpData, _ := GetCheckDataFromCache(ruleCache, k, data, v)
				groupByKey = groupByKey + tmpData
			}
			groupByKey = common.XXHash64(groupByKey)

			switch rule.Threshold.CountType {
			case "":
				groupByKey = "F_" + groupByKey

				redisSetNXRes, err := common.RedisSetNX(groupByKey, 1, rule.Threshold.RangeInt)
				if err != nil {
					//todo
				}
				if !redisSetNXRes {
					groupByValue, err := common.RedisIncr(groupByKey)
					if err != nil {
						//todo
					} else {
						if groupByValue > int64(rule.Threshold.Value) {
							ruleCheckRes = true
							_ = common.RedisDel(groupByKey)
						}
					}
				}
			case "SUM":
				groupByKey = "FS_" + groupByKey

				sumDataStr, ok := GetCheckDataFromCache(ruleCache, rule.Threshold.CountField, data, rule.Threshold.CountFieldList)
				if !ok {
					break
				}
				sumData, err := strconv.Atoi(sumDataStr)
				if err != nil {
					//todo
					break
				}

				redisSetNXRes, err := common.RedisSetNX(groupByKey, sumData, rule.Threshold.RangeInt)
				if err != nil {
					//todo
					break
				}

				if !redisSetNXRes {
					groupByValue, err := common.RedisIncrby(groupByKey, int64(sumData))
					if err != nil {
						//todo
						break
					} else {
						if groupByValue > int64(rule.Threshold.Value) {
							ruleCheckRes = true
							_ = common.RedisDel(groupByKey)
						}
					}
				}

			case "CLASSIFY":
				groupByKey = "FC_" + groupByKey
				classifyData, ok := GetCheckDataFromCache(ruleCache, rule.Threshold.CountField, data, rule.Threshold.CountFieldList)
				if !ok {
					break
				}

				tmpKey := groupByKey + "_" + common.XXHash64(classifyData)
				_, err := common.RedisSet(tmpKey, 1, rule.Threshold.RangeInt)
				if err != nil {
					//todo
					break
				}

				tmpRes, err := common.RedisKeys(groupByKey + "*")
				if err != nil {
					//todo
					break
				}

				if len(tmpRes) > rule.Threshold.Value {
					ruleCheckRes = true
					for i := range tmpRes {
						_ = common.RedisDel(tmpRes[i])
					}
				}
			}
		}

		if !ruleCheckRes {
			continue
		}

		dataCopy := common.MapDeepCopyAction(data).(map[string]interface{})

		//add rule info
		addHitRuleID(dataCopy, r.RulesetID+"."+rule.ID)

		//append process
		for i := range rule.Appends {
			tmpAppend := rule.Appends[i]
			if tmpAppend.Type == "" {
				appendData := tmpAppend.Value
				if strings.HasPrefix(tmpAppend.Value, FromRawSymbol) {
					appendData = GetRuleValueFromRawFromCache(ruleCache, tmpAppend.Value, dataCopy)
				}

				dataCopy[tmpAppend.FieldName] = appendData
			} else {
				//plugin
				args := GetPluginRealArgs(tmpAppend.PluginArgs, dataCopy, ruleCache)
				res := tmpAppend.Plugin.FuncEval(args)[0]
				dataCopy[tmpAppend.FieldName] = res
			}
		}

		//plugin process
		for i := range rule.Plugins {
			p := rule.Plugins[i]
			args := GetPluginRealArgs(p.PluginArgs, dataCopy, ruleCache)

			resList := p.Plugin.FuncEval(args)
			pluginRes := resList[0]
			err := resList[1]

			if err == nil {
				dataCopy = pluginRes.(map[string]interface{})
			} else {
				//todo
			}
		}

		//del process
		for i := range rule.DelList {
			common.MapDel(dataCopy, rule.DelList[i])
		}

		// add to final result
		finalRes = append(finalRes, dataCopy)
	}
	return finalRes
}

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
