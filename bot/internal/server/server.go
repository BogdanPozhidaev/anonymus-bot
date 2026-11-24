package server

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"log/slog"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/google/uuid"
)

type SendMessageRequest struct {
	ContentType         string `json:"content_type"`
	FileID              string `json:"file_id"`
	RecipientTelegramID int64  `json:"recipient_telegram_id"`
	SenderLabel         string `json:"sender_label"`
	Text                string `json:"text"`
	SessionID           int64  `json:"session_id"`
	Language            string `json:"language"`
}

type AlertButton struct {
	Label        string `json:"label"`
	CallbackData string `json:"callback_data"`
}

type SendAlertRequest struct {
	Text    string        `json:"text"`
	Buttons []AlertButton `json:"buttons,omitempty"`
}

type messageSender interface {
	Send(tgbotapi.Chattable) (tgbotapi.Message, error)
}

type Server struct {
	bot                 messageSender
	operatorGroupID     int64
	logger              *slog.Logger
	internalToken       string
	photosDir, voiceDir string
	panelBaseURL        string
}

func New(bot messageSender, operatorGroupID int64, logger *slog.Logger, internalToken, photosDir, voiceDir, panelBaseURL string) *Server {
	return &Server{bot: bot, operatorGroupID: operatorGroupID, logger: logger, internalToken: internalToken, photosDir: photosDir, voiceDir: voiceDir, panelBaseURL: panelBaseURL}
}

func (s *Server) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /internal/send-message", s.handleSendMessage)
	mux.HandleFunc("POST /internal/send-alert", s.handleSendAlert)
	expected := []byte(s.internalToken)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		values := r.Header.Values("X-Internal-Token")
		r.Header.Del("X-Internal-Token")
		if len(expected) != 64 || len(values) != 1 || subtle.ConstantTimeCompare([]byte(values[0]), expected) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		mux.ServeHTTP(w, r)
	})
}

func (s *Server) Start(ctx context.Context, addr string) error {
	srv := &http.Server{
		Addr:              addr,
		Handler:           s.handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	listen := make(chan error, 1)
	go func() {
		listen <- srv.ListenAndServe()
	}()

	s.logger.Info("bot internal server starting", "addr", addr)
	select {
	case err := <-listen:
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			_ = srv.Close()
			return err
		}
		return <-listen
	}
}

func (s *Server) handleSendMessage(w http.ResponseWriter, r *http.Request) {
	var req SendMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	var keyboard any
	if req.SessionID > 0 {
		replyLabel := "Ответить сюда"
		if req.Language == "en" {
			replyLabel = "Reply here"
		}
		keyboard = tgbotapi.NewInlineKeyboardMarkup(tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(replyLabel, "switch_session:"+strconv.FormatInt(req.SessionID, 10)),
		))
	}
	var msg tgbotapi.Chattable
	switch req.ContentType {
	case "", "text":
		body := req.Text
		if req.SenderLabel != "" {
			body = req.SenderLabel + " " + body
		}
		text := tgbotapi.NewMessage(req.RecipientTelegramID, body)
		text.ReplyMarkup = keyboard
		msg = text
	case "photo", "voice":
		id, err := uuid.Parse(req.FileID)
		if err != nil || id == uuid.Nil || id.String() != req.FileID {
			http.Error(w, "invalid media file id", http.StatusBadRequest)
			return
		}
		if req.ContentType == "photo" {
			photo := tgbotapi.NewPhoto(req.RecipientTelegramID, tgbotapi.FilePath(filepath.Join(s.photosDir, req.FileID+".jpg")))
			photo.Caption = req.SenderLabel
			if req.Text != "" {
				photo.Caption = req.Text
			}
			photo.ReplyMarkup = keyboard
			msg = photo
		} else {
			voice := tgbotapi.NewVoice(req.RecipientTelegramID, tgbotapi.FilePath(filepath.Join(s.voiceDir, req.FileID+".ogg")))
			voice.Caption = req.SenderLabel
			if req.Text != "" {
				voice.Caption = req.Text
			}
			voice.ReplyMarkup = keyboard
			msg = voice
		}
	default:
		http.Error(w, "invalid content type", http.StatusBadRequest)
		return
	}

	if _, err := s.bot.Send(msg); err != nil {
		s.logger.Error("Telegram delivery failed")
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
			action, rawID, _ := strings.Cut(b.CallbackData, ":")
			id, err := strconv.ParseInt(rawID, 10, 64)
			var path string
			switch action {
			case "open_session", "close_session":
				path = "/sessions/"
			case "open_request":
				path = "/incoming-requests/"
			}
			if strings.HasPrefix(s.panelBaseURL, "https://") && err == nil && id > 0 && path != "" {
				row = append(row, tgbotapi.NewInlineKeyboardButtonURL(b.Label, s.panelBaseURL+path+strconv.FormatInt(id, 10)))
			} else {
				row = append(row, tgbotapi.NewInlineKeyboardButtonData(b.Label, b.CallbackData))
			}
		}
		msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(row)
	}

	if _, err := s.bot.Send(msg); err != nil {
		s.logger.Error("failed to send alert to operator group")
		http.Error(w, "failed to send alert", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}
