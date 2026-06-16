package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestRulesetTestRejectsNonObjectArrayItems(t *testing.T) {
	body, err := json.Marshal(map[string]interface{}{
		"content": `<root name="test" type="DETECTION"></root>`,
		"data": []interface{}{
			map[string]interface{}{"event_type": "login"},
			"not-an-object",
		},
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/test-ruleset-content", bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := testRuleset(c); err != nil {
		t.Fatalf("testRuleset returned error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d body=%s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Success bool   `json:"success"`
		Error   string `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v body=%s", err, rec.Body.String())
	}
	if resp.Success {
		t.Fatalf("expected failure response, got body=%s", rec.Body.String())
	}
	if !strings.Contains(resp.Error, "data[1] must be an object") {
		t.Fatalf("expected data item error, got %#v", resp)
	}
}

func TestOutputContentTestUsesProvidedConfig(t *testing.T) {
	body, err := json.Marshal(map[string]interface{}{
		"content": "type: print",
		"data": map[string]interface{}{
			"message": "hello output content test",
		},
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/test-output-content", bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := testOutput(c); err != nil {
		t.Fatalf("testOutput returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Success    bool                   `json:"success"`
		IsTemp     bool                   `json:"isTemp"`
		OutputType string                 `json:"outputType"`
		Metrics    map[string]interface{} `json:"metrics"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v body=%s", err, rec.Body.String())
	}
	if !resp.Success {
		t.Fatalf("expected success response, got body=%s", rec.Body.String())
	}
	if !resp.IsTemp {
		t.Fatalf("expected content-mode output test to be marked temporary, got %#v", resp)
	}
	if resp.OutputType != "print" {
		t.Fatalf("expected print output type, got %q", resp.OutputType)
	}
	if got, ok := resp.Metrics["produceTotal"].(float64); !ok || got != 1 {
		t.Fatalf("expected produceTotal 1, got %#v", resp.Metrics)
	}
}

func TestInputConnectCheckWithConfigReturnsStructuredConfigError(t *testing.T) {
	body, err := json.Marshal(map[string]interface{}{
		"raw": "type: unsupported_input_type",
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/connect-check/input/test-input", bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("type", "id")
	c.SetParamValues("input", "test-input")

	if err := connectCheck(c); err != nil {
		t.Fatalf("connectCheck returned error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d body=%s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Status  string `json:"status"`
		Message string `json:"message"`
		Details struct {
			ConnectionStatus string                   `json:"connection_status"`
			ConnectionErrors []map[string]interface{} `json:"connection_errors"`
		} `json:"details"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v body=%s", err, rec.Body.String())
	}
	if resp.Status != "error" {
		t.Fatalf("expected error status, got %#v", resp)
	}
	if resp.Details.ConnectionStatus != "configuration_error" {
		t.Fatalf("expected configuration_error, got %#v", resp)
	}
	if len(resp.Details.ConnectionErrors) == 0 {
		t.Fatalf("expected connection_errors detail, got %#v", resp)
	}
	if !strings.Contains(resp.Message, "Failed to create temporary input") {
		t.Fatalf("expected temporary input error message, got %#v", resp)
	}
}
