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

func main() {
	var (
		cfgRoot   = flag.String("config_root", "", "directory containing config.yaml and component sub dirs (required)")
		isLeader  = flag.Bool("leader", false, "run as cluster leader")
		port      = flag.Int("port", 8080, "HTTP listen port")
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

	// Detect local IP & init cluster manager
	ip, _ := common.GetLocalIP()
	common.Config.LocalIP = ip

	// Reinitialize logger with Redis error log writing capability and correct NodeID
	logger.InitLoggerWithRedisAndNodeID(ip, func(entry logger.RedisErrorLogEntry) error {
		// Convert logger entry to common entry format
		commonEntry := common.ErrorLogEntry{
			Timestamp: entry.Timestamp,
			Level:     entry.Level,
			Message:   entry.Message,
			Source:    entry.Source,
			NodeID:    entry.NodeID,
			Function:  entry.Function,
			File:      entry.File,
			Line:      entry.Line,
			Error:     entry.Error,
			Details:   entry.Details,
		}
		return common.WriteErrorLogToRedis(commonEntry)
	})

	// Reinitialize plugin logger with Redis error log writing capability and correct NodeID
	logger.InitPluginLoggerWithRedisAndNodeID(ip, func(entry logger.RedisErrorLogEntry) error {
		// Convert logger entry to common entry format
		commonEntry := common.ErrorLogEntry{
			Timestamp: entry.Timestamp,
			Level:     entry.Level,
			Message:   entry.Message,
			Source:    entry.Source,
			NodeID:    entry.NodeID,
			Function:  entry.Function,
			File:      entry.File,
			Line:      entry.Line,
			Error:     entry.Error,
			Details:   entry.Details,
		}
		return common.WriteErrorLogToRedis(commonEntry)
	})

	// Initialize daily statistics manager (tracks real message counts)
	common.InitDailyStatsManager()

	// Initialize new cluster system
	cluster.InitCluster(ip, *isLeader)

	// IMPORTANT: Set centralized cluster state
	common.SetClusterState(*isLeader, ip)

	// IMPORTANT: Also set the legacy global IsLeader variable for component compatibility
	common.SetLeaderState(*isLeader, ip)

	// Register project command handler with cluster package
	cluster.SetProjectCommandHandler(project.GetProjectCommandHandler().(cluster.ProjectCommandHandler))

	if *isLeader {
		// Leader mode
		common.Config.Leader = ip
		token, err := readToken(true)
		if err != nil {
			logger.Error("Failed to read or create leader token", "error", err)
			return
		}
		common.Config.Token = token

		// Store leader token in Redis for followers to use (no TTL)
		if err := api.WriteTokenToRedis(token); err != nil {
			logger.Warn("Failed to store leader token in Redis", "error", err)
		}

		loadLocalComponents()
		loadLocalProjects()
	} else {
		logger.Info("Follower mode initialized")
	}

	// Init monitors
	common.InitSystemMonitor(ip)

	if *isLeader {
		common.InitClusterSystemManager()
		cluster.GlobalClusterManager.Start()

		listenAddr := fmt.Sprintf("0.0.0.0:%d", *port)
		go api.ServerStart(listenAddr) // start Echo API on specified port
	} else {
		// Follower services
		// Note: Error log uploader removed - all nodes write directly to Redis in real-time
		// Note: Operation history uploader removed - all nodes write directly to Redis

		// Token will be read by follower API server at startup
		cluster.GlobalClusterManager.Start()

		// Start follower API server (read-only endpoints)
		followerAddr := fmt.Sprintf("0.0.0.0:%d", *port)
		go api.ServerStartFollower(followerAddr) // start follower API server
		logger.Info("Follower API server starting", "address", followerAddr)
	}

	// ========== Graceful shutdown handling ==========
	shutdownCtx, stopSignal := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignal()

	go func() {
		<-shutdownCtx.Done()
		logger.Info("shutdown signal received, starting graceful shutdown process...")

		// Create a timeout context for the entire shutdown process
		shutdownTimeout := 60 * time.Second
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer shutdownCancel()

		// Channel to track shutdown completion
		shutdownComplete := make(chan struct{})

		go func() {
			defer close(shutdownComplete)

			// Stop all running projects (Stop method handles data drain internally)
			logger.Info("Stopping all running projects")
			project.ForEachProject(func(id string, proj *project.Project) bool {
				if proj.Status == common.StatusRunning {
					logger.Info("Stopping project during shutdown", "project", proj.Id)
					err := proj.Stop()
					if err != nil {
						logger.Error("Failed to stop project during shutdown", "project", proj.Id, "error", err)
					} else {
						logger.Info("Project stopped successfully during shutdown", "project", proj.Id)
					}
				}
				return true
			})

			if cluster.GlobalClusterManager != nil {
				cluster.GlobalClusterManager.Stop()
			}

			common.StopClusterSystemManager()
			common.StopDailyStatsManager()
			if rsm := common.GetRedisSampleManager(); rsm != nil {
				rsm.Close()
			}
		}()

		// Wait for shutdown completion or timeout
		select {
		case <-shutdownComplete:
			logger.Info("Shutdown completed within timeout")
		case <-shutdownCtx.Done():
			logger.Error("Shutdown timeout exceeded, forcing exit")
			// Force cleanup of critical resources
			project.ForEachProject(func(id string, p *project.Project) bool {
				if p.Status == common.StatusRunning || p.Status == common.StatusStarting {
					logger.Warn("Force stopping project", "id", id)
					p.SetProjectStatus(common.StatusStopped, nil)
				}
				return true
			})
		}

		logger.Info("Hub shutdown complete — bye")
		os.Exit(0)
	}()

	select {}
}

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
	var err error
	// Only leader loads local components
	root := common.Config.ConfigRoot

	// plugins
	for _, f := range traverseComponents(path.Join(root, "plugin"), ".go") {
		name := common.GetFileNameWithoutExt(f)
		common.GlobalMu.Lock()
		if content, err := os.ReadFile(f); err == nil {
			// Update global config map
			common.AllPluginsRawConfig[name] = string(content)
		}
		err = plugin.NewPlugin(f, "", name, plugin.YAEGI_PLUGIN)
		if err != nil {
			logger.Error("Failed to load plugin", "file", f, "error", err)
		}
		common.GlobalMu.Unlock()
	}
	// Load plugin .new files
	for _, f := range traverseComponents(path.Join(root, "plugin"), ".go.new") {
		common.GlobalMu.Lock()
		name := strings.TrimSuffix(common.GetFileNameWithoutExt(f), ".go")
		if content, err := os.ReadFile(f); err != nil {
			logger.Error("Failed to load new plugin", "file", f, "error", err)
		} else {
			plugin.PluginsNew[name] = string(content)
		}
		common.GlobalMu.Unlock()
	}

	// inputs
	for _, f := range traverseComponents(path.Join(root, "input"), ".yaml") {
		id := common.GetFileNameWithoutExt(f)
		common.GlobalMu.Lock()
		if content, err := os.ReadFile(f); err == nil {
			// Update global config map
			common.AllInputsRawConfig[id] = string(content)
		}
		if inp, err := input.NewInput(f, "", id); err != nil {
			logger.Error("Failed to load new input", "file", f, "error", err)
		} else {
			project.SetInput(id, inp)
		}
		common.GlobalMu.Unlock()
	}
	// Load input .new files
	for _, f := range traverseComponents(path.Join(root, "input"), ".yaml.new") {
		common.GlobalMu.Lock()
		id := strings.TrimSuffix(common.GetFileNameWithoutExt(f), ".yaml")
		if content, err := os.ReadFile(f); err != nil {
			logger.Error("Failed to load new input", "file", f, "error", err)
		} else {
			project.SetInputNew(id, string(content))
		}
		common.GlobalMu.Unlock()
	}

	// outputs
	for _, f := range traverseComponents(path.Join(root, "output"), ".yaml") {
		id := common.GetFileNameWithoutExt(f)
		common.GlobalMu.Lock()
		if content, err := os.ReadFile(f); err == nil {
			// Update global config map
			common.AllOutputsRawConfig[id] = string(content)
		}
		if out, err := output.NewOutput(f, "", id); err != nil {
			logger.Error("Failed to load output", "file", f, "error", err)
		} else {
			project.SetOutput(id, out)
		}
		common.GlobalMu.Unlock()
	}
	// Load output .new files
	for _, f := range traverseComponents(path.Join(root, "output"), ".yaml.new") {
		id := strings.TrimSuffix(common.GetFileNameWithoutExt(f), ".yaml")
		common.GlobalMu.Lock()
		if content, err := os.ReadFile(f); err != nil {
			logger.Error("Failed to load new output", "file", f, "error", err)
		} else {
			project.SetOutputNew(id, string(content))
		}
		common.GlobalMu.Unlock()
	}

	// rulesets
	for _, f := range traverseComponents(path.Join(root, "ruleset"), ".xml") {
		id := common.GetFileNameWithoutExt(f)
		common.GlobalMu.Lock()
		if content, err := os.ReadFile(f); err == nil {
			// Update global config map
			common.AllRulesetsRawConfig[id] = string(content)
		}
		if rs, err := rules_engine.NewRuleset(f, "", id); err != nil {
			logger.Error("Failed to load ruleset", "file", f, "error", err)
		} else {
			project.SetRuleset(id, rs)
		}
		common.GlobalMu.Unlock()
	}
	// Load ruleset .new files
	for _, f := range traverseComponents(path.Join(root, "ruleset"), ".xml.new") {
		id := strings.TrimSuffix(common.GetFileNameWithoutExt(f), ".xml")
		common.GlobalMu.Lock()
		if content, err := os.ReadFile(f); err != nil {
			logger.Error("Failed to load new ruleset", "file", f, "error", err)
		} else {
			project.SetRulesetNew(id, string(content))
		}
		common.GlobalMu.Unlock()
	}

	logger.Info("Leader finished loading local components")
}

func loadLocalProjects() {
	root := common.Config.ConfigRoot
	for _, f := range traverseComponents(path.Join(root, "project"), ".yaml") {
		id := common.GetFileNameWithoutExt(f)
		// Read project content for global config map (NewProject will also update it, but we do it here for consistency)
		if content, err := os.ReadFile(f); err == nil {
			// Update global config map
			if common.AllProjectRawConfig == nil {
				common.AllProjectRawConfig = make(map[string]string)
			}
			common.GlobalMu.Lock()
			common.AllProjectRawConfig[id] = string(content)
			common.GlobalMu.Unlock()
		}

		if p, err := project.NewProject(f, "", id, false); err == nil {
			project.SetProject(id, p)

			// Try to restore project status from Redis based on user intention
			if userWantsRunning, err := common.GetProjectUserIntention(id); err == nil && userWantsRunning {
				// User wants project to be running, start it
				logger.Info("Restoring project to running state based on user intention", "id", p.Id)
				common.GlobalMu.Lock()
				if err := p.Start(); err != nil {
					logger.Error("Failed to start project during restore", "project", p.Id, "error", err)
					// Record failed restore operation
					common.RecordProjectOperation(common.OpTypeProjectStart, p.Id, "failed", err.Error(), map[string]interface{}{
						"triggered_by": "system_restore",
						"node_id":      common.Config.LocalIP,
					})
				} else {
					logger.Info("Successfully restored project to running state", "id", p.Id)
					// Record successful restore operation
					common.RecordProjectOperation(common.OpTypeProjectStart, p.Id, "success", "", map[string]interface{}{
						"triggered_by": "system_restore",
						"node_id":      common.Config.LocalIP,
					})
				}
				common.GlobalMu.Unlock()
			} else {
				p.Status = common.StatusStopped
				logger.Info("Project not intended to be running by user, defaulting to stopped", "id", p.Id)
			}
		} else {
			logger.Error("Failed to create project", "project", id, "error", err)
		}
	}

	// Load project .new files
	for _, f := range traverseComponents(path.Join(root, "project"), ".yaml.new") {
		id := strings.TrimSuffix(common.GetFileNameWithoutExt(f), ".yaml")
		if content, err := os.ReadFile(f); err != nil {
			logger.Error("Failed to read new project", "project", id, "error", err)
		} else {
			common.GlobalMu.Lock()
			project.SetProjectNew(id, string(content))
			common.GlobalMu.Unlock()
		}
	}
	logger.Info("Finished loading and start local projects", "total_projects", project.GetProjectsCount())
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
