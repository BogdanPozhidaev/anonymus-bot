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

type PhotoHandler struct {
	bot        *tgbotapi.BotAPI
	backend    *backendclient.Client
	photosDir  string
	logger     *slog.Logger
	downloader mediaDownloader
	strip      func(string, string) error
}

func NewPhotoHandler(bot *tgbotapi.BotAPI, backend *backendclient.Client, photosDir string, logger *slog.Logger) *PhotoHandler {
	return &PhotoHandler{bot: bot, backend: backend, photosDir: photosDir, logger: logger, downloader: telegramMediaDownloader, strip: fileutil.StripMetadata}
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

	if err := h.downloader.download(ctx, h.bot.Token, largest.FileID, origPath); err != nil {
		h.logger.Error("failed to download photo", "error", err)
		h.sendErrorNotice(message.Chat.ID, message.From)
		return
	}

	stripErr := h.strip(origPath, strippedPath)

	// Оригинал с метаданными удаляется немедленно и безусловно,
	// вне зависимости от результата — не оставляем файл с EXIF на диске
	// даже при ошибке обработки.
	if err := fileutil.RemoveFile(origPath); err != nil {
		h.logger.Error("failed to remove original photo", "error", err)
	}

	if stripErr != nil {
		if err := fileutil.RemoveFile(strippedPath); err != nil {
			h.logger.Error("failed to remove incomplete photo", "error", err)
		}
		h.logger.Error("failed to strip photo metadata", "error", stripErr)
		h.sendErrorNotice(message.Chat.ID, message.From)
		return
	}

	req := backendclient.RelayMessageRequest{
		SenderTelegramID: message.From.ID,
		ContentType:      "photo",
		Content:          message.Caption,
		FileID:           fileID,
	}

	resp, err := h.backend.RelayMessage(ctx, req)
	if err != nil {
		h.logger.Error("failed to relay photo message", "error", err)
		// A failed response does not prove the backend did not save the message.
		h.sendErrorNotice(message.Chat.ID, message.From)
		return
	}

	if resp.Blocked {
		h.logger.Info("photo message blocked", "reason", resp.BlockReason)
		if resp.MessageID == 0 && resp.BlockReason != "counterparty_not_bound_yet" {
			if err := fileutil.RemoveFile(strippedPath); err != nil {
				h.logger.Error("failed to remove unpublished media", "error", err)
			}
		}
		h.handleBlockedReason(message.Chat.ID, resp.BlockReason, message.From)
		return
	}

	h.logger.Info("photo message relayed", "message_id", resp.MessageID)
}

func (h *PhotoHandler) handleBlockedReason(chatID int64, reason string, user *tgbotapi.User) {
	var text string
	switch reason {
	case "no_active_session":
		text = tr(user, "У вас нет активной сессии. Обратитесь к менеджеру.", "You have no active session. Contact your manager.")
	case "session_not_active":
		text = tr(user, "Сессия сейчас недоступна для отправки сообщений.", "This session cannot receive messages right now.")
	case "counterparty_not_bound_yet":
		text = tr(user, "Вторая сторона ещё не подключилась к сессии. Ваше фото будет доставлено, как только это произойдёт.", "Your photo will be delivered when the other person joins.")
	case "moderation_violation":
		text = tr(user, "Фото не отправлено. Проверьте подпись на наличие контактных данных.", "Photo blocked. Check the caption for contact details.")
	case "moderation_unavailable":
		text = tr(user, "Проверка подписи недоступна. Попробуйте отправить сообщение позже.", "Caption screening is unavailable. Try again later.")
	default:
		text = tr(user, "Сообщение не может быть доставлено.", "The message cannot be delivered.")
	}

	h.sendMessage(chatID, text)
}

func (h *PhotoHandler) sendErrorNotice(chatID int64, user *tgbotapi.User) {
	h.sendMessage(chatID, tr(user, "Произошла ошибка при обработке фото. Попробуйте ещё раз.", "Unable to process the photo. Try again."))
}

func (h *PhotoHandler) sendMessage(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	if _, err := h.bot.Send(msg); err != nil {
		h.logger.Error("failed to send message")
	}
}
