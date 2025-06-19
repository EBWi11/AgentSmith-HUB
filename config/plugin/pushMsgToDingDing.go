package plugin

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"
)

type response struct {
	Code int    `json:"errcode"`
	Msg  string `json:"errmsg"`
}

// SendMessageText Function to send message
func Eval(AccessToken string, Secret string, text string) error {
	msg := map[string]interface{}{
		"msgtype": "text",
		"text": map[string]string{
			"content": text,
		},
	}

	b, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	resp, err := http.Post(getURL(AccessToken, Secret), "application/json", bytes.NewBuffer(b))
	if err != nil {
		return err
	}

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	var r response
	err = json.Unmarshal(body, &r)
	if err != nil {
		return err
	}
	if r.Code != 0 {
		return errors.New(fmt.Sprintf("response error: %s", string(body)))
	}
	return err
}

func hmacSha256(stringToSign string, secret string) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(stringToSign))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

func getURL(AccessToken string, Secret string) string {
	wh := "https://oapi.dingtalk.com/robot/send?access_token=" + AccessToken
	timestamp := time.Now().UnixNano() / 1e6
	stringToSign := fmt.Sprintf("%d\n%s", timestamp, Secret)
	sign := hmacSha256(stringToSign, Secret)
	url := fmt.Sprintf("%s&timestamp=%d&sign=%s", wh, timestamp, sign)
	return url
}
