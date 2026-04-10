package rules_engine

// Load rulesets and event batches from ./testdata (rulesets/*.xml, events/*.json, events/*.jsonl).
// Use from tests/benchmarks to mirror production: build ruleset once, then pump events through EngineCheck.

import (
	"bufio"
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func reportPipelineEVTPerSec(b *testing.B, eventsPerIteration int) {
	b.Helper()
	sec := b.Elapsed().Seconds()
	if sec <= 0 {
		return
	}
	b.ReportMetric(float64(b.N*eventsPerIteration)/sec, "evt/s")
}

func testdataJoin(elem ...string) string {
	return filepath.Join(append([]string{"testdata"}, elem...)...)
}

// loadBuiltRulesetFromTestdata reads XML under testdata/, runs ParseRuleset + RulesetBuild + SetTestMode,
// registers cleanup, and returns a ready ruleset (same core path as Hub after loading raw XML).
func loadBuiltRulesetFromTestdata(tb testing.TB, relPath string) *Ruleset {
	tb.Helper()
	raw, err := os.ReadFile(testdataJoin(relPath))
	if err != nil {
		tb.Fatalf("read ruleset %s: %v", relPath, err)
	}
	rs, err := ParseRuleset(raw)
	if err != nil {
		tb.Fatalf("ParseRuleset %s: %v", relPath, err)
	}
	base := strings.TrimSuffix(filepath.Base(relPath), filepath.Ext(relPath))
	rs.RulesetID = "TESTDATA." + strings.ToUpper(base)
	if err := RulesetBuild(rs); err != nil {
		tb.Fatalf("RulesetBuild %s: %v", relPath, err)
	}
	rs.SetTestMode()
	tb.Cleanup(func() { rs.cleanup() })
	return rs
}

type eventsEnvelope struct {
	Events []map[string]interface{} `json:"events"`
}

// loadEventsFromTestdataJSON loads testdata/events/*.json with top-level "events" array.
func loadEventsFromTestdataJSON(tb testing.TB, relPath string) []map[string]interface{} {
	tb.Helper()
	raw, err := os.ReadFile(testdataJoin(relPath))
	if err != nil {
		tb.Fatalf("read events %s: %v", relPath, err)
	}
	var env eventsEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		tb.Fatalf("json events %s: %v", relPath, err)
	}
	if len(env.Events) == 0 {
		tb.Fatalf("no events in %s", relPath)
	}
	return env.Events
}

// loadEventsFromTestdataJSONL loads one JSON object per non-empty line.
func loadEventsFromTestdataJSONL(tb testing.TB, relPath string) []map[string]interface{} {
	tb.Helper()
	f, err := os.Open(testdataJoin(relPath))
	if err != nil {
		tb.Fatalf("open jsonl %s: %v", relPath, err)
	}
	defer f.Close()
	var out []map[string]interface{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		var m map[string]interface{}
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			tb.Fatalf("jsonl line in %s: %v", relPath, err)
		}
		out = append(out, m)
	}
	if err := sc.Err(); err != nil {
		tb.Fatalf("read jsonl %s: %v", relPath, err)
	}
	if len(out) == 0 {
		tb.Fatalf("no events in %s", relPath)
	}
	return out
}

// cloneEventForReplay copies a payload so repeated pipeline runs do not alias nested maps/slices
// mutated or reused by the engine (shallow map + meta + tags — enough for testdata fixtures).
func cloneEventForReplay(m map[string]interface{}) map[string]interface{} {
	out := maps.Clone(m)
	if meta, ok := m["meta"].(map[string]interface{}); ok {
		out["meta"] = maps.Clone(meta)
	}
	if tags, ok := m["tags"].([]interface{}); ok {
		cp := make([]interface{}, len(tags))
		copy(cp, tags)
		out["tags"] = cp
	}
	return out
}

// runRulesEnginePipeline calls EngineCheck for each event in order (minimal "pipeline" without Hub).
func runRulesEnginePipeline(rs *Ruleset, events []map[string]interface{}) {
	for i := range events {
		_ = rs.EngineCheck(cloneEventForReplay(events[i]))
	}
}

func TestRulesEngine_Pipeline_TestdataSmoke(t *testing.T) {
	rs := loadBuiltRulesetFromTestdata(t, "rulesets/tps_composite.xml")
	events := loadEventsFromTestdataJSON(t, "events/tps_composite_events.json")
	runRulesEnginePipeline(rs, events)

	rsCEP := loadBuiltRulesetFromTestdata(t, "rulesets/cep_login_access.xml")
	cepEvents := loadEventsFromTestdataJSONL(t, "events/cep_login_access.jsonl")
	runRulesEnginePipeline(rsCEP, cepEvents)
}
