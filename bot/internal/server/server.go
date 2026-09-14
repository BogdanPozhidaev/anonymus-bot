package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type SendMessageRequest struct {
	RecipientTelegramID int64  `json:"recipient_telegram_id"`
	SenderLabel         string `json:"sender_label"`
	Text                string `json:"text"`
	SessionID           int64  `json:"session_id"`
}

type AlertButton struct {
	Label        string `json:"label"`
	CallbackData string `json:"callback_data"`
}

type SendAlertRequest struct {
	Text    string        `json:"text"`
	Buttons []AlertButton `json:"buttons,omitempty"`
}

type Server struct {
	bot             *tgbotapi.BotAPI
	operatorGroupID int64
	logger          *slog.Logger
}

func New(bot *tgbotapi.BotAPI, operatorGroupID int64, logger *slog.Logger) *Server {
	return &Server{bot: bot, operatorGroupID: operatorGroupID, logger: logger}
}

func (s *Server) Start(ctx context.Context, addr string) error {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /internal/send-message", s.handleSendMessage)
	mux.HandleFunc("POST /internal/send-alert", s.handleSendAlert)

	srv := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	go func() {
		<-ctx.Done()
		_ = srv.Close()
	}()

	s.logger.Info("bot internal server starting", "addr", addr)
	return srv.ListenAndServe()
}

func (s *Server) handleSendMessage(w http.ResponseWriter, r *http.Request) {
	var req SendMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	fullText := req.SenderLabel + " " + req.Text

	msg := tgbotapi.NewMessage(req.RecipientTelegramID, fullText)

	if req.SessionID > 0 {
		callbackData := "switch_session:" + strconv.FormatInt(req.SessionID, 10)
		msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("Ответить сюда", callbackData),
			),
		)
	}

	if _, err := s.bot.Send(msg); err != nil {
		s.logger.Error("failed to send operator message", "error", err, "recipient_telegram_id", req.RecipientTelegramID)
		http.Error(w, "failed to send message", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleSendAlert(w http.ResponseWriter, r *http.Request) {
	var req SendAlertRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	msg := tgbotapi.NewMessage(s.operatorGroupID, req.Text)

	if len(req.Buttons) > 0 {
		var row []tgbotapi.InlineKeyboardButton
		for _, b := range req.Buttons {
			row = append(row, tgbotapi.NewInlineKeyboardButtonData(b.Label, b.CallbackData))
		}
		msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(row)
	}

	if _, err := s.bot.Send(msg); err != nil {
		s.logger.Error("failed to send alert to operator group", "error", err)
		http.Error(w, "failed to send alert", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}
