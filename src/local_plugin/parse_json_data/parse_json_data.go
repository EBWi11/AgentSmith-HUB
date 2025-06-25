package parse_json_data

import (
	"encoding/json"
	"errors"
)

// parseJSONData parses JSON string and returns parsed data (for testing interface{} return type)
func Eval(args ...interface{}) (interface{}, bool, error) {
	if len(args) != 1 {
		return nil, false, errors.New("parseJSON requires exactly one argument")
	}

	jsonStr, ok := args[0].(string)
	if !ok {
		return nil, false, errors.New("argument must be a JSON string")
	}

	var result interface{}
	err := json.Unmarshal([]byte(jsonStr), &result)
	if err != nil {
		return nil, false, err
	}

	return result, true, nil
}
