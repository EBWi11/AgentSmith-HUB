package rules_engine

import (
	"AgentSmith-HUB/common"
	b64 "encoding/base64"
	"encoding/json"
	"fmt"
	regexp "github.com/BurntSushi/rure-go"
	"strings"
)

// EngineCheck executes all rules in the ruleset on the provided data.
func (r *Ruleset) EngineCheck(data map[string]interface{}) {
	engineCache := make(map[string]common.CheckCoreCache)
	for _, rule := range r.Rules {
		//filter check process
		if len(rule.Filter.FieldList) > 0 {
			checkData, exist := GetCheckDataFromCache(engineCache, rule.Filter.Field, data, rule.Filter.FieldList)
			if exist {
				filterValue := rule.Filter.Value
				if strings.HasPrefix(rule.Filter.Value, FromRawSymbol) {
					filterValue = GetRuleValueFromRawFromCache(engineCache, rule.Filter.Value, data)
				}

				filterCheckRes, _ := INCL(checkData, filterValue)
				if !filterCheckRes {
					continue
				}
			}
		}

		//checklist process
		checkIndex := 0
		checkListRes := false
		for checkIndex = range rule.Checklist.CheckNodes {
			var checkListFlag = false
			var needCheckData string
			var checkNodeValue string
			var checkNodeValueFromRaw = false

			needCheckData, _ = common.GetCheckData(data, rule.Checklist.CheckNodes[checkIndex].FieldList)
			checkNodeValue = rule.Checklist.CheckNodes[checkIndex].Value

			if strings.HasPrefix(rule.Checklist.CheckNodes[checkIndex].Value, FromRawSymbol) {
				checkNodeValue = GetRuleValueFromRawFromCache(engineCache, rule.Checklist.CheckNodes[checkIndex].Value, data)
				checkNodeValueFromRaw = true
			}

			switch rule.Checklist.CheckNodes[checkIndex].Type {
			case "REGEX":
				if !checkNodeValueFromRaw {
					checkListFlag, _ = REGEX(needCheckData, rule.Checklist.CheckNodes[checkIndex].Regex)
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
				checkListFlag, _ = rule.Checklist.CheckNodes[checkIndex].CheckFunc(needCheckData, checkNodeValue)
			}

			if !checkListFlag {
				checkIndex = checkIndex - 1
				break
			}
		}

		if rule.ChecklistLen == 0 {
			checkListRes = true
		} else {
			if r.IsDetection {
				if checkIndex == rule.ChecklistLen {
					checkListRes = true
				}
			} else {
				if checkIndex != rule.ChecklistLen {
					checkListRes = true
				}
			}
		}

		if !checkListRes {
			break
		}

		//threshold process
		if rule.ThresholdCheck {
			var groupByKey = ""
			for k, v := range rule.Threshold.GroupByList {
				tmpData, _ := GetCheckDataFromCache(engineCache, k, data, v)
				groupByKey = groupByKey + tmpData
			}
			groupByKey = "FQ_" + b64.StdEncoding.EncodeToString([]byte(groupByKey))
			fmt.Println(groupByKey)
		}

		//append process
		for i := range rule.Appends {
			tmpAppend := rule.Appends[i]
			if tmpAppend.Type == "" {
				appendData := tmpAppend.Value
				if strings.HasPrefix(tmpAppend.Value, FromRawSymbol) {
					appendData = GetRuleValueFromRawFromCache(engineCache, tmpAppend.Value, data)
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
