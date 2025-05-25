package rules_engine

import (
	"AgentSmith-HUB/common"
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
	"github.com/panjf2000/ants/v2"
)

// FromRawSymbol is the prefix indicating a value should be fetched from raw data.
const FromRawSymbol = "_$"
const PluginArgFromRawSymbol = "_$ORIDATA"
const FromRawSymbolLen = len(FromRawSymbol)

const MinPoolSize = 4
const MaxPoolSize = 512

var ConditionRegex = regexp.MustCompile("^([a-z]+|\\(|\\)|\\s)+$")

// Ruleset represents a collection of rules and associated metadata.
type Ruleset struct {
	XMLName     xml.Name `xml:"root"`
	RulesetID   string
	RulesetName string `xml:"name,attr"`
	Type        string `xml:"type,attr"`

	IsDetection bool
	Rules       []Rule `xml:"rule"`

	UpStream   map[string]*chan map[string]interface{}
	DownStream map[string]*chan map[string]interface{}

	stopChan chan struct{} // 用于Start/Stop的控制
	antsPool *ants.Pool    // ants线程池

	rulesMu sync.RWMutex // Mutex for hot update of rules
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

	Plugin     *common.Plugin
	PluginArgs []*PluginArg
}

type PluginArg struct {
	//0 Value == RealValue
	//1 RealValue == GetCheckData(Value)
	//2 RealValue == ORI DATA
	Type      int
	Value     interface{}
	RealValue interface{}
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

	Plugin     *common.Plugin
	PluginArgs []*PluginArg
}

type Plugin struct {
	Value      string `xml:",chardata"`
	Plugin     *common.Plugin
	PluginArgs []*PluginArg
}

func NewRuleset(path string, id string) (*Ruleset, error) {
	xmlFile, err := os.Open(path)
	if err != nil {
		panic(err)
	}
	defer xmlFile.Close()

	rawRuleset, err := io.ReadAll(xmlFile)
	if err != nil {
		panic(err)
	}

	ruleset, err := ParseRulesetFromByte(rawRuleset)
	if err != nil {
		return nil, err
	}

	ruleset.UpStream = make(map[string]*chan map[string]interface{}, 0)
	ruleset.DownStream = make(map[string]*chan map[string]interface{}, 0)

	ruleset.RulesetID = id
	return ruleset, nil
}

func ParseFunctionCall(input string) (string, []*PluginArg, error) {
	input = strings.TrimSpace(input)

	re := regexpgo.MustCompile(`^([a-zA-Z_][a-zA-Z0-9_]*)\s*\((.*)\)$`)
	matches := re.FindStringSubmatch(input)
	if len(matches) != 3 {
		return "", nil, errors.New("invalid function call syntax: must be in the form func(arg1, arg2, ...)")
	}

	funcName := matches[1]
	argStr := matches[2]

	args, err := parseArgs(argStr)
	if err != nil {
		return "", nil, err
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
				return errors.New("APPEND TYPE OR FIELD_NAME CANNOT BE EMPTY")
			}

			if appendNode.Type == "PLUGIN" {
				pluginName, args, err := ParseFunctionCall(appendValue)
				if err != nil {
					return err
				}

				if p, ok := common.Plugins[pluginName]; ok {
					appendNode.Plugin = p
				} else {
					return errors.New("NOT FUND THIS PLUGIN")
				}

				appendNode.PluginArgs = args
			}
		}

		for i := range rule.Plugins {
			pluginNode := &rule.Plugins[i]
			value := strings.TrimSpace(pluginNode.Value)

			if value == "" {
				return errors.New("PLUGIN VALUE CANNOT BE EMPTY")
			}

			pluginName, args, err := ParseFunctionCall(value)
			if err != nil {
				return err
			}

			if p, ok := common.Plugins[pluginName]; ok {
				pluginNode.Plugin = p
			} else {
				return errors.New("NOT FUND THIS PLUGIN")
			}

			pluginNode.PluginArgs = args
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
		rule.Filter.Field = strings.TrimSpace(rule.Filter.Field)
		rule.Filter.FieldList = common.StringToList(strings.TrimSpace(rule.Filter.Field))

		// Parse each node's field path and assign check function
		for j := range rule.Checklist.CheckNodes {
			node := &rule.Checklist.CheckNodes[j]
			node.FieldList = common.StringToList(strings.TrimSpace(node.Field))

			if rule.Checklist.ConditionFlag {
				id := strings.TrimSpace(node.ID)
				node.ID = id

				if id == "" {
					return errors.New("CHECK NODE ID CANNOT BE EMPTY")
				}

				if _, ok := rule.Checklist.ConditionMap[id]; ok {
					return errors.New("CHECK NODE ID CANNOT BE REPEATED")
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

				if p, ok := common.Plugins[pluginName]; ok {
					node.Plugin = p
				} else {
					return errors.New("NOT FUND THIS PLUGIN")
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

		rule.Checklist.CheckNodes = sortCheckNodes(rule.Checklist.CheckNodes)

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
