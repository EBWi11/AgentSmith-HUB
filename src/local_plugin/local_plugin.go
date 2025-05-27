package local_plugin

import "AgentSmith-HUB/local_plugin/is_local_ip"

// for checknode
var LocalPluginBoolRes = map[string]func(...interface{}) (bool, error){
	"isLocalIP": is_local_ip.Eval,
}

// for append
var LocalPluginInterfaceAndBoolRes = map[string]func(...interface{}) (interface{}, bool, error){}
