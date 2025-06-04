package main

import (
	"AgentSmith-HUB/api"
	"AgentSmith-HUB/cluster"
	"AgentSmith-HUB/common"
	"AgentSmith-HUB/logger"
	"AgentSmith-HUB/plugin"
	"AgentSmith-HUB/project"
	"flag"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

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

	if err := yaml.Unmarshal(data, &common.Config); err != nil {
		return fmt.Errorf("failed to parse hub config: %w", err)
	}

	if common.Config.Redis == "" {
		return fmt.Errorf("redis is null")
	}

	common.Config.ConfigRoot = configRoot

	if common.Config.Listen == "" {
		common.Config.Listen = "0.0.0.0:8080"
	}

	return nil
}

func LoadLocalProject() {
	if cluster.IsLeader {
		projectList, err := traverseProject(path.Join(common.Config.ConfigRoot, "project"))
		if err != nil {
			logger.Error("travers project error", "error", err)
			return
		}

		for _, projectPath := range projectList {
			_, err := project.NewProject(projectPath, "", "")
			if err != nil {
				logger.Error("project init error", "err", err, "project_path", projectPath)
				continue
			}
		}
	} else {
		for id, raw := range common.AllProjectRawConfig {
			_, err := project.NewProject("", raw, id)
			if err != nil {
				logger.Error("project init error", "err", err, "project_id", id)
				continue
			}
		}
	}
}

func StartAllProject() {
	var err error

	if project.GlobalProject != nil {
		for _, p := range project.GlobalProject.Projects {
			err = p.Start()
			if err != nil {
				logger.Error("project start error", "error", err, "project_id", p.Id)
			} else {
				logger.Info("project start successful", "project_id", p.Id)
			}
		}
	}
}

func LoadLeaderConfigAndComponents() error {
	var err error
	var leaderConfig map[string]string

	// Get hub leader config
	leaderConfig, err = api.GetLeaderConfig()
	if err != nil {
		return err
	}

	common.Config.Redis = leaderConfig["redis"]
	common.Config.Redis = leaderConfig["redis_password"]

	plugins, err := api.GetAllComponents("plugin")
	if err != nil {
		logger.Error("load leader plugins error", "error", err.Error())
	}
	common.AllPluginsRawConfig = make(map[string]string, len(plugins))
	for _, v := range plugins {
		common.AllPluginsRawConfig[v["name"].(string)] = v["payload"].(string)
	}

	inputs, err := api.GetAllComponents("input")
	if err != nil {
		logger.Error("load leader inputs error", "error", err.Error())
	}
	common.AllInputsRawConfig = make(map[string]string, len(inputs))
	for _, v := range inputs {
		common.AllInputsRawConfig[v["id"].(string)] = v["raw"].(string)
	}

	outputs, err := api.GetAllComponents("output")
	if err != nil {
		logger.Error("load leader outputs error", "error", err.Error())
	}
	common.AllOutputsRawConfig = make(map[string]string, len(outputs))
	for _, v := range outputs {
		common.AllOutputsRawConfig[v["id"].(string)] = v["raw"].(string)
	}

	rulesets, err := api.GetAllComponents("ruleset")
	if err != nil {
		logger.Error("load leader rulesets error", "error", err.Error())
	}
	common.AllRulesetsRawConfig = make(map[string]string, len(rulesets))
	for _, v := range rulesets {
		common.AllRulesetsRawConfig[v["id"].(string)] = v["raw"].(string)
	}

	projects, err := api.GetAllComponents("project")
	if err != nil {
		logger.Error("load leader projects error", "error", err.Error())
	}
	common.AllProjectRawConfig = make(map[string]string, len(projects))
	for _, v := range projects {
		common.AllProjectRawConfig[v["id"].(string)] = v["raw"].(string)
	}

	return nil
}

func main() {
	var err error
	// init global config
	common.Config = &common.HubConfig{}

	configRoot := flag.String("config_root", "", "agent smith hub config path, only leader need")
	leaderAddr := flag.String("leader", "", "hub cluster leader address")

	flag.Parse()

	logger.Info("hub_starting", "config_root", *configRoot, "leader", leaderAddr)

	if (*configRoot != "" && *leaderAddr != "") || (*configRoot == "" && *leaderAddr == "") {
		fmt.Println("If the instance is a Leader, only 'config_root' needs to be given; if it is a Follower, only 'leader' needs to be given")
		return
	}

	// load self ip & init cluster
	common.Config.LocalIP, err = common.GetLocalIP()
	if err != nil {
		logger.Error("get local ip error", "error", err)
	}
	cl := cluster.ClusterInit(common.Config.LocalIP, common.Config.LocalIP)

	if *configRoot != "" {
		//set self is leader
		common.Config.Leader = common.Config.LocalIP
		cl.SetLeader(common.Config.LocalIP, common.Config.LocalIP)
		cl.StartHeartbeatLoop()

		//leader read local config
		err := loadHubConfig(*configRoot)
		if err != nil {
			logger.Error("load hub config error", "error", err)
			return
		}

		//leader creates or reads token
		common.Config.Token, err = readToken(true)
		if err != nil {
			logger.Error("read or create token error", "error", err)
			return
		}
	} else if *leaderAddr != "" {
		common.Config.Leader = *leaderAddr
		//set leader
		cl.SetLeader(common.Config.Leader, common.Config.Leader)
		cl.StartHeartbeatLoop()

		//read token
		common.Config.Token, err = readToken(false)
		if err != nil {
			logger.Error("read token error", "error", err)
			return
		}

		//init hub request
		err = api.InitRequest(common.Config.Leader, common.Config.Token)
		if err != nil {
			logger.Error("hub init request error", "error", err)
			return
		}

		//load leader config, and init project/input/output/ruleset/plugin
		err = LoadLeaderConfigAndComponents()
		if err != nil {
			logger.Error("load leader config or components error", "error", err)
			return
		}
	}

	// project/plugin/redis init
	err = common.RedisInit(common.Config.Redis, common.Config.RedisPassword)
	if err != nil {
		logger.Error("redis init error", "error", err)
		return
	}

	// load project/input/output/ruleset
	LoadLocalProject()

	// load plugin
	err = plugin.PluginInit(path.Join(common.Config.ConfigRoot, "plugin"))
	if err != nil {
		logger.Error("plugin init error", "error", err)
		return
	}

	// start all projects
	StartAllProject()

	// start Api
	err = api.ServerStart(common.Config.Listen)
	if err != nil {
		logger.Error("server start err", "error", err.Error())
	}

	select {}
}
