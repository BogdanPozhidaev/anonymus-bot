package handlers

import (
	"context"
	"log/slog"
	"path/filepath"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/google/uuid"

	"github.com/fastcheck/anonymus_bot/bot/internal/backendclient"
	"github.com/fastcheck/anonymus_bot/bot/internal/fileutil"
)

type VoiceHandler struct {
	bot        *tgbotapi.BotAPI
	backend    *backendclient.Client
	voiceDir   string
	logger     *slog.Logger
	downloader mediaDownloader
}

func NewVoiceHandler(bot *tgbotapi.BotAPI, backend *backendclient.Client, voiceDir string, logger *slog.Logger) *VoiceHandler {
	return &VoiceHandler{bot: bot, backend: backend, voiceDir: voiceDir, logger: logger, downloader: telegramMediaDownloader}
}

func (h *VoiceHandler) Handle(ctx context.Context, message *tgbotapi.Message) {
	if message.Voice == nil {
		return
	}

	fileID := uuid.New().String()
	filePath := filepath.Join(h.voiceDir, fileID+".ogg")

	if err := h.downloader.download(ctx, h.bot.Token, message.Voice.FileID, filePath); err != nil {
		h.logger.Error("failed to download voice", "error", err)
		h.sendErrorNotice(message.Chat.ID)
		return
	}

	req := backendclient.RelayMessageRequest{
		SenderTelegramID: message.From.ID,
		ContentType:      "voice",
		Content:          message.Caption,
		FileID:           fileID,
	}

	resp, err := h.backend.RelayMessage(ctx, req)
	if err != nil {
		h.logger.Error("failed to relay voice message", "error", err)
		// A failed response does not prove the backend did not save the message.
		h.sendErrorNotice(message.Chat.ID)
		return
	}

	if resp.Blocked {
		h.logger.Info("voice message blocked", "reason", resp.BlockReason)
		if resp.MessageID == 0 && resp.BlockReason != "counterparty_not_bound_yet" {
			if err := fileutil.RemoveFile(filePath); err != nil {
				h.logger.Error("failed to remove unpublished media", "error", err)
			}
		}
		h.handleBlockedReason(message.Chat.ID, resp.BlockReason)
		return
	}

	h.logger.Info("voice message relayed", "message_id", resp.MessageID)
}

func (h *VoiceHandler) handleBlockedReason(chatID int64, reason string) {
	var text string
	switch reason {
	case "no_active_session":
		text = "У вас нет активной сессии. Обратитесь к менеджеру."
	case "session_not_active":
		text = "Сессия сейчас недоступна для отправки сообщений."
	case "counterparty_not_bound_yet":
		text = "Вторая сторона ещё не подключилась к сессии. Ваше голосовое сообщение будет доставлено, как только это произойдёт."
	case "moderation_violation":
		text = "Голосовое сообщение не отправлено. Проверьте подпись на наличие контактных данных."
	case "moderation_unavailable":
		text = "Проверка подписи недоступна. Попробуйте отправить сообщение позже."
	default:
		text = "Сообщение не может быть доставлено."
	}

	h.sendMessage(chatID, text)
}

func (h *VoiceHandler) sendErrorNotice(chatID int64) {
	h.sendMessage(chatID, "Произошла ошибка при обработке голосового сообщения. Попробуйте ещё раз.")
}

func (h *VoiceHandler) sendMessage(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	if _, err := h.bot.Send(msg); err != nil {
		h.logger.Error("failed to send message")
	}
}
