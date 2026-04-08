package rules_engine

import (
	"fmt"
	"testing"
	"time"
)

func buildStressRuleset(t *testing.T, xmlContent string) *Ruleset {
	t.Helper()
	ruleset, err := ParseRuleset([]byte(xmlContent))
	if err != nil {
		t.Fatalf("ParseRuleset failed: %v", err)
	}
	ruleset.RulesetID = fmt.Sprintf("stress-%d", time.Now().UnixNano())
	ruleset.IsDetection = true
	if err := RulesetBuild(ruleset); err != nil {
		t.Fatalf("RulesetBuild failed: %v", err)
	}
	return ruleset
}
