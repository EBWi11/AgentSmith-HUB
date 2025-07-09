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

func ParseRuleset(rawRuleset []byte) (*Ruleset, error) {
	decoder := xml.NewDecoder(strings.NewReader(string(rawRuleset)))

	var ruleset Ruleset
	var currentRule *Rule
	var currentChecklist *Checklist
	var inChecklist bool
	var operatorIDCounter int

	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("error parsing XML: %v", err)
		}

		switch element := token.(type) {
		case xml.StartElement:
			switch element.Name.Local {
			case "root":
				// Parse root attributes with validation
				for _, attr := range element.Attr {
					switch attr.Name.Local {
					case "type":
						if attr.Value != "DETECTION" && attr.Value != "WHITELIST" {
							return nil, fmt.Errorf("root type must be 'DETECTION' or 'WHITELIST', got '%s'", attr.Value)
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
							return nil, fmt.Errorf("rule id cannot be empty")
						}
						currentRule.ID = attr.Value
					case "name":
						currentRule.Name = attr.Value
					}
				}

				if currentRule.ID == "" {
					return nil, fmt.Errorf("rule id is required")
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
								return nil, fmt.Errorf("checklist condition cannot be empty")
							}
							// Validate condition syntax
							if _, _, ok := ConditionRegex.Find(condition); !ok {
								return nil, fmt.Errorf("checklist condition is not a valid expression: %s", condition)
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
					checkNode, err := parseCheckNode(element, decoder)
					if err != nil {
						return nil, fmt.Errorf("error parsing check node: %v", err)
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
					threshold, err := parseThreshold(element, decoder)
					if err != nil {
						return nil, fmt.Errorf("error parsing threshold: %v", err)
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
					appendOp, err := parseAppend(element, decoder)
					if err != nil {
						return nil, fmt.Errorf("error parsing append: %v", err)
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
					plugin, err := parsePlugin(element, decoder)
					if err != nil {
						return nil, fmt.Errorf("error parsing plugin: %v", err)
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
					delFields, err := parseDel(element, decoder)
					if err != nil {
						return nil, fmt.Errorf("error parsing del: %v", err)
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
						return nil, fmt.Errorf("unsupported element '<%s>' in rule '%s'. The 'node' tag has been deprecated, please use 'check' instead", element.Name.Local, currentRule.ID)
					} else if element.Name.Local == "filter" {
						return nil, fmt.Errorf("unsupported element '<%s>' in rule '%s'. The 'filter' tag has been removed in the new syntax", element.Name.Local, currentRule.ID)
					} else if inChecklist {
						return nil, fmt.Errorf("unsupported element '<%s>' inside checklist in rule '%s'", element.Name.Local, currentRule.ID)
					} else {
						return nil, fmt.Errorf("unsupported element '<%s>' in rule '%s'", element.Name.Local, currentRule.ID)
					}
				} else {
					// Outside of rules, only certain elements are allowed at root level
					return nil, fmt.Errorf("unsupported element '<%s>' at root level", element.Name.Local)
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
	return &ruleset, nil
}

func parseCheckNode(element xml.StartElement, decoder *xml.Decoder) (CheckNodes, error) {
	var checkNode CheckNodes

	// Parse attributes with validation
	for _, attr := range element.Attr {
		switch attr.Name.Local {
		case "id":
			checkNode.ID = attr.Value
		case "type":
			nodeType := strings.TrimSpace(attr.Value)
			if nodeType == "" {
				return checkNode, fmt.Errorf("check node type cannot be empty")
			}
			// Validate node type
			validTypes := []string{
				"PLUGIN", "END", "START", "NEND", "NSTART", "INCL", "NI",
				"NCS_END", "NCS_START", "NCS_NEND", "NCS_NSTART", "NCS_INCL", "NCS_NI",
				"MT", "LT", "REGEX", "ISNULL", "NOTNULL", "EQU", "NEQ", "NCS_EQU", "NCS_NEQ",
			}
			isValid := false
			for _, validType := range validTypes {
				if nodeType == validType {
					isValid = true
					break
				}
			}
			if !isValid {
				return checkNode, fmt.Errorf("check node type must be one of: %s, got '%s'", strings.Join(validTypes, ", "), nodeType)
			}
			checkNode.Type = nodeType
		case "field":
			field := strings.TrimSpace(attr.Value)
			if field == "" && checkNode.Type != "PLUGIN" {
				return checkNode, fmt.Errorf("check node field cannot be empty for type '%s'", checkNode.Type)
			}
			checkNode.Field = field
		case "logic":
			logic := strings.TrimSpace(attr.Value)
			if logic != "" && logic != "AND" && logic != "OR" {
				return checkNode, fmt.Errorf("check node logic must be 'AND' or 'OR', got '%s'", logic)
			}
			checkNode.Logic = logic
		case "delimiter":
			checkNode.Delimiter = attr.Value
		}
	}

	// Parse content
	for {
		token, err := decoder.Token()
		if err != nil {
			return checkNode, err
		}

		switch t := token.(type) {
		case xml.CharData:
			value := strings.TrimSpace(string(t))
			if checkNode.Type == "REGEX" && value == "" {
				return checkNode, fmt.Errorf("REGEX node value cannot be empty")
			}
			if checkNode.Type == "PLUGIN" && value == "" {
				return checkNode, fmt.Errorf("PLUGIN node value cannot be empty")
			}
			checkNode.Value = value
		case xml.EndElement:
			if t.Name.Local == "check" {
				// Additional validation
				if checkNode.Type == "REGEX" && checkNode.Value != "" {
					// Validate regex pattern
					if _, err := regexp.Compile(checkNode.Value); err != nil {
						return checkNode, fmt.Errorf("invalid regex pattern: %v", err)
					}
				}

				if checkNode.Type == "PLUGIN" && checkNode.Value != "" {
					// Validate plugin call syntax
					pluginName, args, err := ParseFunctionCall(checkNode.Value)
					if err != nil {
						return checkNode, fmt.Errorf("invalid plugin call syntax: %v", err)
					}

					// Check if plugin exists
					if _, ok := plugin.Plugins[pluginName]; !ok {
						if _, tempExists := plugin.PluginsNew[pluginName]; tempExists {
							return checkNode, fmt.Errorf("cannot reference temporary plugin '%s', please save it first", pluginName)
						}
						return checkNode, fmt.Errorf("plugin not found: %s", pluginName)
					}

					// Store parsed plugin info
					checkNode.Plugin = plugin.Plugins[pluginName]
					checkNode.PluginArgs = args
				}

				// Validate logic and delimiter combination
				if checkNode.Logic != "" && checkNode.Delimiter == "" {
					return checkNode, fmt.Errorf("delimiter cannot be empty when logic is specified")
				}
				if checkNode.Logic == "" && checkNode.Delimiter != "" {
					return checkNode, fmt.Errorf("logic cannot be empty when delimiter is specified")
				}

				return checkNode, nil
			}
		}
	}
}

func parseThreshold(element xml.StartElement, decoder *xml.Decoder) (Threshold, error) {
	var threshold Threshold

	// Parse attributes with validation
	for _, attr := range element.Attr {
		switch attr.Name.Local {
		case "group_by":
			groupBy := strings.TrimSpace(attr.Value)
			if groupBy == "" {
				return threshold, fmt.Errorf("threshold group_by cannot be empty")
			}
			threshold.GroupBy = groupBy
		case "range":
			rangeStr := strings.TrimSpace(attr.Value)
			if rangeStr == "" {
				return threshold, fmt.Errorf("threshold range cannot be empty")
			}
			// Validate range format
			if _, err := common.ParseDurationToSecondsInt(rangeStr); err != nil {
				return threshold, fmt.Errorf("invalid threshold range format: %v", err)
			}
			threshold.Range = rangeStr
		case "local_cache":
			localCache := strings.TrimSpace(attr.Value)
			if localCache != "true" && localCache != "false" {
				return threshold, fmt.Errorf("threshold local_cache must be 'true' or 'false', got '%s'", localCache)
			}
			threshold.LocalCache = localCache == "true"
		case "count_type":
			countType := strings.TrimSpace(attr.Value)
			if countType != "" && countType != "SUM" && countType != "CLASSIFY" {
				return threshold, fmt.Errorf("threshold count_type must be empty, 'SUM', or 'CLASSIFY', got '%s'", countType)
			}
			threshold.CountType = countType
		case "count_field":
			countField := strings.TrimSpace(attr.Value)
			threshold.CountField = countField
		}
	}

	// Parse content (threshold value)
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
					return threshold, fmt.Errorf("threshold value must be a positive integer, got '%s'", content)
				} else if val <= 0 {
					return threshold, fmt.Errorf("threshold value must be greater than 0, got %d", val)
				} else {
					threshold.Value = val
				}
			}
		case xml.EndElement:
			if t.Name.Local == "threshold" {
				// Additional validation
				if threshold.GroupBy == "" {
					return threshold, fmt.Errorf("threshold group_by is required")
				}
				if threshold.Range == "" {
					return threshold, fmt.Errorf("threshold range is required")
				}
				if threshold.Value <= 0 {
					return threshold, fmt.Errorf("threshold value is required and must be positive")
				}

				// Validate count_field requirement
				if (threshold.CountType == "SUM" || threshold.CountType == "CLASSIFY") && threshold.CountField == "" {
					return threshold, fmt.Errorf("threshold count_field cannot be empty when count_type is '%s'", threshold.CountType)
				}

				return threshold, nil
			}
		}
	}
}

func parseAppend(element xml.StartElement, decoder *xml.Decoder) (Append, error) {
	var appendElem Append

	// Parse attributes with validation
	for _, attr := range element.Attr {
		switch attr.Name.Local {
		case "type":
			appendType := strings.TrimSpace(attr.Value)
			if appendType != "" && appendType != "PLUGIN" {
				return appendElem, fmt.Errorf("append type must be empty or 'PLUGIN', got '%s'", appendType)
			}
			appendElem.Type = appendType
		case "field":
			field := strings.TrimSpace(attr.Value)
			if field == "" {
				return appendElem, fmt.Errorf("append field cannot be empty")
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
				return appendElem, fmt.Errorf("append plugin value cannot be empty")
			}
			appendElem.Value = value
		case xml.EndElement:
			if t.Name.Local == "append" {
				// Additional validation
				if appendElem.FieldName == "" {
					return appendElem, fmt.Errorf("append field is required")
				}

				if appendElem.Type == "PLUGIN" && appendElem.Value != "" {
					// Validate plugin call syntax
					pluginName, args, err := ParseFunctionCall(appendElem.Value)
					if err != nil {
						return appendElem, fmt.Errorf("invalid plugin call syntax: %v", err)
					}

					// Check if plugin exists
					if _, ok := plugin.Plugins[pluginName]; !ok {
						if _, tempExists := plugin.PluginsNew[pluginName]; tempExists {
							return appendElem, fmt.Errorf("cannot reference temporary plugin '%s', please save it first", pluginName)
						}
						return appendElem, fmt.Errorf("plugin not found: %s", pluginName)
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

func parsePlugin(element xml.StartElement, decoder *xml.Decoder) (Plugin, error) {
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
				return pluginElem, fmt.Errorf("plugin value cannot be empty")
			}
			pluginElem.Value = value
		case xml.EndElement:
			if t.Name.Local == "plugin" {
				// Validate plugin call syntax
				pluginName, args, err := ParseFunctionCall(pluginElem.Value)
				if err != nil {
					return pluginElem, fmt.Errorf("invalid plugin call syntax: %v", err)
				}

				// Check if plugin exists
				if _, ok := plugin.Plugins[pluginName]; !ok {
					if _, tempExists := plugin.PluginsNew[pluginName]; tempExists {
						return pluginElem, fmt.Errorf("cannot reference temporary plugin '%s', please save it first", pluginName)
					}
					return pluginElem, fmt.Errorf("plugin not found: %s", pluginName)
				}

				// Store parsed plugin info
				pluginElem.Plugin = plugin.Plugins[pluginName]
				pluginElem.PluginArgs = args

				return pluginElem, nil
			}
		}
	}
}

func parseDel(element xml.StartElement, decoder *xml.Decoder) ([][]string, error) {
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
				return delFields, fmt.Errorf("del content cannot be empty")
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
					return delFields, fmt.Errorf("del must specify at least one field")
				}
				return delFields, nil
			}
		}
	}
}
