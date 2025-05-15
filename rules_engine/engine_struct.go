package rules_engine

import (
	"AgentSmith-HUB/common"
	"encoding/xml"
)

type Ruleset struct {
	XMLName     xml.Name `xml:"root"`
	RulesetID   string   `xml:"ruleset_id,attr"`
	RulesetName string   `xml:"ruleset_name,attr"`
	Type        string   `xml:"type,attr"`
	Rules       []Rule   `xml:"rule"`
}

type Rule struct {
	ID        string    `xml:"id,attr"`
	Name      string    `xml:"name,attr"`
	Author    string    `xml:"author,attr"`
	Filter    Filter    `xml:"filter"`
	Checklist Checklist `xml:"checklist"`
	Threshold Threshold `xml:"threshold"`
	Appends   []Append  `xml:"append"`
	Del       string    `xml:"del"`
}

type Filter struct {
	Field     string   `xml:"field,attr"`
	FieldList []string // 导出字段
	Value     string   `xml:",chardata"`
}

type Checklist struct {
	Condition string `xml:"condition,attr"`
	Nodes     []Node `xml:"node"`
}

type Node struct {
	ID        string   `xml:"id,attr"`
	Type      string   `xml:"type,attr"`
	Field     string   `xml:"field,attr"`
	FieldList []string // 导出字段
	Logic     string   `xml:"logic,attr"`
	Delimiter string   `xml:"delimiter,attr"`
	Value     string   `xml:",chardata"`
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

// rulesetFieldProcess parses and fills the FieldList/CountFieldList/FieldNameList fields for all rules in the ruleset.
// This enables convenient field path access for later logic.
func rulesetFieldProcess(ruleset *Ruleset) {
	for i := range ruleset.Rules {
		rule := &ruleset.Rules[i]
		// Parse filter field path
		rule.Filter.FieldList = common.StringToList(rule.Filter.Field)
		// Parse threshold count field path
		rule.Threshold.CountFieldList = common.StringToList(rule.Threshold.CountField)

		// Parse each node's field path in checklist
		for j := range rule.Checklist.Nodes {
			rule.Checklist.Nodes[j].FieldList = common.StringToList(rule.Checklist.Nodes[j].Field)
		}

		// Parse each append's field name path
		for j := range rule.Appends {
			rule.Appends[j].FieldNameList = common.StringToList(rule.Appends[j].FieldName)
		}
	}
}

// ParseRulesetFromByte parses XML bytes into a Ruleset struct and processes field paths.
func ParseRulesetFromByte(rawRuleset []byte) (*Ruleset, error) {
	var ruleset Ruleset
	if err := xml.Unmarshal(rawRuleset, &ruleset); err != nil {
		return nil, err
	}
	rulesetFieldProcess(&ruleset)
	return &ruleset, nil
}
