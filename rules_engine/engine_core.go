package rules_engine

import (
	"AgentSmith-HUB/common"
	"fmt"
	regexp "github.com/BurntSushi/rure-go"
	json "github.com/bytedance/sonic"
	"strconv"
	"strings"
)

func checkNodeLogic(checkNode *CheckNodes, data map[string]interface{}, checkNodeValue string, checkNodeValueFromRaw bool) bool {
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
		//todo
	default:
		checkListFlag, _ = checkNode.CheckFunc(needCheckData, checkNodeValue)
	}

	return checkListFlag
}

// EngineCheck executes all rules in the ruleset on the provided data.
func (r *Ruleset) EngineCheck(data map[string]interface{}) {
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
				checkListRes = checkNodeLogic(&checkNode, data, checkNodeValue, checkNodeValueFromRaw)
			case "AND":
				for _, v := range checkNode.DelimiterFieldList {
					if strings.HasPrefix(v, FromRawSymbol) {
						checkNodeValue = GetRuleValueFromRawFromCache(ruleCache, v, data)
						checkNodeValueFromRaw = true
					}
					if checkListRes = checkNodeLogic(&checkNode, data, v, checkNodeValueFromRaw); !checkListRes {
						break
					}
				}
			case "OR":
				for _, v := range checkNode.DelimiterFieldList {
					if strings.HasPrefix(v, FromRawSymbol) {
						checkNodeValue = GetRuleValueFromRawFromCache(ruleCache, v, data)
						checkNodeValueFromRaw = true
					}

					if checkListRes = checkNodeLogic(&checkNode, data, v, checkNodeValueFromRaw); checkListRes {
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

		//append process
		for i := range rule.Appends {
			tmpAppend := rule.Appends[i]
			if tmpAppend.Type == "" {
				appendData := tmpAppend.Value
				if strings.HasPrefix(tmpAppend.Value, FromRawSymbol) {
					appendData = GetRuleValueFromRawFromCache(ruleCache, tmpAppend.Value, data)
				}

				data[tmpAppend.FieldName] = appendData
			} else {
				//plugin_test
			}
		}

		//del process
		for i := range rule.DelList {
			common.MapDel(data, rule.DelList[i])
		}

		dataStr, _ := json.Marshal(data)
		fmt.Println(string(dataStr))
	}
}
