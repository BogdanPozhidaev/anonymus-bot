package handlers

import (
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type AlertCallbackHandler struct {
	bot          *tgbotapi.BotAPI
	panelBaseURL string
	logger       *slog.Logger
}

func NewAlertCallbackHandler(bot *tgbotapi.BotAPI, panelBaseURL string, logger *slog.Logger) *AlertCallbackHandler {
	return &AlertCallbackHandler{bot: bot, panelBaseURL: strings.TrimRight(panelBaseURL, "/"), logger: logger}
}

func (h *AlertCallbackHandler) Handle(callback *tgbotapi.CallbackQuery) {
	switch {
	case strings.HasPrefix(callback.Data, "open_session:"):
		h.handleOpenSession(callback)
	case strings.HasPrefix(callback.Data, "take_request:"), strings.HasPrefix(callback.Data, "open_request:"):
		h.handleOpenRequest(callback)
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

func (h *AlertCallbackHandler) handleOpenRequest(callback *tgbotapi.CallbackQuery) {
	_, requestIDStr, _ := strings.Cut(callback.Data, ":")
	requestID, err := strconv.ParseInt(requestIDStr, 10, 64)
	if err != nil || requestID <= 0 {
		h.answerCallback(callback.ID, tr(callback.From, "Обращение недоступно. Откройте список в панели.", "Request unavailable. Open the queue in the panel."))
		return
	}

	h.answerCallback(callback.ID, fmt.Sprintf(tr(callback.From,
		"Взять в работу можно в панели: %s/incoming-requests/%d",
		"Claim the request in the panel: %s/incoming-requests/%d"), h.panelBaseURL, requestID))
}

func (h *AlertCallbackHandler) handleCloseSessionHint(callback *tgbotapi.CallbackQuery) {
	sessionID := strings.TrimPrefix(callback.Data, "close_session:")
	link := fmt.Sprintf("%s/sessions/%s", h.panelBaseURL, sessionID)
	h.answerCallback(callback.ID, "Закройте сессию в панели: "+link)
}

func (h *AlertCallbackHandler) answerCallback(callbackID, text string) {
	callbackConfig := tgbotapi.NewCallback(callbackID, text)
	if _, err := h.bot.Request(callbackConfig); err != nil {
		h.logger.Error("failed to answer callback")
	}
}
