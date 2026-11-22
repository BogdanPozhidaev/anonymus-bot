package handlers

import (
	"context"
	"errors"
	"log/slog"
	"strings"

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
	if message.From == nil || message.Chat == nil || !message.Chat.IsPrivate() {
		return
	}
	operator, err := h.backend.GetOperatorByTelegramID(ctx, message.From.ID)

	if err != nil || !isActiveOperator(operator) {
		if err != nil && !errors.Is(err, backendclient.ErrNotFound) {
			h.logger.Error("failed to check operator whitelist", "error", err)
		}
		h.sendMessage(message.Chat.ID, tr(message.From, "Команда недоступна.", "This command is unavailable."))
		return
	}

	h.logger.Info("operator accessed operator mode", "operator_id", operator.ID)

	msg := tgbotapi.NewMessage(message.Chat.ID, tr(message.From, "Панель оператора:", "Operator menu:"))
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(tr(message.From, "📋 Мои сессии", "📋 My sessions"), operatorMenuCallbackPrefix+"sessions"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(tr(message.From, "📩 Входящие обращения", "📩 Incoming requests"), operatorMenuCallbackPrefix+"incoming"),
		),
	)

	if _, err := h.bot.Send(msg); err != nil {
		h.logger.Error("failed to send operator menu")
	}
}

func (h *OperatorHandler) HandleMenuCallback(ctx context.Context, callback *tgbotapi.CallbackQuery) {
	if callback.From == nil || callback.Message == nil || callback.Message.Chat == nil || !callback.Message.Chat.IsPrivate() {
		h.answerCallback(callback.ID, tr(callback.From, "Ошибка", "Error"))
		return
	}
	action, page, paginated := strings.Cut(strings.TrimPrefix(callback.Data, operatorMenuCallbackPrefix), ":")
	offset := 0
	if paginated {
		var ok bool
		offset, ok = parsePageOffset(page)
		if !ok {
			h.answerCallback(callback.ID, tr(callback.From, "Ошибка", "Error"))
			return
		}
	}

	// Recheck operator status on every callback because it may have changed.
	operator, err := h.backend.GetOperatorByTelegramID(ctx, callback.From.ID)
	if err != nil || !isActiveOperator(operator) {
		h.answerCallback(callback.ID, tr(callback.From, "Доступ ограничен", "Access denied"))
		return
	}

	switch action {
	case "sessions":
		h.showOperatorSessions(ctx, callback, offset)
	case "incoming":
		h.showIncomingRequests(ctx, callback, offset)
	default:
		h.answerCallback(callback.ID, "")
	}
}

func (h *OperatorHandler) showOperatorSessions(ctx context.Context, callback *tgbotapi.CallbackQuery, offset int) {
	h.answerCallback(callback.ID, "")

	sessions, hasMore, err := h.backend.ListOperatorSessions(ctx, callback.From.ID, offset)
	if err != nil {
		h.logger.Error("failed to list operator sessions", "error", err)
		h.sendMessage(callback.Message.Chat.ID, tr(callback.From, "Не удалось загрузить список сессий.", "Unable to load your sessions."))
		return
	}

	if len(sessions) == 0 && offset == 0 {
		h.sendMessage(callback.Message.Chat.ID, tr(callback.From, "У вас нет активных сессий.", "You have no active sessions."))
		return
	}

	text := tr(callback.From, "Ваши сессии:\n\n", "Your sessions:\n\n")
	for _, s := range sessions {
		text += "• " + menuExcerpt(s.Title) + " — " + sessionStatusLabel(callback.From, s.Status) + "\n"
	}

	h.sendMenuPage(callback.Message.Chat.ID, text, "sessions", offset, hasMore, callback.From)
}

func (h *OperatorHandler) showIncomingRequests(ctx context.Context, callback *tgbotapi.CallbackQuery, offset int) {
	h.answerCallback(callback.ID, "")

	requests, hasMore, err := h.backend.ListIncomingRequestsInternal(ctx, offset)
	if err != nil {
		h.logger.Error("failed to list incoming requests", "error", err)
		h.sendMessage(callback.Message.Chat.ID, tr(callback.From, "Не удалось загрузить входящие обращения.", "Unable to load incoming requests."))
		return
	}

	if len(requests) == 0 && offset == 0 {
		h.sendMessage(callback.Message.Chat.ID, tr(callback.From, "Новых обращений нет.", "No new requests."))
		return
	}

	text := tr(callback.From, "Новые обращения:\n\n", "New requests:\n\n")
	for _, r := range requests {
		name := tr(callback.From, "Без имени", "Unknown name")
		if r.FirstName != "" {
			name = r.FirstName
		}
		text += "• " + menuExcerpt(name)
		if r.FirstMessageText != "" {
			text += ": " + menuExcerpt(r.FirstMessageText)
		}
		text += "\n"
	}

	h.sendMenuPage(callback.Message.Chat.ID, text, "incoming", offset, hasMore, callback.From)
}

func (h *OperatorHandler) sendMenuPage(chatID int64, text, action string, offset int, hasMore bool, user *tgbotapi.User) {
	msg := tgbotapi.NewMessage(chatID, text)
	if buttons := pageButtons(operatorMenuCallbackPrefix+action+":", offset, hasMore, user); len(buttons) > 0 {
		msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(buttons)
	}
	if _, err := h.bot.Send(msg); err != nil {
		h.logger.Error("failed to send operator list")
	}
}

func isActiveOperator(operator *backendclient.OperatorResponse) bool {
	return operator != nil && operator.Status == "active"
}

func (h *OperatorHandler) answerCallback(callbackID, text string) {
	callbackConfig := tgbotapi.NewCallback(callbackID, text)
	if _, err := h.bot.Request(callbackConfig); err != nil {
		h.logger.Error("failed to answer callback")
	}
}

func (h *OperatorHandler) sendMessage(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	if _, err := h.bot.Send(msg); err != nil {
		h.logger.Error("failed to send message")
	}
}
