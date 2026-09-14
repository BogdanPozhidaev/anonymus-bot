package retention

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/fastcheck/anonymus_bot/backend/internal/models"
	"github.com/fastcheck/anonymus_bot/backend/internal/repository"
)

type MessageCleaner struct {
	messageRepo *repository.MessageRepository
	photosDir   string
	voiceDir    string
	config      Config
	logger      *slog.Logger
}

func NewMessageCleaner(
	messageRepo *repository.MessageRepository,
	photosDir, voiceDir string,
	config Config,
	logger *slog.Logger,
) *MessageCleaner {
	return &MessageCleaner{
		messageRepo: messageRepo,
		photosDir:   photosDir,
		voiceDir:    voiceDir,
		config:      config,
		logger:      logger,
	}
}

type MessageCleanupResult struct {
	MessagesDeleted int
	FilesDeleted    int
	Errors          []error
}

// Run удаляет сообщения (и связанные медиафайлы) сессий, закрытых раньше
// MessageRetentionPeriod назад. При dryRun=true ничего не удаляется —
// только подсчитывается, что было бы удалено.
func (m *MessageCleaner) Run(ctx context.Context, dryRun bool) (*MessageCleanupResult, error) {
	threshold := time.Now().Add(-m.config.MessageRetentionPeriod)

	messages, err := m.messageRepo.ListForDeletion(ctx, threshold)
	if err != nil {
		return nil, fmt.Errorf("failed to list messages for deletion: %w", err)
	}

	result := &MessageCleanupResult{}
	var idsToDelete []int64

	for _, msg := range messages {
		if dryRun {
			m.logger.Info("[DRY RUN] would delete message", "message_id", msg.ID, "content_type", msg.ContentType)
			result.MessagesDeleted++
			if msg.FileID != nil {
				result.FilesDeleted++
			}
			continue
		}

		if msg.FileID != nil {
			if err := m.deleteMediaFile(msg); err != nil {
				m.logger.Error("failed to delete media file", "error", err, "message_id", msg.ID, "file_id", *msg.FileID)
				result.Errors = append(result.Errors, fmt.Errorf("message %d: %w", msg.ID, err))
				// Продолжаем — отсутствие файла на диске не должно блокировать
				// удаление записи о сообщении, если файл уже был удалён ранее
				// (например, повторный запуск после сбоя).
			} else {
				result.FilesDeleted++
			}
		}

		idsToDelete = append(idsToDelete, msg.ID)
	}

	if !dryRun && len(idsToDelete) > 0 {
		if err := m.messageRepo.DeleteByIDs(ctx, idsToDelete); err != nil {
			return result, fmt.Errorf("failed to delete message records: %w", err)
		}
		result.MessagesDeleted = len(idsToDelete)
	}

	return result, nil
}

func (m *MessageCleaner) deleteMediaFile(msg *models.Message) error {
	if msg.FileID == nil {
		return nil
	}

	var dir, ext string
	switch msg.ContentType {
	case models.ContentTypePhoto:
		dir, ext = m.photosDir, ".jpg"
	case models.ContentTypeVoice:
		dir, ext = m.voiceDir, ".ogg"
	default:
		return nil
	}

	path := filepath.Join(dir, *msg.FileID+ext)

	err := os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove file %s: %w", path, err)
	}

	return nil
}
