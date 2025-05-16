package rules_engine

import (
	"AgentSmith-HUB/common"
	"encoding/xml"
	"errors"
	regexp "github.com/BurntSushi/rure-go"
	"strconv"
	"strings"
)

// FromRawSymbol is the prefix indicating a value should be fetched from raw data.
const FromRawSymbol = "_$"
const FromRawSymbolLen = len(FromRawSymbol)

// Ruleset represents a collection of rules and associated metadata.
type Ruleset struct {
	XMLName     xml.Name `xml:"root"`
	RulesetID   string   `xml:"ruleset_id,attr"`
	RulesetName string   `xml:"ruleset_name,attr"`
	Type        string   `xml:"type,attr"`
	IsDetection bool
	Rules       []Rule `xml:"rule"`
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
	Del            string     `xml:"del"`
	DelList        [][]string // parsed field path
}

// Filter defines the field and value for rule filtering.
type Filter struct {
	Field     string   `xml:"field,attr"`
	FieldList []string // parsed field path
	Value     string   `xml:",chardata"`
}

// Checklist contains the logical condition and nodes to check.
type Checklist struct {
	Condition  string       `xml:"condition,attr"`
	CheckNodes []CheckNodes `xml:"node"`
}

// CheckNodes represents a single check operation in a checklist.
type CheckNodes struct {
	ID                 string                              `xml:"id,attr"`
	Type               string                              `xml:"type,attr"`
	CheckFunc          func(string, string) (bool, string) // function pointer for check logic
	Field              string                              `xml:"field,attr"`
	FieldList          []string                            // parsed field path
	Logic              string                              `xml:"logic,attr"`
	Delimiter          string                              `xml:"delimiter,attr"`
	DelimiterFieldList []string
	Value              string `xml:",chardata"`
	Regex              *regexp.Regex
}

// Threshold defines aggregation and counting logic for a rule.
type Threshold struct {
	GroupBy        string `xml:"group_by,attr"`
	GroupByList    map[string][]string
	Range          string `xml:"range,attr"`
	RangeInt       int
	LocalCache     string   `xml:"local_cache,attr"`
	CountType      string   `xml:"count_type,attr"`
	CountField     string   `xml:"count_field,attr"`
	CountFieldList []string // parsed field path
	Value          int      `xml:",chardata"`
}

// Append defines additional fields to append after rule matching.
type Append struct {
	Type      string `xml:"type,attr"`
	FieldName string `xml:"field_name,attr"`
	Value     string `xml:",chardata"`
}

// rulesetBuild parses and validates a Ruleset, initializing all field paths and check functions.
func rulesetBuild(ruleset *Ruleset) error {
	var err error

	if strings.TrimSpace(ruleset.RulesetID) == "" {
		return errors.New("RulesetID cannot be empty")
	}

	if strings.TrimSpace(ruleset.RulesetName) == "" {
		return errors.New("RulesetName cannot be empty")
	}

	if strings.TrimSpace(ruleset.Type) == "" || strings.TrimSpace(ruleset.Type) == "DETECTION" {
		ruleset.IsDetection = true
	} else if strings.TrimSpace(ruleset.Type) == "WHITELIST" {
		ruleset.IsDetection = false
	} else {
		return errors.New("ruleset Type Only SUPPORT WHITELIST OR DETECTION")
	}

	for i := range ruleset.Rules {
		rule := &ruleset.Rules[i]

		// Validate required fields for rule
		if strings.TrimSpace(rule.ID) == "" {
			return errors.New("RuleID cannot be empty")
		}
		if strings.TrimSpace(rule.Name) == "" {
			return errors.New("RuleName cannot be empty")
		}
		if strings.TrimSpace(rule.Author) == "" {
			return errors.New("rule author cannot be empty")
		}

		for j := range rule.Appends {
			if rule.Appends[j].Type != "" && rule.Appends[j].Type != "PLUGIN" {
				return errors.New("APPEND TYPE OR FIELD_NAME CANNOT BE EMPTY")
			}
		}

		if rule.Threshold.GroupBy == "" && rule.Threshold.Range == "" && rule.Threshold.Value == 0 {
			rule.ThresholdCheck = false
		} else {
			if rule.Threshold.GroupBy == "" {
				return errors.New("THRESHOLD GROUPBY CANNOT BE EMPTY")
			}
			if rule.Threshold.Range == "" {
				return errors.New("THRESHOLD RANGE CANNOT BE EMPTY")
			}
			if rule.Threshold.Value == 0 {
				return errors.New("THRESHOLD RANGE CANNOT BE EMPTY")
			}

			if !(rule.Threshold.CountType == "" || rule.Threshold.CountType == "SUM" || rule.Threshold.CountType == "CLASSIFY") {
				return errors.New("THRESHOLD COUNT TYPE MUST BE 'SUM' OR 'CLASSIFY'")
			}

			if rule.Threshold.CountType == "SUM" || rule.Threshold.CountType == "CLASSIFY" {
				if rule.Threshold.CountField == "" {
					return errors.New("THRESHOLD COUNT FIELD CANNOT BE EMPTY")
				} else {
					// Parse threshold count field path
					rule.Threshold.CountFieldList = common.StringToList(strings.TrimSpace(rule.Threshold.CountField))
				}
			}

			rule.Threshold.RangeInt, err = strconv.Atoi(rule.Threshold.Range)
			if err != nil {
				return errors.New("THRESHOLD RANGE CANNOT BE INT")
			}

			if !(rule.Threshold.Value > 1) {
				return errors.New("THRESHOLD VALUE MUST BE GREATER THAN 1")
			}

			rule.ThresholdCheck = true
		}

		thresholdGroupBYList := strings.Split(strings.TrimSpace(rule.Threshold.GroupBy), ",")
		rule.Threshold.GroupByList = make(map[string][]string, len(thresholdGroupBYList))
		for i := range thresholdGroupBYList {
			tmpList := common.StringToList(thresholdGroupBYList[i])
			rule.Threshold.GroupByList[thresholdGroupBYList[i]] = make([]string, len(tmpList))
			rule.Threshold.GroupByList[thresholdGroupBYList[i]] = tmpList
		}

		// Parse filter field path
		rule.Filter.FieldList = common.StringToList(strings.TrimSpace(rule.Filter.Field))

		// Parse each node's field path and assign check function
		for j := range rule.Checklist.CheckNodes {
			node := &rule.Checklist.CheckNodes[j]
			node.FieldList = common.StringToList(node.Field)

			switch strings.TrimSpace(node.Type) {
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
				return errors.New("UNKNOWN CHECK NODE TYPE, " + common.AnyToString(j))
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
					return errors.New("LOGIC CANNOT BE EMPTY")
				}

				if node.Logic != "AND" && node.Logic != "OR" {
					return errors.New("THRESHOLD COUNT TYPE MUST BE 'AND' OR 'OR'")
				}

				if node.Delimiter == "" {
					return errors.New("DELIMITER CANNOT BE EMPTY")
				}

				if strings.Contains(strings.TrimSpace(node.Value), node.Delimiter) {
					node.DelimiterFieldList = strings.Split(strings.TrimSpace(node.Value), node.Delimiter)
					if node.Logic == "OR" {
						rule.ChecklistLen = len(rule.Checklist.CheckNodes)
					} else {
						rule.ChecklistLen = len(rule.Checklist.CheckNodes) + len(node.DelimiterFieldList) - 1
					}
				} else {
					return errors.New("CHECK NODE VALUE DOES NOT EXIST IN DELIMITER")
				}
			} else {
				rule.ChecklistLen = len(rule.Checklist.CheckNodes) - 1
			}
		}

		delList := strings.Split(strings.TrimSpace(rule.Del), ",")

		rule.DelList = make([][]string, len(delList))
		for i := range delList {
			tmpList := common.StringToList(delList[i])
			rule.DelList[i] = make([]string, len(tmpList))
			rule.DelList[i] = tmpList
		}
	}
	return nil
}

// ParseRulesetFromByte parses XML bytes into a Ruleset struct and processes field paths.
func ParseRulesetFromByte(rawRuleset []byte) (*Ruleset, error) {
	var ruleset Ruleset
	if err := xml.Unmarshal(rawRuleset, &ruleset); err != nil {
		return nil, err
	}
	err := rulesetBuild(&ruleset)
	if err != nil {
		return nil, err
	}
	return &ruleset, nil
}
