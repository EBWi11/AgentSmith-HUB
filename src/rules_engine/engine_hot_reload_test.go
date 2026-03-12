package rules_engine

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestRulesetReloadInPlace_UpdatesRunningInstance(t *testing.T) {
	initialXML := `<root name="test" type="DETECTION">
		<rule id="r1" name="city chicago">
			<check type="EQU" field="city">Chicago</check>
		</rule>
	</root>`

	initialTemplate, err := NewRuleset("", initialXML, "hot_reload_ruleset")
	if err != nil {
		t.Fatalf("NewRuleset initial template failed: %v", err)
	}
	t.Cleanup(func() { CleanupDetachedReloadCandidate(initialTemplate) })

	instance, err := NewFromExisting(initialTemplate, "TEST.project1.RULESET.hot_reload_ruleset")
	if err != nil {
		t.Fatalf("NewFromExisting initial instance failed: %v", err)
	}

	inputCh := make(chan map[string]interface{}, 16)
	outputCh := make(chan map[string]interface{}, 16)
	instance.UpStream = map[string]*chan map[string]interface{}{"test_in": &inputCh}
	instance.DownStream = map[string]*chan map[string]interface{}{"test_out": &outputCh}

	if err := instance.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer func() {
		if err := instance.Stop(); err != nil {
			t.Fatalf("Stop failed: %v", err)
		}
	}()

	inputCh <- map[string]interface{}{"city": "Chicago"}
	expectOutput(t, outputCh, true, "initial Chicago event should match")

	updatedXML := `<root name="test" type="DETECTION">
		<rule id="r1" name="city houston">
			<check type="EQU" field="city">Houston</check>
		</rule>
	</root>`
	updatedTemplate, err := NewRuleset("", updatedXML, "hot_reload_ruleset")
	if err != nil {
		t.Fatalf("NewRuleset updated template failed: %v", err)
	}
	t.Cleanup(func() { CleanupDetachedReloadCandidate(updatedTemplate) })

	reloadedInstance, err := NewFromExisting(updatedTemplate, instance.ProjectNodeSequence)
	if err != nil {
		t.Fatalf("NewFromExisting updated instance failed: %v", err)
	}

	if err := instance.ReloadInPlace(reloadedInstance); err != nil {
		t.Fatalf("ReloadInPlace failed: %v", err)
	}

	inputCh <- map[string]interface{}{"city": "Chicago"}
	expectOutput(t, outputCh, false, "old Chicago event should not match after reload")

	inputCh <- map[string]interface{}{"city": "Houston"}
	expectOutput(t, outputCh, true, "updated Houston event should match after reload")
}

func expectOutput(t *testing.T, ch <-chan map[string]interface{}, shouldReceive bool, message string) {
	t.Helper()

	select {
	case <-ch:
		if !shouldReceive {
			t.Fatalf("unexpected output: %s", message)
		}
	case <-time.After(500 * time.Millisecond):
		if shouldReceive {
			t.Fatalf("expected output: %s", message)
		}
	}
}

func TestRulesetConfigVersion_IsolatesRuntimeNamespaceAcrossReloads(t *testing.T) {
	initialXML := `<root name="test" type="DETECTION">
		<rule id="r1" name="city chicago">
			<sequence within="5s" group_by="ip" local_cache="true">
				<event id="e1">
					<check type="EQU" field="city">Chicago</check>
				</event>
				<condition>e1</condition>
			</sequence>
		</rule>
	</root>`
	updatedXML := `<root name="test" type="DETECTION">
		<rule id="r1" name="city houston">
			<sequence within="5s" group_by="ip" local_cache="true">
				<event id="e1">
					<check type="EQU" field="city">Houston</check>
				</event>
				<condition>e1</condition>
			</sequence>
		</rule>
	</root>`

	initialRuleset, err := NewRuleset("", initialXML, "hot_reload_ruleset")
	if err != nil {
		t.Fatalf("NewRuleset initial failed: %v", err)
	}
	t.Cleanup(func() { CleanupDetachedReloadCandidate(initialRuleset) })

	updatedRuleset, err := NewRuleset("", updatedXML, "hot_reload_ruleset")
	if err != nil {
		t.Fatalf("NewRuleset updated failed: %v", err)
	}
	t.Cleanup(func() { CleanupDetachedReloadCandidate(updatedRuleset) })

	initialNamespace := initialRuleset.ruleRuntimeNamespace("r1")
	updatedNamespace := updatedRuleset.ruleRuntimeNamespace("r1")

	if initialNamespace == updatedNamespace {
		t.Fatalf("expected different runtime namespaces across reloads, got same value %q", initialNamespace)
	}
	if !strings.HasPrefix(initialNamespace, "hot_reload_ruleset:r1:") {
		t.Fatalf("unexpected initial namespace format: %q", initialNamespace)
	}
	if !strings.HasPrefix(updatedNamespace, "hot_reload_ruleset:r1:") {
		t.Fatalf("unexpected updated namespace format: %q", updatedNamespace)
	}
}

func TestRulesetRuntimeNamespace_IsolatesProjectInstances(t *testing.T) {
	xml := `<root name="test" type="DETECTION">
		<rule id="r1" name="city chicago">
			<sequence within="5s" group_by="ip" local_cache="true">
				<event id="e1">
					<check type="EQU" field="city">Chicago</check>
				</event>
				<condition>e1</condition>
			</sequence>
		</rule>
	</root>`

	template, err := NewRuleset("", xml, "shared_ruleset")
	if err != nil {
		t.Fatalf("NewRuleset failed: %v", err)
	}
	t.Cleanup(func() { CleanupDetachedReloadCandidate(template) })

	projectA, err := NewFromExisting(template, "INPUT.projectA.RULESET.shared_ruleset")
	if err != nil {
		t.Fatalf("NewFromExisting projectA failed: %v", err)
	}
	projectB, err := NewFromExisting(template, "INPUT.projectB.RULESET.shared_ruleset")
	if err != nil {
		t.Fatalf("NewFromExisting projectB failed: %v", err)
	}

	nsA := projectA.ruleRuntimeNamespace("r1")
	nsB := projectB.ruleRuntimeNamespace("r1")
	if nsA == nsB {
		t.Fatalf("expected project instances to have different runtime namespaces, got %q", nsA)
	}
}

func TestPebbleCEPValueStoreClose_RemovesInstanceDirectory(t *testing.T) {
	store, err := NewPebbleCEPValueStore("hot_reload_ruleset")
	if err != nil {
		t.Fatalf("NewPebbleCEPValueStore failed: %v", err)
	}

	dbPath := store.dbPath
	if dbPath == "" {
		t.Fatal("expected dbPath to be populated")
	}
	if _, err := os.Stat(dbPath); err != nil {
		t.Fatalf("expected store directory to exist before close: %v", err)
	}

	if err := store.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	if _, err := os.Stat(dbPath); !os.IsNotExist(err) {
		t.Fatalf("expected store directory to be removed on close, stat err=%v", err)
	}
}
