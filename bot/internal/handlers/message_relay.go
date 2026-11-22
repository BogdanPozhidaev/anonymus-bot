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
		h.sendErrorNotice(message.Chat.ID, message.From)
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
				h.sendErrorNotice(message.Chat.ID, message.From)
				return
			}
			h.sendMessage(message.Chat.ID, tr(message.From, "Ваше обращение принято.", "Your request has been received."))
			return
		}
		h.logger.Info("message blocked", "reason", resp.BlockReason)
		h.handleBlockedReason(message.Chat.ID, resp.BlockReason, message.From)
		return
	}

	h.logger.Info("message relayed", "message_id", resp.MessageID)
}

func (h *MessageRelayHandler) handleBlockedReason(chatID int64, reason string, user *tgbotapi.User) {
	var text string
	switch reason {
	case "no_active_session":
		text = tr(user, "У вас нет активной сессии. Обратитесь к менеджеру.", "You have no active session. Contact your manager.")
	case "session_not_active":
		text = tr(user, "Сессия сейчас недоступна для отправки сообщений.", "This session cannot receive messages right now.")
	case "counterparty_not_bound_yet":
		text = tr(user, "Вторая сторона ещё не подключилась к сессии. Ваше сообщение будет доставлено, как только это произойдёт.", "The other person has not joined. Your message will be delivered when they join.")
	case "moderation_violation":
		text = tr(user, "Сообщение не отправлено: обмен личными контактами и прямые контакты в обход платформы запрещены правилами сервиса.", "Message blocked. The service does not allow sharing personal contact details or contacting someone outside the platform.")
	case "moderation_unavailable":
		text = tr(user, "Проверка сообщения недоступна. Попробуйте отправить его позже.", "Message screening is unavailable. Try sending it again later.")
	default:
		text = tr(user, "Сообщение не может быть доставлено.", "The message cannot be delivered.")
	}

	h.sendMessage(chatID, text)
}

func (h *MessageRelayHandler) sendErrorNotice(chatID int64, user *tgbotapi.User) {
	h.sendMessage(chatID, tr(user, "Произошла ошибка при отправке сообщения. Попробуйте ещё раз.", "Unable to send the message. Try again."))
}

func (h *MessageRelayHandler) sendMessage(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	if _, err := h.bot.Send(msg); err != nil {
		h.logger.Error("failed to send message")
	}
}
