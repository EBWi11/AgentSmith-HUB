package api

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
)

var hubRequest *HubRequest

type HubRequest struct {
	Leader string
	Token  string
}

func InitRequest(leader string, Token string) error {
	var err error
	var req *http.Request

	hubRequest = &HubRequest{
		Leader: leader,
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

func GetConfigRoot() (string, error) {
	var configRoot string
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

	var result struct {
		ConfigRoot string `json:"config_root"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return configRoot, err
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
	err = unzip(configZipPath, unzipDir)
	if err != nil {
		return errors.New("unzip config.zip failed: " + err.Error())
	}

	err = copyDir(unzipDir, confRoot)
	if err != nil {
		return errors.New("copy config to confRoot failed: " + err.Error())
	}

	return nil
}

func unzip(src, dest string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		fpath := filepath.Join(dest, f.Name)
		if f.FileInfo().IsDir() {
			os.MkdirAll(fpath, os.ModePerm)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(fpath), os.ModePerm); err != nil {
			return err
		}
		outFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			outFile.Close()
			return err
		}
		_, err = io.Copy(outFile, rc)
		outFile.Close()
		rc.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func copyDir(src string, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		targetPath := filepath.Join(dst, relPath)
		if info.IsDir() {
			return os.MkdirAll(targetPath, info.Mode())
		}
		srcFile, err := os.Open(path)
		if err != nil {
			return err
		}
		defer srcFile.Close()
		dstFile, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
		if err != nil {
			return err
		}
		defer dstFile.Close()
		_, err = io.Copy(dstFile, srcFile)
		return err
	})
}
