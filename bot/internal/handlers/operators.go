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

const operatorMenuCallbackPrefix = "op_menu:"

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

	msg := tgbotapi.NewMessage(message.Chat.ID, "Панель оператора:")
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📋 Мои сессии", operatorMenuCallbackPrefix+"sessions"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📩 Входящие обращения", operatorMenuCallbackPrefix+"incoming"),
		),
	)

	if _, err := h.bot.Send(msg); err != nil {
		h.logger.Error("failed to send operator menu", "error", err, "chat_id", message.Chat.ID)
	}
}

func (h *OperatorHandler) HandleMenuCallback(ctx context.Context, callback *tgbotapi.CallbackQuery) {
	action := callback.Data[len(operatorMenuCallbackPrefix):]

	// Повторная проверка whitelist на каждое действие — callback мог прийти
	// спустя время, за которое статус оператора мог измениться (suspended/disabled).
	operator, err := h.backend.GetOperatorByTelegramID(ctx, callback.From.ID)
	if err != nil || !isActiveOperator(operator) {
		h.answerCallback(callback.ID, "Доступ ограничен")
		return
	}

	switch action {
	case "sessions":
		h.showOperatorSessions(ctx, callback)
	case "incoming":
		h.showIncomingRequests(ctx, callback)
	default:
		h.answerCallback(callback.ID, "")
	}
}

func (h *OperatorHandler) showOperatorSessions(ctx context.Context, callback *tgbotapi.CallbackQuery) {
	h.answerCallback(callback.ID, "")

	sessions, err := h.backend.ListOperatorSessions(ctx, callback.From.ID)
	if err != nil {
		h.logger.Error("failed to list operator sessions", "error", err, "telegram_id", callback.From.ID)
		h.sendMessage(callback.Message.Chat.ID, "Не удалось загрузить список сессий.")
		return
	}

	if len(sessions) == 0 {
		h.sendMessage(callback.Message.Chat.ID, "У вас нет активных сессий.")
		return
	}

	text := "Ваши сессии:\n\n"
	for _, s := range sessions {
		text += "• " + s.Title + " — " + s.Status + "\n"
	}

	h.sendMessage(callback.Message.Chat.ID, text)
}

func (h *OperatorHandler) showIncomingRequests(ctx context.Context, callback *tgbotapi.CallbackQuery) {
	h.answerCallback(callback.ID, "")

	requests, err := h.backend.ListIncomingRequestsInternal(ctx)
	if err != nil {
		h.logger.Error("failed to list incoming requests", "error", err, "telegram_id", callback.From.ID)
		h.sendMessage(callback.Message.Chat.ID, "Не удалось загрузить входящие обращения.")
		return
	}

	if len(requests) == 0 {
		h.sendMessage(callback.Message.Chat.ID, "Новых обращений нет.")
		return
	}

	text := "Новые обращения:\n\n"
	for _, r := range requests {
		name := "Без имени"
		if r.FirstName != "" {
			name = r.FirstName
		}
		text += "• " + name
		if r.FirstMessageText != "" {
			text += ": " + r.FirstMessageText
		}
		text += "\n"
	}

	h.sendMessage(callback.Message.Chat.ID, text)
}

func isActiveOperator(operator *backendclient.OperatorResponse) bool {
	return operator != nil && operator.Status == "active"
}

func (h *OperatorHandler) sendNeutralResponse(chatID int64) {
	h.sendMessage(chatID, "Команда недоступна.")
}

func (h *OperatorHandler) answerCallback(callbackID, text string) {
	callbackConfig := tgbotapi.NewCallback(callbackID, text)
	if _, err := h.bot.Request(callbackConfig); err != nil {
		h.logger.Error("failed to answer callback", "error", err)
	}
}

func (h *OperatorHandler) sendMessage(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	if _, err := h.bot.Send(msg); err != nil {
		h.logger.Error("failed to send message", "error", err, "chat_id", chatID)
	}
}
