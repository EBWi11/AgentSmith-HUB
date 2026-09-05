package api

import (
	"AgentSmith-HUB/input"
	"AgentSmith-HUB/output"
	projectpkg "AgentSmith-HUB/project"
	"AgentSmith-HUB/rules_engine"
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

func TestProjectContentTestRoutesEveryInputThroughMergedNode(t *testing.T) {
	const (
		inputAID  = "api_project_multi_input_a"
		inputBID  = "api_project_multi_input_b"
		rulesetID = "api_project_multi_input_ruleset"
		outputID  = "api_project_multi_input_output"
	)

	newInput := func(id string) *input.Input {
		testInput, err := input.NewInput("", `
type: kafka
kafka:
  brokers: ["localhost:9092"]
  group: project-test
  topic: project-test
`, id)
		if err != nil {
			t.Fatalf("create test input %s: %v", id, err)
		}
		return testInput
	}

	testRuleset, err := rules_engine.NewRuleset("", `
<root type="DETECTION" name="multi-input-project-test">
  <rule id="pass" name="pass">
    <check type="EQU" field="event">login</check>
  </rule>
</root>`, rulesetID)
	if err != nil {
		t.Fatalf("create test ruleset: %v", err)
	}
	testOutput, err := output.NewOutput("", `type: print`, outputID)
	if err != nil {
		t.Fatalf("create test output: %v", err)
	}

	previousInputA, hadInputA := projectpkg.GetInput(inputAID)
	previousInputB, hadInputB := projectpkg.GetInput(inputBID)
	previousRuleset, hadRuleset := projectpkg.GetRuleset(rulesetID)
	previousOutput, hadOutput := projectpkg.GetOutput(outputID)
	projectpkg.SetInput(inputAID, newInput(inputAID))
	projectpkg.SetInput(inputBID, newInput(inputBID))
	projectpkg.SetRuleset(rulesetID, testRuleset)
	projectpkg.SetOutput(outputID, testOutput)
	defer func() {
		if hadInputA {
			projectpkg.SetInput(inputAID, previousInputA)
		} else {
			projectpkg.DeleteInput(inputAID)
		}
		if hadInputB {
			projectpkg.SetInput(inputBID, previousInputB)
		} else {
			projectpkg.DeleteInput(inputBID)
		}
		if hadRuleset {
			projectpkg.SetRuleset(rulesetID, previousRuleset)
		} else {
			projectpkg.DeleteRuleset(rulesetID)
		}
		if hadOutput {
			projectpkg.SetOutput(outputID, previousOutput)
		} else {
			projectpkg.DeleteOutput(outputID)
		}
	}()

	graphs := []struct {
		name    string
		content string
	}{
		{
			name: "inbound edges declared first",
			content: "content: |\n" +
				"  INPUT." + inputAID + " -> RULESET." + rulesetID + "\n" +
				"  INPUT." + inputBID + " -> RULESET." + rulesetID + "\n" +
				"  RULESET." + rulesetID + " -> OUTPUT." + outputID + "\n",
		},
		{
			name: "outbound edge declared first",
			content: "content: |\n" +
				"  RULESET." + rulesetID + " -> OUTPUT." + outputID + "\n" +
				"  INPUT." + inputBID + " -> RULESET." + rulesetID + "\n" +
				"  INPUT." + inputAID + " -> RULESET." + rulesetID + "\n",
		},
	}

	for _, graph := range graphs {
		t.Run(graph.name, func(t *testing.T) {
			for _, inputID := range []string{inputAID, inputBID} {
				t.Run(inputID, func(t *testing.T) {
					body, err := json.Marshal(map[string]interface{}{
						"content": graph.content,
						"data": map[string]interface{}{
							"event":   "login",
							"message": inputID,
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
					if !resp.Success || resp.Timeout {
						t.Fatalf("expected successful test response, got body=%s", rec.Body.String())
					}
					gotOutput := resp.Outputs[outputID]
					if len(gotOutput) != 1 {
						t.Fatalf("expected input %s to produce one output, got body=%s", inputID, rec.Body.String())
					}
					if gotOutput[0]["message"] != inputID {
						t.Fatalf("expected payload from input %s, got %#v", inputID, gotOutput[0])
					}
				})
			}
		})
	}
}

func TestProjectContentTestFanOutDeliversEveryOutput(t *testing.T) {
	const (
		inputID   = "api_project_fanout_input"
		outputAID = "api_project_fanout_output_a"
		outputBID = "api_project_fanout_output_b"
	)

	testInput, err := input.NewInput("", `
type: kafka
kafka:
  brokers: ["localhost:9092"]
  group: project-fanout-test
  topic: project-fanout-test
`, inputID)
	if err != nil {
		t.Fatalf("create test input: %v", err)
	}
	outputA, err := output.NewOutput("", `type: print`, outputAID)
	if err != nil {
		t.Fatalf("create output A: %v", err)
	}
	outputB, err := output.NewOutput("", `type: print`, outputBID)
	if err != nil {
		t.Fatalf("create output B: %v", err)
	}

	previousInput, hadInput := projectpkg.GetInput(inputID)
	previousOutputA, hadOutputA := projectpkg.GetOutput(outputAID)
	previousOutputB, hadOutputB := projectpkg.GetOutput(outputBID)
	projectpkg.SetInput(inputID, testInput)
	projectpkg.SetOutput(outputAID, outputA)
	projectpkg.SetOutput(outputBID, outputB)
	t.Cleanup(func() {
		if hadInput {
			projectpkg.SetInput(inputID, previousInput)
		} else {
			projectpkg.DeleteInput(inputID)
		}
		if hadOutputA {
			projectpkg.SetOutput(outputAID, previousOutputA)
		} else {
			projectpkg.DeleteOutput(outputAID)
		}
		if hadOutputB {
			projectpkg.SetOutput(outputBID, previousOutputB)
		} else {
			projectpkg.DeleteOutput(outputBID)
		}
	})

	body, err := json.Marshal(map[string]interface{}{
		"content": "content: |\n" +
			"  INPUT." + inputID + " -> OUTPUT." + outputAID + "\n" +
			"  INPUT." + inputID + " -> OUTPUT." + outputBID + "\n",
		"data": map[string]interface{}{"message": "fanout"},
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
	if !resp.Success || resp.Timeout {
		t.Fatalf("expected successful fan-out response, got body=%s", rec.Body.String())
	}
	for _, outputID := range []string{outputAID, outputBID} {
		got := resp.Outputs[outputID]
		if len(got) != 1 || got[0]["message"] != "fanout" {
			t.Fatalf("expected output %s to receive fan-out payload, got %#v", outputID, got)
		}
	}
}

func TestBuildPNSDirectCanonicalizesMergedNode(t *testing.T) {
	tests := []struct {
		name      string
		flowNodes []projectpkg.FlowNode
	}{
		{
			name: "inputs declared first",
			flowNodes: []projectpkg.FlowNode{
				{FromType: "INPUT", FromID: "a", ToType: "RULESET", ToID: "shared"},
				{FromType: "INPUT", FromID: "b", ToType: "RULESET", ToID: "shared"},
				{FromType: "RULESET", FromID: "shared", ToType: "OUTPUT", ToID: "out"},
			},
		},
		{
			name: "outbound edge declared first",
			flowNodes: []projectpkg.FlowNode{
				{FromType: "RULESET", FromID: "shared", ToType: "OUTPUT", ToID: "out"},
				{FromType: "INPUT", FromID: "b", ToType: "RULESET", ToID: "shared"},
				{FromType: "INPUT", FromID: "a", ToType: "RULESET", ToID: "shared"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buildPNSDirect(tt.flowNodes)

			var rulesetPNS string
			for _, node := range tt.flowNodes {
				if node.FromType == "RULESET" && node.FromID == "shared" {
					if rulesetPNS != "" && rulesetPNS != node.FromPNS {
						t.Fatalf("merged ruleset has inconsistent PNS values %q and %q", rulesetPNS, node.FromPNS)
					}
					rulesetPNS = node.FromPNS
				}
				if node.ToType == "RULESET" && node.ToID == "shared" {
					if rulesetPNS != "" && rulesetPNS != node.ToPNS {
						t.Fatalf("merged ruleset has inconsistent PNS values %q and %q", rulesetPNS, node.ToPNS)
					}
					rulesetPNS = node.ToPNS
				}
			}
			if rulesetPNS == "" {
				t.Fatal("merged ruleset PNS was not generated")
			}
		})
	}
}
