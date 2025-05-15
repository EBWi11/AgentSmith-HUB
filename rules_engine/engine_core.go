package rules_engine

import (
	"AgentSmith-HUB/common"
	"fmt"
)

// EngineCheck executes all rules in the ruleset on the provided data.
func (r *Ruleset) EngineCheck(data map[string]interface{}) {
	engineCache := make(map[string]common.CheckCoreCache)
	for _, rule := range r.Rules {
		if len(rule.Filter.FieldList) > 0 {
			//filter check process
			checkData, exist := common.GetCheckDataFromCache(engineCache, rule.Filter.Field, data, rule.Filter.FieldList)
			if exist {
				filterCheckRes, _ := INCL(checkData, rule.Filter.Value)
				if !filterCheckRes {
					continue
				}
			}

			//checklist process
			i := 0
			for i = range rule.Checklist.CheckNodes {
				checkListFlag := false
				d, _ := common.GetCheckData(data, rule.Checklist.CheckNodes[i].FieldList)

				if "REGEX" == rule.Checklist.CheckNodes[i].Type {
					checkListFlag, _ = REGEX(d, rule.Checklist.CheckNodes[i].Regex)
				} else {
					checkListFlag, _ = rule.Checklist.CheckNodes[i].CheckFunc(d, rule.Checklist.CheckNodes[i].Value)
				}

				if (r.IsDetection && !checkListFlag) || (!r.IsDetection && checkListFlag) {
					break
				}
			}

			if i == rule.ChecklistLen {
				fmt.Println("BINGO!!!", rule.ID)
			}
		}

	}
}

func (r *Ruleset) checkListRun() {
	// TODO: implement checklist logic
}
