package common

import "testing"

func TestBuildAliyunSLSFieldsStringifiesStructuredValues(t *testing.T) {
	msg := map[string]interface{}{
		"message": "ok",
		"count":   3,
		"active":  true,
		"meta": map[string]interface{}{
			"env": "prod",
		},
		"list": []interface{}{"a", 2, true},
		"nil":  nil,
	}

	fields := buildAliyunSLSFields(msg)

	if fields["message"] != "ok" {
		t.Fatalf("expected message field to stay unchanged, got %q", fields["message"])
	}
	if fields["count"] != "3" {
		t.Fatalf("expected count to stringify to 3, got %q", fields["count"])
	}
	if fields["active"] != "true" {
		t.Fatalf("expected active to stringify to true, got %q", fields["active"])
	}
	if fields["meta"] != `{"env":"prod"}` {
		t.Fatalf("expected nested map to JSON stringify, got %q", fields["meta"])
	}
	if fields["list"] != `["a",2,true]` {
		t.Fatalf("expected slice to JSON stringify, got %q", fields["list"])
	}
	if fields["nil"] != "" {
		t.Fatalf("expected nil to stringify to empty string, got %q", fields["nil"])
	}
}

func TestResolveAliyunSLSRoutingValuePrefersFieldPath(t *testing.T) {
	msg := map[string]interface{}{
		"meta": map[string]interface{}{
			"topic": "dynamic-topic",
		},
	}

	got := resolveAliyunSLSRoutingValue(msg, "static-topic", "meta.topic")
	if got != "dynamic-topic" {
		t.Fatalf("expected dynamic-topic, got %q", got)
	}

	fallback := resolveAliyunSLSRoutingValue(msg, "static-topic", "meta.missing")
	if fallback != "static-topic" {
		t.Fatalf("expected static-topic fallback, got %q", fallback)
	}
}
