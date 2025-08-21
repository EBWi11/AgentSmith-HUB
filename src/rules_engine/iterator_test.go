package rules_engine

import (
	"testing"
)

// helper to build and run a ruleset against data
func buildRulesetFromXML(t *testing.T, xml string) *Ruleset {
	t.Helper()
	rs, err := ParseRuleset([]byte(xml))
	if err != nil {
		t.Fatalf("ParseRuleset error: %v", err)
	}
	rs.RulesetID = "TEST.RS"
	if err := RulesetBuild(rs); err != nil {
		t.Fatalf("RulesetBuild error: %v", err)
	}
	rs.SetTestMode()
	return rs
}

func TestIterator_ANY_PrimitiveArray(t *testing.T) {
	xml := `
<root type="DETECTION" name="iter-any">
  <rule id="r1" name="r1">
    <iterator type="ANY" field="arr" variable="it">
      <check type="INCL" field="it" >a</check>
    </iterator>
  </rule>
 </root>`

	rs := buildRulesetFromXML(t, xml)
	data := map[string]interface{}{
		"arr": []interface{}{"x", "abc", "y"},
	}
	out := rs.EngineCheck(data)
	if len(out) != 1 {
		t.Fatalf("expected 1 match for ANY, got %d", len(out))
	}
}

func TestIterator_ALL_ObjectArray(t *testing.T) {
	xml := `
<root type="DETECTION" name="iter-all">
  <rule id="r1" name="r1">
    <iterator type="ALL" field="items" variable="it">
      <check type="NOTNULL" field="it.value" />
    </iterator>
  </rule>
 </root>`

	rs := buildRulesetFromXML(t, xml)
	data := map[string]interface{}{
		"items": []interface{}{
			map[string]interface{}{"value": "a"},
			map[string]interface{}{"value": "b"},
		},
	}
	out := rs.EngineCheck(data)
	if len(out) != 1 {
		t.Fatalf("expected 1 match for ALL, got %d", len(out))
	}
}

func TestIterator_VariableValidation(t *testing.T) {
	// invalid variable starting with reserved prefix _$
	xml := `
<root type="DETECTION" name="iter-bad-var">
  <rule id="r1" name="r1">
    <iterator type="ANY" field="arr" variable="_$bad">
      <check type="NOTNULL" field="it" />
    </iterator>
  </rule>
 </root>`

	if _, err := ParseRuleset([]byte(xml)); err == nil {
		t.Fatalf("expected ParseRuleset to fail due to invalid iterator variable name")
	}
}

func TestIterator_ChecklistInsideIterator(t *testing.T) {
	xml := `
<root type="DETECTION" name="iter-cl">
  <rule id="r1" name="r1">
    <iterator type="ANY" field="items" variable="it">
      <checklist>
        <check id="n1" type="INCL" field="it.value">x</check>
        <check id="n2" type="NOTNULL" field="it.value" />
      </checklist>
    </iterator>
  </rule>
 </root>`

	rs := buildRulesetFromXML(t, xml)
	data := map[string]interface{}{
		"items": []interface{}{
			map[string]interface{}{"value": "x"},
			map[string]interface{}{"value": "z"},
		},
	}
	out := rs.EngineCheck(data)
	if len(out) != 1 {
		t.Fatalf("expected 1 match for checklist inside iterator (ANY), got %d", len(out))
	}
}
