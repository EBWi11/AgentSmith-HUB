package main

import (
	"AgentSmith-HUB/api"
	"AgentSmith-HUB/cluster"
	"AgentSmith-HUB/common"
	"AgentSmith-HUB/logger"
	"AgentSmith-HUB/project"
	"archive/zip"
	"bytes"
	"flag"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

var HubConfig *common.HubConfig

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

func readToken(create bool) (string, error) {
	tokenPath := ".token"
	data, err := os.ReadFile(tokenPath)
	if err == nil {
		return string(data), nil
	}

	//leader create new token
	if create {
		f, err := os.Create(tokenPath)
		if err != nil {
			return "", err
		}

		defer f.Close()

		uuid := common.NewUUID() // 假设 common.NewUUID() 返回 uuid 字符串
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

	if err := yaml.Unmarshal(data, &HubConfig); err != nil {
		return fmt.Errorf("failed to parse hub config: %w", err)
	}

	if HubConfig.Redis == "" {
		return fmt.Errorf("redis is null")
	}

	if HubConfig.Leader == "" {
		return fmt.Errorf("leader is null")
	}

	HubConfig.ConfigRoot = configRoot

	if HubConfig.Listen == "" {
		HubConfig.Listen = "0.0.0.0:8080"
	}

	return nil
}

func LoadLocalProject(configRoot string) {
	projectList, err := traverseProject(path.Join(configRoot, "project"))
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
}

func LoadLeaderProject() error {
	tmpDir := "/tmp"
	configZipPath := filepath.Join(tmpDir, "config.zip")
	configDir := filepath.Join(tmpDir, "config")

	// Step 1: Download config from leader
	resp, err := http.Get(fmt.Sprintf("http://%s/config/download", HubConfig.Leader))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return err
	}

	out, err := os.Create(configZipPath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return err
	}

	// Step 2: Verify config using leader's verify API
	file, err := os.Open(configZipPath)
	if err != nil {
		return err
	}
	defer file.Close()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("config", filepath.Base(configZipPath))
	if err != nil {
		return err
	}

	_, err = io.Copy(part, file)
	if err != nil {
		return err
	}

	err = writer.Close()
	if err != nil {
		return err
	}

	verifyResp, err := http.Post(fmt.Sprintf("http://%s/config/verify", HubConfig.Leader), writer.FormDataContentType(), body)
	if err != nil {
		return err
	}
	defer verifyResp.Body.Close()

	if verifyResp.StatusCode != http.StatusOK {
		return err
	}

	// Step 3: Unzip config to tmp folder
	r, err := zip.OpenReader(configZipPath)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		fPath := filepath.Join(tmpDir, f.Name)
		if f.FileInfo().IsDir() {
			err = os.MkdirAll(fPath, os.ModePerm)
			if err != nil {
				return err
			}
			continue
		}

		err = os.MkdirAll(filepath.Dir(fPath), os.ModePerm)
		if err != nil {
			return err
		}

		outFile, err := os.OpenFile(fPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return err
		}

		rc, err := f.Open()
		if err != nil {
			return err
		}

		_, err = io.Copy(outFile, rc)
		outFile.Close()
		rc.Close()

		if err != nil {
			return err
		}
	}

	// Step 4: Read config.yaml to get configRoot path
	configYamlPath := filepath.Join(configDir, "config.yaml")
	data, err := os.ReadFile(configYamlPath)
	if err != nil {
		return err
	}

	var config struct {
		ConfigRoot string `yaml:"config_root"`
	}
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		return err
	}

	if config.ConfigRoot == "" {
		return err
	}

	// Step 5: Move config folder to configRoot path
	err = os.MkdirAll(config.ConfigRoot, os.ModePerm)
	if err != nil {
		return err
	}

	err = filepath.Walk(configDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(configDir, path)
		if err != nil {
			return err
		}

		destPath := filepath.Join(config.ConfigRoot, relPath)
		if info.IsDir() {
			return os.MkdirAll(destPath, info.Mode())
		}

		srcFile, err := os.Open(path)
		if err != nil {
			return err
		}
		defer srcFile.Close()

		destFile, err := os.Create(destPath)
		if err != nil {
			return err
		}
		defer destFile.Close()

		_, err = io.Copy(destFile, srcFile)
		return err
	})
	if err != nil {
		return err
	}

	// Cleanup tmp folder
	err = os.RemoveAll(tmpDir)
	if err != nil {
		return err
	}

	return nil
}

func main() {
	var err error
	configRoot := flag.String("config_root", "", "agent smith hub config path, only leader need")
	leaderAddr := flag.String("leader", "", "hub cluster leader address")

	flag.Parse()

	logger.Info("hub_starting", "config_root", *configRoot, "leader", leaderAddr)

	if (*configRoot != "" && *leaderAddr != "") || (*configRoot == "" && *leaderAddr == "") {
		fmt.Println("If the instance is a Leader, only 'config_root' needs to be given; if it is a Follower, only 'leader' needs to be given")
		return
	}

	// load self ip & init cluster
	HubConfig.LocalIP, err = common.GetLocalIP()
	if err != nil {
		logger.Error("get local ip error", "error", err)
	}
	cl := cluster.ClusterInit(HubConfig.LocalIP, HubConfig.LocalIP)

	if *configRoot != "" {
		//set self is leader
		HubConfig.Leader = HubConfig.LocalIP
		cl.SetLeader(HubConfig.LocalIP, HubConfig.LocalIP)

		//leader read local config
		err := loadHubConfig(*configRoot)
		if err != nil {
			logger.Error("load hub config error", "error", err)
			return
		}

		//leader creates or read token
		HubConfig.Token, err = readToken(true)
		if err != nil {
			logger.Error("read or create token error", "error", err)
			return
		}
	} else if *leaderAddr != "" {
		HubConfig.Leader = *leaderAddr
		//set leader
		cl.SetLeader(HubConfig.Leader, HubConfig.Leader)

		//read token
		HubConfig.Token, err = readToken(false)
		if err != nil {
			logger.Error("read token error", "error", err)
			return
		}

		//download leader config
		err = LoadLeaderProject()
		if err != nil {
			logger.Error("load leader config error", "error", err)
			return
		}
	}

	//err = loadHubConfig(*configRoot)
	//if err != nil {
	//	logger.Error("load hub config error", "error", err)
	//	return
	//}
	//
	////todo create node id
	//cl.SetLeader(HubConfig.Leader, HubConfig.Leader)
	//cl.StartHeartbeatLoop()
	//
	//// Load and start projects
	//if cl.IsLeader() {
	//	LoadLocalProject(HubConfig.ConfigRoot)
	//} else {
	//	LoadLeaderProject()
	//}
	//
	//// init
	//err = common.RedisInit(HubConfig.Redis, HubConfig.RedisPassword)
	//if err != nil {
	//	logger.Error("redis init error", "error", err)
	//	return
	//}
	//err = plugin.PluginInit(path.Join(project.ConfigRoot, "plugin"))
	//if err != nil {
	//	logger.Error("plugin init error", "error", err)
	//	return
	//}

	// Start Api
	err = api.ServerStart(HubConfig.Listen, HubConfig)
	if err != nil {
		logger.Error("server start err", "error", err.Error())
	}

	select {}
}
