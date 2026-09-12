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
	req := backendclient.RelayMessageRequest{
		SenderTelegramID: message.From.ID,
		ContentType:      "text",
		Content:          message.Text,
	}

	resp, err := h.backend.RelayMessage(ctx, req)
	if err != nil {
		h.logger.Error("failed to relay message", "error", err, "chat_id", message.Chat.ID)
		h.sendErrorNotice(message.Chat.ID)
		return
	}

	if resp.Blocked {
		h.logger.Info("message blocked", "reason", resp.BlockReason, "chat_id", message.Chat.ID)
		h.handleBlockedReason(message.Chat.ID, resp.BlockReason)
		return
	}

	h.logger.Info("message relayed", "message_id", resp.MessageID, "chat_id", message.Chat.ID)
	h.deliverToRecipient(resp.RecipientTelegramID, resp.SenderLabel, message.Text)
}

// deliverToRecipient отправляет копию сообщения получателю от имени бота.
// Это НЕ forward — обычное новое сообщение, полностью отвязанное от оригинального
// отправителя. Telegram API не передаёт сюда никаких метаданных исходного сообщения
// (from, forward_from, reply_to), потому что мы формируем совершенно новый message
// через NewMessage, а не пересылаем существующий через Forward.
func (h *MessageRelayHandler) deliverToRecipient(recipientTelegramID int64, senderLabel, text string) {
	fullText := senderLabel + " " + text

	msg := tgbotapi.NewMessage(recipientTelegramID, fullText)
	msg.ReplyMarkup = replyHereKeyboard()

	if _, err := h.bot.Send(msg); err != nil {
		h.logger.Error("failed to deliver message to recipient", "error", err, "recipient_telegram_id", recipientTelegramID)
	}
}

func replyHereKeyboard() tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Ответить сюда", "reply_here"),
		),
	)
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
		h.logger.Error("failed to send message", "error", err, "chat_id", chatID)
	}
}
