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
	"net/http"
	_ "net/http/pprof"
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
		buildVers = "v0.1.6"
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

	// Initialize component monitor with 30 second interval
	common.GlobalComponentMonitor = common.NewComponentMonitor(30 * time.Second)
	if err := common.GlobalComponentMonitor.Start(); err != nil {
		logger.Error("Failed to start component monitor", "error", err)
	} else {
		logger.Info("Component monitor started successfully")
	}

	// Start pprof server if enabled
	startPprofServer()

	if *isLeader {
		common.InitClusterSystemManager()
		err := cluster.GlobalClusterManager.Start()
		if err != nil {
			logger.Error("InitClusterSystemManager Error", "error", err)
			return
		}

		listenAddr := fmt.Sprintf("0.0.0.0:%d", *port)
		go api.ServerStart(listenAddr) // start Echo API on specified port
	} else {
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
			// Collect running projects first to avoid deadlock
			var runningProjects []*project.Project
			project.ForEachProject(func(id string, proj *project.Project) bool {
				if proj.Status == common.StatusRunning {
					runningProjects = append(runningProjects, proj)
				}
				return true
			})

			// Stop projects without holding locks
			for _, proj := range runningProjects {
				logger.Info("Stopping project during shutdown", "project", proj.Id)
				err := proj.Stop(true)
				if err != nil {
					logger.Error("Failed to stop project during shutdown", "project", proj.Id, "error", err)
				} else {
					logger.Info("Project stopped successfully during shutdown", "project", proj.Id)
				}
			}

			if cluster.GlobalClusterManager != nil {
				cluster.GlobalClusterManager.Stop()
			}

			// Stop component monitor
			if common.GlobalComponentMonitor != nil {
				logger.Info("Stopping component monitor")
				if err := common.GlobalComponentMonitor.Stop(); err != nil {
					logger.Error("Failed to stop component monitor", "error", err)
				} else {
					logger.Info("Component monitor stopped successfully")
				}
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
		if content, err := os.ReadFile(f); err == nil {
			// Update global config map
			common.SetRawConfig("plugin", name, string(content))
		}
		err = plugin.NewPlugin(f, "", name, plugin.YAEGI_PLUGIN)
		if err != nil {
			logger.Error("Failed to load plugin", "file", f, "error", err)
			// Create an error placeholder plugin to show in list
			errorPlugin := &plugin.Plugin{
				Name:   name,
				Path:   f,
				Type:   plugin.YAEGI_PLUGIN,
				Status: common.StatusError,
				Err:    err,
			}
			// Read raw config for display purposes
			if content, readErr := os.ReadFile(f); readErr == nil {
				errorPlugin.Payload = content
			}
			// Add to global plugin map with mutex protection
			common.GlobalMu.Lock()
			plugin.Plugins[name] = errorPlugin
			common.GlobalMu.Unlock()
		}
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
		if content, err := os.ReadFile(f); err == nil {
			// Update global config map
			common.SetRawConfig("input", id, string(content))
		}
		if inp, err := input.NewInput(f, "", id); err != nil {
			logger.Error("Failed to load new input", "file", f, "error", err)
			// Create an error placeholder input to show in list
			errorInput := &input.Input{
				Id:     id,
				Path:   f,
				Status: common.StatusError,
				Err:    err,
			}
			// Read raw config for display purposes
			if content, readErr := os.ReadFile(f); readErr == nil {
				cfg := &input.InputConfig{RawConfig: string(content)}
				errorInput.Config = cfg
			}
			project.SetInput(id, errorInput)
		} else {
			project.SetInput(id, inp)
		}
	}
	// Load input .new files
	for _, f := range traverseComponents(path.Join(root, "input"), ".yaml.new") {
		id := strings.TrimSuffix(common.GetFileNameWithoutExt(f), ".yaml")
		if content, err := os.ReadFile(f); err != nil {
			logger.Error("Failed to load new input", "file", f, "error", err)
		} else {
			project.SetInputNew(id, string(content))
		}
	}

	// outputs
	for _, f := range traverseComponents(path.Join(root, "output"), ".yaml") {
		id := common.GetFileNameWithoutExt(f)
		if content, err := os.ReadFile(f); err == nil {
			// Update global config map
			common.SetRawConfig("output", id, string(content))
		}
		if out, err := output.NewOutput(f, "", id); err != nil {
			logger.Error("Failed to load output", "file", f, "error", err)
			// Create an error placeholder output to show in list
			errorOutput := &output.Output{
				Id:     id,
				Path:   f,
				Status: common.StatusError,
				Err:    err,
			}
			// Read raw config for display purposes
			if content, readErr := os.ReadFile(f); readErr == nil {
				cfg := &output.OutputConfig{RawConfig: string(content)}
				errorOutput.Config = cfg
			}
			project.SetOutput(id, errorOutput)
		} else {
			project.SetOutput(id, out)
		}
	}
	// Load output .new files
	for _, f := range traverseComponents(path.Join(root, "output"), ".yaml.new") {
		id := strings.TrimSuffix(common.GetFileNameWithoutExt(f), ".yaml")
		if content, err := os.ReadFile(f); err != nil {
			logger.Error("Failed to load new output", "file", f, "error", err)
		} else {
			project.SetOutputNew(id, string(content))
		}
	}

	// rulesets
	for _, f := range traverseComponents(path.Join(root, "ruleset"), ".xml") {
		id := common.GetFileNameWithoutExt(f)
		if content, err := os.ReadFile(f); err == nil {
			// Update global config map
			common.SetRawConfig("ruleset", id, string(content))
		}
		if rs, err := rules_engine.NewRuleset(f, "", id); err != nil {
			logger.Error("Failed to load ruleset", "file", f, "error", err)
			// Create an error placeholder ruleset to show in list
			errorRuleset := &rules_engine.Ruleset{
				RulesetID: id,
				Path:      f,
				Status:    common.StatusError,
				Err:       err,
			}
			// Read raw config for display purposes
			if content, readErr := os.ReadFile(f); readErr == nil {
				errorRuleset.RawConfig = string(content)
			}
			project.SetRuleset(id, errorRuleset)
		} else {
			project.SetRuleset(id, rs)
		}
	}
	// Load ruleset .new files
	for _, f := range traverseComponents(path.Join(root, "ruleset"), ".xml.new") {
		id := strings.TrimSuffix(common.GetFileNameWithoutExt(f), ".xml")
		if content, err := os.ReadFile(f); err != nil {
			logger.Error("Failed to load new ruleset", "file", f, "error", err)
		} else {
			project.SetRulesetNew(id, string(content))
		}
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
			common.SetRawConfig("project", id, string(content))
		}

		if p, err := project.NewProject(f, "", id, false); err == nil {
			project.SetProject(id, p)

			// Try to restore project status from Redis based on user intention
			if userWantsRunning, err := common.GetProjectUserIntention(id); err == nil && userWantsRunning {
				// User wants project to be running, start it
				logger.Info("Restoring project to running state based on user intention", "id", p.Id)
				if err := p.Start(true); err != nil {
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
			} else {
				p.Status = common.StatusStopped
				logger.Info("Project not intended to be running by user, defaulting to stopped", "id", p.Id)
			}
		} else {
			logger.Error("Failed to create project", "project", id, "error", err)
			// Create an error placeholder project to show in list
			errorProject := &project.Project{
				Id:     id,
				Status: common.StatusError,
				Err:    err,
			}
			// Read raw config for display purposes
			if content, readErr := os.ReadFile(f); readErr == nil {
				cfg := &project.ProjectConfig{
					RawConfig: string(content),
					Path:      f,
				}
				errorProject.Config = cfg
			}
			project.SetProject(id, errorProject)
		}
	}

	// Load project .new files
	for _, f := range traverseComponents(path.Join(root, "project"), ".yaml.new") {
		id := strings.TrimSuffix(common.GetFileNameWithoutExt(f), ".yaml")
		if content, err := os.ReadFile(f); err != nil {
			logger.Error("Failed to read new project", "project", id, "error", err)
		} else {
			project.SetProjectNew(id, string(content))
		}
	}
	logger.Info("Finished loading and start local projects", "total_projects", project.GetProjectsCount())
}

// readToken reads token from environment variable first, then from .token file, or creates one when create==true.
func readToken(create bool) (string, error) {
	// First check environment variable
	if envToken := os.Getenv("AGENTSMITH_TOKEN"); envToken != "" {
		logger.Info("Using token from environment variable")
		return strings.TrimSpace(envToken), nil
	}

	// Fallback to file-based token
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
	// Initialize config
	common.Config = &common.HubConfig{}

	// Try to load from config file first
	cfgFile := filepath.Join(root, "config.yaml")
	if data, err := os.ReadFile(cfgFile); err == nil {
		if err := yaml.Unmarshal(data, &common.Config); err != nil {
			logger.Error("Failed to parse config.yaml", "error", err)
		}
	}

	// Override with environment variables if set
	if envRedis := os.Getenv("REDIS_HOST"); envRedis != "" {
		common.Config.Redis = envRedis
		logger.Info("Using Redis host from environment variable", "host", envRedis)
	}

	if envRedisPort := os.Getenv("REDIS_PORT"); envRedisPort != "" {
		// If REDIS_HOST is set, append port to it
		if common.Config.Redis == "" {
			common.Config.Redis = "localhost:" + envRedisPort
		} else {
			// Extract host from current Redis config and append new port
			if strings.Contains(common.Config.Redis, ":") {
				host := strings.Split(common.Config.Redis, ":")[0]
				common.Config.Redis = host + ":" + envRedisPort
			} else {
				common.Config.Redis = common.Config.Redis + ":" + envRedisPort
			}
		}
		logger.Info("Using Redis port from environment variable", "port", envRedisPort)
	}

	if envRedisPassword := os.Getenv("REDIS_PASSWORD"); envRedisPassword != "" {
		common.Config.RedisPassword = envRedisPassword
		logger.Info("Using Redis password from environment variable")
	}

	// Set config root
	common.Config.ConfigRoot = root

	// Validate Redis configuration
	if common.Config.Redis == "" {
		return fmt.Errorf("Redis host not configured. Please set REDIS_HOST environment variable or configure in config.yaml")
	}

	logger.Info("Final Redis configuration", "host", common.Config.Redis, "password_set", common.Config.RedisPassword != "")

	return nil
}

// startPprofServer starts the pprof HTTP server if enabled in configuration
func startPprofServer() {
	if common.Config.Pprof == nil || !common.Config.Pprof.Enabled {
		logger.Debug("pprof server disabled in configuration")
		return
	}

	port := common.Config.Pprof.Port
	if port <= 0 {
		port = 6060 // Default pprof port
	}

	pprofAddr := fmt.Sprintf("0.0.0.0:%d", port)

	go func() {
		logger.Info("Starting pprof server", "address", pprofAddr,
			"endpoints", []string{
				fmt.Sprintf("http://%s/debug/pprof/", pprofAddr),
				fmt.Sprintf("http://%s/debug/pprof/goroutine", pprofAddr),
				fmt.Sprintf("http://%s/debug/pprof/heap", pprofAddr),
				fmt.Sprintf("http://%s/debug/pprof/profile", pprofAddr),
				fmt.Sprintf("http://%s/debug/pprof/trace", pprofAddr),
			})

		// Create a simple HTTP server for pprof
		server := &http.Server{
			Addr:    pprofAddr,
			Handler: http.DefaultServeMux, // pprof handlers are registered to DefaultServeMux
		}

		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("pprof server failed", "error", err, "address", pprofAddr)
		}
	}()

	logger.Info("pprof server enabled", "address", pprofAddr,
		"help", "Access performance profiles at http://"+pprofAddr+"/debug/pprof/")
}
