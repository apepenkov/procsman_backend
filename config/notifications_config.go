package config

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"time"
)

var httpClient = &http.Client{
	Timeout: 10 * time.Second,
}

func (nc *NotificationsConfig) Save() error {
	f, err := os.OpenFile(NotificationConfigFileName, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewEncoder(f).Encode(nc)
}

type SendResult struct {
	Success   bool
	ChatId    int64
	MessageId int
	Error     string
}

func (nc *NotificationsConfig) SendTelegramMessage(text string, chatId int64) SendResult {
	requestMap := map[string]interface{}{
		"chat_id": chatId,
		"text":    text,
	}

	var reader io.Reader
	if b, err := json.Marshal(requestMap); err != nil {
		return SendResult{
			ChatId: chatId,
			Error:  err.Error(),
		}
	} else {
		reader = bytes.NewReader(b)
	}

	req, err := http.NewRequest("POST", "https://api.telegram.org/bot"+nc.TelegramBotToken+"/sendMessage", reader)
	if err != nil {
		return SendResult{
			ChatId: chatId,
			Error:  err.Error(),
		}
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return SendResult{
			ChatId: chatId,
			Error:  err.Error(),
		}
	}

	defer resp.Body.Close()
	var responseMap map[string]interface{}
	if err = json.NewDecoder(resp.Body).Decode(&responseMap); err != nil {
		return SendResult{
			ChatId: chatId,
			Error:  err.Error(),
		}
	}

	if b, ok := responseMap["ok"].(bool); !ok || !b || resp.StatusCode >= 400 {
		var description string
		if desc, ok2 := responseMap["description"].(string); ok2 {
			description = desc
		} else {
			description = "unknown error"
		}
		return SendResult{
			ChatId: chatId,
			Error:  description,
		}
	}

	messageId, ok := responseMap["result"].(map[string]interface{})["message_id"].(float64)

	if !ok {
		return SendResult{
			ChatId: chatId,
			Error:  "could not get message_id",
		}
	}

	return SendResult{
		ChatId:    chatId,
		MessageId: int(messageId),
		Success:   true,
	}
}

func (nc *NotificationsConfig) SendMessage(text string) []SendResult {
	if !nc.Enabled {
		return nil
	}
	results := make([]SendResult, 0, len(nc.TelegramTargetChatIDS))
	for _, chatId := range nc.TelegramTargetChatIDS {
		results = append(results, nc.SendTelegramMessage(text, chatId))
	}
	return results
}

const NotificationConfigFileName = "notifications.json"

func LoadOrCreateNotificationsConfig() (*NotificationsConfig, error) {
	f, err := os.Open(NotificationConfigFileName)
	if err != nil {
		if os.IsNotExist(err) {
			cfg := &NotificationsConfig{
				Enabled:               false,
				TelegramBotToken:      "",
				TelegramTargetChatIDS: make([]int64, 0),
			}
			if err = cfg.Save(); err != nil {
				return nil, err
			}
			return cfg, nil
		}
		return nil, err
	}

	defer f.Close()
	cfg := &NotificationsConfig{}
	if err = json.NewDecoder(f).Decode(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

type NotificationsConfig struct {
	Enabled               bool    `json:"enabled"`
	TelegramBotToken      string  `json:"telegram_bot_token"`
	TelegramTargetChatIDS []int64 `json:"telegram_target_chat_ids"`
	// maybe save names of chats?
}
