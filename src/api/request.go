package api

import (
	"AgentSmith-HUB/common"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

var hubRequest *HubRequest

type HubRequest struct {
	Leader string
	Token  string
}

func InitRequest(leader string, Token string) error {
	var err error
	var req *http.Request

	// Ensure proper URL format for leader address
	var normalizedLeader string
	if strings.HasPrefix(leader, "http://") || strings.HasPrefix(leader, "https://") {
		normalizedLeader = leader
	} else {
		normalizedLeader = fmt.Sprintf("http://%s", leader)
	}

	hubRequest = &HubRequest{
		Leader: normalizedLeader,
		Token:  Token,
	}

	pingRes, err := http.Get(fmt.Sprintf("%s/ping", hubRequest.Leader))
	if err != nil {
		return err
	}
	defer pingRes.Body.Close()

	if pingRes.StatusCode != http.StatusOK {
		return errors.New("ping leader get error status code: " + strconv.Itoa(pingRes.StatusCode))
	}

	req, err = http.NewRequest("GET", fmt.Sprintf("%s/ping", hubRequest.Leader), nil)
	if err != nil {
		return err
	}
	req.Header.Set("token", hubRequest.Token)

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return errors.New("Leader authentication failed : " + err.Error())
	}

	if res.StatusCode != http.StatusOK {
		return errors.New("Leader authentication failed, got error http status code: " + strconv.Itoa(pingRes.StatusCode))
	}

	return nil
}

func GetLeaderConfig() (map[string]string, error) {
	configRoot := make(map[string]string, 3)
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/config_root", hubRequest.Leader), nil)
	if err != nil {
		return configRoot, err
	}
	req.Header.Set("token", hubRequest.Token)

	client := &http.Client{}
	resp, err := client.Do(req)

	if err != nil {
		return configRoot, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return configRoot, fmt.Errorf("get config_root failed, status code: %d", resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(&configRoot); err != nil {
		return nil, err
	}

	return configRoot, err
}

func DownloadConfig(confRoot string) error {
	tmpDir := "/tmp"
	configZipPath := filepath.Join(tmpDir, "config.zip")

	req, err := http.NewRequest("GET", fmt.Sprintf("%s/config/download", hubRequest.Leader), nil)
	if err != nil {
		return err
	}
	req.Header.Set("token", hubRequest.Token)

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return errors.New("DownloadConfig error: " + err.Error())
	}

	if res.StatusCode != http.StatusOK {
		return errors.New("Download Config failed, got error http status code: " + strconv.Itoa(res.StatusCode))
	}
	defer res.Body.Close()

	out, err := os.Create(configZipPath)
	if err != nil {
		return err
	}
	defer out.Close()

	hasher := sha256.New()
	multiWriter := io.MultiWriter(out, hasher)

	_, err = io.Copy(multiWriter, res.Body)
	if err != nil {
		return errors.New("Write Config failed, error: " + strconv.Itoa(res.StatusCode))
	}

	sha256local := fmt.Sprintf("%x", hasher.Sum(nil))
	sha256server := res.Header.Get("X-Config-Sha256")
	if sha256local != sha256server {
		return errors.New("Config sha256 local and server are inconsistent, local: " + sha256local + " server: " + sha256server)
	}

	unzipDir := filepath.Join(tmpDir, "config_unzip")
	_ = os.RemoveAll(unzipDir)
	err = common.Unzip(configZipPath, unzipDir)
	if err != nil {
		return errors.New("unzip config.zip failed: " + err.Error())
	}

	err = common.CopyDir(unzipDir, confRoot)
	if err != nil {
		return errors.New("copy config to confRoot failed: " + err.Error())
	}

	return nil
}

func GetComponentDetail(componentType string, id string) (map[string]interface{}, error) {
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/%s/%s", hubRequest.Leader, componentType, id), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("token", hubRequest.Token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get project failed, status code: %d", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result, nil
}

func GetAllComponents(componentType string) ([]map[string]interface{}, error) {
	realRes := make([]map[string]interface{}, 10)
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/%s", hubRequest.Leader, componentType), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("token", hubRequest.Token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("get all components failed, status code: " + fmt.Sprint(resp.StatusCode))
	}

	var result []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	for _, v := range result {
		var id string
		if componentType == "plugin" {
			id = v["name"].(string)
		} else {
			id = v["id"].(string)
		}

		tmp, err := GetComponentDetail(componentType, id)
		if err != nil {
			return nil, errors.New("get all project failed, err" + err.Error())
		}
		realRes = append(realRes, tmp)
	}

	return realRes, nil
}
