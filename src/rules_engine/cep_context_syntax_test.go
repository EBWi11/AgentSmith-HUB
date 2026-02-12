package rules_engine

import "testing"

func getFirstSequenceFromRuleset(t *testing.T, rs *Ruleset) Sequence {
	t.Helper()
	if rs == nil || len(rs.Rules) == 0 {
		t.Fatal("ruleset is empty")
	}
	for _, seq := range rs.Rules[0].SequenceMap {
		return seq
	}
	t.Fatal("sequence not found")
	return Sequence{}
}

func TestCEP_Parse_EventAppendForSequenceContext(t *testing.T) {
	xml := `<root name="test" type="DETECTION">
		<rule id="r1" name="ctx parse">
			<sequence within="10m" group_by="user_id" local_cache="true">
				<event id="download">
					<check type="EQU" field="event_type">download</check>
					<append field="_@file.current">_$file_path</append>
				</event>
				<event id="exec">
					<check type="EQU" field="event_type">exec</check>
				</event>
				<condition>download -> exec</condition>
			</sequence>
		</rule>
	</root>`

	rs := buildTestRuleset(t, xml)
	seq := getFirstSequenceFromRuleset(t, rs)
	download := seq.Events["download"]
	if download == nil {
		t.Fatal("download event missing")
	}
	if len(download.Appends) != 1 {
		t.Fatalf("expected 1 append in download event, got %d", len(download.Appends))
	}
	if download.Appends[0].FieldName != "_@file.current" {
		t.Fatalf("expected append field _@file.current, got %s", download.Appends[0].FieldName)
	}
}

func TestCEP_SequenceContext_BasicWriteRead(t *testing.T) {
	xml := `<root name="test" type="DETECTION">
		<rule id="r1" name="ctx basic">
			<sequence within="10m" group_by="user_id" local_cache="true">
				<event id="download">
					<check type="EQU" field="event_type">download</check>
					<append field="_@file.current">_$file_path</append>
				</event>
				<event id="exec">
					<check type="EQU" field="event_type">exec</check>
					<check type="EQU" field="file_path">_@file.current</check>
				</event>
				<condition>download -> exec</condition>
			</sequence>
		</rule>
	</root>`

	rs := buildTestRuleset(t, xml)
	if out := rs.EngineCheck(map[string]interface{}{
		"user_id":    "u-1",
		"event_type": "download",
		"file_path":  "/tmp/a",
	}); len(out) != 0 {
		t.Fatalf("expected no hit after stage1, got %d", len(out))
	}

	out := rs.EngineCheck(map[string]interface{}{
		"user_id":    "u-1",
		"event_type": "exec",
		"file_path":  "/tmp/a",
	})
	if len(out) != 1 {
		t.Fatalf("expected 1 hit when _@ matches, got %d", len(out))
	}
}

func TestCEP_SequenceContext_UpdateAcrossStages(t *testing.T) {
	xml := `<root name="test" type="DETECTION">
		<rule id="r1" name="ctx update">
			<sequence within="10m" group_by="user_id" local_cache="true">
				<event id="download">
					<check type="EQU" field="event_type">download</check>
					<append field="_@file.current">_$name</append>
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
		</rule>
	</root>`

	rs := buildTestRuleset(t, xml)
	rs.EngineCheck(map[string]interface{}{"user_id": "u-2", "event_type": "download", "name": "a"})
	rs.EngineCheck(map[string]interface{}{"user_id": "u-2", "event_type": "rename", "src": "a", "dst": "b"})
	out := rs.EngineCheck(map[string]interface{}{"user_id": "u-2", "event_type": "exec", "file_path": "b"})
	if len(out) != 1 {
		t.Fatalf("expected 1 hit after context update chain, got %d", len(out))
	}
}

func TestCEP_SequenceContext_IsolatedByGroupByKey(t *testing.T) {
	xml := `<root name="test" type="DETECTION">
		<rule id="r1" name="ctx isolate">
			<sequence within="10m" group_by="user_id" local_cache="true">
				<event id="download">
					<check type="EQU" field="event_type">download</check>
					<append field="_@file.current">_$file_path</append>
				</event>
				<event id="exec">
					<check type="EQU" field="event_type">exec</check>
					<check type="EQU" field="file_path">_@file.current</check>
				</event>
				<condition>download -> exec</condition>
			</sequence>
		</rule>
	</root>`

	rs := buildTestRuleset(t, xml)
	rs.EngineCheck(map[string]interface{}{"user_id": "u-A", "event_type": "download", "file_path": "/tmp/a"})

	// Different group key should not reuse u-A context.
	out := rs.EngineCheck(map[string]interface{}{"user_id": "u-B", "event_type": "exec", "file_path": "/tmp/a"})
	if len(out) != 0 {
		t.Fatalf("expected 0 hits for isolated key, got %d", len(out))
	}
}

func TestCEP_SequenceContext_MissingKeyDoesNotMatch(t *testing.T) {
	xml := `<root name="test" type="DETECTION">
		<rule id="r1" name="ctx missing">
			<sequence within="10m" group_by="user_id" local_cache="true">
				<event id="a">
					<check type="EQU" field="event_type">a</check>
				</event>
				<event id="b">
					<check type="EQU" field="event_type">b</check>
					<check type="EQU" field="k">_@not.exists</check>
				</event>
				<condition>a -> b</condition>
			</sequence>
		</rule>
	</root>`

	rs := buildTestRuleset(t, xml)
	rs.EngineCheck(map[string]interface{}{"user_id": "u-3", "event_type": "a"})
	out := rs.EngineCheck(map[string]interface{}{"user_id": "u-3", "event_type": "b", "k": "whatever"})
	if len(out) != 0 {
		t.Fatalf("expected 0 hits when context key missing, got %d", len(out))
	}
}
