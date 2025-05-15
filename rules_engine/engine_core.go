package rules_engine

import (
	"AgentSmith-HUB/common"
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

			if "REGEX" == rule.Checklist.CheckNodes[checkIndex].Type {
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
			} else {
				checkListFlag, _ = rule.Checklist.CheckNodes[checkIndex].CheckFunc(needCheckData, checkNodeValue)
			}

			if !checkListFlag {
				checkIndex = checkIndex - 1
				break
			}
		}

		if rule.ChecklistLen == 0 {
			fmt.Println("BINGO", rule.ID)
		} else {
			if r.IsDetection {
				if checkIndex == rule.ChecklistLen {
					fmt.Println("BINGO", rule.ID)
				}
			} else {
				if checkIndex != rule.ChecklistLen {
					fmt.Println("BINGO", rule.ID)
				}
			}
		}
	}
}

func (r *Ruleset) checkListRun() {
	// TODO: implement checklist logic
}
