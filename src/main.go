package main

import (
	"AgentSmith-HUB/common"
	"AgentSmith-HUB/logger"
	"AgentSmith-HUB/plugin"
	"AgentSmith-HUB/project"
	"flag"
	"fmt"
	"gopkg.in/yaml.v3"
	"os"
	"path"
	"path/filepath"
	"strings"
)

var HubConfig *hubConfig

type hubConfig struct {
	Redis         string `yaml:"redis"`
	RedisPassword string `yaml:"redis_password,omitempty"`
}

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

func loadHubConfig(configRoot string) error {
	configPath := path.Join(configRoot, "config.yaml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to read hub config: %w", err)
	}

	if err := yaml.Unmarshal(data, &HubConfig); err != nil {
		return fmt.Errorf("failed to parse hub config: %w", err)
	}

	return nil
}

func main() {
	configRoot := flag.String("config", "./config", "agent smith hub config path, default is ./config")
	flag.Parse()

	logger.Info("hub_starting", "config_root", *configRoot)

	err := project.SetConfigRoot(*configRoot)
	if err != nil {
		logger.Error("set config root error", "error", err)
		return
	}

	err = loadHubConfig(*configRoot)
	if err != nil {
		logger.Error("load hub config error", "error", err)
		return
	}

	// init
	err = common.RedisInit(HubConfig.Redis, HubConfig.RedisPassword)
	if err != nil {
		logger.Error("redis init error", "error", err)
		return
	}
	err = plugin.PluginInit(path.Join(project.ConfigRoot, "plugin"))
	if err != nil {
		logger.Error("plugin init error", "error", err)
		return
	}

	// Load and start projects
	projectList, err := traverseProject(path.Join(*configRoot, "project"))
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

	select {}
}
