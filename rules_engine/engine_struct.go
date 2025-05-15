package rules_engine

import (
	"AgentSmith-HUB/common"
	"encoding/xml"
	"errors"
	regexp "github.com/BurntSushi/rure-go"
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
	ID           string    `xml:"id,attr"`
	Name         string    `xml:"name,attr"`
	Author       string    `xml:"author,attr"`
	Filter       Filter    `xml:"filter"`
	Checklist    Checklist `xml:"checklist"`
	ChecklistLen int
	Threshold    Threshold  `xml:"threshold"`
	Appends      []Append   `xml:"append"`
	Del          string     `xml:"del"`
	DelList      [][]string // parsed field path
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
	ID        string                              `xml:"id,attr"`
	Type      string                              `xml:"type,attr"`
	CheckFunc func(string, string) (bool, string) // function pointer for check logic
	Field     string                              `xml:"field,attr"`
	FieldList []string                            // parsed field path
	Logic     string                              `xml:"logic,attr"`
	Delimiter string                              `xml:"delimiter,attr"`
	Value     string                              `xml:",chardata"`
	Regex     *regexp.Regex
}

// Threshold defines aggregation and counting logic for a rule.
type Threshold struct {
	GroupBy        string   `xml:"group_by,attr"`
	Range          string   `xml:"range,attr"`
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
	// Validate required fields for ruleset
	if strings.TrimSpace(ruleset.RulesetID) == "" {
		return errors.New("RulesetID cannot be empty")
	}
	if strings.TrimSpace(ruleset.RulesetName) == "" {
		return errors.New("RulesetName cannot be empty")
	}

	// Set detection mode based on type
	switch strings.ToLower(strings.TrimSpace(ruleset.Type)) {
	case "":
		ruleset.IsDetection = true
	case "detection":
		ruleset.IsDetection = true
	case "whitelist":
		ruleset.IsDetection = false
	default:
		return errors.New("UNKNOWN RULESET TYPE, " + ruleset.Type)
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

		// Precompute checklist length for fast access
		rule.ChecklistLen = len(rule.Checklist.CheckNodes) - 1

		// Parse filter field path
		rule.Filter.FieldList = common.StringToList(rule.Filter.Field)
		// Parse threshold count field path
		rule.Threshold.CountFieldList = common.StringToList(rule.Threshold.CountField)

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
		}

		delList := strings.Split(rule.Del, ",")
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
