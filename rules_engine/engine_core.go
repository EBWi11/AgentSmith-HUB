package rules_engine

import (
	"AgentSmith-HUB/common"
	"fmt"
)

// EngineRun executes all rules in the ruleset on the provided data.
// For each rule, it checks if the filter field exists and applies the filter logic.
func (r *Ruleset) EngineRun(data map[string]interface{}) {
	for _, rule := range r.Rules {
		// Only process if the filter field path is valid
		if len(rule.Filter.FieldList) > 0 {
			// Retrieve the value from data using the field path
			checkData, exist := common.GetCheckData(data, rule.Filter.FieldList)
			if exist {
				// Apply filter logic (example: INCL)
				fmt.Println(INCL(checkData, rule.Filter.Value))
			}
		}
	}
}

func (r *Ruleset) checkListRun() {
	// TODO: implement checklist logic
}
