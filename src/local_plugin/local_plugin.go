package local_plugin

import (
	"AgentSmith-HUB/local_plugin/is_local_ip"
	"encoding/json"
	"errors"
)

// parseJSONData parses JSON string and returns parsed data (for testing interface{} return type)
func parseJSONData(args ...interface{}) (interface{}, bool, error) {
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

// for checknode
var LocalPluginBoolRes = map[string]func(...interface{}) (bool, error){
	"isLocalIP": is_local_ip.Eval,
}

// for append
var LocalPluginInterfaceAndBoolRes = map[string]func(...interface{}) (interface{}, bool, error){
	"parseJSON": parseJSONData,
}
