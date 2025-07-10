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
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"gopkg.in/yaml.v3"
)

// stopPeriodicSync is closed on graceful shutdown to stop the component sync goroutine
var stopPeriodicSync chan struct{}

func main() {
	var (
		cfgRoot   = flag.String("config_root", "", "directory containing config.yaml and component sub dirs (required)")
		isLeader  = flag.Bool("leader", false, "run as cluster leader")
		port      = flag.Int("port", 8080, "HTTP listen port (leader only)")
		showVer   = flag.Bool("version", false, "show version")
		buildVers = "v0.1.5"
	)
	flag.Parse()

	if *showVer {
		fmt.Println(buildVers)
		return
	}

	// config_root is required for both leader and follower
	if *cfgRoot == "" {
		fmt.Println("config_root is required")
		return
	}

	// Load hub config (redis etc.)
	if err := loadHubConfig(*cfgRoot); err != nil {
		logger.Error("load hub config", "error", err)
		return
	}

	if *isLeader {
		// Initialize Redis-based sample manager (stores component data samples)
		common.InitRedisSampleManager()
		logger.Info("Starting in leader mode", "config_root", *cfgRoot)
	} else {
		logger.Info("Starting in follower mode", "config_root", *cfgRoot)
	}

	// Init Redis (mandatory). If fails, terminate Hub immediately.
	if err := common.RedisInit(common.Config.Redis, common.Config.RedisPassword); err != nil {
		logger.Error("failed to connect redis, hub will exit", "error", err)
		os.Exit(1)
	}

	// Initialize daily statistics manager (tracks real message counts)
	common.InitDailyStatsManager()

	// Detect local IP & init cluster manager
	ip, _ := common.GetLocalIP()
	common.Config.LocalIP = ip

	// Self address used by cluster components (include port for both leader and follower)
	selfAddr := fmt.Sprintf("%s:%d", ip, *port)

	cm := cluster.ClusterInit(ip, selfAddr)
	cluster.NodeID = ip

	// Register project command handler with cluster package
	cluster.SetProjectCommandHandler(project.GetProjectCommandHandler().(cluster.ProjectCommandHandler))

	if *isLeader {
		// Leader mode
		common.Config.Leader = ip
		cm.SetLeader(ip, selfAddr)
		cm.StartProjectStatesSyncLoop()
		token, _ := readToken(true)
		common.Config.Token = token

		// Start leader-specific cluster services
		cm.StartRedisHeartbeatSubscriber()
		cm.StartLeaderServices()

		// Initialize global component raw config maps for follower access
		initializeGlobalComponentMaps()
	} else {
		// Follower mode
		cm.StartHeartbeatLoop() // follower heartbeats only
		// Followers don't expose HTTP API, no token needed
		common.Config.Token = ""
		logger.Info("Follower mode initialized")

		// Sync components from leader before loading local components
		if err := syncComponentsFromLeader(); err != nil {
			logger.Warn("Failed to sync components from leader, using local components", "error", err)
		}

		// Start periodic sync check to ensure we have latest configurations
		stopPeriodicSync = make(chan struct{})
		go startPeriodicComponentSync()
	}

	// Load components & projects
	loadLocalComponents()
	loadLocalProjects()

	// Init monitors
	common.InitSystemMonitor(cluster.NodeID)

	// Start cluster background processes (heartbeat, cleanup, etc.)
	cm.Start()

	if *isLeader {
		// Leader extra services
		common.InitQPSManager()
		common.InitClusterSystemManager()

		listenAddr := fmt.Sprintf("0.0.0.0:%d", *port)
		go api.ServerStart(listenAddr) // start Echo API on specified port
	} else {
		// Start error log uploader for follower nodes
		api.StartErrorLogUploader()

		// Start operation history uploader for follower nodes
		api.StartOperationHistoryUploader()
	}

	// Start QPS collector on all nodes (leader and followers)
	common.InitQPSCollector(cluster.NodeID, func() []common.QPSMetrics {
		return project.GetQPSDataForNode(cluster.NodeID)
	}, func() *common.SystemMetrics {
		if common.GlobalSystemMonitor != nil {
			return common.GlobalSystemMonitor.GetCurrentMetrics()
		}
		return nil
	})

	// ========== Graceful shutdown handling ==========
	shutdownCtx, stopSignal := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignal()

	go func() {
		<-shutdownCtx.Done()
		logger.Info("shutdown signal received, stopping Hub components ...")
		if stopPeriodicSync != nil {
			close(stopPeriodicSync)
		}

		// 1. Stop all running projects (but don't save status - preserve user intention)
		for id, p := range project.GlobalProject.Projects {
			if p.Status == project.ProjectStatusRunning || p.Status == project.ProjectStatusStarting {
				logger.Info("stopping project for shutdown", "id", id)
				// Set a flag to indicate this is a shutdown stop (don't save status)
				p.SetShutdownMode(true)
				if err := p.Stop(); err != nil {
					logger.Warn("project shutdown stop error", "id", id, "error", err)
				}
			}
		}

		// 2. Stop collectors & managers
		common.StopQPSCollector()
		common.StopQPSManager()
		common.StopClusterSystemManager()
		common.StopDailyStatsManager()
		if rsm := common.GetRedisSampleManager(); rsm != nil {
			rsm.Close()
		}

		logger.Info("hub shutdown complete — bye ❄️")
		os.Exit(0)
	}()

	select {}
}

// ===== helpers (copied & simplified) =====

func traverseComponents(dir, suffix string) []string {
	var files []string
	filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && strings.HasSuffix(p, suffix) {
			files = append(files, p)
		}
		return nil
	})
	return files
}

func loadLocalComponents() {
	root := common.Config.ConfigRoot

	// plugins
	for _, f := range traverseComponents(path.Join(root, "plugin"), ".go") {
		name := common.GetFileNameWithoutExt(f)
		_ = plugin.NewPlugin(f, "", name, plugin.YAEGI_PLUGIN)
	}
	// Load plugin .new files
	for _, f := range traverseComponents(path.Join(root, "plugin"), ".go.new") {
		name := strings.TrimSuffix(common.GetFileNameWithoutExt(f), ".go")
		if content, err := os.ReadFile(f); err == nil {
			plugin.PluginsNew[name] = string(content)
		}
	}

	// inputs
	for _, f := range traverseComponents(path.Join(root, "input"), ".yaml") {
		id := common.GetFileNameWithoutExt(f)
		if inp, err := input.NewInput(f, "", id); err == nil {
			project.GlobalProject.Inputs[id] = inp
		}
	}
	// Load input .new files
	for _, f := range traverseComponents(path.Join(root, "input"), ".yaml.new") {
		id := strings.TrimSuffix(common.GetFileNameWithoutExt(f), ".yaml")
		if content, err := os.ReadFile(f); err == nil {
			project.GlobalProject.InputsNew[id] = string(content)
		}
	}

	// outputs
	for _, f := range traverseComponents(path.Join(root, "output"), ".yaml") {
		id := common.GetFileNameWithoutExt(f)
		if out, err := output.NewOutput(f, "", id); err == nil {
			project.GlobalProject.Outputs[id] = out
		}
	}
	// Load output .new files
	for _, f := range traverseComponents(path.Join(root, "output"), ".yaml.new") {
		id := strings.TrimSuffix(common.GetFileNameWithoutExt(f), ".yaml")
		if content, err := os.ReadFile(f); err == nil {
			project.GlobalProject.OutputsNew[id] = string(content)
		}
	}

	// rulesets
	for _, f := range traverseComponents(path.Join(root, "ruleset"), ".xml") {
		id := common.GetFileNameWithoutExt(f)
		if rs, err := rules_engine.NewRuleset(f, "", id); err == nil {
			project.GlobalProject.Rulesets[id] = rs
		}
	}
	// Load ruleset .new files
	for _, f := range traverseComponents(path.Join(root, "ruleset"), ".xml.new") {
		id := strings.TrimSuffix(common.GetFileNameWithoutExt(f), ".xml")
		if content, err := os.ReadFile(f); err == nil {
			project.GlobalProject.RulesetsNew[id] = string(content)
		}
	}
}

func loadLocalProjects() {
	root := common.Config.ConfigRoot
	for _, f := range traverseComponents(path.Join(root, "project"), ".yaml") {
		id := common.GetFileNameWithoutExt(f)
		if p, err := project.NewProject(f, "", id); err == nil {
			// Load persisted status from .project_status file (if any)
			if savedStatus, err2 := p.LoadProjectStatus(); err2 == nil {
				// If project was running before restart, start it now
				if savedStatus == project.ProjectStatusRunning {
					if err := p.Start(); err != nil {
						logger.Error("Failed to start project from saved status", "project", p.Id, "error", err)
						if cluster.IsLeader {
							common.RecordProjectOperation(common.OpTypeProjectStart, p.Id, "failed", err.Error(), nil)
						}
					} else {
						logger.Info("Successfully started project from saved status", "project", p.Id)
						if cluster.IsLeader {
							common.RecordProjectOperation(common.OpTypeProjectStart, p.Id, "success", "", nil)
						}
					}
				}
			} else {
				logger.Warn("Failed to load project status, using default stopped status", "id", id, "error", err2)
			}

			project.GlobalProject.Projects[id] = p

			// Ensure project status is synced to Redis for cluster visibility
			// This is important for follower nodes so leader can see their project states
			if err := p.SaveProjectStatus(); err != nil {
				logger.Warn("Failed to sync project status to Redis", "id", p.Id, "error", err)
			} else {
				logger.Debug("Synced project status to Redis", "id", p.Id, "status", p.Status)
			}
		} else {
			logger.Error("Failed to create project", "id", id, "error", err)
		}
	}

	// Load project .new files
	for _, f := range traverseComponents(path.Join(root, "project"), ".yaml.new") {
		id := strings.TrimSuffix(common.GetFileNameWithoutExt(f), ".yaml")
		if content, err := os.ReadFile(f); err == nil {
			project.GlobalProject.ProjectsNew[id] = string(content)
		}
	}

	logger.Info("Finished loading and start local projects", "total_projects", len(project.GlobalProject.Projects))
}

// readToken reads existing .token or creates one when create==true.
func readToken(create bool) (string, error) {
	tokenPath := common.GetConfigPath(".token")
	if data, err := os.ReadFile(tokenPath); err == nil {
		return strings.TrimSpace(string(data)), nil
	} else if create {
		token := common.NewUUID()
		if err := os.WriteFile(tokenPath, []byte(token), 0600); err != nil {
			return "", err
		}
		return token, nil
	}
	return "", fmt.Errorf("token file not found")
}

// syncComponentsFromLeader syncs components from Redis during follower startup
func syncComponentsFromLeader() error {
	logger.Info("Syncing components from Redis")

	// Wait for Redis connection to be established
	for i := 0; i < 30; i++ {
		if err := common.RedisPing(); err == nil {
			break
		}
		if i == 29 {
			return fmt.Errorf("failed to connect to Redis after 30 attempts")
		}
		time.Sleep(1 * time.Second)
	}

	configRoot := common.Config.ConfigRoot
	syncCount := 0

	// Sync components directly from Redis
	// Try to get the latest components from Redis keys used by the leader

	// Sync inputs
	if err := syncComponentTypeFromRedis("input", configRoot); err != nil {
		logger.Warn("Failed to sync inputs from Redis", "error", err)
	} else {
		syncCount++
	}

	// Sync outputs
	if err := syncComponentTypeFromRedis("output", configRoot); err != nil {
		logger.Warn("Failed to sync outputs from Redis", "error", err)
	} else {
		syncCount++
	}

	// Sync rulesets
	if err := syncComponentTypeFromRedis("ruleset", configRoot); err != nil {
		logger.Warn("Failed to sync rulesets from Redis", "error", err)
	} else {
		syncCount++
	}

	// Sync projects
	if err := syncComponentTypeFromRedis("project", configRoot); err != nil {
		logger.Warn("Failed to sync projects from Redis", "error", err)
	} else {
		syncCount++
	}

	// Sync plugins
	if err := syncComponentTypeFromRedis("plugin", configRoot); err != nil {
		logger.Warn("Failed to sync plugins from Redis", "error", err)
	} else {
		syncCount++
	}

	if syncCount > 0 {
		logger.Info("Successfully synced components from Redis", "synced_types", syncCount)
	} else {
		logger.Warn("No components synced from Redis, using local components")
	}

	return nil
}

// startPeriodicComponentSync periodically checks for component updates from leader
func startPeriodicComponentSync() {
	ticker := time.NewTicker(30 * time.Second) // Check every 30 seconds
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// Only sync if we're still a follower
			if !cluster.IsLeader {
				if err := syncComponentsFromLeader(); err != nil {
					logger.Debug("Periodic component sync failed", "error", err)
				}
			} else {
				// If we became leader, stop periodic sync
				return
			}
		case <-stopPeriodicSync:
			logger.Info("Stopping periodic component sync due to graceful shutdown")
			return
		}
	}
}

// syncComponentTypeFromRedis syncs a specific component type from Redis to local files
func syncComponentTypeFromRedis(componentType, configRoot string) error {
	// Get the appropriate global config map
	var configMap map[string]string
	var ext, dir string

	switch componentType {
	case "input":
		configMap = common.AllInputsRawConfig
		ext = ".yaml"
		dir = "input"
	case "output":
		configMap = common.AllOutputsRawConfig
		ext = ".yaml"
		dir = "output"
	case "ruleset":
		configMap = common.AllRulesetsRawConfig
		ext = ".xml"
		dir = "ruleset"
	case "project":
		configMap = common.AllProjectRawConfig
		ext = ".yaml"
		dir = "project"
	case "plugin":
		configMap = common.AllPluginsRawConfig
		ext = ".go"
		dir = "plugin"
	default:
		return fmt.Errorf("unsupported component type: %s", componentType)
	}

	if len(configMap) == 0 {
		logger.Debug("No components in global config map", "type", componentType)
		return nil
	}

	// Create directory if it doesn't exist
	dirPath := filepath.Join(configRoot, dir)
	if err := os.MkdirAll(dirPath, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dirPath, err)
	}

	// Write each component to file
	for id, content := range configMap {
		filePath := filepath.Join(dirPath, id+ext)
		if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
			logger.Error("Failed to write component file", "type", componentType, "id", id, "path", filePath, "error", err)
			continue
		}
		logger.Debug("Synced component from global config map", "type", componentType, "id", id, "path", filePath)
	}

	logger.Info("Synced component type from global config map", "type", componentType, "count", len(configMap))
	return nil
}

// initializeGlobalComponentMaps initializes the global component raw config maps with current components
func initializeGlobalComponentMaps() {
	logger.Info("Initializing global component raw config maps")

	// Initialize the maps if they are nil
	if common.AllInputsRawConfig == nil {
		common.AllInputsRawConfig = make(map[string]string)
	}
	if common.AllOutputsRawConfig == nil {
		common.AllOutputsRawConfig = make(map[string]string)
	}
	if common.AllRulesetsRawConfig == nil {
		common.AllRulesetsRawConfig = make(map[string]string)
	}
	if common.AllProjectRawConfig == nil {
		common.AllProjectRawConfig = make(map[string]string)
	}
	if common.AllPluginsRawConfig == nil {
		common.AllPluginsRawConfig = make(map[string]string)
	}

	common.GlobalMu.Lock()
	defer common.GlobalMu.Unlock()

	// Populate inputs
	for id, input := range project.GlobalProject.Inputs {
		common.AllInputsRawConfig[id] = input.Config.RawConfig
	}

	// Populate outputs
	for id, output := range project.GlobalProject.Outputs {
		common.AllOutputsRawConfig[id] = output.Config.RawConfig
	}

	// Populate rulesets
	for id, ruleset := range project.GlobalProject.Rulesets {
		common.AllRulesetsRawConfig[id] = ruleset.RawConfig
	}

	// Populate projects
	for id, proj := range project.GlobalProject.Projects {
		common.AllProjectRawConfig[id] = proj.Config.RawConfig
	}

	// Populate plugins
	for name, plug := range plugin.Plugins {
		if plug.Type == plugin.YAEGI_PLUGIN {
			common.AllPluginsRawConfig[name] = string(plug.Payload)
		}
	}

	logger.Info("Global component raw config maps initialized",
		"inputs", len(common.AllInputsRawConfig),
		"outputs", len(common.AllOutputsRawConfig),
		"rulesets", len(common.AllRulesetsRawConfig),
		"projects", len(common.AllProjectRawConfig),
		"plugins", len(common.AllPluginsRawConfig))
}

// loadHubConfig loads config.yaml inside given root directory into common.Config.
func loadHubConfig(root string) error {
	cfgFile := filepath.Join(root, "config.yaml")
	data, err := os.ReadFile(cfgFile)
	if err != nil {
		return err
	}
	if err := yaml.Unmarshal(data, &common.Config); err != nil {
		return err
	}
	common.Config.ConfigRoot = root
	return nil
}
