package plugin

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
)

func formatMapToReadableString(data map[string]interface{}) string {
	b, _ := json.MarshalIndent(data, "", "  ")
	return string(b)
}

// Eval sends a message via Telegram bot.
// Param1: BotToken
// Param2: ChatID (string)
func Eval(BotToken string, ChatID string, data map[string]interface{}) (bool, error) {
	if BotToken == "" || ChatID == "" {
		return false, errors.New("bot token and chat id required")
	}
	text := url.QueryEscape(formatMapToReadableString(data))
	apiUrl := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage?chat_id=%s&text=%s", BotToken, ChatID, text)

	resp, err := http.Get(apiUrl)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	body, _ := ioutil.ReadAll(resp.Body)
	if resp.StatusCode/100 != 2 {
		return false, errors.New(string(body))
	}
	return true, nil
}
