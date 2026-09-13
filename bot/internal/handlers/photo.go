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

type PhotoHandler struct {
	bot       *tgbotapi.BotAPI
	backend   *backendclient.Client
	photosDir string
	logger    *slog.Logger
}

func NewPhotoHandler(bot *tgbotapi.BotAPI, backend *backendclient.Client, photosDir string, logger *slog.Logger) *PhotoHandler {
	return &PhotoHandler{bot: bot, backend: backend, photosDir: photosDir, logger: logger}
}

func (h *PhotoHandler) Handle(ctx context.Context, message *tgbotapi.Message) {
	if len(message.Photo) == 0 {
		return
	}

	// Telegram присылает несколько размеров одного фото — берём наибольший (последний в массиве).
	largest := message.Photo[len(message.Photo)-1]

	fileID := uuid.New().String()
	origPath := filepath.Join(h.photosDir, fileID+"_orig.jpg")
	strippedPath := filepath.Join(h.photosDir, fileID+".jpg")

	if err := h.downloadTelegramFile(largest.FileID, origPath); err != nil {
		h.logger.Error("failed to download photo", "error", err, "chat_id", message.Chat.ID)
		h.sendErrorNotice(message.Chat.ID)
		return
	}

	stripErr := fileutil.StripMetadata(origPath, strippedPath)

	// Оригинал с метаданными удаляется немедленно и безусловно,
	// вне зависимости от результата — не оставляем файл с EXIF на диске
	// даже при ошибке обработки.
	if err := fileutil.RemoveFile(origPath); err != nil {
		h.logger.Error("failed to remove original photo", "error", err)
	}

	if stripErr != nil {
		h.logger.Error("failed to strip photo metadata", "error", stripErr, "chat_id", message.Chat.ID)
		h.sendErrorNotice(message.Chat.ID)
		return
	}

	req := backendclient.RelayMessageRequest{
		SenderTelegramID: message.From.ID,
		ContentType:      "photo",
		FileID:           fileID,
	}

	resp, err := h.backend.RelayMessage(ctx, req)
	if err != nil {
		h.logger.Error("failed to relay photo message", "error", err, "chat_id", message.Chat.ID)
		_ = fileutil.RemoveFile(strippedPath)
		h.sendErrorNotice(message.Chat.ID)
		return
	}

	if resp.Blocked {
		h.logger.Info("photo message blocked", "reason", resp.BlockReason, "chat_id", message.Chat.ID)
		// Сессия недоступна/не найдена — файл больше не понадобится.
		if resp.BlockReason == "no_active_session" || resp.BlockReason == "session_not_active" || resp.BlockReason == "sender_not_in_session" {
			_ = fileutil.RemoveFile(strippedPath)
		}
		// При "counterparty_not_bound_yet" файл сознательно НЕ удаляется —
		// он понадобится для отложенной доставки при подключении второй стороны.
		h.handleBlockedReason(message.Chat.ID, resp.BlockReason)
		return
	}

	h.logger.Info("photo message relayed", "message_id", resp.MessageID, "chat_id", message.Chat.ID)
	h.deliverPhoto(resp.RecipientTelegramID, resp.SenderLabel, strippedPath)
}

func (h *PhotoHandler) downloadTelegramFile(telegramFileID, destPath string) error {
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

func (h *PhotoHandler) deliverPhoto(recipientTelegramID int64, senderLabel, filePath string) {
	photo := tgbotapi.NewPhoto(recipientTelegramID, tgbotapi.FilePath(filePath))
	photo.Caption = senderLabel
	photo.ReplyMarkup = replyHereKeyboard()

	if _, err := h.bot.Send(photo); err != nil {
		h.logger.Error("failed to deliver photo to recipient", "error", err, "recipient_telegram_id", recipientTelegramID)
		return
	}

	if err := fileutil.RemoveFile(filePath); err != nil {
		h.logger.Error("failed to remove delivered photo file", "error", err, "file_path", filePath)
	}
}

func (h *PhotoHandler) handleBlockedReason(chatID int64, reason string) {
	var text string
	switch reason {
	case "no_active_session":
		text = "У вас нет активной сессии. Обратитесь к менеджеру."
	case "session_not_active":
		text = "Сессия сейчас недоступна для отправки сообщений."
	case "counterparty_not_bound_yet":
		text = "Вторая сторона ещё не подключилась к сессии. Ваше фото будет доставлено, как только это произойдёт."
	default:
		text = "Сообщение не может быть доставлено."
	}

	h.sendMessage(chatID, text)
}

func (h *PhotoHandler) sendErrorNotice(chatID int64) {
	h.sendMessage(chatID, "Произошла ошибка при обработке фото. Попробуйте ещё раз.")
}

func (h *PhotoHandler) sendMessage(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	if _, err := h.bot.Send(msg); err != nil {
		h.logger.Error("failed to send message", "error", err, "chat_id", chatID)
	}
}
