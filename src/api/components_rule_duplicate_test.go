package api

import (
	"strings"
	"testing"
)

const (
	ruleOneChecklist = `
  <rule id="whitelist_ip_1" name="Whitelist IP 1">
    <checklist condition="a and b">
      <check id="a" type="EQ" field="src_ip">192.168.1.1</check>
      <check id="b" type="EQ" field="dst_port">80</check>
    </checklist>
  </rule>`
	ruleOneChecklistReordered = `
  <rule id="whitelist_ip_2" name="Whitelist IP 2">
    <checklist condition="a and b">
      <check id="b" type="EQ" field="dst_port">80</check>
      <check id="a" type="EQ" field="src_ip">192.168.1.1</check>
    </checklist>
  </rule>`
	ruleDifferentLogic = `
  <rule id="other" name="Other">
    <checklist condition="x">
      <check id="x" type="EQ" field="src_ip">10.0.0.1</check>
    </checklist>
  </rule>`
)

func TestSimpleRuleFingerprintFromXML_SameLogicDifferentOrder(t *testing.T) {
	fp1, err := simpleRuleFingerprintFromXML(ruleOneChecklist)
	if err != nil {
		t.Fatalf("fingerprint 1: %v", err)
	}
	fp2, err := simpleRuleFingerprintFromXML(ruleOneChecklistReordered)
	if err != nil {
		t.Fatalf("fingerprint 2: %v", err)
	}
	if fp1 != fp2 {
		t.Errorf("same logic different node order should have same fingerprint:\n  fp1=%q\n  fp2=%q", fp1, fp2)
	}
}

func TestSimpleRuleFingerprintFromXML_DifferentLogic(t *testing.T) {
	fp1, err := simpleRuleFingerprintFromXML(ruleOneChecklist)
	if err != nil {
		t.Fatalf("fingerprint 1: %v", err)
	}
	fp2, err := simpleRuleFingerprintFromXML(ruleDifferentLogic)
	if err != nil {
		t.Fatalf("fingerprint 2: %v", err)
	}
	if fp1 == fp2 {
		t.Errorf("different logic should have different fingerprint:\n  fp1=%q\n  fp2=%q", fp1, fp2)
	}
}

func TestSimpleRuleFingerprintFromXML_InvalidReturnsError(t *testing.T) {
	_, err := simpleRuleFingerprintFromXML("<rule id=\"x\">")
	if err == nil {
		t.Error("invalid XML should return error")
	}
	_, err = simpleRuleFingerprintFromXML("<root></root>")
	if err == nil {
		t.Error("no rule in root should return error")
	}
}

func TestPendingRuleDuplicate_SameLogicDifferentOrder(t *testing.T) {
	rulesetXML := strings.TrimSpace(`
<root type="DETECTION" name="test">
` + ruleOneChecklist + `
</root>`)
	newRuleRaw := strings.TrimSpace(ruleOneChecklistReordered)
	if !pendingRuleDuplicate(rulesetXML, newRuleRaw) {
		t.Error("pendingRuleDuplicate should be true when new rule has same logic as existing (different node order)")
	}
}

func TestPendingRuleDuplicate_DifferentLogic(t *testing.T) {
	rulesetXML := strings.TrimSpace(`
<root type="DETECTION" name="test">
` + ruleOneChecklist + `
</root>`)
	newRuleRaw := strings.TrimSpace(ruleDifferentLogic)
	if pendingRuleDuplicate(rulesetXML, newRuleRaw) {
		t.Error("pendingRuleDuplicate should be false when new rule has different logic")
	}
}

func TestPendingRuleDuplicate_EmptyCurrent(t *testing.T) {
	rulesetXML := `<root type="DETECTION" name="test"></root>`
	newRuleRaw := strings.TrimSpace(ruleOneChecklist)
	if pendingRuleDuplicate(rulesetXML, newRuleRaw) {
		t.Error("pendingRuleDuplicate should be false when current has no rules")
	}
}

func TestPendingRuleDuplicate_InvalidNewRule(t *testing.T) {
	rulesetXML := strings.TrimSpace(`
<root type="DETECTION" name="test">
` + ruleOneChecklist + `
</root>`)
	if pendingRuleDuplicate(rulesetXML, "not valid xml") {
		t.Error("pendingRuleDuplicate should be false when new rule XML is invalid")
	}
}
