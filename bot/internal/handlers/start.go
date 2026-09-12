package handlers

import (
	"context"
	"errors"
	"log/slog"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/fastcheck/anonymus_bot/bot/internal/backendclient"
)

type StartHandler struct {
	bot     *tgbotapi.BotAPI
	backend *backendclient.Client
	logger  *slog.Logger
}

func NewStartHandler(bot *tgbotapi.BotAPI, backend *backendclient.Client, logger *slog.Logger) *StartHandler {
	return &StartHandler{bot: bot, backend: backend, logger: logger}
}

func (h *StartHandler) Handle(ctx context.Context, message *tgbotapi.Message) {
	payload := message.CommandArguments()

	if payload == "" {
		h.handleWithoutPayload(ctx, message)
		return
	}

	h.handleWithPayload(ctx, message, payload)
}

func (h *StartHandler) handleWithPayload(ctx context.Context, message *tgbotapi.Message, payload string) {
	parsed, err := ParseDeepLinkPayload(payload)
	if err != nil {
		h.logger.Warn("invalid deeplink payload", "payload", payload, "chat_id", message.Chat.ID)
		h.sendNeutralGreeting(message.Chat.ID)
		return
	}

	req := backendclient.BindSessionRequest{
		TelegramID: message.From.ID,
		Username:   message.From.UserName,
		FirstName:  message.From.FirstName,
		Role:       string(parsed.Role),
	}

	resp, err := h.backend.BindSession(ctx, parsed.SessionID, req)
	if err != nil {
		if errors.Is(err, backendclient.ErrNotFound) {
			h.logger.Warn("session not found for deeplink", "session_id", parsed.SessionID, "chat_id", message.Chat.ID)
		} else {
			h.logger.Error("failed to bind session", "error", err, "chat_id", message.Chat.ID)
		}
		h.sendNeutralGreeting(message.Chat.ID)
		return
	}

	h.logger.Info("session bound successfully",
		"session_id", resp.SessionID,
		"role", resp.SessionType,
		"chat_id", message.Chat.ID,
	)

	greeting := "Здравствуйте! Вы подключены к сессии в компании «Посредник». Ваши сообщения будут переданы менеджеру."
	h.sendMessage(message.Chat.ID, greeting)
}

func (h *StartHandler) handleWithoutPayload(ctx context.Context, message *tgbotapi.Message) {
	req := backendclient.CreateIncomingRequestRequest{
		TelegramID:   message.From.ID,
		Username:     message.From.UserName,
		FirstName:    message.From.FirstName,
		LanguageCode: message.From.LanguageCode,
	}

	if _, err := h.backend.CreateIncomingRequest(ctx, req); err != nil {
		h.logger.Error("failed to create incoming request", "error", err, "chat_id", message.Chat.ID)
	}

	h.sendNeutralGreeting(message.Chat.ID)
}

func (h *StartHandler) sendNeutralGreeting(chatID int64) {
	text := "Спасибо за обращение в компанию «Посредник». Ваш запрос принят, менеджер свяжется с вами."
	h.sendMessage(chatID, text)
}

func (h *StartHandler) sendMessage(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	if _, err := h.bot.Send(msg); err != nil {
		h.logger.Error("failed to send message", "error", err, "chat_id", chatID)
	}
}
