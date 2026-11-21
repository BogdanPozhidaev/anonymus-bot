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
	const unavailable = "Приглашение недоступно. Запросите новую ссылку у оператора и откройте её в личном чате с ботом."
	if !validInvitePayload(payload) || !message.Chat.IsPrivate() || message.From == nil {
		h.sendMessage(message.Chat.ID, unavailable)
		return
	}
	resp, err := h.backend.BindSession(ctx, backendclient.BindSessionRequest{
		TelegramID: message.From.ID, Username: message.From.UserName, FirstName: message.From.FirstName, Token: payload,
	})
	if err != nil {
		if errors.Is(err, backendclient.ErrNotFound) {
			h.sendMessage(message.Chat.ID, unavailable)
		} else {
			h.logger.Error("failed to redeem session invitation")
			h.sendNeutralGreeting(message.Chat.ID)
		}
		return
	}

	h.logger.Info("session bound successfully",
		"session_id", resp.SessionID,
		"role", resp.SessionType,
	)

	greeting := "Здравствуйте! Вы подключены к сессии в компании «Посредник». Ваши сообщения будут переданы менеджеру."
	h.sendMessage(message.Chat.ID, greeting)

}

func (h *StartHandler) handleWithoutPayload(ctx context.Context, message *tgbotapi.Message) {
	if message.From == nil || message.Chat == nil || !message.Chat.IsPrivate() {
		return
	}
	req := backendclient.CreateIncomingRequestRequest{
		TelegramID:   message.From.ID,
		Username:     message.From.UserName,
		FirstName:    message.From.FirstName,
		LanguageCode: message.From.LanguageCode,
	}

	if _, err := h.backend.CreateIncomingRequest(ctx, req); err != nil {
		h.logger.Error("failed to create incoming request", "error", err)
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
		h.logger.Error("failed to send message")
	}
}
