# CEP Sequence Detection - Syntax Design Document

## 1. Overview

Complex Event Processing (CEP) enables detection of patterns and relationships across multiple events over time. Unlike single-event matching (which the existing rules engine handles), CEP detects **ordered sequences** of events correlated by shared fields within a time window.

### Typical Use Cases

| Scenario | Pattern | Description |
|----------|---------|-------------|
| Brute force then lateral movement | `brute -> login_success -> rdp` | Multiple failed logins, then success, then RDP session |
| Login without MFA | `login -> !mfa` | Login occurred but MFA verification never followed |
| APT attack chain | `recon -> exploit -> persist -> exfil` | Full kill chain detection |
| Account takeover | `login -> password_change` | Login from new location followed by credential change |
| Data exfiltration | `access -> (upload or email)` | Sensitive data access followed by external transfer |

### Design Principles

1. **Integrated into existing rule syntax** -- CEP is a new operation type (`<sequence>`) within `<rule>`, not a separate component
2. **Reuse existing check syntax** -- Event matching inside `<sequence>` uses the same `<check>`, `<checklist>`, and `<threshold>` elements
3. **Minimal new concepts** -- Only `<sequence>`, `<event>`, `<condition>`, and the `->` operator are new
4. **Dual state backend** -- Redis (distributed) or local cache (single-node), consistent with threshold

---

## 2. Syntax Structure

### 2.1 Complete Structure

```xml
<rule id="rule_id" name="Rule Name">
    <!-- Optional: pre-filter checks (applied to every incoming event) -->
    <check type="..." field="...">value</check>

    <!-- Sequence detection -->
    <sequence within="TIME_WINDOW" group_by="FIELD" local_cache="true|false">
        <event id="EVENT_ID" event_time="FIELD" group_by="FIELD">
            <!-- Event matching criteria: check / checklist / threshold -->
        </event>

        <event id="EVENT_ID" event_time="FIELD" group_by="FIELD">
            <!-- Event matching criteria -->
        </event>

        <!-- ... more events ... -->

        <condition>EXPRESSION</condition>
    </sequence>

    <!-- Optional: post-match operations (only execute when sequence completes) -->
    <append field="...">value</append>
    <plugin>func(args)</plugin>
</rule>
```

### 2.2 Element Reference

#### `<sequence>` Element

Container for the entire CEP definition. Placed inside `<rule>`, same level as `<check>`, `<checklist>`, etc.

| Attribute | Required | Description |
|-----------|----------|-------------|
| `within` | Yes | Time window for the entire sequence. Events must complete within this duration. Supports: `30s`, `5m`, `1h`, `1d` |
| `group_by` | No | Default correlation field(s) for all events. Comma-separated for multiple fields. Can be overridden per-event |
| `local_cache` | No | `true` to use local cache, `false` (default) to use Redis for distributed state |

#### `<event>` Element

Defines the matching criteria for a single event in the sequence. Must be inside `<sequence>`.

| Attribute | Required | Description |
|-----------|----------|-------------|
| `id` | Yes | Unique identifier within this sequence. Referenced in `<condition>` expression |
| `event_time` | No | Field name containing the event's timestamp. If omitted, detection time (processing time) is used |
| `group_by` | No | Correlation field(s) for this specific event. Overrides the sequence-level `group_by`. Used for multi-source scenarios where different event types have different field names for the same entity |

**Allowed child elements inside `<event>`:**

- `<check>` -- Single check operation (all existing check types supported)
- `<checklist>` -- Complex condition combination with `condition` expression
- `<threshold>` -- Frequency/aggregation detection

An event matches when **all** its child elements pass (AND logic), consistent with existing rule execution semantics.

#### `<condition>` Element

Defines the temporal pattern using a dedicated expression language. Must be the **last child** of `<sequence>`, after all `<event>` definitions.

The expression references event IDs and uses operators to describe temporal relationships.

---

## 3. Condition Expression Language

### 3.1 Operators

| Operator | Name | Meaning | Example |
|----------|------|---------|---------|
| `->` | Sequence | Event A must occur before event B | `a -> b` |
| `and` | And | Both conditions on the **same event** | `a and b` |
| `or` | Or | Either condition on the **same event** | `a or b` |
| `!` | Absence/Not | Event must **NOT** occur within time window | `a -> !b` |
| `()` | Grouping | Group sub-expressions | `a -> (b or c)` |

### 3.2 Operator Precedence (highest to lowest)

1. `()` -- Grouping
2. `!` -- Absence/Not
3. `and` -- Logical AND (same event)
4. `or` -- Logical OR (same event)
5. `->` -- Sequence (lowest precedence, across events)

### 3.3 Semantic Rules

**`->` (Sequence Operator)**

Separates the expression into **stages**. Each stage is evaluated against individual events. Stages must match in chronological order.

```
a -> b -> c
```
Means: an event matching `a`, followed by an event matching `b`, followed by an event matching `c`.

**`and` / `or` (Within a Stage)**

Within a single stage, `and` and `or` operate on the **same event**, exactly like existing checklist conditions.

```
(a and b) -> (c or d)
```
Means: one event where both `a` AND `b` match, followed by another event where either `c` OR `d` matches.

**`!` (Absence Operator)**

Used after `->` to express that an event should NOT appear.

```
a -> !b
```
Means: event `a` occurs, then event `b` does NOT occur within the time window. The sequence triggers when the time window expires without `b` being observed.

**Complex Absence:**

```
a -> !b -> c
```
Means: event `a` occurs, event `b` does NOT occur, then event `c` occurs. This is a three-stage pattern: presence, absence, presence.

---

## 4. Correlation (group_by)

### 4.1 Single Data Source

When all events come from the same data source and share the same field names, use `group_by` on `<sequence>`:

```xml
<sequence within="10m" group_by="source_ip">
    <event id="a">...</event>
    <event id="b">...</event>
    <condition>a -> b</condition>
</sequence>
```

All events are correlated by `source_ip` -- only events with the **same** `source_ip` value are matched together.

### 4.2 Multi Data Source

When events come from different sources with different field names for the same entity, use `group_by` on each `<event>`:

```xml
<sequence within="10m">
    <event id="fw_block" group_by="src_ip">
        <check type="EQU" field="action">block</check>
    </event>

    <event id="login" group_by="client_ip">
        <check type="EQU" field="event_type">login</check>
    </event>

    <condition>fw_block -> login</condition>
</sequence>
```

The engine correlates by **positional mapping**: `fw_block.src_ip` corresponds to `login.client_ip`. Events match when these field values are equal.

### 4.3 Mixed Mode

Use sequence-level `group_by` as default, with per-event overrides:

```xml
<sequence within="10m" group_by="source_ip">
    <event id="a">...</event>                       <!-- uses default: source_ip -->
    <event id="b" group_by="client_ip">...</event>  <!-- overrides: client_ip -->
    <condition>a -> b</condition>
</sequence>
```

### 4.4 Multi-Field Correlation

Comma-separated fields, matched positionally:

```xml
<event id="a" group_by="src_ip,src_port">
<event id="b" group_by="client_ip,client_port">
<!-- Correlation: a.src_ip == b.client_ip AND a.src_port == b.client_port -->
```

---

## 5. Event Time Ordering

### 5.1 event_time Attribute

By default, the engine uses **detection time** (when the event is processed) to determine event ordering. In scenarios where events may arrive out of order, specify the `event_time` attribute to use the event's own timestamp:

```xml
<event id="login" event_time="timestamp">
    <check type="EQU" field="event_type">login</check>
</event>
```

The engine will read the value of the `timestamp` field from the event data and use it for sequence ordering.

### 5.2 Rules

- If `event_time` is specified, the engine parses the field value as a timestamp
- If `event_time` is omitted, the current processing time is used
- Different events within the same sequence can use different `event_time` fields (or mix specified/omitted)
- Supported timestamp formats: Unix epoch (seconds/milliseconds), ISO 8601, RFC 3339

---

## 6. Complete Examples

### 6.1 Basic Sequence: Login Then Data Exfiltration

```xml
<rule id="login_then_exfil" name="Post-login data exfiltration">
    <sequence within="10m" group_by="source_ip">
        <event id="login" event_time="timestamp">
            <check type="EQU" field="event_type">login</check>
            <check type="EQU" field="result">success</check>
        </event>

        <event id="exfil" event_time="timestamp">
            <check type="EQU" field="event_type">file_transfer</check>
            <check type="MT" field="file_size">10485760</check>
            <check type="INCL" field="direction">outbound</check>
        </event>

        <condition>login -> exfil</condition>
    </sequence>

    <append field="alert_type">post_login_data_exfiltration</append>
    <append field="severity">high</append>
</rule>
```

### 6.2 Multi-Step Attack Chain

```xml
<rule id="attack_chain" name="Full attack chain detection">
    <sequence within="1h" group_by="source_ip">
        <event id="recon" event_time="timestamp">
            <check type="EQU" field="event_type">port_scan</check>
        </event>

        <event id="exploit" event_time="timestamp">
            <check type="EQU" field="event_type">exploit_attempt</check>
            <check type="EQU" field="result">success</check>
        </event>

        <event id="persist" event_time="timestamp">
            <check type="INCL" field="event_type">persistence</check>
        </event>

        <event id="exfil" event_time="timestamp">
            <check type="EQU" field="event_type">data_transfer</check>
            <check type="INCL" field="direction">outbound</check>
        </event>

        <condition>recon -> exploit -> persist -> exfil</condition>
    </sequence>

    <append field="alert_type">apt_attack_chain</append>
    <append field="severity">critical</append>
</rule>
```

### 6.3 Absence Detection: Login Without MFA

```xml
<rule id="login_no_mfa" name="Login without MFA verification">
    <sequence within="2m" group_by="user_id">
        <event id="login">
            <check type="EQU" field="event_type">login</check>
            <check type="EQU" field="result">success</check>
        </event>

        <event id="mfa">
            <check type="EQU" field="event_type">mfa_verify</check>
        </event>

        <condition>login -> !mfa</condition>
    </sequence>

    <append field="alert_type">missing_mfa</append>
    <append field="severity">medium</append>
</rule>
```

Triggers when: a successful login occurs and no MFA verification follows within 2 minutes.

### 6.4 Branch Sequence: Login Then Suspicious Activity

```xml
<rule id="login_suspicious" name="Post-login suspicious activity">
    <sequence within="15m" group_by="user_id">
        <event id="login">
            <check type="EQU" field="event_type">login</check>
        </event>

        <event id="priv_esc">
            <check type="EQU" field="event_type">privilege_escalation</check>
        </event>

        <event id="data_access">
            <check type="EQU" field="event_type">sensitive_data_access</check>
        </event>

        <condition>login -> (priv_esc or data_access)</condition>
    </sequence>

    <append field="alert_type">suspicious_post_login</append>
</rule>
```

Triggers when: login is followed by either privilege escalation OR sensitive data access.

### 6.5 Complex Conditions Within Events (Using Checklist)

```xml
<rule id="targeted_brute_lateral" name="Targeted brute force then lateral movement">
    <sequence within="30m" group_by="source_ip" local_cache="true">
        <event id="brute" event_time="timestamp">
            <checklist condition="a and b">
                <check id="a" type="EQU" field="event_type">login</check>
                <check id="b" type="EQU" field="result">failure</check>
            </checklist>
            <threshold group_by="source_ip" range="5m">10</threshold>
        </event>

        <event id="success" event_time="timestamp">
            <check type="EQU" field="event_type">login</check>
            <check type="EQU" field="result">success</check>
        </event>

        <event id="lateral" event_time="timestamp">
            <checklist condition="x and (y or z)">
                <check id="x" type="NOTNULL" field="dest_ip"></check>
                <check id="y" type="EQU" field="event_type">rdp_session</check>
                <check id="z" type="EQU" field="event_type">ssh_session</check>
            </checklist>
        </event>

        <condition>brute -> success -> lateral</condition>
    </sequence>

    <append field="alert_type">brute_force_lateral_movement</append>
    <append field="severity">critical</append>
</rule>
```

### 6.6 Multi-Source Correlation

```xml
<rule id="multi_source_attack" name="Firewall block then auth bypass">
    <sequence within="5m">
        <event id="fw_block" group_by="src_ip" event_time="fw_timestamp">
            <check type="EQU" field="action">block</check>
            <check type="EQU" field="rule_category">exploit</check>
        </event>

        <event id="auth_success" group_by="client_ip" event_time="auth_time">
            <check type="EQU" field="event_type">authentication</check>
            <check type="EQU" field="result">success</check>
        </event>

        <condition>fw_block -> auth_success</condition>
    </sequence>

    <append field="alert_type">firewall_bypass</append>
    <append field="severity">critical</append>
</rule>
```

Correlates across different data sources: firewall logs (`src_ip`) and authentication logs (`client_ip`).

### 6.7 Combined with Pre-filter and Plugin

```xml
<rule id="filtered_sequence" name="Internal network attack chain">
    <!-- Pre-filter: only process events from internal network -->
    <check type="START" field="source_ip">10.0.</check>

    <sequence within="20m" group_by="source_ip">
        <event id="scan">
            <check type="EQU" field="event_type">port_scan</check>
        </event>

        <event id="exploit">
            <check type="EQU" field="event_type">exploit</check>
        </event>

        <condition>scan -> exploit</condition>
    </sequence>

    <!-- Post-match: enrich and alert -->
    <append field="alert_type">internal_attack</append>
    <plugin>sendAlert('telegram', _$source_ip, 'Internal attack detected')</plugin>
</rule>
```

---

## 7. Execution Semantics

### 7.1 Rule Execution Flow

```
For each incoming event:

1. Execute operations BEFORE <sequence> in the rule queue
   - If any pre-filter check fails, skip this rule entirely

2. Execute <sequence>:
   a. Try to match the event against each <event> definition
   b. If matched, check correlation (group_by) against existing partial matches
   c. If the match completes the sequence -> return TRUE
   d. If the match advances a partial sequence -> store state, return FALSE
   e. If no match -> return FALSE

3. If <sequence> returns TRUE:
   - Execute operations AFTER <sequence> (append, plugin, etc.)
   - The rule "hits"

4. If <sequence> returns FALSE:
   - Skip remaining operations, rule does not hit
```

### 7.2 Partial Match State

When an event matches a stage in the sequence but doesn't complete it, a **partial match** is stored:

- **Key**: `SEQ_{rulesetID}_{ruleID}_{correlate_value}`
- **Value**: List of completed stage IDs with their timestamps and stored field values
- **TTL**: Equal to the `within` time window (auto-expires)

### 7.3 Absence Detection Execution

For `!` (absence) events:

1. When the preceding stage matches, record the partial match with an "awaiting absence timeout" flag
2. A background scanner periodically checks partial matches with absence stages
3. When the time window expires and the absent event was NOT observed, the sequence triggers
4. If the absent event IS observed before timeout, the partial match is discarded

### 7.4 Data Context on Match

When a sequence completes, the **current event** (the one that triggered completion) provides the primary data context for subsequent operations (append, plugin, etc.).

Fields from earlier events in the sequence can be accessed using the `_$#event_id.field` syntax:

```xml
<append field="initial_login_ip">_$#login.source_ip</append>
<append field="exfil_dest">_$dest_ip</append>  <!-- current event field -->
```

- `_$#login.source_ip` -- Field `source_ip` from the event that matched stage `login`
- `_$dest_ip` -- Field `dest_ip` from the current (final) event (standard syntax)

### 7.5 Sequence Context (`_@`)

CEP supports a sequence-scoped context map for cross-event state transfer.

- Read with `_@path.to.key` (only inside `<sequence>`)
- Write with `<append field="_@path.to.key">...</append>` inside an `<event>`
- Scope and lifecycle are identical to the current sequence state (same correlation key)

Example (download -> move -> exec):

```xml
<sequence within="10m" group_by="agent_id" local_cache="true">
    <event id="download">
        <check type="EQU" field="event_type">download</check>
        <append field="_@file.current">_$output_file</append>
    </event>

    <event id="mv">
        <check type="EQU" field="event_type">rename</check>
        <check type="EQU" field="src">_@file.current</check>
        <append field="_@file.current">_$dst</append>
    </event>

    <event id="exec">
        <check type="EQU" field="event_type">exec</check>
        <check type="EQU" field="file_path">_@file.current</check>
    </event>

    <condition>download -> mv -> exec</condition>
</sequence>
```

---

## 8. State Management

### 8.1 Redis Mode

Partial matches are stored in Redis, enabling distributed sequence detection across cluster nodes.

- **Storage**: Redis Hash or String with JSON-encoded partial match state
- **TTL**: Automatically set to the `within` duration
- **Concurrency**: Atomic operations (SETNX, HSET) ensure consistency
- **Key format**: `SEQ_{rulesetID}_{ruleID}_{correlate_hash}`

### 8.2 Local Cache Mode (Recommended)

When `local_cache="true"`, CEP uses a split storage model:

- **Key/state in memory**: sequence indexes and stage progression are kept in local Ristretto cache
- **Value snapshots on disk**: matched event snapshots are stored in local Pebble KV
- **Higher throughput**: no network RTT on hot-path key/state operations
- **Lower memory pressure**: large event payloads are offloaded to disk, loaded back only on sequence completion
- **Suitable for**: high-frequency rules on single-node deployments, or clusters with sticky routing / key affinity

> Recommendation: prefer `local_cache="true"` for CEP-heavy rulesets unless you explicitly need cross-node shared sequence state.

### 8.3 State Cleanup

- Completed sequences: State deleted immediately after match
- Expired sequences: TTL-based automatic cleanup (Redis TTL or Ristretto TTL)
- Discarded sequences (absence event observed): State deleted immediately

---

## 9. Syntax Quick Reference

### Elements

| Element | Parent | Description |
|---------|--------|-------------|
| `<sequence>` | `<rule>` | CEP sequence detection container |
| `<event>` | `<sequence>` | Single event definition with matching criteria |
| `<condition>` | `<sequence>` | Temporal pattern expression (must be last child) |

### Attributes

| Element | Attribute | Required | Default | Description |
|---------|-----------|----------|---------|-------------|
| `<sequence>` | `within` | Yes | - | Time window (`30s`, `5m`, `1h`, `1d`) |
| `<sequence>` | `group_by` | No | - | Default correlation field(s) for all events |
| `<sequence>` | `local_cache` | No | `false` | Use local cache (Ristretto + Pebble) instead of Redis; recommended for CEP-heavy workloads |
| `<event>` | `id` | Yes | - | Unique identifier, referenced in condition |
| `<event>` | `event_time` | No | Detection time | Field containing event timestamp |
| `<event>` | `group_by` | No | Sequence default | Per-event correlation field(s), overrides sequence default |

### Condition Operators

| Operator | Precedence | Type | Description |
|----------|------------|------|-------------|
| `()` | 1 (highest) | Grouping | Group sub-expressions |
| `!` | 2 | Unary | Absence: event must NOT occur |
| `and` | 3 | Binary | Same-event logical AND |
| `or` | 4 | Binary | Same-event logical OR |
| `->` | 5 (lowest) | Binary | Temporal sequence: A followed by B |

### Allowed Children of `<event>`

| Element | Description |
|---------|-------------|
| `<check>` | Single field check (all existing types: EQU, INCL, MT, LT, REGEX, etc.) |
| `<checklist>` | Complex condition combination with `condition` attribute |
| `<threshold>` | Frequency/aggregation detection |
| `<append>` | Optional side effect. When `field` starts with `_@`, writes into sequence context |

---

## 10. Validation Rules

1. `<sequence>` must contain at least 2 `<event>` elements
2. `<sequence>` must contain exactly 1 `<condition>` element as its last child
3. Every event `id` referenced in `<condition>` must have a corresponding `<event>` definition
4. Every `<event>` definition must be referenced in `<condition>`
5. `within` attribute is required and must be a valid duration
6. When no sequence-level `group_by` is set, every `<event>` must have its own `group_by`
7. When events have per-event `group_by` with multiple fields, the field count must be consistent across all events
8. `<event>` must contain at least one child element (`<check>`, `<checklist>`, or `<threshold>`)
9. `<condition>` expression must be syntactically valid (balanced parentheses, valid operators, valid event references)
10. `!` operator can only appear after `->` (absence is temporal, not logical within a single stage)
