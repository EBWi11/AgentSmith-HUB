package local_plugin

import "AgentSmith-HUB/local_plugin/is_local_ip"

var LocalPluginBoolRes = map[string]func(...interface{}) interface{}{
	"isLocalIP": func(args ...interface{}) interface{} {
		if len(args) == 1 {
			if ipStr, ok := args[0].(string); ok {
				return is_local_ip.Eval(ipStr)
			}
		}
		return false
	},
}
