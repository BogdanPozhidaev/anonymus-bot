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
		h.logger.Error("failed to stop session", "error", err)
		h.sendMessage(message.Chat.ID, tr(message.From, "У вас нет активной сессии для завершения.", "You have no active session to end."))
		return
	}

	h.sendMessage(message.Chat.ID, tr(message.From, "Ваш запрос передан менеджеру. Сессия переведена в режим ожидания решения.", "Your request was sent to the manager. The session is awaiting a decision."))

	if resp.CounterpartTelegramID != 0 {
		language := ResolveLanguage(ctx, h.backend, resp.CounterpartTelegramID, "ru")
		h.sendMessage(resp.CounterpartTelegramID, tr(&tgbotapi.User{LanguageCode: language}, "Вторая сторона запросила завершение диалога. Ожидайте решения менеджера.", "The other person asked to end the conversation. Please wait for the manager's decision."))
	}
}

func (h *StopHandler) sendMessage(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	if _, err := h.bot.Send(msg); err != nil {
		h.logger.Error("failed to send message")
	}
}
