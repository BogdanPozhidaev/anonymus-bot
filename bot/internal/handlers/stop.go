package handlers

import (
	"context"
	"log/slog"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/fastcheck/anonymus_bot/bot/internal/backendclient"
)

type StopHandler struct {
	bot     *tgbotapi.BotAPI
	backend *backendclient.Client
	logger  *slog.Logger
}

func NewStopHandler(bot *tgbotapi.BotAPI, backend *backendclient.Client, logger *slog.Logger) *StopHandler {
	return &StopHandler{bot: bot, backend: backend, logger: logger}
}

func (h *StopHandler) Handle(ctx context.Context, message *tgbotapi.Message) {
	resp, err := h.backend.StopSession(ctx, message.From.ID)
	if err != nil {
		h.logger.Error("failed to stop session", "error", err, "chat_id", message.Chat.ID)
		h.sendMessage(message.Chat.ID, "У вас нет активной сессии для завершения.")
		return
	}

	h.sendMessage(message.Chat.ID, "Ваш запрос передан менеджеру. Сессия переведена в режим ожидания решения.")

	if resp.CounterpartTelegramID != 0 {
		h.sendMessage(resp.CounterpartTelegramID, "Вторая сторона запросила завершение диалога. Ожидайте решения менеджера.")
	}
}

func (h *StopHandler) sendMessage(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	if _, err := h.bot.Send(msg); err != nil {
		h.logger.Error("failed to send message", "error", err, "chat_id", chatID)
	}
}
