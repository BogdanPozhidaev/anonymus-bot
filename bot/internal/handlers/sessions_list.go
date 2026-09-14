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

type SessionsListHandler struct {
	bot     *tgbotapi.BotAPI
	backend *backendclient.Client
	logger  *slog.Logger
}

func NewSessionsListHandler(bot *tgbotapi.BotAPI, backend *backendclient.Client, logger *slog.Logger) *SessionsListHandler {
	return &SessionsListHandler{bot: bot, backend: backend, logger: logger}
}

const switchSessionCallbackPrefix = "switch_session:"

var statusLabelsRU = map[string]string{
	"active":        "активна",
	"paused":        "на паузе",
	"pending_close": "ожидает закрытия",
}

var roleLabelsRU = map[string]string{
	"client":   "клиент",
	"executor": "исполнитель",
}

func (h *SessionsListHandler) HandleSessionsCommand(ctx context.Context, message *tgbotapi.Message) {
	sessions, err := h.backend.ListUserSessions(ctx, message.From.ID)
	if err != nil {
		h.logger.Error("failed to list user sessions", "error", err, "chat_id", message.Chat.ID)
		h.sendMessage(message.Chat.ID, "Не удалось загрузить список сессий. Попробуйте позже.")
		return
	}

	if len(sessions) == 0 {
		h.sendMessage(message.Chat.ID, "У вас пока нет активных сессий.")
		return
	}

	msg := tgbotapi.NewMessage(message.Chat.ID, "Ваши сессии:")
	msg.ReplyMarkup = buildSessionsKeyboard(sessions)

	if _, err := h.bot.Send(msg); err != nil {
		h.logger.Error("failed to send sessions list", "error", err, "chat_id", message.Chat.ID)
	}
}

func buildSessionsKeyboard(sessions []backendclient.UserSessionListItem) tgbotapi.InlineKeyboardMarkup {
	var rows [][]tgbotapi.InlineKeyboardButton

	for _, s := range sessions {
		indicator := ""
		if s.IsCurrent {
			indicator = "✓ "
		}

		roleLabel := roleLabelsRU[s.Role]
		statusLabel := statusLabelsRU[s.Status]

		label := fmt.Sprintf("%s%s (%s, %s)", indicator, s.Title, roleLabel, statusLabel)
		callbackData := switchSessionCallbackPrefix + strconv.FormatInt(s.SessionID, 10)

		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(label, callbackData),
		))
	}

	return tgbotapi.NewInlineKeyboardMarkup(rows...)
}

func (h *SessionsListHandler) HandleSwitchCallback(ctx context.Context, callback *tgbotapi.CallbackQuery) {
	sessionIDStr := strings.TrimPrefix(callback.Data, switchSessionCallbackPrefix)
	sessionID, err := strconv.ParseInt(sessionIDStr, 10, 64)
	if err != nil {
		h.logger.Warn("invalid session id in switch callback", "data", callback.Data)
		h.answerCallback(callback.ID, "Ошибка")
		return
	}

	if err := h.backend.SwitchActiveSession(ctx, callback.From.ID, sessionID); err != nil {
		h.logger.Error("failed to switch session", "error", err, "telegram_id", callback.From.ID, "session_id", sessionID)
		h.answerCallback(callback.ID, "Не удалось переключиться на эту сессию")
		return
	}

	h.answerCallback(callback.ID, "Сессия переключена")

	edit := tgbotapi.NewEditMessageText(callback.Message.Chat.ID, callback.Message.MessageID, "Активная сессия изменена.")
	if _, err := h.bot.Send(edit); err != nil {
		h.logger.Error("failed to edit message after switch", "error", err)
	}
}

func (h *SessionsListHandler) HandleCurrentCommand(ctx context.Context, message *tgbotapi.Message) {
	sessions, err := h.backend.ListUserSessions(ctx, message.From.ID)
	if err != nil {
		h.logger.Error("failed to list user sessions for current", "error", err, "chat_id", message.Chat.ID)
		h.sendMessage(message.Chat.ID, "Не удалось получить текущую сессию.")
		return
	}

	for _, s := range sessions {
		if s.IsCurrent {
			roleLabel := roleLabelsRU[s.Role]
			statusLabel := statusLabelsRU[s.Status]
			text := fmt.Sprintf("Текущая сессия: %s (%s, %s)", s.Title, roleLabel, statusLabel)
			h.sendMessage(message.Chat.ID, text)
			return
		}
	}

	h.sendMessage(message.Chat.ID, "У вас нет выбранной активной сессии. Используйте /sessions для выбора.")
}

func (h *SessionsListHandler) answerCallback(callbackID, text string) {
	callbackConfig := tgbotapi.NewCallback(callbackID, text)
	if _, err := h.bot.Request(callbackConfig); err != nil {
		h.logger.Error("failed to answer callback", "error", err)
	}
}

func (h *SessionsListHandler) sendMessage(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	if _, err := h.bot.Send(msg); err != nil {
		h.logger.Error("failed to send message", "error", err, "chat_id", chatID)
	}
}
