package api

import (
	"AgentSmith-HUB/input"
	"AgentSmith-HUB/output"
	projectpkg "AgentSmith-HUB/project"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestProjectContentTestAcceptsPrefixedInputAndCollectsOutput(t *testing.T) {
	inputID := "api_project_test_input"
	outputID := "api_project_test_output"

	testInput, err := input.NewInput("", `
type: kafka
kafka:
  brokers: ["localhost:9092"]
  group: project-test
  topic: project-test
`, inputID)
	if err != nil {
		t.Fatalf("create test input: %v", err)
	}

	testOutput, err := output.NewOutput("", `type: print`, outputID)
	if err != nil {
		t.Fatalf("create test output: %v", err)
	}

	previousInput, hadInput := projectpkg.GetInput(inputID)
	previousOutput, hadOutput := projectpkg.GetOutput(outputID)
	projectpkg.SetInput(inputID, testInput)
	projectpkg.SetOutput(outputID, testOutput)
	defer func() {
		if hadInput {
			projectpkg.SetInput(inputID, previousInput)
		} else {
			projectpkg.DeleteInput(inputID)
		}
		if hadOutput {
			projectpkg.SetOutput(outputID, previousOutput)
		} else {
			projectpkg.DeleteOutput(outputID)
		}
	}()

	projectContent := "content: |\n  INPUT." + inputID + " -> OUTPUT." + outputID + "\n"
	body, err := json.Marshal(map[string]interface{}{
		"content": projectContent,
		"data": map[string]interface{}{
			"message": "hello project test",
		},
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/test-project-content/input."+inputID, bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("inputNode")
	c.SetParamValues("input." + inputID)

	if err := testProject(c); err != nil {
		t.Fatalf("testProject returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Success bool                                `json:"success"`
		Timeout bool                                `json:"timeout"`
		Outputs map[string][]map[string]interface{} `json:"outputs"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !resp.Success {
		t.Fatalf("expected success response, got body=%s", rec.Body.String())
	}
	if resp.Timeout {
		t.Fatalf("expected project test to complete without timeout, got body=%s", rec.Body.String())
	}
	gotOutput := resp.Outputs[outputID]
	if len(gotOutput) != 1 {
		t.Fatalf("expected one output message for %s, got %#v", outputID, resp.Outputs)
	}
	if gotOutput[0]["message"] != "hello project test" {
		t.Fatalf("expected output message payload, got %#v", gotOutput[0])
	}
}
