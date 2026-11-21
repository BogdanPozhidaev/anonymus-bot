package handlers

import (
	"context"
	"log/slog"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/fastcheck/anonymus_bot/bot/internal/backendclient"
)

type MessageRelayHandler struct {
	bot     *tgbotapi.BotAPI
	backend *backendclient.Client
	logger  *slog.Logger
}

func NewMessageRelayHandler(bot *tgbotapi.BotAPI, backend *backendclient.Client, logger *slog.Logger) *MessageRelayHandler {
	return &MessageRelayHandler{bot: bot, backend: backend, logger: logger}
}

func (h *MessageRelayHandler) HandleText(ctx context.Context, message *tgbotapi.Message) {
	if message.From == nil || message.Chat == nil || !message.Chat.IsPrivate() {
		return
	}
	req := backendclient.RelayMessageRequest{
		SenderTelegramID: message.From.ID,
		ContentType:      "text",
		Content:          message.Text,
	}

	resp, err := h.backend.RelayMessage(ctx, req)
	if err != nil {
		h.logger.Error("failed to relay message", "error", err)
		h.sendErrorNotice(message.Chat.ID)
		return
	}

	if resp.Blocked {
		if resp.BlockReason == "no_active_session" {
			_, err := h.backend.CreateIncomingRequest(ctx, backendclient.CreateIncomingRequestRequest{
				TelegramID: message.From.ID, Username: message.From.UserName, FirstName: message.From.FirstName,
				FirstMessageText: message.Text, LanguageCode: message.From.LanguageCode,
			})
			if err != nil {
				h.logger.Error("failed to submit first message")
				h.sendErrorNotice(message.Chat.ID)
				return
			}
			h.sendMessage(message.Chat.ID, "Ваше обращение принято.")
			return
		}
		h.logger.Info("message blocked", "reason", resp.BlockReason)
		h.handleBlockedReason(message.Chat.ID, resp.BlockReason)
		return
	}

	h.logger.Info("message relayed", "message_id", resp.MessageID)
}

func (h *MessageRelayHandler) handleBlockedReason(chatID int64, reason string) {
	var text string
	switch reason {
	case "no_active_session":
		text = "У вас нет активной сессии. Обратитесь к менеджеру."
	case "session_not_active":
		text = "Сессия сейчас недоступна для отправки сообщений."
	case "counterparty_not_bound_yet":
		text = "Вторая сторона ещё не подключилась к сессии. Ваше сообщение будет доставлено, как только это произойдёт."
	case "moderation_violation":
		text = "Сообщение не отправлено: обмен личными контактами и прямые контакты в обход платформы запрещены правилами сервиса."
	case "moderation_unavailable":
		text = "Проверка сообщения недоступна. Попробуйте отправить его позже."
	default:
		text = "Сообщение не может быть доставлено."
	}

	h.sendMessage(chatID, text)
}

func (h *MessageRelayHandler) sendErrorNotice(chatID int64) {
	h.sendMessage(chatID, "Произошла ошибка при отправке сообщения. Попробуйте ещё раз.")
}

func (h *MessageRelayHandler) sendMessage(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	if _, err := h.bot.Send(msg); err != nil {
		h.logger.Error("failed to send message")
	}
}
