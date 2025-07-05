package cluster

import (
	"AgentSmith-HUB/common"
	"AgentSmith-HUB/logger"
	"context"
	"encoding/json"
)

// CompSyncEvt defines component sync event structure
type CompSyncEvt struct {
	Op        string `json:"op"`   // add|update|delete
	Type      string `json:"type"` // input|output|ruleset|plugin|project
	ID        string `json:"id"`
	Raw       string `json:"raw,omitempty"`
	IsRunning bool   `json:"is_running,omitempty"`
}

// PublishComponentSync is called by leader after component change
func PublishComponentSync(evt *CompSyncEvt) {
	if evt == nil {
		return
	}
	data, err := json.Marshal(evt)
	if err != nil {
		return
	}
	_ = common.RedisPublish("cluster:component_sync", string(data))
}

// startComponentSyncSubscriber starts follower listener
func (cm *ClusterManager) startComponentSyncSubscriber() {
	if cm.IsLeader() {
		return // leader doesn't need to subscribe
	}
	client := common.GetRedisClient()
	if client == nil {
		return
	}
	sub := client.Subscribe(context.Background(), "cluster:component_sync")
	go func() {
		ch := sub.Channel()
		for msg := range ch {
			var evt CompSyncEvt
			if err := json.Unmarshal([]byte(msg.Payload), &evt); err != nil {
				continue
			}
			applyComponentSyncFollower(&evt)
		}
	}()
}

// applyComponentSyncFollower handles component change on follower (simplified)
func applyComponentSyncFollower(evt *CompSyncEvt) {
	// For brevity we only update raw config maps; deeper hot-update reuse existing logic may be added later
	common.GlobalMu.Lock()
	defer common.GlobalMu.Unlock()

	switch evt.Type {
	case "ruleset":
		if evt.Op == "delete" {
			delete(common.AllRulesetsRawConfig, evt.ID)
		} else {
			common.AllRulesetsRawConfig[evt.ID] = evt.Raw
		}
	case "input":
		if evt.Op == "delete" {
			delete(common.AllInputsRawConfig, evt.ID)
		} else {
			common.AllInputsRawConfig[evt.ID] = evt.Raw
		}
	case "output":
		if evt.Op == "delete" {
			delete(common.AllOutputsRawConfig, evt.ID)
		} else {
			common.AllOutputsRawConfig[evt.ID] = evt.Raw
		}
	case "project":
		if evt.Op == "delete" {
			delete(common.AllProjectRawConfig, evt.ID)
		} else {
			common.AllProjectRawConfig[evt.ID] = evt.Raw
		}
	case "plugin":
		if evt.Op == "delete" {
			delete(common.AllPluginsRawConfig, evt.ID)
		} else {
			common.AllPluginsRawConfig[evt.ID] = evt.Raw
		}
	}

	logger.Info("Applied component sync", "type", evt.Type, "op", evt.Op, "id", evt.ID)
}
