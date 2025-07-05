package cluster

import (
	"encoding/json"

	"AgentSmith-HUB/common"
)

// publishProjCmd publishes a project command (start/stop/restart) to a follower via Redis Pub/Sub.
// desiredStatus should be one of "running", "stopped" or other to imply restart.
func publishProjCmd(nodeID, projectID, desiredStatus string) {
	var action string
	switch desiredStatus {
	case "running":
		action = "start"
	case "stopped":
		action = "stop"
	default:
		action = "restart"
	}

	evt := map[string]string{
		"node_id":    nodeID,
		"project_id": projectID,
		"action":     action,
	}
	if data, err := json.Marshal(evt); err == nil {
		_ = common.RedisPublish("cluster:proj_cmd", string(data))
	}
}
