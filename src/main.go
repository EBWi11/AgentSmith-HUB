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

	"runtime"
	"sync"

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

	// Initialize daily statistics manager (tracks real message counts)
	common.InitDailyStatsManager()

	// Detect local IP & init cluster manager
	ip, _ := common.GetLocalIP()
	common.Config.LocalIP = ip

	// Initialize new cluster system
	cluster.InitCluster(ip, *isLeader)

	// IMPORTANT: Set centralized cluster state
	common.SetClusterState(*isLeader, ip)

	// Register project command handler with cluster package
	cluster.SetProjectCommandHandler(project.GetProjectCommandHandler().(cluster.ProjectCommandHandler))

	if *isLeader {
		// Leader mode
		common.Config.Leader = ip
		token, _ := readToken(true)
		common.Config.Token = token

		// Store leader token in Redis for followers to use (no TTL)
		if err := api.WriteTokenToRedis(token); err != nil {
			logger.Warn("Failed to store leader token in Redis", "error", err)
		}

		logger.Info("Leader mode initialized")
	} else {
		// Follower mode - token will be read by follower API server when it starts
		logger.Info("Follower mode initialized")
	}

	// Load components & projects (this will use the synced configurations)
	loadLocalComponents()
	loadLocalProjects()

	// Init monitors
	common.InitSystemMonitor(ip)

	// Start cluster background processes (heartbeat, cleanup, etc.)
	if cluster.GlobalClusterManager != nil {
		cluster.GlobalClusterManager.Start()
	}

	if *isLeader {
		// Leader extra services
		common.InitQPSManager()
		common.InitClusterSystemManager()

		listenAddr := fmt.Sprintf("0.0.0.0:%d", *port)
		go api.ServerStart(listenAddr) // start Echo API on specified port
	} else {
		// Follower services
		// Start error log uploader for follower nodes
		api.StartErrorLogUploader()

		// Start operation history uploader for follower nodes
		api.StartOperationHistoryUploader()

		// Token will be read by follower API server at startup

		// Start follower API server (read-only endpoints)
		followerAddr := fmt.Sprintf("0.0.0.0:%d", *port)
		go api.ServerStartFollower(followerAddr) // start follower API server
		logger.Info("Follower API server starting", "address", followerAddr)
	}

	// Start QPS collector on all nodes (leader and followers)
	common.InitQPSCollector(ip, func() []common.QPSMetrics {
		return project.GetQPSDataForNode(ip)
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
		logger.Info("shutdown signal received, starting graceful shutdown process...")

		// Create a timeout context for the entire shutdown process
		shutdownTimeout := 60 * time.Second
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer shutdownCancel()

		// Channel to track shutdown completion
		shutdownComplete := make(chan struct{})

		go func() {
			defer close(shutdownComplete)

			// Phase 1: Stop accepting new requests and notify cluster
			logger.Info("Phase 1: Stopping API server and notifying cluster...")

			// Notify other cluster nodes that this node is shutting down
			if *isLeader {
				// Remove leader ready flag
				if err := common.RedisDel(common.ClusterLeaderReadyKey); err != nil {
					logger.Warn("Failed to remove leader ready flag", "error", err)
				}
			}

			// Phase 2: Stop all running projects gracefully
			logger.Info("Phase 2: Stopping all running projects...")
			projectStopTimeout := 30 * time.Second
			projectStopCtx, projectStopCancel := context.WithTimeout(context.Background(), projectStopTimeout)
			defer projectStopCancel()

			var projectWg sync.WaitGroup
			projectStopResults := make(chan struct {
				id  string
				err error
			}, len(project.GlobalProject.Projects))

			for id, p := range project.GlobalProject.Projects {
				if p.Status == project.ProjectStatusRunning || p.Status == project.ProjectStatusStarting {
					projectWg.Add(1)
					go func(projectID string, proj *project.Project) {
						defer projectWg.Done()
						logger.Info("Stopping project for shutdown", "id", projectID)

						// Create a timeout for individual project stop
						projectCtx, projectCancel := context.WithTimeout(projectStopCtx, 20*time.Second)
						defer projectCancel()

						stopComplete := make(chan error, 1)
						go func() {
							// Use StopForShutdown to avoid changing project status in Redis
							stopComplete <- proj.StopForShutdown()
						}()

						select {
						case err := <-stopComplete:
							projectStopResults <- struct {
								id  string
								err error
							}{projectID, err}
						case <-projectCtx.Done():
							logger.Warn("Project stop timeout, will force cleanup", "id", projectID)
							projectStopResults <- struct {
								id  string
								err error
							}{projectID, fmt.Errorf("stop timeout")}
						}
					}(id, p)
				}
			}

			// Wait for all projects to stop or timeout
			go func() {
				projectWg.Wait()
				close(projectStopResults)
			}()

			// Collect results
			stoppedCount := 0
			failedCount := 0
			for result := range projectStopResults {
				if result.err != nil {
					logger.Warn("Project stop failed", "id", result.id, "error", result.err)
					failedCount++
				} else {
					logger.Info("Project stopped successfully", "id", result.id)
					stoppedCount++
				}
			}

			logger.Info("Project shutdown phase completed", "stopped", stoppedCount, "failed", failedCount)

			// Phase 3: Stop cluster services
			logger.Info("Phase 3: Stopping cluster services...")
			if cluster.GlobalClusterManager != nil {
				cluster.GlobalClusterManager.Stop()
			}

			// Phase 4: Stop background services
			logger.Info("Phase 4: Stopping background services...")
			common.StopQPSCollector()
			common.StopQPSManager()
			common.StopClusterSystemManager()
			common.StopDailyStatsManager()
			if rsm := common.GetRedisSampleManager(); rsm != nil {
				rsm.Close()
			}

			// Phase 5: Final cleanup
			logger.Info("Phase 5: Final cleanup...")
			// Force garbage collection to clean up any remaining resources
			runtime.GC()
			time.Sleep(100 * time.Millisecond)

			logger.Info("Graceful shutdown completed successfully")
		}()

		// Wait for shutdown completion or timeout
		select {
		case <-shutdownComplete:
			logger.Info("Shutdown completed within timeout")
		case <-shutdownCtx.Done():
			logger.Error("Shutdown timeout exceeded, forcing exit")
			// Force cleanup of critical resources
			for id, p := range project.GlobalProject.Projects {
				if p.Status == project.ProjectStatusRunning || p.Status == project.ProjectStatusStarting {
					logger.Warn("Force stopping project", "id", id)
					// Don't wait for graceful stop, just mark as stopped
					p.Status = project.ProjectStatusStopped
				}
			}
		}

		logger.Info("Hub shutdown complete — bye ❄️")
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
	// Followers don't read local component files, they receive components via cluster sync
	if !common.IsCurrentNodeLeader() {
		logger.Info("Follower node: skipping local component loading, will receive components via cluster sync")
		return
	}

	// Only leader loads local components
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

	logger.Info("Leader finished loading local components")
}

func loadLocalProjects() {
	// Followers don't have local project files, only leaders do
	if !common.IsCurrentNodeLeader() {
		logger.Info("Follower node: skipping local project loading, will receive projects via cluster sync")
		return
	}

	root := common.Config.ConfigRoot
	for _, f := range traverseComponents(path.Join(root, "project"), ".yaml") {
		id := common.GetFileNameWithoutExt(f)
		if p, err := project.NewProject(f, "", id); err == nil {
			project.GlobalProject.Projects[id] = p

			// Try to restore project status from Redis and actually start if needed
			hashKey := "cluster:proj_states:" + common.Config.LocalIP
			if savedStatus, err := common.RedisHGet(hashKey, id); err == nil && savedStatus != "" {
				// Restore status from Redis
				switch savedStatus {
				case "running":
					// Actually start the project, don't just set status
					logger.Info("Restoring project to running state", "id", p.Id)
					if err := p.Start(); err != nil {
						logger.Error("Failed to start project during restore", "id", p.Id, "error", err)
						p.Status = project.ProjectStatusStopped
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
				case "stopped":
					p.Status = project.ProjectStatusStopped
					logger.Info("Restored project status from Redis", "id", p.Id, "status", "stopped")
				default:
					// For other statuses (starting, stopping, error), default to stopped
					p.Status = project.ProjectStatusStopped
					logger.Info("Restored project status from Redis with fallback", "id", p.Id, "saved_status", savedStatus, "final_status", "stopped")
				}
			} else {
				// No saved status or Redis error, default to stopped
				p.Status = project.ProjectStatusStopped
				logger.Info("No saved status in Redis, defaulting to stopped", "id", p.Id)
			}

			// Sync current status to Redis for cluster visibility
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
