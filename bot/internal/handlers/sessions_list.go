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
const sessionsPagePrefix = "sessions_page:"

var statusLabelsRU = map[string]string{
	"active":        "активна",
	"paused":        "на паузе",
	"pending_close": "ожидает закрытия",
}

func sessionStatusLabel(sender *tgbotapi.User, status string) string {
	if label := statusLabelsRU[status]; label != "" {
		return tr(sender, label, strings.ReplaceAll(status, "_", " "))
	}
	return status
}

var roleLabelsRU = map[string]string{
	"client":   "клиент",
	"executor": "исполнитель",
}

func (h *SessionsListHandler) HandleSessionsCommand(ctx context.Context, message *tgbotapi.Message) {
	if message.From == nil || message.Chat == nil || !message.Chat.IsPrivate() {
		return
	}
	h.showPage(ctx, message.Chat.ID, message.From, 0)
}

func (h *SessionsListHandler) HandlePageCallback(ctx context.Context, callback *tgbotapi.CallbackQuery) {
	offset, ok := parsePageOffset(strings.TrimPrefix(callback.Data, sessionsPagePrefix))
	if !ok || callback.From == nil || callback.Message == nil || callback.Message.Chat == nil || !callback.Message.Chat.IsPrivate() {
		h.answerCallback(callback.ID, tr(callback.From, "Ошибка", "Error"))
		return
	}
	h.answerCallback(callback.ID, "")
	h.showPage(ctx, callback.Message.Chat.ID, callback.From, offset)
}

func (h *SessionsListHandler) showPage(ctx context.Context, chatID int64, sender *tgbotapi.User, offset int) {
	sessions, hasMore, err := h.backend.ListUserSessions(ctx, sender.ID, offset)
	if err != nil {
		h.logger.Error("failed to list user sessions", "error", err)
		h.sendMessage(chatID, tr(sender, "Не удалось загрузить список сессий. Попробуйте позже.", "Unable to load your sessions. Try again later."))
		return
	}

	if len(sessions) == 0 && offset == 0 {
		h.sendMessage(chatID, tr(sender, "У вас пока нет активных сессий.", "You have no active sessions yet."))
		return
	}

	msg := tgbotapi.NewMessage(chatID, tr(sender, "Ваши сессии:", "Your sessions:"))
	keyboard := buildSessionsKeyboard(sessions, sender)
	if buttons := pageButtons(sessionsPagePrefix, offset, hasMore, sender); len(buttons) > 0 {
		keyboard.InlineKeyboard = append(keyboard.InlineKeyboard, buttons)
	}
	msg.ReplyMarkup = keyboard

	if _, err := h.bot.Send(msg); err != nil {
		h.logger.Error("failed to send sessions list")
	}
}

func buildSessionsKeyboard(sessions []backendclient.UserSessionListItem, sender *tgbotapi.User) tgbotapi.InlineKeyboardMarkup {
	var rows [][]tgbotapi.InlineKeyboardButton

	for _, s := range sessions {
		indicator := ""
		if s.IsCurrent {
			indicator = "✓ "
		}

		roleLabel := tr(sender, roleLabelsRU[s.Role], s.Role)
		statusLabel := sessionStatusLabel(sender, s.Status)

		label := fmt.Sprintf("%s%s (%s, %s)", indicator, menuExcerpt(s.Title), roleLabel, statusLabel)
		callbackData := switchSessionCallbackPrefix + strconv.FormatInt(s.SessionID, 10)

		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(label, callbackData),
		))
	}

	return tgbotapi.NewInlineKeyboardMarkup(rows...)
}

func (h *SessionsListHandler) HandleSwitchCallback(ctx context.Context, callback *tgbotapi.CallbackQuery) {
	if callback.From == nil || callback.Message == nil || callback.Message.Chat == nil || !callback.Message.Chat.IsPrivate() {
		h.answerCallback(callback.ID, tr(callback.From, "Ошибка", "Error"))
		return
	}
	sessionIDStr := strings.TrimPrefix(callback.Data, switchSessionCallbackPrefix)
	sessionID, err := strconv.ParseInt(sessionIDStr, 10, 64)
	if err != nil {
		h.logger.Warn("invalid session id in switch callback")
		h.answerCallback(callback.ID, tr(callback.From, "Ошибка", "Error"))
		return
	}

	if err := h.backend.SwitchActiveSession(ctx, callback.From.ID, sessionID); err != nil {
		h.logger.Error("failed to switch session", "error", err, "session_id", sessionID)
		h.answerCallback(callback.ID, tr(callback.From, "Не удалось переключиться на эту сессию", "Unable to switch to this session"))
		return
	}

	h.answerCallback(callback.ID, tr(callback.From, "Сессия переключена", "Session switched"))

	edit := tgbotapi.NewEditMessageText(callback.Message.Chat.ID, callback.Message.MessageID, tr(callback.From, "Активная сессия изменена.", "Active session changed."))
	if _, err := h.bot.Send(edit); err != nil {
		h.logger.Error("failed to edit message after switch")
	}
}

func (h *SessionsListHandler) HandleCurrentCommand(ctx context.Context, message *tgbotapi.Message) {
	if message.From == nil || message.Chat == nil || !message.Chat.IsPrivate() {
		return
	}
	sessions, _, err := h.backend.ListUserSessions(ctx, message.From.ID, 0)
	if err != nil {
		h.logger.Error("failed to list user sessions for current", "error", err)
		h.sendMessage(message.Chat.ID, tr(message.From, "Не удалось получить текущую сессию.", "Unable to load the current session."))
		return
	}

	for _, s := range sessions {
		if s.IsCurrent {
			roleLabel := tr(message.From, roleLabelsRU[s.Role], s.Role)
			statusLabel := sessionStatusLabel(message.From, s.Status)
			text := fmt.Sprintf(tr(message.From, "Текущая сессия: %s (%s, %s)", "Current session: %s (%s, %s)"), s.Title, roleLabel, statusLabel)
			h.sendMessage(message.Chat.ID, text)
			return
		}
	}

	h.sendMessage(message.Chat.ID, tr(message.From, "У вас нет выбранной активной сессии. Используйте /sessions для выбора.", "No active session selected. Use /sessions to choose one."))
}

func (h *SessionsListHandler) answerCallback(callbackID, text string) {
	callbackConfig := tgbotapi.NewCallback(callbackID, text)
	if _, err := h.bot.Request(callbackConfig); err != nil {
		h.logger.Error("failed to answer callback")
	}
}

func (h *SessionsListHandler) sendMessage(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	if _, err := h.bot.Send(msg); err != nil {
		h.logger.Error("failed to send message")
	}
}
