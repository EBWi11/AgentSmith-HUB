package main

import (
	"AgentSmith-HUB/api"
	"AgentSmith-HUB/cluster"
	"AgentSmith-HUB/common"
	"AgentSmith-HUB/input"
	"AgentSmith-HUB/logger"
	"AgentSmith-HUB/output"
	"AgentSmith-HUB/plugin"
	"AgentSmith-HUB/project"
	"AgentSmith-HUB/rules_engine"
	"flag"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

func traverseComponents(dir string, suffix string) ([]string, error) {
	var files []string

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(path, suffix) {
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

	// Leader creates a new token
	if create {
		f, err := os.Create(tokenPath)
		if err != nil {
			return "", err
		}

		defer f.Close()

		uuid := common.NewUUID() // Assumes common.NewUUID() returns a uuid string
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

func LoadComponents() {
	if cluster.IsLeader {
		pluginList, err := traverseComponents(path.Join(common.Config.ConfigRoot, "plugin"), ".go")
		if err != nil {
			logger.Error("travers plugin error", "error", err)
		}
		for _, v := range pluginList {
			name := common.GetFileNameWithoutExt(v)
			err := plugin.NewPlugin(v, "", name, plugin.YAEGI_PLUGIN)
			if err != nil {
				logger.Error("failed to create input instance", "error", err, "path", v)
			}
		}

		inputList, err := traverseComponents(path.Join(common.Config.ConfigRoot, "input"), ".yaml")
		if err != nil {
			logger.Error("travers input error", "error", err)
		}
		for _, v := range inputList {
			id := common.GetFileNameWithoutExt(v)
			tmp, err := input.NewInput(v, "", id)
			if err != nil {
				logger.Error("failed to create input instance", "error", err, "path", v)
			}
			if tmp != nil {
				project.GlobalProject.Inputs[id] = tmp
			}
		}

		outputList, err := traverseComponents(path.Join(common.Config.ConfigRoot, "output"), ".yaml")
		if err != nil {
			logger.Error("travers output error", "error", err)
		}
		for _, v := range outputList {
			id := common.GetFileNameWithoutExt(v)
			tmp, err := output.NewOutput(v, "", id)
			if err != nil {
				logger.Error("failed to create output instance", "error", err, "path", v)
				continue
			}
			project.GlobalProject.Outputs[id] = tmp
		}

		rulesetList, err := traverseComponents(path.Join(common.Config.ConfigRoot, "ruleset"), ".xml")
		if err != nil {
			logger.Error("travers ruleset error", "error", err)
		}
		for _, v := range rulesetList {
			id := common.GetFileNameWithoutExt(v)
			tmp, err := rules_engine.NewRuleset(v, "", id)
			if err != nil {
				logger.Error("failed to create ruleset instance", "error", err, "path", v)
				continue
			}
			project.GlobalProject.Rulesets[id] = tmp
		}

		// read new components
		pluginNewList, err := traverseComponents(path.Join(common.Config.ConfigRoot, "plugin"), ".go.new")
		if err != nil {
			logger.Error("travers plugin new error", "error", err)
		}
		for _, v := range pluginNewList {
			name := common.GetFileNameWithoutExt(v)
			data, err := os.ReadFile(v)
			if err != nil {
				logger.Error("failed to read component", "error", err, "path", v)
				continue
			}
			plugin.PluginsNew[name] = string(data)
		}

		inputListNew, err := traverseComponents(path.Join(common.Config.ConfigRoot, "input"), ".yaml.new")
		if err != nil {
			logger.Error("travers input new error", "error", err)
		}
		for _, v := range inputListNew {
			id := common.GetFileNameWithoutExt(v)
			data, err := os.ReadFile(v)
			if err != nil {
				logger.Error("failed to read component", "error", err, "path", v)
				continue
			}
			project.GlobalProject.InputsNew[id] = string(data)
		}

		outputListNew, err := traverseComponents(path.Join(common.Config.ConfigRoot, "output"), ".yaml.new")
		if err != nil {
			logger.Error("travers output new error", "error", err)
		}
		for _, v := range outputListNew {
			id := common.GetFileNameWithoutExt(v)
			data, err := os.ReadFile(v)
			if err != nil {
				logger.Error("failed to read component", "error", err, "path", v)
				continue
			}
			project.GlobalProject.OutputsNew[id] = string(data)
		}

		rulesetListNew, err := traverseComponents(path.Join(common.Config.ConfigRoot, "ruleset"), ".xml,new")
		if err != nil {
			logger.Error("travers ruleset new error", "error", err)
		}
		for _, v := range rulesetListNew {
			id := common.GetFileNameWithoutExt(v)
			data, err := os.ReadFile(v)
			if err != nil {
				logger.Error("failed to read component", "error", err, "path", v)
				continue
			}
			project.GlobalProject.RulesetsNew[id] = string(data)
		}
	} else {
		for name, raw := range common.AllPluginsRawConfig {
			err := plugin.NewPlugin("", raw, name, plugin.YAEGI_PLUGIN)
			if err != nil {
				logger.Error("failed to new plugin", "error", err)
				continue
			}
		}

		for id, raw := range common.AllInputsRawConfig {
			tmp, err := input.NewInput("", raw, id)
			if err != nil {
				logger.Error("failed to create input instance", "error", err, "id", id)
				continue
			}
			project.GlobalProject.Inputs[id] = tmp
		}

		for id, raw := range common.AllOutputsRawConfig {
			tmp, err := output.NewOutput("", raw, id)
			if err != nil {
				logger.Error("failed to create output instance", "error", err, "id", id)
				continue
			}
			project.GlobalProject.Outputs[id] = tmp
		}

		for id, raw := range common.AllRulesetsRawConfig {
			tmp, err := rules_engine.NewRuleset("", raw, id)
			if err != nil {
				logger.Error("failed to create ruleset instance", "error", err, "id", id)
				continue
			}
			project.GlobalProject.Rulesets[id] = tmp
		}
	}
}

func LoadProject() {
	if cluster.IsLeader {
		projectList, err := traverseComponents(path.Join(common.Config.ConfigRoot, "project"), ".yaml")
		if err != nil {
			logger.Error("travers project error", "error", err)
			return
		}

		for _, projectPath := range projectList {
			id := common.GetFileNameWithoutExt(projectPath)
			p, err := project.NewProject(projectPath, "", id)
			if err != nil {
				logger.Error("project init error", "err", err, "project_path", projectPath)
			}

			if p != nil {
				project.GlobalProject.Projects[id] = p
			}
		}

		//read new project
		projectListNew, err := traverseComponents(path.Join(common.Config.ConfigRoot, "project"), ".yaml.new")
		if err != nil {
			logger.Error("travers project new error", "error", err)
			return
		}

		for _, projectPath := range projectListNew {
			id := common.GetFileNameWithoutExt(projectPath)
			data, err := os.ReadFile(projectPath)
			if err != nil {
				logger.Error("failed to read component", "error", err, "path", projectPath)
				continue
			}
			plugin.PluginsNew[id] = string(data)
		}
	} else {
		for id, raw := range common.AllProjectRawConfig {
			p, err := project.NewProject("", raw, id)
			if err != nil {
				logger.Error("project init error", "err", err, "project_id", id)
			}
			project.GlobalProject.Projects[id] = p
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

	// load project/input/output/ruleset/plugin
	LoadComponents()
	LoadProject()

	// start all projects
	StartAllProject()

	// start Api
	err = api.ServerStart(common.Config.Listen)
	if err != nil {
		logger.Error("server start err", "error", err.Error())
	}

	select {}
}
