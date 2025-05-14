package rules_engine

import (
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
	Field string `xml:"field,attr"`
	Value string `xml:",chardata"`
}

type Checklist struct {
	Condition string `xml:"condition,attr"`
	Nodes     []Node `xml:"node"`
}

type Node struct {
	ID        string `xml:"id,attr"`
	Type      string `xml:"type,attr"`
	Field     string `xml:"field,attr"`
	Logic     string `xml:"logic,attr"`
	Delimiter string `xml:"delimiter,attr"`
	Value     string `xml:",chardata"`
}

type Threshold struct {
	GroupBy    string `xml:"group_by,attr"`
	Range      string `xml:"range,attr"`
	LocalCache string `xml:"local_cache,attr"`
	CountType  string `xml:"count_type,attr"`
	CountField string `xml:"count_field,attr"`
	Value      int    `xml:",chardata"`
}

type Append struct {
	Type      string `xml:"type,attr"`
	FieldName string `xml:"field_name,attr"`
	Value     string `xml:",chardata"`
}

func ParseRulesetFromByte(rawRuleset []byte) (*Ruleset, error) {
	var ruleset Ruleset

	if err := xml.Unmarshal(rawRuleset, &ruleset); err != nil {
		return nil, err
	}
	return &ruleset, nil
}
