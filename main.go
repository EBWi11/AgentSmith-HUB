package main

import (
	"AgentSmith-HUB/rules_engine"
	"fmt"
	"io/ioutil"
	"os"
)

func main() {
	xmlFile, err := os.Open("rules_engine/ruleset_demo.xml")
	if err != nil {
		panic(err)
	}
	defer xmlFile.Close()

	rawRuleset, err := ioutil.ReadAll(xmlFile)
	if err != nil {
		panic(err)
	}
	ruleset, err := rules_engine.ParseRulesetFromByte(rawRuleset)
	if err != nil {
		panic(err)
	}
	fmt.Println(ruleset.RulesetID)
}
