package main

import (
	"AgentSmith-HUB/api"
	"AgentSmith-HUB/cluster"
	"AgentSmith-HUB/common"
	"AgentSmith-HUB/logger"
	"AgentSmith-HUB/project"
	"flag"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

var HubConfig *common.HubConfig

func traverseProject(dir string) ([]string, error) {
	var files []string

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(path, ".yaml") {
			files = append(files, path)
		}
		return nil
	})

	return files, err
}

func readToken(create bool) (string, error) {
	tokenPath := ".token"
	data, err := os.ReadFile(tokenPath)
	if err == nil {
		return string(data), nil
	}

	//leader create new token
	if create {
		f, err := os.Create(tokenPath)
		if err != nil {
			return "", err
		}

		defer f.Close()

		uuid := common.NewUUID() // 假设 common.NewUUID() 返回 uuid 字符串
		_, err = f.WriteString(uuid)
		if err != nil {
			logger.Error("failed to write uuid to .token file", "error", err)
		}
		return uuid, nil
	} else {
		return "", err
	}
}

func loadHubConfig(configRoot string) error {
	configPath := path.Join(configRoot, "config.yaml")

	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to read hub config: %w", err)
	}

	if err := yaml.Unmarshal(data, &HubConfig); err != nil {
		return fmt.Errorf("failed to parse hub config: %w", err)
	}

	if HubConfig.Redis == "" {
		return fmt.Errorf("redis is null")
	}

	if HubConfig.Leader == "" {
		return fmt.Errorf("leader is null")
	}

	HubConfig.ConfigRoot = configRoot

	if HubConfig.Listen == "" {
		HubConfig.Listen = "0.0.0.0:8080"
	}

	return nil
}

func LoadLocalProject(configRoot string) {
	projectList, err := traverseProject(path.Join(configRoot, "project"))
	if err != nil {
		logger.Error("travers project error", "error", err)
		return
	}

	for _, projectPath := range projectList {
		p, err := project.NewProject("test.yaml")
		if err != nil {
			logger.Error("project init error", "err", err, "project_path", projectPath)
			continue
		}

		err = p.Start()
		if err != nil {
			logger.Error("project start error", "error", err, "project_path", projectPath)
			continue
		}

		logger.Info("project start successful", "project_id", p.Id, "project_path", projectPath)
	}
}

func LoadLeaderProject() error {
	var err error
	var confRoot string

	// Get hub leader config root
	confRoot, err = api.GetConfigRoot()
	if err != nil {
		return err
	}

	// Download config.zip from leader, and unzip to conf root
	err = api.DownloadConfig(confRoot)
	if err != nil {
		return err
	}

	// Read config.yaml to get configRoot path
	err = loadHubConfig(confRoot)
	if err != nil {
		logger.Error("load hub config error", "error", err)
	}

	return nil
}

func main() {
	var err error
	configRoot := flag.String("config_root", "", "agent smith hub config path, only leader need")
	leaderAddr := flag.String("leader", "", "hub cluster leader address")

	flag.Parse()

	logger.Info("hub_starting", "config_root", *configRoot, "leader", leaderAddr)

	if (*configRoot != "" && *leaderAddr != "") || (*configRoot == "" && *leaderAddr == "") {
		fmt.Println("If the instance is a Leader, only 'config_root' needs to be given; if it is a Follower, only 'leader' needs to be given")
		return
	}

	// load self ip & init cluster
	HubConfig.LocalIP, err = common.GetLocalIP()
	if err != nil {
		logger.Error("get local ip error", "error", err)
	}
	cl := cluster.ClusterInit(HubConfig.LocalIP, HubConfig.LocalIP)

	if *configRoot != "" {
		//set self is leader
		HubConfig.Leader = HubConfig.LocalIP
		cl.SetLeader(HubConfig.LocalIP, HubConfig.LocalIP)

		//leader read local config
		err := loadHubConfig(*configRoot)
		if err != nil {
			logger.Error("load hub config error", "error", err)
			return
		}

		//leader creates or read token
		HubConfig.Token, err = readToken(true)
		if err != nil {
			logger.Error("read or create token error", "error", err)
			return
		}
	} else if *leaderAddr != "" {
		HubConfig.Leader = *leaderAddr
		//set leader
		cl.SetLeader(HubConfig.Leader, HubConfig.Leader)

		//read token
		HubConfig.Token, err = readToken(false)
		if err != nil {
			logger.Error("read token error", "error", err)
			return
		}

		//init hub request
		err = api.InitRequest(HubConfig.Leader, HubConfig.Token)
		if err != nil {
			logger.Error("hub init request error", "error", err)
			return
		}

		//download leader config
		err = LoadLeaderProject()
		if err != nil {
			logger.Error("load leader config error", "error", err)
			return
		}
	}

	//err = loadHubConfig(*configRoot)
	//if err != nil {
	//	logger.Error("load hub config error", "error", err)
	//	return
	//}
	//
	////todo create node id
	//cl.SetLeader(HubConfig.Leader, HubConfig.Leader)
	//cl.StartHeartbeatLoop()
	//
	//// Load and start projects
	//if cl.IsLeader() {
	//	LoadLocalProject(HubConfig.ConfigRoot)
	//} else {
	//	LoadLeaderProject()
	//}
	//
	//// init
	//err = common.RedisInit(HubConfig.Redis, HubConfig.RedisPassword)
	//if err != nil {
	//	logger.Error("redis init error", "error", err)
	//	return
	//}
	//err = plugin.PluginInit(path.Join(project.ConfigRoot, "plugin"))
	//if err != nil {
	//	logger.Error("plugin init error", "error", err)
	//	return
	//}

	// Start Api
	err = api.ServerStart(HubConfig.Listen, HubConfig)
	if err != nil {
		logger.Error("server start err", "error", err.Error())
	}

	select {}
}
