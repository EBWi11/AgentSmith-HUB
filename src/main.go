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

	"gopkg.in/yaml.v3"
)

func main() {
	var (
		cfgRoot   = flag.String("config_root", "", "directory containing config.yaml and component sub dirs (required)")
		isLeader  = flag.Bool("leader", false, "run as cluster leader")
		port      = flag.Int("port", 8080, "HTTP listen port (leader only)")
		showVer   = flag.Bool("version", false, "show version")
		buildVers = "v0.2.0"
	)
	flag.Parse()

	if *showVer {
		fmt.Println(buildVers)
		return
	}
	if *cfgRoot == "" {
		fmt.Println("config_root is required")
		return
	}

	// Load hub config (redis etc.)
	if err := loadHubConfig(*cfgRoot); err != nil {
		logger.Error("load hub config", "error", err)
		return
	}

	// Init Redis (mandatory). If fails, terminate Hub immediately.
	if err := common.RedisInit(common.Config.Redis, common.Config.RedisPassword); err != nil {
		logger.Error("failed to connect redis, hub will exit", "error", err)
		os.Exit(1)
	}

	// Initialize Redis-based sample manager (stores component data samples)
	common.InitRedisSampleManager()

	// Initialize daily statistics manager (tracks real message counts)
	common.InitDailyStatsManager()

	// Detect local IP & init cluster manager
	ip, _ := common.GetLocalIP()
	common.Config.LocalIP = ip

	// Self address used by cluster components (include port only for leader)
	selfAddr := ip
	if *isLeader {
		selfAddr = fmt.Sprintf("%s:%d", ip, *port)
	}

	cm := cluster.ClusterInit(ip, selfAddr)
	cluster.NodeID = ip

	if *isLeader {
		common.Config.Leader = ip
		cm.SetLeader(ip, selfAddr)
		cm.StartHeartbeatLoop()
		cm.StartProjectStatesSyncLoop()
		token, _ := readToken(true)
		common.Config.Token = token
	} else {
		cm.StartHeartbeatLoop() // follower heartbeats only
		// Followers don't expose HTTP API, no token needed
		common.Config.Token = ""
	}

	// Load components & projects
	loadLocalComponents()
	loadLocalProjects()

	// Start/Stop projects depending on status file
	StartAllProject()

	// Init monitors
	common.InitSystemMonitor(cluster.NodeID)

	if *isLeader {
		// Leader extra services
		common.InitQPSManager()
		common.InitClusterSystemManager()

		listenAddr := fmt.Sprintf("0.0.0.0:%d", *port)
		go api.ServerStart(listenAddr) // start Echo API on specified port
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

		// 1. Stop all running projects (allow 2-min timeout inside Stop())
		for id, p := range project.GlobalProject.Projects {
			if p.Status == project.ProjectStatusRunning || p.Status == project.ProjectStatusStarting {
				logger.Info("stopping project", "id", id)
				if err := p.Stop(); err != nil {
					logger.Warn("project stop error", "id", id, "error", err)
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

	// inputs
	for _, f := range traverseComponents(path.Join(root, "input"), ".yaml") {
		id := common.GetFileNameWithoutExt(f)
		if inp, err := input.NewInput(f, "", id); err == nil {
			project.GlobalProject.Inputs[id] = inp
		}
	}

	// outputs
	for _, f := range traverseComponents(path.Join(root, "output"), ".yaml") {
		id := common.GetFileNameWithoutExt(f)
		if out, err := output.NewOutput(f, "", id); err == nil {
			project.GlobalProject.Outputs[id] = out
		}
	}

	// rulesets
	for _, f := range traverseComponents(path.Join(root, "ruleset"), ".xml") {
		id := common.GetFileNameWithoutExt(f)
		if rs, err := rules_engine.NewRuleset(f, "", id); err == nil {
			project.GlobalProject.Rulesets[id] = rs
		}
	}
}

func loadLocalProjects() {
	root := common.Config.ConfigRoot
	for _, f := range traverseComponents(path.Join(root, "project"), ".yaml") {
		id := common.GetFileNameWithoutExt(f)
		if p, err := project.NewProject(f, "", id); err == nil {
			project.GlobalProject.Projects[id] = p
		}
	}
}

// readToken reads existing .token or creates one when create==true.
func readToken(create bool) (string, error) {
	tokenPath := common.GetConfigPath(".token")
	if data, err := os.ReadFile(tokenPath); err == nil {
		return strings.TrimSpace(string(data)), nil
	}
	if !create {
		return "", fmt.Errorf("token not found")
	}
	if err := os.MkdirAll(filepath.Dir(tokenPath), 0755); err != nil {
		return "", err
	}
	uuid := common.NewUUID()
	if err := os.WriteFile(tokenPath, []byte(uuid), 0644); err != nil {
		return "", err
	}
	return uuid, nil
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

// StartAllProject starts all loaded projects (simple version).
func StartAllProject() {
	for _, p := range project.GlobalProject.Projects {
		if p.Status != project.ProjectStatusRunning {
			if err := p.Start(); err != nil {
				logger.Error("project start", "project", p.Id, "error", err)
				api.RecordProjectOperation(api.OpTypeProjectStart, p.Id, "failed", err.Error(), nil)
			} else {
				api.RecordProjectOperation(api.OpTypeProjectStart, p.Id, "success", "", nil)
			}
		}
	}
}
