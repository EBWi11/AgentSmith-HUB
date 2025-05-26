package main

import (
	"AgentSmith-HUB/project"
	"flag"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
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

func main() {
	configRoot := flag.String("config", "./config", "agent smith hub config path, default is ./config")
	flag.Parse()
	fmt.Printf("Config: %s\n", *configRoot)

	err := project.SetConfigRoot(*configRoot)
	if err != nil {
		fmt.Println(err)
	}

	projectList, err := traverseProject(path.Join(*configRoot, "project"))
	if err != nil {
		fmt.Println(err)
	}

	for _, p := range projectList {
		fmt.Printf("Loading Project: %s\n", p)
		p, err := project.NewProject("test.yaml")
		if err != nil {
			fmt.Println(err)
		}

		err = p.Start()
		if err != nil {
			panic(err)
		}

		fmt.Printf("Project %s started successfully\n", p.Name)
	}

	select {}
}
