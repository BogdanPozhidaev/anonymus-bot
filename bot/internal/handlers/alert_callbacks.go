package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/fastcheck/anonymus_bot/bot/internal/backendclient"
)

type AlertCallbackHandler struct {
	bot          *tgbotapi.BotAPI
	backend      *backendclient.Client
	panelBaseURL string
	logger       *slog.Logger
}

func NewAlertCallbackHandler(bot *tgbotapi.BotAPI, backend *backendclient.Client, panelBaseURL string, logger *slog.Logger) *AlertCallbackHandler {
	return &AlertCallbackHandler{bot: bot, backend: backend, panelBaseURL: panelBaseURL, logger: logger}
}

func (h *AlertCallbackHandler) Handle(ctx context.Context, callback *tgbotapi.CallbackQuery) {
	switch {
	case strings.HasPrefix(callback.Data, "open_session:"):
		h.handleOpenSession(callback)
	case strings.HasPrefix(callback.Data, "take_request:"):
		h.handleTakeRequest(ctx, callback)
	case strings.HasPrefix(callback.Data, "close_session:"):
		h.handleCloseSessionHint(callback)
	default:
		h.answerCallback(callback.ID, "")
	}
}

func (h *AlertCallbackHandler) handleOpenSession(callback *tgbotapi.CallbackQuery) {
	sessionID := strings.TrimPrefix(callback.Data, "open_session:")
	link := fmt.Sprintf("%s/sessions/%s", h.panelBaseURL, sessionID)
	h.answerCallback(callback.ID, "Открыть: "+link)
}

func (h *AlertCallbackHandler) handleTakeRequest(ctx context.Context, callback *tgbotapi.CallbackQuery) {
	requestIDStr := strings.TrimPrefix(callback.Data, "take_request:")
	requestID, err := strconv.ParseInt(requestIDStr, 10, 64)
	if err != nil {
		h.answerCallback(callback.ID, "Ошибка")
		return
	}

	if err := h.backend.MarkIncomingRequestProcessed(ctx, requestID); err != nil {
		h.logger.Error("failed to mark request processed", "error", err, "request_id", requestID)
		h.answerCallback(callback.ID, "Не удалось обработать")
		return
	}

	h.answerCallback(callback.ID, "Взято в работу")

	edit := tgbotapi.NewEditMessageText(callback.Message.Chat.ID, callback.Message.MessageID, callback.Message.Text+"\n\n✅ Взято в работу")
	if _, err := h.bot.Send(edit); err != nil {
		h.logger.Error("failed to edit alert message", "error", err)
	}
}

func (h *AlertCallbackHandler) handleCloseSessionHint(callback *tgbotapi.CallbackQuery) {
	sessionID := strings.TrimPrefix(callback.Data, "close_session:")
	link := fmt.Sprintf("%s/sessions/%s", h.panelBaseURL, sessionID)
	h.answerCallback(callback.ID, "Закройте сессию в панели: "+link)
}

func (h *AlertCallbackHandler) answerCallback(callbackID, text string) {
	callbackConfig := tgbotapi.NewCallback(callbackID, text)
	if _, err := h.bot.Request(callbackConfig); err != nil {
		h.logger.Error("failed to answer callback", "error", err)
	}
}
