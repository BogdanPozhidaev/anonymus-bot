package retention

import (
	"context"
	"fmt"
	"log/slog"
	"os"
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

	for _, msg := range messages {
		if dryRun {
			hasFile, err := m.deleteMediaFile(msg, true)
			if err != nil {
				result.Errors = append(result.Errors, fmt.Errorf("message %d: %w", msg.ID, err))
				continue
			}
			m.logger.Info("[DRY RUN] would delete message", "message_id", msg.ID, "content_type", msg.ContentType)
			result.MessagesDeleted++
			if hasFile {
				result.FilesDeleted++
			}
			continue
		}

		removedFile := false
		deleted, err := m.messageRepo.DeleteExpired(ctx, msg, threshold, func() error {
			var err error
			removedFile, err = m.deleteMediaFile(msg, false)
			return err
		})
		if removedFile {
			result.FilesDeleted++
		}
		if err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("message %d: %w", msg.ID, err))
		}
		if deleted {
			result.MessagesDeleted++
		}
	}

	return result, nil
}

func (m *MessageCleaner) deleteMediaFile(msg *models.Message, dryRun bool) (bool, error) {
	if msg.FileID == nil {
		return false, nil
	}

	var dir, ext string
	switch msg.ContentType {
	case models.ContentTypePhoto:
		dir, ext = m.photosDir, ".jpg"
	case models.ContentTypeVoice:
		dir, ext = m.voiceDir, ".ogg"
	default:
		return false, nil
	}

	if !models.ValidMediaFileID(*msg.FileID) || dir == "" {
		return false, fmt.Errorf("invalid media file reference")
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return false, err
	}
	defer func() { _ = root.Close() }()
	path := *msg.FileID + ext
	info, err := root.Lstat(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !info.Mode().IsRegular() {
		return false, fmt.Errorf("media path is not a regular file")
	}
	if dryRun {
		return true, nil
	}

	err = root.Remove(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	return err == nil, err
}
