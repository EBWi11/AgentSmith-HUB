package cluster

import (
	"encoding/json"

	"AgentSmith-HUB/common"
	"AgentSmith-HUB/logger"
)

// publishProjCmd publishes a project command (start/stop/restart) to a follower via Redis Pub/Sub.
// action should be one of "start", "stop", "restart"
func publishProjCmd(nodeID, projectID, action string) {
	// Validate action
	if action != "start" && action != "stop" && action != "restart" {
		logger.Error("Invalid project action", "action", action, "project_id", projectID)
		return
	}

	evt := map[string]string{
		"node_id":    nodeID,
		"project_id": projectID,
		"action":     action,
	}

	data, err := json.Marshal(evt)
	if err != nil {
		logger.Error("Failed to marshal project command", "error", err)
		return
	}

	if err := common.RedisPublish("cluster:proj_cmd", string(data)); err != nil {
		logger.Error("Failed to publish project command", "error", err)
	} else {
		logger.Debug("Published project command", "node_id", nodeID, "project_id", projectID, "action", action)
	}
}
