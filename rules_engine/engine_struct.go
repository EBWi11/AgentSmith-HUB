package rules_engine

import (
	"AgentSmith-HUB/common"
	"encoding/xml"
	"errors"
	regexp "github.com/BurntSushi/rure-go"
)

type Ruleset struct {
	XMLName     xml.Name `xml:"root"`
	RulesetID   string   `xml:"ruleset_id,attr"`
	RulesetName string   `xml:"ruleset_name,attr"`
	Type        string   `xml:"type,attr"`
	IsDetection bool
	Rules       []Rule `xml:"rule"`
}

type Rule struct {
	ID           string    `xml:"id,attr"`
	Name         string    `xml:"name,attr"`
	Author       string    `xml:"author,attr"`
	Filter       Filter    `xml:"filter"`
	Checklist    Checklist `xml:"checklist"`
	ChecklistLen int
	Threshold    Threshold `xml:"threshold"`
	Appends      []Append  `xml:"append"`
	Del          string    `xml:"del"`
}

type Filter struct {
	Field     string   `xml:"field,attr"`
	FieldList []string // 导出字段
	Value     string   `xml:",chardata"`
}

type Checklist struct {
	Condition  string       `xml:"condition,attr"`
	CheckNodes []CheckNodes `xml:"node"`
}

type T interface{ string | *regexp.Regex }

type CheckNodes struct {
	ID   string `xml:"id,attr"`
	Type string `xml:"type,attr"`
	//not include REGEX
	CheckFunc func(string, string) (bool, string)
	Field     string   `xml:"field,attr"`
	FieldList []string // 导出字段
	Logic     string   `xml:"logic,attr"`
	Delimiter string   `xml:"delimiter,attr"`
	Value     string   `xml:",chardata"`
	Regex     *regexp.Regex
}

type Threshold struct {
	GroupBy        string   `xml:"group_by,attr"`
	Range          string   `xml:"range,attr"`
	LocalCache     string   `xml:"local_cache,attr"`
	CountType      string   `xml:"count_type,attr"`
	CountField     string   `xml:"count_field,attr"`
	CountFieldList []string // 导出字段
	Value          int      `xml:",chardata"`
}

type Append struct {
	Type          string   `xml:"type,attr"`
	FieldName     string   `xml:"field_name,attr"`
	FieldNameList []string // 导出字段
	Value         string   `xml:",chardata"`
}

func rulesetBuild(ruleset *Ruleset) error {
	var err error
	switch ruleset.Type {
	case "detection":
		ruleset.IsDetection = true
	case "whitelist":
		ruleset.IsDetection = false
	default:
		return errors.New(common.GetPkgName() + ": UNKNOWN RULESET TYPE, " + ruleset.Type)
	}

	for i := range ruleset.Rules {
		rule := &ruleset.Rules[i]
		rule.ChecklistLen = len(rule.Checklist.CheckNodes) - 1

		// Parse filter field path
		rule.Filter.FieldList = common.StringToList(rule.Filter.Field)
		// Parse threshold count field path
		rule.Threshold.CountFieldList = common.StringToList(rule.Threshold.CountField)

		// Parse each node's field path in checklist
		for j := range rule.Checklist.CheckNodes {
			rule.Checklist.CheckNodes[j].FieldList = common.StringToList(rule.Checklist.CheckNodes[j].Field)

			switch rule.Checklist.CheckNodes[j].Type {
			case "END":
				rule.Checklist.CheckNodes[j].CheckFunc = END
			case "START":
				rule.Checklist.CheckNodes[j].CheckFunc = START
			case "NEND":
				rule.Checklist.CheckNodes[j].CheckFunc = NEND
			case "NSTART":
				rule.Checklist.CheckNodes[j].CheckFunc = NSTART
			case "INCL":
				rule.Checklist.CheckNodes[j].CheckFunc = INCL
			case "NI":
				rule.Checklist.CheckNodes[j].CheckFunc = NI
			case "NCS_END":
				rule.Checklist.CheckNodes[j].CheckFunc = NCS_END
			case "NCS_START":
				rule.Checklist.CheckNodes[j].CheckFunc = NCS_START
			case "NCS_NEND":
				rule.Checklist.CheckNodes[j].CheckFunc = NCS_NEND
			case "NCS_NSTART":
				rule.Checklist.CheckNodes[j].CheckFunc = NCS_START
			case "NCS_INCL":
				rule.Checklist.CheckNodes[j].CheckFunc = NCS_INCL
			case "NCS_NI":
				rule.Checklist.CheckNodes[j].CheckFunc = NCS_NI
			case "MT":
				rule.Checklist.CheckNodes[j].CheckFunc = MT
			case "LT":
				rule.Checklist.CheckNodes[j].CheckFunc = LT
			case "REGEX":
			case "ISNULL":
				rule.Checklist.CheckNodes[j].CheckFunc = ISNULL
			case "NOTNULL":
				rule.Checklist.CheckNodes[j].CheckFunc = NOTNULL
			case "EQU":
				rule.Checklist.CheckNodes[j].CheckFunc = EQU
			case "NEQ":
				rule.Checklist.CheckNodes[j].CheckFunc = NEQ
			case "NCS_EQU":
				rule.Checklist.CheckNodes[j].CheckFunc = NCS_EQU
			case "NCS_NEQ":
				rule.Checklist.CheckNodes[j].CheckFunc = NCS_NEQ
			default:
				return errors.New(common.GetPkgName() + ": UNKNOWN CHECK NODE TYPE, " + common.AnyToString(j))
			}

			if "REGEX" == rule.Checklist.CheckNodes[j].Type {
				rule.Checklist.CheckNodes[j].Regex, err = regexp.Compile(rule.Checklist.CheckNodes[j].Value)
				if err != nil {
					return err
				}
			}
		}

		// Parse each appends field name path
		for j := range rule.Appends {
			rule.Appends[j].FieldNameList = common.StringToList(rule.Appends[j].FieldName)
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
