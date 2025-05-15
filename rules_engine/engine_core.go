package rules_engine

import (
	"AgentSmith-HUB/common"
	"encoding/json"
	"fmt"
	regexp "github.com/BurntSushi/rure-go"
	"strconv"
	"strings"
)

// EngineCheck executes all rules in the ruleset on the provided data.
func (r *Ruleset) EngineCheck(data map[string]interface{}) {
	ruleCache := make(map[string]common.CheckCoreCache)
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
		checkIndex := 0
		ruleCheckRes := false
		for checkIndex, checkNode := range rule.Checklist.CheckNodes {
			var checkListFlag = false
			var needCheckData string
			var checkNodeValue string
			var checkNodeValueFromRaw = false

			needCheckData, _ = common.GetCheckData(data, checkNode.FieldList)
			checkNodeValue = checkNode.Value

			if strings.HasPrefix(checkNode.Value, FromRawSymbol) {
				checkNodeValue = GetRuleValueFromRawFromCache(ruleCache, checkNode.Value, data)
				checkNodeValueFromRaw = true
			}

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

			if !checkListFlag {
				checkIndex = checkIndex - 1
				break
			}
		}

		if rule.ChecklistLen == 0 {
			ruleCheckRes = true
		} else {
			if r.IsDetection {
				if checkIndex == rule.ChecklistLen {
					ruleCheckRes = true
				}
			} else {
				if checkIndex != rule.ChecklistLen {
					ruleCheckRes = true
				}
			}
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

				ruleCheckRes = false
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
				//plugin
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
