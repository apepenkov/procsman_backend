package api

import (
	"context"
	"net/http"
	"procsman_backend/config"
)

func (srv *HttpServer) GetNotificationSettings(w http.ResponseWriter, r *http.Request) {
	rw := r.Context().Value(ContextKeyWrappedRequest).(*ReqWrapper)

	rw.MarshalAndRespondWithStatus(srv.ProcessManager.Notifications, http.StatusOK)
}

type PatchNotificationsConfig struct {
	Enabled               bool    `json:"enabled"`
	TelegramBotToken      string  `json:"telegram_bot_token"`
	TelegramTargetChatIDS []int64 `json:"telegram_target_chat_ids"`
	// maybe save names of chats?
}

func (nc *PatchNotificationsConfig) Validate(ctx context.Context, srv *HttpServer) *Error {
	return nil
}

func (srv *HttpServer) UpdateNotificationSettings(w http.ResponseWriter, r *http.Request) {
	rw := r.Context().Value(ContextKeyWrappedRequest).(*ReqWrapper)
	req := r.Context().Value(ContextKeyUnmarshalledJson).(*PatchNotificationsConfig)

	srv.ProcessManager.Notifications.Enabled = req.Enabled
	srv.ProcessManager.Notifications.TelegramBotToken = req.TelegramBotToken
	srv.ProcessManager.Notifications.TelegramTargetChatIDS = req.TelegramTargetChatIDS

	if err := srv.ProcessManager.Notifications.Save(); err != nil {
		rw.E(MessageCodeInternalError, "Internal error", http.StatusInternalServerError, err.Error())
		return
	}
	rw.MarshalAndRespondWithStatus(srv.ProcessManager.Notifications, http.StatusOK)
}

type TestMessage struct {
	SendEverywhere bool   `json:"send_everywhere"`
	SendToChatId   int64  `json:"send_to_chat_id"`
	Text           string `json:"text"`
}

func (srv *HttpServer) TestNotification(w http.ResponseWriter, r *http.Request) {
	rw := r.Context().Value(ContextKeyWrappedRequest).(*ReqWrapper)
	req := r.Context().Value(ContextKeyUnmarshalledJson).(*TestMessage)

	var res []config.SendResult
	if req.SendEverywhere {
		res = srv.ProcessManager.Notifications.SendMessage(req.Text)
	} else {
		res = []config.SendResult{srv.ProcessManager.Notifications.SendTelegramMessage(req.Text, req.SendToChatId)}
	}
	rw.MarshalAndRespondWithStatus(res, http.StatusOK)
}
