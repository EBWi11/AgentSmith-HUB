# CEP Sequence Review Checklist

This document provides a systematic checklist for reviewing sequence functionality end-to-end, ensuring consistent and thorough code reviews.

## Review Process

1. **Before Review**: Run all sequence-related tests
2. **During Review**: Go through each section systematically
3. **After Review**: Update "Known Issues" section with findings
4. **Before Commit**: Verify fixes against checklist

---

## 1. Parser Layer (`engine_parser.go`, `cep_condition.go`)

### XML Parsing
- [ ] Sequence attributes parsed correctly (`within`, `group_by`, `local_cache`)
- [ ] Event definitions parsed with all attributes (`id`, `event_time`, `group_by`)
- [ ] Nested `<append>` within `<event>` parsed correctly
- [ ] Condition expression parsed correctly
- [ ] Error handling for invalid XML structure

### Condition Expression Parsing
- [ ] Tokenizer handles all operators (`->`, `or`, `and`, `not`, `!`, `()`)
- [ ] Precedence correct: `-> < or < and < ! < ()`
- [ ] Stage flattening extracts ordered stages correctly
- [ ] Absence detection (`!` at stage level) identified correctly
- [ ] Event ID validation (alphanumeric, underscore, hyphen)
- [ ] Nested `->` operator rejected (validation)

### Field Path Parsing
- [ ] `group_by` field paths parsed (comma-separated, nested paths)
- [ ] `event_time` field path parsed
- [ ] Per-event `group_by` vs sequence-level `group_by` handled

---

## 2. Execution Layer (`engine_core.go`)

### Event Matching
- [ ] `evaluateEventDef` checks all conditions (AND logic)
- [ ] Checklists evaluated correctly
- [ ] Thresholds evaluated correctly
- [ ] Dynamic field references (`_$`, `_$#event_id`, `_@path`) resolved

### Stage Evaluation
- [ ] `EvaluateEvent` returns correct stage indices
- [ ] `evaluateStageExpr` handles OR/AND/NOT correctly
- [ ] `EvaluateEventBindings` returns correct bound event IDs
- [ ] OR branch binding: **left-first** policy applied consistently
- [ ] AND branch binding: all matched IDs merged correctly
- [ ] NOT/absence: binding semantics correct (empty binding when satisfied)

### OR Branch Consistency (Critical)
- [ ] `_sequence_events` output uses bound branch only
- [ ] `_@` context writes (`applySequenceEventAppends`) use bound branch only
- [ ] `event_time` extraction (`extractEventTimestamp`) uses bound branch only
- [ ] `group_by` extraction (`extractCorrelateValuesForStateLookup`) uses bound branch
- [ ] All four paths use same `selectedMatchedEventIDs` derived from `stageBindings`

### Timestamp Handling
- [ ] `extractEventTimestamp` uses bound event's `event_time` field
- [ ] Fallback to processing time when `event_time` missing/invalid
- [ ] `parseTimestampToNs` handles: Unix s/ms/µs/ns, float seconds, ISO 8601, RFC 3339
- [ ] Temporal ordering: `CheckComplete` enforces strictly increasing timestamps
- [ ] Out-of-order arrival handled correctly (sorted by timestamp, not arrival order)

### Group By Correlation
- [ ] Sequence-level `group_by` used when present
- [ ] Per-event `group_by` preferred from **matched event definitions** (not first in EventOrder)
- [ ] Multi-source sequences: different field names (`src_ip` vs `client_ip`) produce same key for same logical entity
- [ ] Empty correlation key: warning logged, event dropped
- [ ] Field path resolution: nested paths (`a.b.c`) handled correctly

### State Key Derivation
- [ ] `BuildStateKey` uses `GroupByID` (ruleset:rule) + correlate values
- [ ] `extractCorrelateValuesForStateLookup` called **before** state load (for `_@`-dependent checks)
- [ ] First-pass `matchedEventIDs` used for correlation (even if empty, before `_@` injection)
- [ ] Fallback to `EventOrder` first event only when no matches found

---

## 3. State Management (`cep_state.go`)

### State Lifecycle
- [ ] `GetOrCreateState`: atomic get-or-create (local cache: direct, Redis: Lua script)
- [ ] `UpdateState`: memory control applied (`trimStageMatches`)
- [ ] `DeleteState`: cleanup complete (main key + absence tracking key)
- [ ] TTL handling: expiration times set correctly

### Concurrency Control
- [ ] Per-key locking: `LockKey` / `UnlockKey` via `sync.Map`
- [ ] Lock held for entire `executeSequence` (read-modify-write cycle)
- [ ] `CleanupKeyLock`: intentionally no-op (to avoid lock replacement races)
- [ ] No deadlocks: lock order consistent

### Memory Control
- [ ] `trimStageMatches`: keeps latest `MaxStageMatchesPerStage` (100) matches
- [ ] Warning logged when partial matches exceed `MaxPartialMatchesWarning` (10000)
- [ ] State serialization: zstd compression for Redis mode

### Local Cache (Ristretto)
- [ ] Cache initialized with correct config
- [ ] `setLocalState`: TTL set correctly
- [ ] `getLocalState`: cache hit/miss handled
- [ ] `deleteLocalState`: cache entry removed

### Redis Mode
- [ ] `getRedisState`: decompression handles both compressed and legacy plain JSON
- [ ] `setRedisState`: zstd compression applied
- [ ] Lua scripts: `luaGetOrCreate`, `luaAtomicUpdate` atomicity verified
- [ ] Fallback on Lua failure: non-atomic get-then-set (known limitation)

---

## 4. Storage Layer (`cep_value_store.go`)

### Pebble Value Store
- [ ] `PutSnapshot`: async write via channel, batch commit
- [ ] `GetSnapshot`: expiry check after read (may return expired data briefly)
- [ ] `DeleteSnapshot`: cleanup complete
- [ ] Batch size: `pebbleWriteBatchSize` (128) respected
- [ ] Queue size: `pebbleWriteQueueSize` (4096) - blocking when full
- [ ] Cleanup loop: runs every `pebbleCleanupTick` (30s), processes up to `pebbleCleanupLimit` (5000)

### Pebble Configuration
- [ ] `DisableWAL: true` - acceptable for ephemeral cache (known trade-off)
- [ ] `DisableAutomaticCompactions: true` - acceptable for high-throughput ephemeral use
- [ ] Memory thresholds: `MemTableSize`, `MemTableStopWritesThreshold` tuned
- [ ] DB path uniqueness: includes PID + timestamp + instance counter

### Known Limitations
- [ ] `PutSnapshot` blocks on full queue (no timeout) - may slow sequence execution
- [ ] Process crash before `commitPutBatch` loses queued writes (no WAL)
- [ ] DB under temp dir: may be cleared by OS

---

## 5. Context (`_@`) System (`engine_core.go`, `engine_utils.go`)

### Context Writes
- [ ] `<append field="_@path.to.key">` writes to `state.Context`
- [ ] Path parsing: `common.StringToList` splits by `.`
- [ ] Nested map creation: `setSequenceContextValue` creates intermediate maps
- [ ] Only bound event's appends executed (OR branch consistency)
- [ ] Plugin appends: `FuncEvalCheckNode` / `FuncEvalOther` handled

### Context Reads
- [ ] `_@path` normalized to `_$#ctx.path` internally
- [ ] `seqEvalDataCopy["#ctx"] = state.Context` injected before re-evaluation
- [ ] `ruleCache` cleared for `_@` / `_#ctx` keys before re-evaluation
- [ ] Missing key: check returns false (does not match)
- [ ] Nested paths: `_@file.nested.key` resolved correctly

### Context Scope
- [ ] Per correlation key: each `group_by` value has isolated context
- [ ] Lifecycle: context cleared when sequence completes or expires
- [ ] Cross-event references: context persists across stages within same sequence

---

## 6. Output Layer (`engine_core.go`)

### Result Enrichment
- [ ] `enrichSequenceResultData`: adds `_sequence_events` and `_sequence_condition`
- [ ] `_sequence_events`: map[event_id] -> first match data for each stage
- [ ] `#event_id` fields: internal cross-event references (removed by `sanitizeOutputData`)
- [ ] OR stages: only bound branch event IDs appear in `_sequence_events`
- [ ] Absence stages: not included in `_sequence_events`

### Output Sanitization
- [ ] `sanitizeOutputData`: removes keys starting with `#`
- [ ] `_sequence_events` and `_sequence_condition` preserved
- [ ] Deep copy: `MapDeepCopy` prevents downstream mutations

### Absence Results
- [ ] `buildAbsenceResult`: uses last non-absence stage match as base data
- [ ] Post-sequence ops (append/modify/del/plugin) executed
- [ ] Pre-sequence checks: **not** re-evaluated (by design)

---

## 7. Absence Detection (`cep_condition.go`, `cep_state.go`, `engine_core.go`)

### Absence Logic
- [ ] `CheckComplete`: absence stage satisfied when no match with `timestamp > prevTimestamp`
- [ ] `CheckAbsenceTimeout`: at expiry, absence stages treated as satisfied
- [ ] Absence observed: sequence NOT complete (match recorded, `CheckComplete` returns false)

### Absence Scanner
- [ ] `absenceScannerLoop`: runs every 1 second
- [ ] Local cache: timing wheel (3600 slots, 1 slot/sec) for efficient scanning
- [ ] Redis mode: scans `SEQABS_*` keys for distributed detection
- [ ] Expired keys: triggers absence completion

### Absence Tracking
- [ ] `TrackAbsenceKey`: stores metadata (ExpiresAt, RuleID, SeqID)
- [ ] Local cache: timing wheel + `absenceKeys` map
- [ ] Redis: `SEQABS_*` keys with TTL
- [ ] `UntrackAbsenceKey`: cleanup complete

---

## 8. API Layer (`api/testing.go`, `web/src/api/index.js`)

### Backend Test API
- [ ] `testRuleset`: supports both `data` (object/array) and `datas` (array)
- [ ] Event order preserved: sequential send with 10ms delay
- [ ] Result collection: timeout (30s), ticker (100ms), task count check
- [ ] Channel setup: `UpStream["test"]` configured correctly
- [ ] Cleanup: `defer tempRuleset.Stop()` ensures resource release

### Frontend API
- [ ] `hubApi.testRuleset`: sends `{ data }` only (object or array)
- [ ] `hubApi.testRulesetContent`: sends `{ content, data }`
- [ ] `datas` field: **not exposed** in frontend (backend supports it)
- [ ] Error handling: HTTP errors and network errors handled

### Project Test API
- [ ] `testProject`: single event only (`req.Data` object)
- [ ] `ProcessTestData`: injects one event, no batch support
- [ ] **Cannot test sequences** via project test API (by design limitation)

---

## 9. Edge Cases & Boundary Scenarios

### Multi-Source Sequences
- [ ] Different `group_by` field names (`src_ip` vs `client_ip`) produce same key
- [ ] First-pass `matchedEventIDs` empty: fallback to `EventOrder` first event
- [ ] Second-pass `_@` injection: state loaded before re-evaluation

### OR Branch Overlapping
- [ ] Single event matches multiple OR branches: left-first binding
- [ ] `_sequence_events` shows only bound branch
- [ ] `_@` writes from only bound branch
- [ ] `event_time` from bound branch
- [ ] `group_by` from bound branch

### Timestamp Edge Cases
- [ ] Missing `event_time`: fallback to processing time
- [ ] Invalid `event_time`: fallback to processing time
- [ ] Mixed `event_time` fields: uses bound event's field
- [ ] Out-of-order arrival: sorted by timestamp, not arrival order

### State Key Edge Cases
- [ ] Empty correlation key: warning logged, event dropped
- [ ] Missing `group_by` fields: empty key, event dropped
- [ ] Nested field paths: resolved correctly

### Absence Edge Cases
- [ ] Absence observed before timeout: sequence NOT complete
- [ ] Absence timeout: sequence completes with absence result
- [ ] Multiple absence stages: all must be satisfied
- [ ] Absence + presence stages: ordering enforced

---

## 10. Test Coverage

### Unit Tests
- [ ] `cep_condition_test.go`: parser, evaluator, completion check
- [ ] `cep_state_test.go`: state manager, absence tracking
- [ ] `cep_context_syntax_test.go`: `_@` syntax parsing and usage

### Integration Tests
- [ ] `cep_integration_test.go`: full pipeline (parse → build → execute)
- [ ] OR branch binding tests
- [ ] Multi-source `group_by` tests
- [ ] `_@` context write/read tests
- [ ] Timestamp ordering tests

### Stress Tests
- [ ] `cep_stress_test.go`: 10-minute stress test with various scenarios

### Missing Test Coverage
- [ ] Redis mode integration tests (only local cache tested)
- [ ] `datas` API field tests
- [ ] Absence timeout via API (end-to-end)
- [ ] Pebble snapshot storage/retrieval tests
- [ ] Redis Lua failure fallback tests
- [ ] Mixed `event_time` fields tests
- [ ] Pre-sequence checks + absence tests

---

## Known Issues & Status

### High Priority

#### H1. `event_time` Selection in Multi-Event OR Stages
- **Status**: Fixed (2026-02-12)
- **Location**: `engine_core.go:extractEventTimestamp()`
- **Issue**: Uses first successfully parsed timestamp among matched events, not necessarily from bound branch
- **Impact**: For `(login or exfil1) -> exfil2` where both match, timestamp may come from wrong event definition
- **Fix**: Already uses `selectedMatchedEventIDs` (bound branch) - verified in code review

#### H2. Multi-Source `group_by` Fallback When First-Pass Empty
- **Status**: Improved (2026-02-12)
- **Location**: `engine_core.go:extractCorrelateValuesForStateLookup()`
- **Issue**: When first-pass `matchedEventIDs` is empty (before `_@` injection), fallback uses `EventOrder` first event, which may have different field names
- **Impact**: Multi-source sequences with `_@`-dependent first stage may mis-correlate
- **Fix**: Added `extractCorrelateValuesForStateLookupWithSequenceFallback()` that tries all event `group_by` fields when per-event extraction fails. This improves fallback behavior, though complete solution may require field name mapping for true multi-source support.

### Medium Priority

#### M1. Frontend `datas` Field Not Exposed
- **Status**: Design Decision
- **Location**: `web/src/api/index.js`
- **Issue**: Backend supports `datas` but frontend only sends `data` (array support exists via `data: [e1, e2, e3]`)
- **Impact**: Users can test sequences via `data` array, but `datas` is undocumented
- **Fix**: Document `data` array support or expose `datas` in UI

#### M2. Pebble `PutSnapshot` Blocks on Full Queue
- **Status**: Known Limitation
- **Location**: `cep_value_store.go:PutSnapshot()`
- **Issue**: Synchronous wait on `req.resp` channel, no timeout
- **Impact**: Sequence execution slows when Pebble write queue is full
- **Fix**: Add timeout or fallback to inline snapshot when queue full

#### M3. Redis Lua Fallback Non-Atomic
- **Status**: Known Limitation
- **Location**: `cep_state.go:getOrCreateRedisStateAtomic()`
- **Issue**: Fallback to get-then-set without CAS when Lua script fails
- **Impact**: Possible lost updates under Redis failures
- **Fix**: Use Redis transaction or accept limitation

#### M4. `buildAbsenceResult` Uses Last Non-Absence Stage Only
- **Status**: By Design
- **Location**: `engine_core.go:buildAbsenceResult()`
- **Issue**: Only last non-absence stage match used as base data
- **Impact**: Earlier stages not included in absence result
- **Fix**: Document behavior or include all non-absence stages

### Low Priority

#### L1. `parseTimestampToNs` Treats Small Numbers as Seconds
- **Status**: By Design (with caveat)
- **Location**: `engine_core.go:parseTimestampToNs()`
- **Issue**: Numbers < 1e9 treated as seconds (e.g., `"1000"` = 1000 seconds, not 1 second)
- **Impact**: Users must use explicit format (e.g., `"1000000"` for milliseconds)
- **Fix**: Document expected format or improve heuristic

#### L2. `_sequence_events` Uses First Match Per Stage
- **Status**: By Design
- **Location**: `engine_core.go:enrichSequenceResultData()`
- **Issue**: Only first match per stage included in output
- **Impact**: Limited observability for high-frequency repeated matches
- **Fix**: Consider `_sequence_events_all` option or document limitation

#### L3. Pebble DB Under Temp Dir
- **Status**: By Design
- **Location**: `cep_value_store.go:NewPebbleCEPValueStore()`
- **Issue**: DB created under `os.TempDir()`, may be cleared by OS
- **Impact**: Ephemeral data loss acceptable for cache use case
- **Fix**: Document ephemeral nature or add configurable path

---

## Review History

| Date | Reviewer | Focus Area | Key Findings |
|------|----------|------------|--------------|
| 2026-02-12 | AI Assistant | OR Branch Binding | Found `_@` writes and `event_time` not using bound branch |
| 2026-02-12 | AI Assistant | Multi-Source `group_by` | Fixed state lookup to prefer matched event's `group_by` |
| 2026-02-12 | AI Assistant | End-to-End Review | Identified 10+ issues across parser/execution/state/storage/API layers |

---

## Next Steps

1. **Fix H1**: Align `event_time` extraction with OR branch binding
2. **Fix H2**: Improve multi-source `group_by` fallback for `_@`-dependent first stage
3. **Add Tests**: Redis mode, `datas` API, absence timeout E2E
4. **Document**: Known limitations and design decisions
5. **Monitor**: Pebble queue depth, Redis Lua failures, state key collisions

---

## Notes

- This checklist should be updated after each major review or fix
- Known issues should be tracked until resolved or accepted as design decisions
- Test coverage gaps should be prioritized based on risk and user impact
