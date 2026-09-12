package handlers

import (
	"context"
	"errors"
	"log/slog"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/fastcheck/anonymus_bot/bot/internal/backendclient"
)

type OperatorHandler struct {
	bot     *tgbotapi.BotAPI
	backend *backendclient.Client
	logger  *slog.Logger
}

func NewOperatorHandler(bot *tgbotapi.BotAPI, backend *backendclient.Client, logger *slog.Logger) *OperatorHandler {
	return &OperatorHandler{bot: bot, backend: backend, logger: logger}
}

func (h *OperatorHandler) Handle(ctx context.Context, message *tgbotapi.Message) {
	operator, err := h.backend.GetOperatorByTelegramID(ctx, message.From.ID)

	if err != nil || !isActiveOperator(operator) {
		if err != nil && !errors.Is(err, backendclient.ErrNotFound) {
			h.logger.Error("failed to check operator whitelist", "error", err, "telegram_id", message.From.ID)
		}
		h.sendNeutralResponse(message.Chat.ID)
		return
	}

	h.logger.Info("operator accessed operator mode", "operator_id", operator.ID, "telegram_id", message.From.ID)

	text := "Панель оператора:\n/sessions_list — список ваших сессий\n/incoming — входящие обращения\n\n(функционал в разработке)"
	h.sendMessage(message.Chat.ID, text)
}

func isActiveOperator(operator *backendclient.OperatorResponse) bool {
	return operator != nil && operator.Status == "active"
}

func (h *OperatorHandler) sendNeutralResponse(chatID int64) {
	text := "Команда недоступна."
	h.sendMessage(chatID, text)
}

func (h *OperatorHandler) sendMessage(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	if _, err := h.bot.Send(msg); err != nil {
		h.logger.Error("failed to send message", "error", err, "chat_id", chatID)
	}
}
