package handlers

import (
	"context"
	"errors"
	"fmt"
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
	if message.Chat == nil {
		return
	}
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
			h.sendMessage(message.Chat.ID, tr(message.From,
				"Не удалось проверить приглашение. Попробуйте открыть ту же ссылку позже.",
				"We couldn't check the invitation. Try opening the same link again later."))
		}
		return
	}

	h.logger.Info("session bound successfully",
		"session_id", resp.SessionID,
		"role", resp.SessionType,
	)

	if resp.SessionType == "executor" {
		text := tr(message.From, "Вы подключены как исполнитель.", "You joined as the executor.")
		if resp.RequestText != "" {
			text += "\n\n" + tr(message.From, "Текст обращения:", "Request details:") + "\n" + resp.RequestText
		} else {
			text += "\n\n" + tr(message.From, "Текст обращения пока недоступен.", "Request details are currently unavailable.")
		}
		h.sendMessage(message.Chat.ID, text)
		return
	}

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

	response, err := h.backend.CreateIncomingRequest(ctx, req)
	if err != nil || response.ID <= 0 {
		h.logger.Error("failed to confirm incoming request")
		h.sendMessage(message.Chat.ID, tr(message.From,
			"Не удалось подтвердить приём обращения. Отправьте /start ещё раз через минуту.",
			"We couldn't confirm your request. Send /start again in a minute."))
		return
	}

	h.sendRequestStatus(message.Chat.ID, message.From, response)
}

func (h *StartHandler) sendRequestStatus(chatID int64, user *tgbotapi.User, response *backendclient.CreateIncomingRequestResponse) {
	switch response.Status {
	case "in_progress":
		h.sendMessage(chatID, fmt.Sprintf(tr(user,
			"Обращение №%d в работе. Если оператор пришлёт ссылку-приглашение, откройте её для подключения.",
			"Request #%d is in progress. If the operator sends an invitation link, open it to join."), response.ID))
		return
	case "processed":
		h.sendMessage(chatID, fmt.Sprintf(tr(user,
			"Обращение №%d уже обработано. Если вопрос остался, попробуйте /start через 10 минут.",
			"Request #%d has already been handled. If you still need help, try /start again in 10 minutes."), response.ID))
		return
	}
	ru := "Спасибо за обращение в компанию «" + h.organizationName() + "»."
	en := "Thank you for contacting us."
	if h.branding.OrganizationName != "" {
		en = "Thank you for contacting " + h.branding.OrganizationName + "."
	}
	greeting := tr(user, brandedText(h.branding.RequestReceivedRU, ru), brandedText(h.branding.RequestReceivedEN, en))
	status := fmt.Sprintf(tr(user,
		"Обращение №%d принято. Опишите вопрос одним сообщением, если ещё не сделали этого.",
		"Request #%d was received. Describe your question in one message if you haven't already."), response.ID)
	h.sendMessage(chatID, greeting+"\n\n"+status)
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
