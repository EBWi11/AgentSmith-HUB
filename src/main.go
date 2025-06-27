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
	"time"

	"gopkg.in/yaml.v3"
)

var (
	Version = "0.1.2"
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
	tokenPath := common.GetConfigPath(".token")
	data, err := os.ReadFile(tokenPath)
	if err == nil {
		return string(data), nil
	}

	// Leader creates a new token
	if create {
		// Ensure directory exists before creating the file
		if err := os.MkdirAll(filepath.Dir(tokenPath), 0755); err != nil {
			return "", fmt.Errorf("failed to create token directory: %w", err)
		}

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
			logger.PluginError("travers plugin error", "error", err)
		}
		for _, v := range pluginList {
			name := common.GetFileNameWithoutExt(v)
			err := plugin.NewPlugin(v, "", name, plugin.YAEGI_PLUGIN)
			if err != nil {
				logger.PluginError("failed to create plugin instance", "error", err, "path", v)
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
			logger.PluginError("travers plugin new error", "error", err)
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

		rulesetListNew, err := traverseComponents(path.Join(common.Config.ConfigRoot, "ruleset"), ".xml.new")
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
		// For follower nodes, read from global config maps with read lock protection
		common.GlobalMu.RLock()

		// Create local copies to avoid holding lock during component creation
		pluginsConfig := make(map[string]string)
		for k, v := range common.AllPluginsRawConfig {
			pluginsConfig[k] = v
		}

		inputsConfig := make(map[string]string)
		for k, v := range common.AllInputsRawConfig {
			inputsConfig[k] = v
		}

		outputsConfig := make(map[string]string)
		for k, v := range common.AllOutputsRawConfig {
			outputsConfig[k] = v
		}

		rulesetsConfig := make(map[string]string)
		for k, v := range common.AllRulesetsRawConfig {
			rulesetsConfig[k] = v
		}

		common.GlobalMu.RUnlock()

		// Create components using local copies
		for name, raw := range pluginsConfig {
			err := plugin.NewPlugin("", raw, name, plugin.YAEGI_PLUGIN)
			if err != nil {
				logger.PluginError("failed to new plugin", "error", err)
				continue
			}
		}

		for id, raw := range inputsConfig {
			tmp, err := input.NewInput("", raw, id)
			if err != nil {
				logger.Error("failed to create input instance", "error", err, "id", id)
				continue
			}
			project.GlobalProject.Inputs[id] = tmp
		}

		for id, raw := range outputsConfig {
			tmp, err := output.NewOutput("", raw, id)
			if err != nil {
				logger.Error("failed to create output instance", "error", err, "id", id)
				continue
			}
			project.GlobalProject.Outputs[id] = tmp
		}

		for id, raw := range rulesetsConfig {
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
			project.GlobalProject.ProjectsNew[id] = string(data)
		}
	} else {
		// For follower nodes, read from global config map with read lock protection
		common.GlobalMu.RLock()

		// Create local copy to avoid holding lock during project creation
		projectsConfig := make(map[string]string)
		for k, v := range common.AllProjectRawConfig {
			projectsConfig[k] = v
		}

		common.GlobalMu.RUnlock()

		// Create projects using local copy
		for id, raw := range projectsConfig {
			p, err := project.NewProject("", raw, id)
			if err != nil {
				logger.Error("project init error", "err", err, "project_id", id)
			}
			project.GlobalProject.Projects[id] = p
		}
	}
}

func StartAllProject() {
	if project.GlobalProject != nil {
		for _, p := range project.GlobalProject.Projects {
			// Read the saved status to determine user's intention
			savedStatus, err := p.LoadProjectStatus()

			// Default behavior: start the project UNLESS explicitly stopped by user
			shouldStart := true

			if err != nil {
				// Project not found in .project_status file or file doesn't exist
				// This means it's a new project or not explicitly managed -> DEFAULT TO START
				logger.Info("Project not found in status file, defaulting to start", "project_id", p.Id, "error", err)
				shouldStart = true
			} else {
				// Project found in .project_status file, check if user explicitly stopped it
				if savedStatus == project.ProjectStatusStopped {
					// User explicitly stopped this project -> DON'T START
					shouldStart = false
					logger.Info("Project explicitly stopped by user, skipping start", "project_id", p.Id, "saved_status", savedStatus)
				} else {
					// Project in file but not stopped (running/error/etc.) -> START
					shouldStart = true
					logger.Info("Project found in status file and not stopped, will start", "project_id", p.Id, "saved_status", savedStatus)
				}
			}

			if shouldStart {
				// Start the project
				logger.Info("Starting project", "project_id", p.Id, "current_status", p.Status)

				// Double check the actual status before starting
				if p.Status == project.ProjectStatusRunning {
					logger.Error("ERROR: Project status is running before calling Start()! This should not happen!", "project_id", p.Id, "status", p.Status)
				}

				err = p.Start()
				if err != nil {
					logger.Error("project start error", "error", err, "project_id", p.Id)
				} else {
					logger.Info("project start successful", "project_id", p.Id)
				}
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
	common.Config.RedisPassword = leaderConfig["redis_password"]

	// Use write lock to safely initialize global config maps
	common.GlobalMu.Lock()
	defer common.GlobalMu.Unlock()

	plugins, err := api.GetAllComponents("plugin")
	if err != nil {
		logger.PluginError("load leader plugins error", "error", err.Error())
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

	// Create a new FlagSet to avoid inheriting testing flags
	fs := flag.NewFlagSet("agentsmith-hub", flag.ExitOnError)

	// Define our application flags
	configRoot := fs.String("config_root", "", "agent smith hub config path, only leader need")
	leaderAddr := fs.String("leader", "", "hub cluster leader address")
	version := fs.Bool("version", false, "show version information")

	// Custom usage function
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage of %s:\n", os.Args[0])
		fs.PrintDefaults()
	}

	// Parse command line flags
	_ = fs.Parse(os.Args[1:])

	// Handle version flag
	if *version {
		fmt.Printf("%s\n", Version)
		return
	}

	logger.Info("hub_starting", "config_root", *configRoot, "leader", *leaderAddr)

	// Ensure config directory exists early
	if err := common.EnsureConfigDirExists(); err != nil {
		logger.Warn("Failed to create config directory, will fallback to current directory", "error", err)
	}

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

	// CRITICAL: Set global cluster.NodeID to ensure QPS data collection works correctly
	cluster.NodeID = common.Config.LocalIP

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

	// Initialize QPS system based on node role
	if cluster.IsLeader {
		// Initialize QPS manager for leader nodes
		common.InitQPSManager()
		logger.Info("QPS manager initialized for leader", "node_id", cluster.NodeID)

		// Initialize cluster system manager for leader nodes
		common.InitClusterSystemManager()
		logger.Info("Cluster system manager initialized for leader", "node_id", cluster.NodeID)

		// Start local QPS data collection for leader node
		go func() {
			ticker := time.NewTicker(10 * time.Second) // Collect every 10 seconds
			defer ticker.Stop()

			for {
				select {
				case <-ticker.C:
					// Collect local QPS data and add to QPS manager
					qpsMetrics := project.GetQPSDataForNode(cluster.NodeID)
					if common.GlobalQPSManager != nil {
						for _, qpsData := range qpsMetrics {
							common.GlobalQPSManager.AddQPSData(&qpsData)
						}
					}
				}
			}
		}()
		logger.Info("Local QPS data collection started for leader", "node_id", cluster.NodeID)

		// Start local system metrics collection for leader node
		go func() {
			ticker := time.NewTicker(30 * time.Second) // Collect every 30 seconds
			defer ticker.Stop()

			for {
				select {
				case <-ticker.C:
					// Collect local system metrics and add to cluster system manager
					if common.GlobalSystemMonitor != nil && common.GlobalClusterSystemManager != nil {
						systemMetrics := common.GlobalSystemMonitor.GetCurrentMetrics()
						if systemMetrics != nil {
							common.GlobalClusterSystemManager.AddSystemMetrics(systemMetrics)
						}
					}
				}
			}
		}()
		logger.Info("Local system metrics collection started for leader", "node_id", cluster.NodeID)
	} else if common.Config.Leader != "" {
		// Initialize QPS collector for follower nodes
		common.InitQPSCollector(
			cluster.NodeID,
			common.Config.Leader,
			func() []common.QPSMetrics {
				return project.GetQPSDataForNode(cluster.NodeID)
			},
			func() *common.SystemMetrics {
				if common.GlobalSystemMonitor != nil {
					return common.GlobalSystemMonitor.GetCurrentMetrics()
				}
				return nil
			},
		)
		logger.Info("QPS collector initialized for follower", "node_id", cluster.NodeID, "leader", common.Config.Leader)
	}

	// start all projects
	StartAllProject()

	// Initialize system monitor for both leader and follower
	common.InitSystemMonitor(cluster.NodeID)
	logger.Info("System monitor initialized", "node_id", cluster.NodeID)

	// start Api
	err = api.ServerStart(common.Config.Listen)
	if err != nil {
		logger.Error("server start err", "error", err.Error())
	}

	select {}
}
