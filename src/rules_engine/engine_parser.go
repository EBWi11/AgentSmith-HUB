package rules_engine

import (
	"AgentSmith-HUB/common"
	"AgentSmith-HUB/plugin"
	"encoding/xml"
	"fmt"
	"io"
	"strconv"
	"strings"

	regexp "github.com/BurntSushi/rure-go"
)

// XMLDecoder is a wrapper around xml.Decoder that tracks line numbers
type XMLDecoder struct {
	*xml.Decoder
	line int
}

// NewXMLDecoder creates a new XMLDecoder
func NewXMLDecoder(r io.Reader) *XMLDecoder {
	return &XMLDecoder{
		Decoder: xml.NewDecoder(r),
		line:    1,
	}
}

// Token wraps the xml.Decoder Token method to track line numbers
func (d *XMLDecoder) Token() (xml.Token, error) {
	token, err := d.Decoder.Token()
	if err != nil {
		return token, err
	}

	// Count newlines in character data to track line numbers
	switch t := token.(type) {
	case xml.CharData:
		d.line += strings.Count(string(t), "\n")
	}

	return token, nil
}

func ParseRuleset(rawRuleset []byte) (*Ruleset, error) {
	// Create a custom decoder that tracks line numbers
	content := string(rawRuleset)
	decoder := NewXMLDecoder(strings.NewReader(content))

	var ruleset Ruleset
	var currentRule *Rule
	var currentChecklist *Checklist
	var inChecklist bool
	var operatorIDCounter int
	var currentLine int = 1

	for {
		// Track current line before getting token
		currentLine = decoder.line

		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("error parsing XML at line %d: %v", currentLine, err)
		}

		switch element := token.(type) {
		case xml.StartElement:
			// Store line number for this element
			elementLine := currentLine

			switch element.Name.Local {
			case "root":
				// Parse root attributes with validation
				for _, attr := range element.Attr {
					switch attr.Name.Local {
					case "type":
						if attr.Value != "DETECTION" && attr.Value != "WHITELIST" {
							return nil, fmt.Errorf("root type must be 'DETECTION' or 'WHITELIST', got '%s' at line %d", attr.Value, elementLine)
						}
						ruleset.Type = attr.Value
						ruleset.IsDetection = strings.ToUpper(attr.Value) == "DETECTION"
					case "name":
						ruleset.Name = attr.Value
					case "author":
						ruleset.Author = attr.Value
					}
				}

			case "rule":
				// Start a new rule
				currentRule = &Rule{
					Queue:        &[]EngineOperator{},
					ChecklistMap: make(map[int]Checklist),
					CheckMap:     make(map[int]CheckNodes),
					ThresholdMap: make(map[int]Threshold),
					AppendsMap:   make(map[int]Append),
					PluginMap:    make(map[int]Plugin),
					DelMap:       make(map[int][][]string),
				}

				// Parse rule attributes
				for _, attr := range element.Attr {
					switch attr.Name.Local {
					case "id":
						if strings.TrimSpace(attr.Value) == "" {
							return nil, fmt.Errorf("rule id cannot be empty at line %d", elementLine)
						}
						currentRule.ID = attr.Value
					case "name":
						currentRule.Name = attr.Value
					}
				}

				if currentRule.ID == "" {
					return nil, fmt.Errorf("rule id is required at line %d", elementLine)
				}

			case "checklist":
				if currentRule != nil {
					inChecklist = true
					currentChecklist = &Checklist{
						CheckNodes: []CheckNodes{},
					}

					// Parse checklist attributes
					for _, attr := range element.Attr {
						if attr.Name.Local == "condition" {
							condition := strings.TrimSpace(attr.Value)
							if condition == "" {
								return nil, fmt.Errorf("checklist condition cannot be empty at line %d", elementLine)
							}
							// Validate condition syntax
							if _, _, ok := ConditionRegex.Find(condition); !ok {
								return nil, fmt.Errorf("checklist condition is not a valid expression: %s at line %d", condition, elementLine)
							}
							currentChecklist.Condition = condition
							currentChecklist.ConditionFlag = true
							currentChecklist.ConditionAST = GetAST(condition)
							currentChecklist.ConditionMap = make(map[string]bool)
						}
					}
				}

			case "check":
				if currentRule != nil {
					checkNode, err := parseCheckNode(element, decoder, elementLine)
					if err != nil {
						return nil, err
					}

					if inChecklist && currentChecklist != nil {
						// Add to current checklist
						currentChecklist.CheckNodes = append(currentChecklist.CheckNodes, checkNode)
					} else {
						// Standalone check node
						operatorIDCounter++
						currentRule.CheckMap[operatorIDCounter] = checkNode
						*currentRule.Queue = append(*currentRule.Queue, EngineOperator{
							Type: T_Check,
							ID:   operatorIDCounter,
						})
					}
				}

			case "threshold":
				if currentRule != nil {
					threshold, err := parseThreshold(element, decoder, elementLine)
					if err != nil {
						return nil, err
					}

					operatorIDCounter++
					currentRule.ThresholdMap[operatorIDCounter] = threshold
					*currentRule.Queue = append(*currentRule.Queue, EngineOperator{
						Type: T_Threshold,
						ID:   operatorIDCounter,
					})
				}

			case "append":
				if currentRule != nil {
					appendOp, err := parseAppend(element, decoder, elementLine)
					if err != nil {
						return nil, err
					}

					operatorIDCounter++
					currentRule.AppendsMap[operatorIDCounter] = appendOp
					*currentRule.Queue = append(*currentRule.Queue, EngineOperator{
						Type: T_Append,
						ID:   operatorIDCounter,
					})
				}

			case "plugin":
				if currentRule != nil {
					plugin, err := parsePlugin(element, decoder, elementLine)
					if err != nil {
						return nil, err
					}

					operatorIDCounter++
					currentRule.PluginMap[operatorIDCounter] = plugin
					*currentRule.Queue = append(*currentRule.Queue, EngineOperator{
						Type: T_Plugin,
						ID:   operatorIDCounter,
					})
				}

			case "del":
				if currentRule != nil {
					delFields, err := parseDel(element, decoder, elementLine)
					if err != nil {
						return nil, err
					}

					operatorIDCounter++
					currentRule.DelMap[operatorIDCounter] = delFields
					*currentRule.Queue = append(*currentRule.Queue, EngineOperator{
						Type: T_Del,
						ID:   operatorIDCounter,
					})
				}

			default:
				// Handle unsupported elements
				if currentRule != nil {
					// Inside a rule, check for common mistakes
					if element.Name.Local == "node" {
						return nil, fmt.Errorf("unsupported element '<%s>' in rule '%s' at line %d. The 'node' tag has been deprecated, please use 'check' instead", element.Name.Local, currentRule.ID, elementLine)
					} else if element.Name.Local == "filter" {
						return nil, fmt.Errorf("unsupported element '<%s>' in rule '%s' at line %d. The 'filter' tag has been removed in the new syntax", element.Name.Local, currentRule.ID, elementLine)
					} else if inChecklist {
						return nil, fmt.Errorf("unsupported element '<%s>' inside checklist in rule '%s' at line %d", element.Name.Local, currentRule.ID, elementLine)
					} else {
						return nil, fmt.Errorf("unsupported element '<%s>' in rule '%s' at line %d", element.Name.Local, currentRule.ID, elementLine)
					}
				} else {
					// Outside of rules, only certain elements are allowed at root level
					return nil, fmt.Errorf("unsupported element '<%s>' at root level at line %d", element.Name.Local, elementLine)
				}
			}

		case xml.EndElement:
			switch element.Name.Local {
			case "checklist":
				if currentRule != nil && inChecklist && currentChecklist != nil {
					operatorIDCounter++
					currentRule.ChecklistMap[operatorIDCounter] = *currentChecklist
					*currentRule.Queue = append(*currentRule.Queue, EngineOperator{
						Type: T_CheckList,
						ID:   operatorIDCounter,
					})
					inChecklist = false
					currentChecklist = nil
				}

			case "rule":
				if currentRule != nil {
					// Convert to final rule structure
					ruleset.Rules = append(ruleset.Rules, *currentRule)
					currentRule = nil
				}
			}
		}
	}

	ruleset.RulesCount = len(ruleset.Rules)
	// Initialize ruleset status to stopped
	ruleset.Status = common.StatusStopped
	return &ruleset, nil
}

func parseCheckNode(element xml.StartElement, decoder *XMLDecoder, elementLine int) (CheckNodes, error) {
	var checkNode CheckNodes

	// Parse attributes with validation
	for _, attr := range element.Attr {
		switch attr.Name.Local {
		case "type":
			checkType := strings.TrimSpace(attr.Value)
			if checkType == "" {
				return checkNode, fmt.Errorf("check type cannot be empty at line %d", elementLine)
			}
			checkNode.Type = checkType
		case "field":
			field := strings.TrimSpace(attr.Value)
			// Check if field is empty and type is not PLUGIN (need to check checkNode.Type)
			if field == "" && checkNode.Type != "PLUGIN" {
				return checkNode, fmt.Errorf("check field cannot be empty at line %d", elementLine)
			}
			checkNode.Field = field
		case "id":
			checkNode.ID = strings.TrimSpace(attr.Value)
		case "logic":
			logic := strings.TrimSpace(attr.Value)
			if logic != "" && logic != "OR" && logic != "AND" {
				return checkNode, fmt.Errorf("check logic must be 'OR' or 'AND', got '%s' at line %d", logic, elementLine)
			}
			checkNode.Logic = logic
		case "delimiter":
			checkNode.Delimiter = attr.Value
		}
	}

	// Validate required attributes
	if checkNode.Type == "" {
		return checkNode, fmt.Errorf("check type is required at line %d", elementLine)
	}

	for {
		token, err := decoder.Token()
		if err != nil {
			return checkNode, err
		}

		switch t := token.(type) {
		case xml.CharData:
			value := strings.TrimSpace(string(t))
			if checkNode.Type == "REGEX" && value == "" {
				return checkNode, fmt.Errorf("REGEX node value cannot be empty at line %d", elementLine)
			}
			if checkNode.Type == "PLUGIN" && value == "" {
				return checkNode, fmt.Errorf("PLUGIN node value cannot be empty at line %d", elementLine)
			}
			checkNode.Value = value
		case xml.EndElement:
			if t.Name.Local == "check" {
				// Additional validation
				if checkNode.Type == "REGEX" && checkNode.Value != "" {
					// Validate regex pattern
					if _, err := regexp.Compile(checkNode.Value); err != nil {
						return checkNode, fmt.Errorf("invalid regex pattern at line %d: %v", elementLine, err)
					}
				}

				if checkNode.Type == "PLUGIN" && checkNode.Value != "" {
					// Validate plugin call syntax
					pluginName, args, isNegated, err := ParseCheckNodePluginCall(checkNode.Value)
					if err != nil {
						return checkNode, fmt.Errorf("invalid plugin call syntax at line %d: %v", elementLine, err)
					}

					// Check if plugin exists
					if _, ok := plugin.Plugins[pluginName]; !ok {
						if _, tempExists := plugin.PluginsNew[pluginName]; tempExists {
							return checkNode, fmt.Errorf("cannot reference temporary plugin '%s' at line %d, please save it first", pluginName, elementLine)
						}
						return checkNode, fmt.Errorf("plugin not found: %s at line %d", pluginName, elementLine)
					}

					// Store parsed plugin info with negation flag
					pluginCopy := *plugin.Plugins[pluginName]
					pluginCopy.IsNegated = isNegated
					checkNode.Plugin = &pluginCopy
					checkNode.PluginArgs = args
				}

				// Validate logic and delimiter combination
				if checkNode.Logic != "" && checkNode.Delimiter == "" {
					return checkNode, fmt.Errorf("delimiter cannot be empty when logic is specified at line %d", elementLine)
				}
				if checkNode.Logic == "" && checkNode.Delimiter != "" {
					return checkNode, fmt.Errorf("logic cannot be empty when delimiter is specified at line %d", elementLine)
				}

				return checkNode, nil
			}
		}
	}
}

func parseThreshold(element xml.StartElement, decoder *XMLDecoder, elementLine int) (Threshold, error) {
	var threshold Threshold

	// Parse attributes with validation
	for _, attr := range element.Attr {
		switch attr.Name.Local {
		case "group_by":
			group_by := strings.TrimSpace(attr.Value)
			if group_by == "" {
				return threshold, fmt.Errorf("threshold group_by cannot be empty at line %d", elementLine)
			}
			threshold.group_by = group_by
		case "range":
			rangeValue := strings.TrimSpace(attr.Value)
			if rangeValue == "" {
				return threshold, fmt.Errorf("threshold range cannot be empty at line %d", elementLine)
			}
			threshold.Range = rangeValue
		case "value":
			// Old syntax support - value as attribute
			if val, err := strconv.Atoi(attr.Value); err != nil {
				return threshold, fmt.Errorf("threshold value must be a positive integer, got '%s' at line %d", attr.Value, elementLine)
			} else if val <= 0 {
				return threshold, fmt.Errorf("threshold value must be greater than 0, got %d at line %d", val, elementLine)
			} else {
				threshold.Value = val
			}
		case "count_type":
			countType := strings.TrimSpace(attr.Value)
			if countType != "" && countType != "SUM" && countType != "CLASSIFY" {
				return threshold, fmt.Errorf("threshold count_type must be empty (default count mode), 'SUM', or 'CLASSIFY', got '%s' at line %d", countType, elementLine)
			}
			threshold.CountType = countType
		case "count_field":
			threshold.CountField = strings.TrimSpace(attr.Value)
		case "local_cache":
			localCache := strings.TrimSpace(attr.Value)
			if localCache != "" && localCache != "true" && localCache != "false" {
				return threshold, fmt.Errorf("threshold local_cache must be 'true' or 'false', got '%s' at line %d", localCache, elementLine)
			}
			threshold.LocalCache = localCache == "true"
		}
	}

	// Validate required attributes early
	if threshold.group_by == "" {
		return threshold, fmt.Errorf("threshold group_by is required at line %d", elementLine)
	}

	for {
		token, err := decoder.Token()
		if err != nil {
			return threshold, err
		}

		switch t := token.(type) {
		case xml.CharData:
			content := strings.TrimSpace(string(t))
			if content != "" {
				if val, err := strconv.Atoi(content); err != nil {
					return threshold, fmt.Errorf("threshold value must be a positive integer, got '%s' at line %d", content, elementLine)
				} else if val <= 0 {
					return threshold, fmt.Errorf("threshold value must be greater than 0, got %d at line %d", val, elementLine)
				} else {
					threshold.Value = val
				}
			}
		case xml.EndElement:
			if t.Name.Local == "threshold" {
				// Additional validation
				if threshold.Range == "" {
					return threshold, fmt.Errorf("threshold range is required at line %d", elementLine)
				}
				if threshold.Value <= 0 {
					return threshold, fmt.Errorf("threshold value is required and must be positive at line %d", elementLine)
				}

				// Validate count_field requirement
				if (threshold.CountType == "SUM" || threshold.CountType == "CLASSIFY") && threshold.CountField == "" {
					return threshold, fmt.Errorf("threshold count_field cannot be empty when count_type is '%s' at line %d", threshold.CountType, elementLine)
				}

				return threshold, nil
			}
		}
	}
}

func parseAppend(element xml.StartElement, decoder *XMLDecoder, elementLine int) (Append, error) {
	var appendElem Append

	// Parse attributes with validation
	for _, attr := range element.Attr {
		switch attr.Name.Local {
		case "type":
			appendType := strings.TrimSpace(attr.Value)
			if appendType != "" && appendType != "PLUGIN" {
				return appendElem, fmt.Errorf("append type must be empty or 'PLUGIN', got '%s' at line %d", appendType, elementLine)
			}
			appendElem.Type = appendType
		case "field":
			field := strings.TrimSpace(attr.Value)
			if field == "" {
				return appendElem, fmt.Errorf("append field cannot be empty at line %d", elementLine)
			}
			appendElem.FieldName = field
		}
	}

	// Parse content
	for {
		token, err := decoder.Token()
		if err != nil {
			return appendElem, err
		}

		switch t := token.(type) {
		case xml.CharData:
			value := strings.TrimSpace(string(t))
			if appendElem.Type == "PLUGIN" && value == "" {
				return appendElem, fmt.Errorf("append plugin value cannot be empty at line %d", elementLine)
			}
			appendElem.Value = value
		case xml.EndElement:
			if t.Name.Local == "append" {
				// Additional validation
				if appendElem.FieldName == "" {
					return appendElem, fmt.Errorf("append field is required at line %d", elementLine)
				}

				if appendElem.Type == "PLUGIN" && appendElem.Value != "" {
					// Validate plugin call syntax
					pluginName, args, err := ParseFunctionCall(appendElem.Value)
					if err != nil {
						return appendElem, fmt.Errorf("invalid plugin call syntax at line %d: %v", elementLine, err)
					}

					// Check if plugin exists
					if _, ok := plugin.Plugins[pluginName]; !ok {
						if _, tempExists := plugin.PluginsNew[pluginName]; tempExists {
							return appendElem, fmt.Errorf("cannot reference temporary plugin '%s' at line %d, please save it first", pluginName, elementLine)
						}
						return appendElem, fmt.Errorf("plugin not found: %s at line %d", pluginName, elementLine)
					}

					// Store parsed plugin info
					appendElem.Plugin = plugin.Plugins[pluginName]
					appendElem.PluginArgs = args
				}

				return appendElem, nil
			}
		}
	}
}

func parsePlugin(element xml.StartElement, decoder *XMLDecoder, elementLine int) (Plugin, error) {
	var pluginElem Plugin

	// Parse content
	for {
		token, err := decoder.Token()
		if err != nil {
			return pluginElem, err
		}

		switch t := token.(type) {
		case xml.CharData:
			value := strings.TrimSpace(string(t))
			if value == "" {
				return pluginElem, fmt.Errorf("plugin value cannot be empty at line %d", elementLine)
			}
			pluginElem.Value = value
		case xml.EndElement:
			if t.Name.Local == "plugin" {
				// Validate plugin call syntax
				pluginName, args, err := ParseFunctionCall(pluginElem.Value)
				if err != nil {
					return pluginElem, fmt.Errorf("invalid plugin call syntax at line %d: %v", elementLine, err)
				}

				// Check if plugin exists
				if _, ok := plugin.Plugins[pluginName]; !ok {
					if _, tempExists := plugin.PluginsNew[pluginName]; tempExists {
						return pluginElem, fmt.Errorf("cannot reference temporary plugin '%s' at line %d, please save it first", pluginName, elementLine)
					}
					return pluginElem, fmt.Errorf("plugin not found: %s at line %d", pluginName, elementLine)
				}

				// Store parsed plugin info
				pluginElem.Plugin = plugin.Plugins[pluginName]
				pluginElem.PluginArgs = args

				return pluginElem, nil
			}
		}
	}
}

func parseDel(element xml.StartElement, decoder *XMLDecoder, elementLine int) ([][]string, error) {
	var delFields [][]string

	// Parse content
	for {
		token, err := decoder.Token()
		if err != nil {
			return delFields, err
		}

		switch t := token.(type) {
		case xml.CharData:
			content := strings.TrimSpace(string(t))
			if content == "" {
				return delFields, fmt.Errorf("del content cannot be empty at line %d", elementLine)
			}

			fields := strings.Split(content, ",")
			for _, field := range fields {
				field = strings.TrimSpace(field)
				if field != "" {
					fieldPath := strings.Split(field, ".")
					delFields = append(delFields, fieldPath)
				}
			}
		case xml.EndElement:
			if t.Name.Local == "del" {
				if len(delFields) == 0 {
					return delFields, fmt.Errorf("del must specify at least one field at line %d", elementLine)
				}
				return delFields, nil
			}
		}
	}
}
