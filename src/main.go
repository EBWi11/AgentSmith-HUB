package main

import (
	"AgentSmith-HUB/common"
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
	fmt.Printf("Config: %s\n", *configRoot)

	err := project.SetConfigRoot(*configRoot)
	if err != nil {
		fmt.Println(err)
		return
	}

	err = loadHubConfig(*configRoot)
	if err != nil {
		fmt.Println(err)
		return
	}

	// init
	err = common.RedisInit(HubConfig.Redis, HubConfig.RedisPassword)
	if err != nil {
		fmt.Println(err)
		return
	}
	err = common.PluginInit(path.Join(project.ConfigRoot, "plugin"))
	if err != nil {
		fmt.Println(err)
		return
	}

	// Load and start projects
	projectList, err := traverseProject(path.Join(*configRoot, "project"))
	if err != nil {
		fmt.Println(err)
		return
	}

	for _, p := range projectList {
		fmt.Printf("Loading Project: %s\n", p)
		p, err := project.NewProject("test.yaml")
		if err != nil {
			fmt.Println(err)
		}

		err = p.Start()
		if err != nil {
			fmt.Println(err)
		}

		fmt.Printf("Project %s started successfully\n", p.Name)
	}

	select {}
}
