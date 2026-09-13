package handlers

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/google/uuid"

	"github.com/fastcheck/anonymus_bot/bot/internal/backendclient"
	"github.com/fastcheck/anonymus_bot/bot/internal/fileutil"
)

type VoiceHandler struct {
	bot      *tgbotapi.BotAPI
	backend  *backendclient.Client
	voiceDir string
	logger   *slog.Logger
}

func NewVoiceHandler(bot *tgbotapi.BotAPI, backend *backendclient.Client, voiceDir string, logger *slog.Logger) *VoiceHandler {
	return &VoiceHandler{bot: bot, backend: backend, voiceDir: voiceDir, logger: logger}
}

func (h *VoiceHandler) Handle(ctx context.Context, message *tgbotapi.Message) {
	if message.Voice == nil {
		return
	}

	fileID := uuid.New().String()
	filePath := filepath.Join(h.voiceDir, fileID+".ogg")

	if err := h.downloadTelegramFile(message.Voice.FileID, filePath); err != nil {
		h.logger.Error("failed to download voice", "error", err, "chat_id", message.Chat.ID)
		h.sendErrorNotice(message.Chat.ID)
		return
	}

	req := backendclient.RelayMessageRequest{
		SenderTelegramID: message.From.ID,
		ContentType:      "voice",
		FileID:           fileID,
	}

	resp, err := h.backend.RelayMessage(ctx, req)
	if err != nil {
		h.logger.Error("failed to relay voice message", "error", err, "chat_id", message.Chat.ID)
		_ = fileutil.RemoveFile(filePath)
		h.sendErrorNotice(message.Chat.ID)
		return
	}

	if resp.Blocked {
		h.logger.Info("voice message blocked", "reason", resp.BlockReason, "chat_id", message.Chat.ID)
		if resp.BlockReason != "counterparty_not_bound_yet" {
			_ = fileutil.RemoveFile(filePath)
		}
		h.handleBlockedReason(message.Chat.ID, resp.BlockReason)
		return
	}

	h.logger.Info("voice message relayed", "message_id", resp.MessageID, "chat_id", message.Chat.ID)
	h.deliverVoice(resp.RecipientTelegramID, resp.SenderLabel, filePath)
}

func (h *VoiceHandler) downloadTelegramFile(telegramFileID, destPath string) error {
	file, err := h.bot.GetFile(tgbotapi.FileConfig{FileID: telegramFileID})
	if err != nil {
		return fmt.Errorf("failed to get file info: %w", err)
	}

	url := file.Link(h.bot.Token)

	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("failed to download file: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code downloading file: %d", resp.StatusCode)
	}

	out, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("failed to create destination file: %w", err)
	}
	defer func() {
		_ = out.Close()
	}()

	if _, err := io.Copy(out, resp.Body); err != nil {
		return fmt.Errorf("failed to save file: %w", err)
	}

	return nil
}

func (h *VoiceHandler) deliverVoice(recipientTelegramID int64, senderLabel, filePath string) {
	voice := tgbotapi.NewVoice(recipientTelegramID, tgbotapi.FilePath(filePath))
	voice.Caption = senderLabel
	voice.ReplyMarkup = replyHereKeyboard()

	if _, err := h.bot.Send(voice); err != nil {
		h.logger.Error("failed to deliver voice to recipient", "error", err, "recipient_telegram_id", recipientTelegramID)
		return
	}

	if err := fileutil.RemoveFile(filePath); err != nil {
		h.logger.Error("failed to remove delivered voice file", "error", err, "file_path", filePath)
	}
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
		h.logger.Error("failed to send message", "error", err, "chat_id", chatID)
	}
}
