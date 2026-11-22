package handlers

import (
	"context"
	"errors"
	"log/slog"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/fastcheck/anonymus_bot/bot/internal/backendclient"
	"github.com/fastcheck/anonymus_bot/bot/internal/config"
)

type StartHandler struct {
	bot      *tgbotapi.BotAPI
	backend  *backendclient.Client
	logger   *slog.Logger
	branding config.StartBranding
}

func NewStartHandler(bot *tgbotapi.BotAPI, backend *backendclient.Client, logger *slog.Logger, branding config.StartBranding) *StartHandler {
	return &StartHandler{bot: bot, backend: backend, logger: logger, branding: branding}
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
	unavailable := tr(message.From, "Приглашение недоступно. Запросите новую ссылку у оператора и откройте её в личном чате с ботом.",
		"This invitation is unavailable. Ask the operator for a new link and open it in a private chat with the bot.")
	if !validInvitePayload(payload) || !message.Chat.IsPrivate() || message.From == nil {
		h.sendMessage(message.Chat.ID, unavailable)
		return
	}
	resp, err := h.backend.BindSession(ctx, backendclient.BindSessionRequest{
		TelegramID: message.From.ID, Username: message.From.UserName, FirstName: message.From.FirstName, Token: payload,
		LanguageCode: languageOf(message.From),
	})
	if err != nil {
		if errors.Is(err, backendclient.ErrNotFound) {
			h.sendMessage(message.Chat.ID, unavailable)
		} else {
			h.logger.Error("failed to redeem session invitation")
			h.sendNeutralGreeting(message.Chat.ID, message.From)
		}
		return
	}

	h.logger.Info("session bound successfully",
		"session_id", resp.SessionID,
		"role", resp.SessionType,
	)

	ru := "Здравствуйте! Вы подключены к сессии в компании «" + h.organizationName() + "». Ваши сообщения будут переданы менеджеру."
	en := "Hello! You have joined the session. Your messages will be sent to the manager."
	if h.branding.OrganizationName != "" {
		en = "Hello! You have joined a session with " + h.branding.OrganizationName + ". Your messages will be sent to the manager."
	}
	greeting := tr(message.From, brandedText(h.branding.JoinedRU, ru), brandedText(h.branding.JoinedEN, en))
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

	h.sendNeutralGreeting(message.Chat.ID, message.From)
}

func (h *StartHandler) sendNeutralGreeting(chatID int64, user *tgbotapi.User) {
	ru := "Спасибо за обращение в компанию «" + h.organizationName() + "». Ваш запрос принят, менеджер свяжется с вами."
	en := "Thank you for contacting us. A manager will get back to you."
	if h.branding.OrganizationName != "" {
		en = "Thank you for contacting " + h.branding.OrganizationName + ". A manager will get back to you."
	}
	text := tr(user, brandedText(h.branding.RequestReceivedRU, ru), brandedText(h.branding.RequestReceivedEN, en))
	h.sendMessage(chatID, text)
}

func (h *StartHandler) organizationName() string {
	return brandedText(h.branding.OrganizationName, "Посредник")
}

func brandedText(custom, fallback string) string {
	if custom != "" {
		return custom
	}
	return fallback
}

func (h *StartHandler) sendMessage(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	if _, err := h.bot.Send(msg); err != nil {
		h.logger.Error("failed to send message")
	}
}
