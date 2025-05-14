package rules_engine

import "AgentSmith-HUB/common"

func (r *Ruleset) EngineInit() {

}

func

func (r *Ruleset) EngineRun(data map[string]interface{}) {
	for _, rule := range r.Rules {
		if rule.Filter.Field != "" {
			checkData,exist := common.GetCheckData(data, rule.Filter.Field)
			if exist {

			}
		}
		return true
	}
}

func (r *Ruleset) checkListRun() {

}
