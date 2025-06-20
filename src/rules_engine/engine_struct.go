package rules_engine

import (
	"AgentSmith-HUB/common"
	"AgentSmith-HUB/plugin"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	regexpgo "regexp"
	"strconv"
	"strings"
	"sync"

	regexp "github.com/BurntSushi/rure-go"
	"github.com/dgraph-io/ristretto/v2"
	"github.com/panjf2000/ants/v2"
)

// FromRawSymbol is the prefix indicating a value should be fetched from raw data.
const FromRawSymbol = "_$"
const PluginArgFromRawSymbol = "_$ORIDATA"
const FromRawSymbolLen = len(FromRawSymbol)

const MinPoolSize = 32
const MaxPoolSize = 256

var ConditionRegex = regexp.MustCompile("^([a-z]+|\\(|\\)|\\s)+$")

// Ruleset represents a collection of rules and associated metadata.
type Ruleset struct {
	Path                string
	XMLName             xml.Name `xml:"root"`
	RulesetID           string   `json:"Id"`
	ProjectNodeSequence string
	Type                string `xml:"type,attr"`

	IsDetection bool
	Rules       []Rule `xml:"rule"`

	RulesByFilter map[string]*RulesByFilter

	UpStream   map[string]*chan map[string]interface{}
	DownStream map[string]*chan map[string]interface{}

	stopChan chan struct{} // Control channel for Start/Stop
	antsPool *ants.Pool    // Ants thread pool

	Cache            *ristretto.Cache[string, int]
	CacheForClassify *ristretto.Cache[string, map[string]bool]
	// only for classify local cache
	CacheMu sync.RWMutex

	RawConfig string
	sampler   *common.Sampler
}

type RulesByFilter struct {
	Filter Filter
	Rules  []*Rule
}

// Rule represents a single rule with its logic and metadata.
type Rule struct {
	ID             string    `xml:"id,attr"`
	Name           string    `xml:"name,attr"`
	Author         string    `xml:"author,attr"`
	Filter         Filter    `xml:"filter"`
	Checklist      Checklist `xml:"checklist"`
	ChecklistLen   int
	ThresholdCheck bool
	Threshold      Threshold  `xml:"threshold"`
	Appends        []Append   `xml:"append"`
	Plugins        []Plugin   `xml:"plugin"`
	Del            string     `xml:"del"`
	DelList        [][]string // parsed field path
}

// Filter defines the field and value for rule filtering.
type Filter struct {
	Field     string   `xml:"field,attr"`
	FieldList []string // parsed field path
	Value     string   `xml:",chardata"`
	Check     bool
}

func (f *Filter) ToStr() string {
	str := f.Field + ">>" + f.Value
	if str == "" {
		return "nil"
	}
	return str
}

// Checklist contains the logical condition and nodes to check.
type Checklist struct {
	Condition     string       `xml:"condition,attr"`
	CheckNodes    []CheckNodes `xml:"node"`
	ConditionAST  *ReCepAST
	ConditionFlag bool
	ConditionMap  map[string]bool
}

// CheckNodes represents a single check operation in a checklist.
type CheckNodes struct {
	ID        string                              `xml:"id,attr"`
	Type      string                              `xml:"type,attr"`
	CheckFunc func(string, string) (bool, string) // function pointer for check logic
	Field     string                              `xml:"field,attr"`
	FieldList []string                            // parsed field path
	Logic     string                              `xml:"logic,attr"`
	Delimiter string                              `xml:"delimiter,attr"`

	DelimiterFieldList []string
	Value              string `xml:",chardata"`
	Regex              *regexp.Regex

	Plugin     *plugin.Plugin
	PluginArgs []*PluginArg
}

type PluginArg struct {
	//0 Value == RealValue
	//1 RealValue == GetCheckData(Value)
	//2 RealValue == ORI DATA
	Type int

	Value     interface{}
	RealValue interface{}
}

// Threshold defines aggregation and counting logic for a rule.
// It supports grouping by fields, time-based ranges, and different counting methods.
type Threshold struct {
	GroupBy        string              `xml:"group_by,attr"` // Field to group by
	GroupByList    map[string][]string // Parsed group by fields
	Range          string              `xml:"range,attr"` // Time range for aggregation
	RangeInt       int                 // Parsed range in seconds
	LocalCache     bool                `xml:"local_cache,attr"` // Whether to use local cache
	CountType      string              `xml:"count_type,attr"`  // Type of counting (SUM/CLASSIFY)
	CountField     string              `xml:"count_field,attr"` // Field to count
	CountFieldList []string            // Parsed count field path
	Value          int                 `xml:",chardata"` // Threshold value
	GroupByID      string              // Unique identifier for grouping
}

// Append defines additional fields to append after rule matching.
// It supports both static values and plugin-based dynamic values.
type Append struct {
	Type      string `xml:"type,attr"`       // Type of append (PLUGIN)
	FieldName string `xml:"field_name,attr"` // Name of field to append
	Value     string `xml:",chardata"`       // Value to append

	Plugin     *plugin.Plugin // Plugin instance if type is PLUGIN
	PluginArgs []*PluginArg   // Arguments for plugin execution
}

// Plugin represents a plugin configuration with its execution parameters
type Plugin struct {
	Value      string         `xml:",chardata"` // Plugin value/configuration
	Plugin     *plugin.Plugin // Plugin instance
	PluginArgs []*PluginArg   // Arguments for plugin execution
}

// ValidationError represents a validation error with line number
type ValidationError struct {
	Line    int    `json:"line"`
	Message string `json:"message"`
	Detail  string `json:"detail,omitempty"`
}

// ValidationWarning represents a validation warning with line number
type ValidationWarning struct {
	Line    int    `json:"line"`
	Message string `json:"message"`
	Detail  string `json:"detail,omitempty"`
}

// ValidationResult represents the complete validation result
type ValidationResult struct {
	IsValid  bool                `json:"is_valid"`
	Errors   []ValidationError   `json:"errors"`
	Warnings []ValidationWarning `json:"warnings"`
}

// ValidateWithDetails performs detailed validation and returns structured errors with line numbers
func ValidateWithDetails(path string, raw string) (*ValidationResult, error) {
	var rawRuleset []byte
	if path != "" {
		xmlFile, err := os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("failed to open resource file at %s: %w", path, err)
		}
		defer xmlFile.Close()

		rawRuleset, err = io.ReadAll(xmlFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read resource file: %w", err)
		}
	} else {
		rawRuleset = []byte(raw)
	}

	result := &ValidationResult{
		IsValid:  true,
		Errors:   []ValidationError{},
		Warnings: []ValidationWarning{},
	}

	// Parse XML first to check basic syntax
	var ruleset Ruleset
	if err := xml.Unmarshal(rawRuleset, &ruleset); err != nil {
		// Extract line number from XML parsing error
		lineNum := extractLineFromXMLError(err.Error())
		result.IsValid = false
		result.Errors = append(result.Errors, ValidationError{
			Line:    lineNum,
			Message: "XML parsing error",
			Detail:  err.Error(),
		})
		return result, nil
	}

	// Perform detailed validation
	validateRulesetStructure(&ruleset, string(rawRuleset), result)

	return result, nil
}

// extractLineFromXMLError extracts line number from XML parsing error message
func extractLineFromXMLError(errorMsg string) int {
	// Try to extract line number from XML error messages
	re := regexpgo.MustCompile(`line (\d+)`)
	matches := re.FindStringSubmatch(errorMsg)
	if len(matches) > 1 {
		if lineNum, err := strconv.Atoi(matches[1]); err == nil {
			return lineNum
		}
	}
	return 1
}

// getLineNumber finds the line number of a pattern in XML content
func getLineNumber(xmlContent, pattern string, occurrence int) int {
	lines := strings.Split(xmlContent, "\n")
	count := 0
	for i, line := range lines {
		if strings.Contains(line, pattern) {
			if count == occurrence {
				return i + 1
			}
			count++
		}
	}
	return 1
}

// findElementInRule finds the line number of an element within a specific rule
func findElementInRule(xmlContent, ruleID, pattern string, ruleIndex, elementIndex int) int {
	lines := strings.Split(xmlContent, "\n")
	var ruleStartLine int

	if ruleID != "" && strings.TrimSpace(ruleID) != "" {
		// Find rule by ID
		for i, line := range lines {
			if strings.Contains(line, fmt.Sprintf(`id="%s"`, ruleID)) {
				ruleStartLine = i + 1
				break
			}
		}
	} else {
		// Find rule by index
		ruleCount := 0
		for i, line := range lines {
			if strings.Contains(line, "<rule") {
				if ruleCount == ruleIndex {
					ruleStartLine = i + 1
					break
				}
				ruleCount++
			}
		}
	}

	// Search for pattern within the rule
	patternCount := 0
	for i := ruleStartLine - 1; i < len(lines); i++ {
		if strings.Contains(lines[i], pattern) {
			if patternCount == elementIndex {
				return i + 1
			}
			patternCount++
		}
		// Stop at next rule or end
		if i > ruleStartLine-1 && (strings.Contains(lines[i], "<rule") || strings.Contains(lines[i], "</root>")) {
			break
		}
	}

	return ruleStartLine
}

// validateRulesetStructure performs detailed validation of ruleset structure
func validateRulesetStructure(ruleset *Ruleset, xmlContent string, result *ValidationResult) {
	// Validate root element type
	if ruleset.Type != "" && ruleset.Type != "DETECTION" && ruleset.Type != "WHITELIST" {
		result.IsValid = false
		result.Errors = append(result.Errors, ValidationError{
			Line:    getLineNumber(xmlContent, "<root", 0),
			Message: "Root type must be 'DETECTION' or 'WHITELIST'",
		})
	}

	// Check for duplicate rule IDs
	ruleIDMap := make(map[string]int)
	for i, rule := range ruleset.Rules {
		if rule.ID != "" {
			if prevIndex, exists := ruleIDMap[rule.ID]; exists {
				result.IsValid = false
				result.Errors = append(result.Errors, ValidationError{
					Line:    getLineNumber(xmlContent, fmt.Sprintf(`id="%s"`, rule.ID), 1),
					Message: fmt.Sprintf("Duplicate rule ID: %s", rule.ID),
					Detail:  fmt.Sprintf("First occurrence at rule index %d", prevIndex),
				})
			} else {
				ruleIDMap[rule.ID] = i
			}
		}
	}

	// Validate each rule
	for ruleIndex, rule := range ruleset.Rules {
		validateRule(&rule, xmlContent, ruleIndex, result)
	}
}

// validateRule validates a single rule
func validateRule(rule *Rule, xmlContent string, ruleIndex int, result *ValidationResult) {
	ruleID := rule.ID
	var ruleLine int

	if ruleID != "" && strings.TrimSpace(ruleID) != "" {
		ruleLine = getLineNumber(xmlContent, fmt.Sprintf(`id="%s"`, ruleID), 0)
	} else {
		ruleLine = getLineNumber(xmlContent, "<rule", ruleIndex)
	}

	// Check required attributes
	if rule.ID == "" || strings.TrimSpace(rule.ID) == "" {
		result.IsValid = false
		result.Errors = append(result.Errors, ValidationError{
			Line:    ruleLine,
			Message: "Rule id cannot be empty",
		})
	}

	if rule.Name == "" || strings.TrimSpace(rule.Name) == "" {
		result.IsValid = false
		result.Errors = append(result.Errors, ValidationError{
			Line:    ruleLine,
			Message: "Rule name cannot be empty",
		})
	}

	if rule.Author == "" || strings.TrimSpace(rule.Author) == "" {
		result.IsValid = false
		result.Errors = append(result.Errors, ValidationError{
			Line:    ruleLine,
			Message: "Rule author cannot be empty",
		})
	}

	// Validate filter
	if rule.Filter.Field == "" || strings.TrimSpace(rule.Filter.Field) == "" {
		result.Warnings = append(result.Warnings, ValidationWarning{
			Line:    findElementInRule(xmlContent, ruleID, "<filter", ruleIndex, 0),
			Message: "Filter field is empty",
		})
	}

	// Validate checklist
	validateChecklist(&rule.Checklist, xmlContent, ruleID, ruleIndex, result)

	// Validate threshold
	validateThreshold(&rule.Threshold, xmlContent, ruleID, ruleIndex, result)

	// Validate appends
	for appendIndex, appendElem := range rule.Appends {
		validateAppend(&appendElem, xmlContent, ruleID, ruleIndex, appendIndex, result)
	}

	// Validate plugins
	for pluginIndex, plugin := range rule.Plugins {
		validatePlugin(&plugin, xmlContent, ruleID, ruleIndex, pluginIndex, result)
	}
}

// validateChecklist validates checklist elements
func validateChecklist(checklist *Checklist, xmlContent, ruleID string, ruleIndex int, result *ValidationResult) {
	if len(checklist.CheckNodes) == 0 {
		result.Warnings = append(result.Warnings, ValidationWarning{
			Line:    findElementInRule(xmlContent, ruleID, "<checklist", ruleIndex, 0),
			Message: "Checklist has no check nodes",
		})
		return
	}

	// Check for duplicate node IDs
	nodeIDMap := make(map[string]int)
	hasCondition := checklist.Condition != "" && strings.TrimSpace(checklist.Condition) != ""

	for nodeIndex, node := range checklist.CheckNodes {
		nodeLine := findElementInRule(xmlContent, ruleID, "<node", ruleIndex, nodeIndex)

		// Check required attributes
		if node.Type == "" || strings.TrimSpace(node.Type) == "" {
			result.IsValid = false
			result.Errors = append(result.Errors, ValidationError{
				Line:    nodeLine,
				Message: "Check node type cannot be empty",
				Detail:  fmt.Sprintf("Rule ID: %s", ruleID),
			})
		} else if node.Type != "REGEX" && node.Type != "INCL" && node.Type != "EQU" && node.Type != "IN" {
			result.IsValid = false
			result.Errors = append(result.Errors, ValidationError{
				Line:    nodeLine,
				Message: "Check node type must be one of: REGEX, INCL, EQU, IN",
				Detail:  fmt.Sprintf("Rule ID: %s", ruleID),
			})
		}

		if node.Field == "" || strings.TrimSpace(node.Field) == "" {
			result.IsValid = false
			result.Errors = append(result.Errors, ValidationError{
				Line:    nodeLine,
				Message: "Check node field cannot be empty",
				Detail:  fmt.Sprintf("Rule ID: %s", ruleID),
			})
		}

		// Check node ID if condition is present
		if hasCondition {
			if node.ID == "" || strings.TrimSpace(node.ID) == "" {
				result.IsValid = false
				result.Errors = append(result.Errors, ValidationError{
					Line:    nodeLine,
					Message: "Check node id cannot be empty when condition is used",
					Detail:  fmt.Sprintf("Rule ID: %s", ruleID),
				})
			} else if prevIndex, exists := nodeIDMap[node.ID]; exists {
				result.IsValid = false
				result.Errors = append(result.Errors, ValidationError{
					Line:    nodeLine,
					Message: fmt.Sprintf("Duplicate node ID: %s", node.ID),
					Detail:  fmt.Sprintf("Rule ID: %s, first occurrence at node index %d", ruleID, prevIndex),
				})
			} else {
				nodeIDMap[node.ID] = nodeIndex
			}
		}

		// Validate regex pattern
		if node.Type == "REGEX" {
			nodeValue := strings.TrimSpace(node.Value)
			if nodeValue == "" {
				result.IsValid = false
				result.Errors = append(result.Errors, ValidationError{
					Line:    nodeLine,
					Message: "REGEX node value cannot be empty",
					Detail:  fmt.Sprintf("Rule ID: %s", ruleID),
				})
			} else {
				if _, err := regexp.Compile(nodeValue); err != nil {
					result.IsValid = false
					result.Errors = append(result.Errors, ValidationError{
						Line:    nodeLine,
						Message: "Invalid regex pattern",
						Detail:  fmt.Sprintf("Rule ID: %s, Error: %s", ruleID, err.Error()),
					})
				}
			}
		}
	}
}

// validateThreshold validates threshold elements
func validateThreshold(threshold *Threshold, xmlContent, ruleID string, ruleIndex int, result *ValidationResult) {
	if threshold.GroupBy == "" && threshold.Range == "" && threshold.Value == 0 {
		// No threshold defined, skip validation
		return
	}

	thresholdLine := findElementInRule(xmlContent, ruleID, "<threshold", ruleIndex, 0)

	if threshold.GroupBy == "" || strings.TrimSpace(threshold.GroupBy) == "" {
		result.IsValid = false
		result.Errors = append(result.Errors, ValidationError{
			Line:    thresholdLine,
			Message: "Threshold group_by cannot be empty",
			Detail:  fmt.Sprintf("Rule ID: %s", ruleID),
		})
	}

	if threshold.Range == "" || strings.TrimSpace(threshold.Range) == "" {
		result.IsValid = false
		result.Errors = append(result.Errors, ValidationError{
			Line:    thresholdLine,
			Message: "Threshold range cannot be empty",
			Detail:  fmt.Sprintf("Rule ID: %s", ruleID),
		})
	}

	if threshold.Value <= 1 {
		result.IsValid = false
		result.Errors = append(result.Errors, ValidationError{
			Line:    thresholdLine,
			Message: "Threshold value must be greater than 1",
			Detail:  fmt.Sprintf("Rule ID: %s", ruleID),
		})
	}
}

// validateAppend validates append elements
func validateAppend(appendElem *Append, xmlContent, ruleID string, ruleIndex, appendIndex int, result *ValidationResult) {
	appendLine := findElementInRule(xmlContent, ruleID, "<append", ruleIndex, appendIndex)

	if appendElem.FieldName == "" || strings.TrimSpace(appendElem.FieldName) == "" {
		result.IsValid = false
		result.Errors = append(result.Errors, ValidationError{
			Line:    appendLine,
			Message: "Append field_name cannot be empty",
			Detail:  fmt.Sprintf("Rule ID: %s", ruleID),
		})
	}

	if appendElem.Type == "PLUGIN" {
		value := strings.TrimSpace(appendElem.Value)
		if value == "" {
			result.IsValid = false
			result.Errors = append(result.Errors, ValidationError{
				Line:    appendLine,
				Message: "Append plugin value cannot be empty",
				Detail:  fmt.Sprintf("Rule ID: %s", ruleID),
			})
		}
	}
}

// validatePlugin validates plugin elements
func validatePlugin(plugin *Plugin, xmlContent, ruleID string, ruleIndex, pluginIndex int, result *ValidationResult) {
	pluginLine := findElementInRule(xmlContent, ruleID, "<plugin", ruleIndex, pluginIndex)

	value := strings.TrimSpace(plugin.Value)
	if value == "" {
		result.IsValid = false
		result.Errors = append(result.Errors, ValidationError{
			Line:    pluginLine,
			Message: "Plugin value cannot be empty",
			Detail:  fmt.Sprintf("Rule ID: %s", ruleID),
		})
	}
}

func Verify(path string, raw string) error {
	var rawRuleset []byte
	if path != "" {
		xmlFile, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("failed to open resource file at %s: %w", path, err)
		}
		defer xmlFile.Close()

		rawRuleset, err = io.ReadAll(xmlFile)
		if err != nil {
			return fmt.Errorf("failed to read resource file: %w", err)
		}
	} else {
		rawRuleset = []byte(raw)
	}

	_, err := ParseRulesetFromByte(rawRuleset)
	if err != nil {
		// Try to extract line number from XML error
		if strings.Contains(err.Error(), "line") {
			return fmt.Errorf("failed to parse resource: %w", err)
		}
		return fmt.Errorf("failed to parse resource: %w (line: unknown)", err)
	}
	return nil
}

// NewRuleset creates a new resource from an XML file
// path: Path to the resource XML file
func NewRuleset(path string, raw string, id string) (*Ruleset, error) {
	var rawRuleset []byte

	err := Verify(path, raw)
	if err != nil {
		return nil, fmt.Errorf("ruleset verify error: %s %w", id, err)
	}

	if path != "" {
		xmlFile, _ := os.Open(path)
		defer xmlFile.Close()

		rawRuleset, _ = io.ReadAll(xmlFile)
	} else {
		rawRuleset = []byte(raw)
	}

	ruleset, _ := ParseRulesetFromByte(rawRuleset)
	ruleset.Path = path

	if len(ruleset.UpStream) == 0 {
		ruleset.UpStream = make(map[string]*chan map[string]interface{}, 0)
	}

	if len(ruleset.DownStream) == 0 {
		ruleset.DownStream = make(map[string]*chan map[string]interface{}, 0)
	}

	ruleset.RulesetID = id
	ruleset.sampler = common.GetSampler("ruleset." + id)
	return ruleset, nil
}

// ParseFunctionCall parses a function call string into its components
// input: Function call string in format "func(arg1, arg2, ...)"
// Returns: function name, arguments, and any error
func ParseFunctionCall(input string) (string, []*PluginArg, error) {
	input = strings.TrimSpace(input)

	re := regexpgo.MustCompile(`^([a-zA-Z_][a-zA-Z0-9_]*)\s*\((.*)\)$`)
	matches := re.FindStringSubmatch(input)
	if len(matches) != 3 {
		return "", nil, fmt.Errorf("invalid function call syntax: %s, must be in format func(arg1, arg2, ...)", input)
	}

	funcName := matches[1]
	argStr := matches[2]

	args, err := parseArgs(argStr)
	if err != nil {
		return "", nil, fmt.Errorf("failed to parse function arguments: %w", err)
	}

	return funcName, args, nil
}

func parseArgs(s string) ([]*PluginArg, error) {
	var args []*PluginArg
	var current strings.Builder
	inQuotes := false
	escaped := false

	for i, ch := range s {
		switch ch {
		case '"':
			if escaped {
				current.WriteRune(ch)
				escaped = false
			} else {
				inQuotes = !inQuotes
				current.WriteRune(ch)
			}
		case '\\':
			if inQuotes {
				escaped = true
			} else {
				current.WriteRune(ch)
			}
		case ',':
			if inQuotes {
				current.WriteRune(ch)
			} else {
				arg := strings.TrimSpace(current.String())
				if arg != "" {
					val, err := parseValue(arg)
					if err != nil {
						return nil, err
					}
					args = append(args, val)
				}
				current.Reset()
			}
		default:
			current.WriteRune(ch)
		}

		if i == len(s)-1 {
			arg := strings.TrimSpace(current.String())
			if arg != "" {
				val, err := parseValue(arg)
				if err != nil {
					return nil, err
				}
				args = append(args, val)
			}
		}
	}

	if inQuotes {
		return nil, errors.New("unterminated string in arguments")
	}

	return args, nil
}

func parseValue(s string) (*PluginArg, error) {
	var res PluginArg
	res.Type = 0

	if PluginArgFromRawSymbol == s {
		res.Value = s
		res.Type = 2
		return &res, nil
	}

	if (strings.HasPrefix(s, `"`) && strings.HasSuffix(s, `"`)) || (strings.HasPrefix(s, `'`) && strings.HasSuffix(s, `'`)) {
		//need check
		value := s[1 : len(s)-1]
		res.Value = value
		res.RealValue = res.Value
		return &res, nil
	}

	if s == "true" {
		res.Value = true
		res.RealValue = true
		return &res, nil
	}
	if s == "false" {
		res.Value = false
		res.RealValue = false
		return &res, nil
	}

	if i, err := strconv.Atoi(s); err == nil {
		res.Value = i
		res.RealValue = i
		return &res, nil
	}

	if f, err := strconv.ParseFloat(s, 64); err == nil {
		res.Value = f
		res.RealValue = f
		return &res, nil
	}

	if matched, _ := regexpgo.MatchString(`^[a-zA-Z_][a-zA-Z0-9_]*$`, s); matched {
		res.Value = s
		res.Type = 1
		return &res, nil
	}

	return nil, fmt.Errorf("unsupported argument: %s", s)
}

// RulesetBuild parses and validates a Ruleset, initializing all field paths and check functions.
func RulesetBuild(ruleset *Ruleset) error {
	var err error
	//for init local cache, local cache only work for threshold check
	var createLocalCache = false
	var createLocalCacheForClassify = false

	if strings.TrimSpace(ruleset.Type) == "" || strings.TrimSpace(ruleset.Type) == "DETECTION" {
		ruleset.IsDetection = true
	} else if strings.TrimSpace(ruleset.Type) == "WHITELIST" {
		ruleset.IsDetection = false
	} else {
		return errors.New("resource type only support whitelist or detection")
	}

	for i := range ruleset.Rules {
		rule := &ruleset.Rules[i]

		// Validate required fields for rule
		if strings.TrimSpace(rule.ID) == "" {
			return errors.New("rule id cannot be empty")
		}

		for i2 := range ruleset.Rules {
			if strings.TrimSpace(ruleset.Rules[i2].ID) == strings.TrimSpace(rule.ID) && i != i2 {
				return errors.New("rule id cannot be repeated")
			}
		}

		if strings.TrimSpace(rule.Name) == "" {
			return errors.New("rule name cannot be empty")
		}
		if strings.TrimSpace(rule.Author) == "" {
			return errors.New("rule author cannot be empty")
		}

		if strings.TrimSpace(rule.Checklist.Condition) != "" {
			if _, _, ok := ConditionRegex.Find(strings.TrimSpace(rule.Checklist.Condition)); ok {
				rule.Checklist.ConditionAST = GetAST(strings.TrimSpace(rule.Checklist.Condition))
				rule.Checklist.ConditionMap = make(map[string]bool, len(rule.Checklist.CheckNodes))
				rule.Checklist.ConditionFlag = true
			} else {
				return errors.New("checklist condition is not a valid expression")
			}
		}

		for i := range rule.Appends {
			appendNode := &rule.Appends[i]
			appendType := strings.TrimSpace(appendNode.Type)
			appendValue := strings.TrimSpace(appendNode.Value)

			if appendType != "" && appendType != "PLUGIN" {
				return errors.New("append type or field name cannot be empty")
			}

			if appendNode.Type == "PLUGIN" {
				pluginName, args, err := ParseFunctionCall(appendValue)
				if err != nil {
					return err
				}

				if p, ok := plugin.Plugins[pluginName]; ok {
					appendNode.Plugin = p
				} else {
					// Check if it's a temporary component, temporary components should not be referenced
					if _, tempExists := plugin.PluginsNew[pluginName]; tempExists {
						return errors.New("cannot reference temporary plugin '" + pluginName + "', please save it first")
					}
					return errors.New("not found this plugin: " + pluginName)
				}

				appendNode.PluginArgs = args
			}
		}

		for i := range rule.Plugins {
			pluginNode := &rule.Plugins[i]
			value := strings.TrimSpace(pluginNode.Value)

			if value == "" {
				return errors.New("plugin value cannot be empty: " + pluginNode.Plugin.Name)
			}

			pluginName, args, err := ParseFunctionCall(value)
			if err != nil {
				return err
			}

			if p, ok := plugin.Plugins[pluginName]; ok {
				pluginNode.Plugin = p
			} else {
				// Check if it's a temporary component, temporary components should not be referenced
				if _, tempExists := plugin.PluginsNew[pluginName]; tempExists {
					return errors.New("cannot reference temporary plugin '" + pluginName + "', please save it first")
				}
				return errors.New("not fount this plugin: " + pluginName)
			}

			pluginNode.PluginArgs = args
		}

		if rule.Threshold.GroupBy == "" && rule.Threshold.Range == "" && rule.Threshold.Value == 0 {
			rule.ThresholdCheck = false
		} else {
			if rule.Threshold.GroupBy == "" {
				return errors.New("threshold groupby cannot be empty: " + rule.ID)
			}
			if rule.Threshold.Range == "" {
				return errors.New("threshold range cannot be empty: " + rule.ID)
			}
			if rule.Threshold.Value == 0 {
				return errors.New("threshold vaule cannot be empty: " + rule.ID)
			}

			if !(rule.Threshold.CountType == "" || rule.Threshold.CountType == "SUM" || rule.Threshold.CountType == "CLASSIFY") {
				return errors.New("threshold count type must be 'SUM' or 'CLASSIFY': " + rule.ID)
			}

			if rule.Threshold.CountType == "SUM" || rule.Threshold.CountType == "CLASSIFY" {
				if rule.Threshold.CountField == "" {
					return errors.New("threshold count field cannot be empty: " + rule.ID)
				} else {
					// Parse threshold count field path
					rule.Threshold.CountFieldList = common.StringToList(strings.TrimSpace(rule.Threshold.CountField))
				}
			}

			rule.Threshold.RangeInt, err = common.ParseDurationToSecondsInt(rule.Threshold.Range)
			if err != nil {
				return errors.New("threshold parse range err: " + err.Error() + ", rule id: " + rule.ID)
			}

			if !(rule.Threshold.Value > 1) {
				return errors.New("threshold value must be greater than 1: " + rule.ID)
			}

			rule.ThresholdCheck = true
			rule.Threshold.GroupByID = ruleset.RulesetID + rule.ID

			if !createLocalCache {
				ruleset.Cache, err = ristretto.NewCache(&ristretto.Config[string, int]{
					NumCounters: 10_000_000,       // number of keys to track frequency of.
					MaxCost:     1024 * 1024 * 64, // maximum cost of cache.
					BufferItems: 32,               // number of keys per Get buffer.
				})

				if err != nil {
					return fmt.Errorf("failed to create local cache: %w", err)
				}
				createLocalCache = true
			}

			if rule.Threshold.CountType == "CLASSIFY" {
				if !createLocalCacheForClassify {
					ruleset.CacheForClassify, err = ristretto.NewCache(&ristretto.Config[string, map[string]bool]{
						NumCounters: 10_000_000,       // number of keys to track frequency of.
						MaxCost:     1024 * 1024 * 64, // maximum cost of cache.
						BufferItems: 32,               // number of keys per Get buffer.
					})

					if err != nil {
						return fmt.Errorf("failed to create local cache: %w", err)
					}
					createLocalCacheForClassify = true
				}
			}
		}

		thresholdGroupBYList := strings.Split(strings.TrimSpace(rule.Threshold.GroupBy), ",")
		rule.Threshold.GroupByList = make(map[string][]string, len(thresholdGroupBYList))
		for i := range thresholdGroupBYList {
			tmpList := common.StringToList(thresholdGroupBYList[i])
			rule.Threshold.GroupByList[thresholdGroupBYList[i]] = make([]string, len(tmpList))
			rule.Threshold.GroupByList[thresholdGroupBYList[i]] = tmpList
		}

		// Parse filter field path
		rule.Filter.Field = strings.TrimSpace(rule.Filter.Field)
		rule.Filter.FieldList = common.StringToList(strings.TrimSpace(rule.Filter.Field))
		if len(rule.Filter.FieldList) > 0 {
			rule.Filter.Check = true
		}

		// Parse each node's field path and assign check function
		for j := range rule.Checklist.CheckNodes {
			node := &rule.Checklist.CheckNodes[j]
			node.FieldList = common.StringToList(strings.TrimSpace(node.Field))

			if rule.Checklist.ConditionFlag {
				id := strings.TrimSpace(node.ID)
				node.ID = id

				if id == "" {
					return errors.New("check node id cannot be empty: " + rule.ID)
				}

				if _, ok := rule.Checklist.ConditionMap[id]; ok {
					return errors.New("check node id cannot be repeated: " + rule.ID)
				} else {
					rule.Checklist.ConditionMap[id] = false
				}
			}

			switch strings.TrimSpace(node.Type) {
			case "PLUGIN":
				pluginName, args, err := ParseFunctionCall(node.Value)
				if err != nil {
					return err
				}

				if p, ok := plugin.Plugins[pluginName]; ok {
					node.Plugin = p
				} else {
					// Check if it's a temporary component, temporary components should not be referenced
					if _, tempExists := plugin.PluginsNew[pluginName]; tempExists {
						return errors.New("cannot reference temporary plugin '" + pluginName + "', please save it first (rule id: " + rule.ID + ")")
					}
					return errors.New("not found this plugin: " + pluginName + " rule id: " + rule.ID)
				}

				node.PluginArgs = args

			case "END":
				node.CheckFunc = END
			case "START":
				node.CheckFunc = START
			case "NEND":
				node.CheckFunc = NEND
			case "NSTART":
				node.CheckFunc = NSTART
			case "INCL":
				node.CheckFunc = INCL
			case "NI":
				node.CheckFunc = NI
			case "NCS_END":
				node.CheckFunc = NCS_END
			case "NCS_START":
				node.CheckFunc = NCS_START
			case "NCS_NEND":
				node.CheckFunc = NCS_NEND
			case "NCS_NSTART":
				node.CheckFunc = NCS_NSTART
			case "NCS_INCL":
				node.CheckFunc = NCS_INCL
			case "NCS_NI":
				node.CheckFunc = NCS_NI
			case "MT":
				node.CheckFunc = MT
			case "LT":
				node.CheckFunc = LT
			case "REGEX":
				// REGEX handled below
			case "ISNULL":
				node.CheckFunc = ISNULL
			case "NOTNULL":
				node.CheckFunc = NOTNULL
			case "EQU":
				node.CheckFunc = EQU
			case "NEQ":
				node.CheckFunc = NEQ
			case "NCS_EQU":
				node.CheckFunc = NCS_EQU
			case "NCS_NEQ":
				node.CheckFunc = NCS_NEQ
			default:
				return errors.New("unknown check node type, " + common.AnyToString(j) + ", rule id: " + rule.ID)
			}

			// Compile regex if needed
			if node.Type == "REGEX" {
				var err error
				node.Regex, err = regexp.Compile(node.Value)
				if err != nil {
					return err
				}
			}

			if node.Logic != "" || node.Delimiter != "" {
				if node.Logic == "" {
					return errors.New("logic cannot be empty: " + rule.ID)
				}

				if node.Logic != "AND" && node.Logic != "OR" {
					return errors.New("threshold count type must be 'AND' or 'OR': " + rule.ID)
				}

				if node.Delimiter == "" {
					return errors.New("delimiter cannot be empty: " + rule.ID)
				}

				if strings.Contains(strings.TrimSpace(node.Value), node.Delimiter) {
					node.DelimiterFieldList = strings.Split(strings.TrimSpace(node.Value), node.Delimiter)
					if node.Logic == "OR" {
						rule.ChecklistLen = len(rule.Checklist.CheckNodes)
					} else {
						rule.ChecklistLen = len(rule.Checklist.CheckNodes) + len(node.DelimiterFieldList) - 1
					}
				} else {
					return errors.New("check node value does not exist in delimiter: " + rule.ID)
				}
			} else {
				rule.ChecklistLen = len(rule.Checklist.CheckNodes)
			}
		}

		rule.Checklist.CheckNodes = sortCheckNodes(rule.Checklist.CheckNodes)

		delList := strings.Split(strings.TrimSpace(rule.Del), ",")

		rule.DelList = make([][]string, len(delList))
		for i := range delList {
			tmpList := common.StringToList(delList[i])
			rule.DelList[i] = make([]string, len(tmpList))
			rule.DelList[i] = tmpList
		}

		// Cluster rules by filter
		filterStr := rule.Filter.ToStr()
		if rulesByFilter, ok := ruleset.RulesByFilter[filterStr]; ok {
			rulesByFilter.Rules = append(rulesByFilter.Rules, rule)
		} else {
			ruleset.RulesByFilter[filterStr] = &RulesByFilter{
				Filter: rule.Filter,
				Rules:  []*Rule{rule},
			}
		}
	}
	return nil
}

// ParseRulesetFromByte parses XML bytes into a Ruleset struct and processes field paths.
func ParseRulesetFromByte(rawRuleset []byte) (*Ruleset, error) {
	var ruleset Ruleset
	ruleset.RawConfig = string(rawRuleset)
	ruleset.RulesByFilter = make(map[string]*RulesByFilter, 0)

	if err := xml.Unmarshal(rawRuleset, &ruleset); err != nil {
		return nil, err
	}
	err := RulesetBuild(&ruleset)
	if err != nil {
		return nil, err
	}
	return &ruleset, nil
}

func sortCheckNodes(checkNodes []CheckNodes) []CheckNodes {
	sortedIndex := 0
	sorted := make([]CheckNodes, len(checkNodes))

	tier1 := make([]int, 0)
	tier2 := make([]int, 0)
	tier3 := make([]int, 0)
	tier4 := make([]int, 0)

	for i, v := range checkNodes {
		if v.Type == "ISNULL" || v.Type == "NOTNULL" {
			tier1 = append(tier1, i)
		} else if v.Type == "REGEX" {
			tier3 = append(tier3, i)
		} else if v.Type == "PLUGIN" {
			tier4 = append(tier4, i)
		} else {
			tier2 = append(tier2, i)
		}
	}

	for _, i := range tier1 {
		sorted[sortedIndex] = checkNodes[i]
		sortedIndex = sortedIndex + 1
	}

	for _, i := range tier2 {
		sorted[sortedIndex] = checkNodes[i]
		sortedIndex = sortedIndex + 1
	}

	for _, i := range tier3 {
		sorted[sortedIndex] = checkNodes[i]
		sortedIndex = sortedIndex + 1
	}

	for _, i := range tier4 {
		sorted[sortedIndex] = checkNodes[i]
		sortedIndex = sortedIndex + 1
	}

	return sorted
}

// Reload reloads the ruleset from its source file or raw config
func (r *Ruleset) Reload() error {
	var rawRuleset []byte
	var err error

	if r.Path != "" {
		xmlFile, err := os.Open(r.Path)
		if err != nil {
			return fmt.Errorf("failed to open ruleset file at %s: %w", r.Path, err)
		}
		defer xmlFile.Close()

		rawRuleset, err = io.ReadAll(xmlFile)
		if err != nil {
			return fmt.Errorf("failed to read ruleset file: %w", err)
		}
	} else if r.RawConfig != "" {
		rawRuleset = []byte(r.RawConfig)
	} else {
		return fmt.Errorf("no source available to reload ruleset")
	}

	newRuleset, err := ParseRulesetFromByte(rawRuleset)
	if err != nil {
		return fmt.Errorf("failed to parse ruleset: %w", err)
	}

	// Update the current ruleset with new data
	r.XMLName = newRuleset.XMLName
	r.Type = newRuleset.Type
	r.IsDetection = newRuleset.IsDetection
	r.Rules = newRuleset.Rules
	r.RulesByFilter = newRuleset.RulesByFilter

	// Build the ruleset
	return RulesetBuild(r)
}
