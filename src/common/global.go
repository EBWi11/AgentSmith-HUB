package common

import "sync"

var Config *HubConfig

// for follower node
// id:raw
var AllInputsRawConfig map[string]string
var AllOutputsRawConfig map[string]string
var AllRulesetsRawConfig map[string]string
var AllProjectRawConfig map[string]string
var AllPluginsRawConfig map[string]string

var GlobalMu sync.RWMutex

// Global cluster state
var (
	IsLeader bool
	Leader   string
)

// SetLeaderState sets the leader state for this node
func SetLeaderState(isLeader bool, leaderID string) {
	IsLeader = isLeader
	Leader = leaderID
}

func init() {
	AllInputsRawConfig = make(map[string]string, 0)
	AllOutputsRawConfig = make(map[string]string, 0)
	AllRulesetsRawConfig = make(map[string]string, 0)
	AllProjectRawConfig = make(map[string]string, 0)
	AllPluginsRawConfig = make(map[string]string, 0)
}
