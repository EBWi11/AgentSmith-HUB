package api

import (
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

func GetConfigRoot() {

}

func DownloadConfig() error {
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
		return errors.New("DownloadConfig failed, got error http status code: " + strconv.Itoa(res.StatusCode))
	}
	defer res.Body.Close()

	out, err := os.Create(configZipPath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, res.Body)
	if err != nil {
		return err
	}

	return nil
}

func VerifyConfigZip() {

}
