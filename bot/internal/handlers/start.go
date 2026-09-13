package handlers

import (
	"context"
	"errors"
	"log/slog"
	"path/filepath"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/fastcheck/anonymus_bot/bot/internal/backendclient"
	"github.com/fastcheck/anonymus_bot/bot/internal/fileutil"
)

type StartHandler struct {
	bot       *tgbotapi.BotAPI
	backend   *backendclient.Client
	logger    *slog.Logger
	photosDir string
	voiceDir  string
}

func NewStartHandler(bot *tgbotapi.BotAPI, backend *backendclient.Client, photosDir, voiceDir string, logger *slog.Logger) *StartHandler {
	return &StartHandler{bot: bot, backend: backend, photosDir: photosDir, voiceDir: voiceDir, logger: logger}
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

	h.deliverPendingMessages(message.Chat.ID, resp.UndeliveredMessages)
}

func (h *StartHandler) deliverPendingMessages(chatID int64, messages []backendclient.UndeliveredMessage) {
	for _, m := range messages {
		switch m.ContentType {
		case "text":
			h.deliverPendingText(chatID, m)
		case "photo":
			h.deliverPendingPhoto(chatID, m)
		case "voice":
			h.deliverPendingVoice(chatID, m)
		default:
			h.logger.Warn("unsupported pending message content type", "content_type", m.ContentType, "message_id", m.MessageID)
		}
	}
}

func (h *StartHandler) deliverPendingText(chatID int64, m backendclient.UndeliveredMessage) {
	text := m.SenderLabel + " " + m.Content
	msg := tgbotapi.NewMessage(chatID, text)
	if _, err := h.bot.Send(msg); err != nil {
		h.logger.Error("failed to deliver pending text message", "error", err, "message_id", m.MessageID, "chat_id", chatID)
	}
}

func (h *StartHandler) deliverPendingPhoto(chatID int64, m backendclient.UndeliveredMessage) {
	filePath := filepath.Join(h.photosDir, m.FileID+".jpg")

	photo := tgbotapi.NewPhoto(chatID, tgbotapi.FilePath(filePath))
	photo.Caption = m.SenderLabel

	if _, err := h.bot.Send(photo); err != nil {
		h.logger.Error("failed to deliver pending photo", "error", err, "message_id", m.MessageID, "chat_id", chatID, "file_path", filePath)
		return
	}

	if err := fileutil.RemoveFile(filePath); err != nil {
		h.logger.Error("failed to remove delivered photo file", "error", err, "file_path", filePath)
	}
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

func (h *StartHandler) deliverPendingVoice(chatID int64, m backendclient.UndeliveredMessage) {
	filePath := filepath.Join(h.voiceDir, m.FileID+".ogg")

	voice := tgbotapi.NewVoice(chatID, tgbotapi.FilePath(filePath))
	voice.Caption = m.SenderLabel

	if _, err := h.bot.Send(voice); err != nil {
		h.logger.Error("failed to deliver pending voice", "error", err, "message_id", m.MessageID, "chat_id", chatID, "file_path", filePath)
		return
	}

	if err := fileutil.RemoveFile(filePath); err != nil {
		h.logger.Error("failed to remove delivered voice file", "error", err, "file_path", filePath)
	}
}
