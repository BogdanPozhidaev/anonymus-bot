package delivery

import (
	"context"
	"log/slog"
	"time"

	"github.com/fastcheck/anonymus_bot/backend/internal/botclient"
	"github.com/fastcheck/anonymus_bot/backend/internal/repository"
)

const leaseDuration = 90 * time.Second
const maxAttempts = 5

type sender interface {
	SendMessage(context.Context, botclient.SendMessageRequest) error
}

type Worker struct {
	repo   *repository.MessageRepository
	bot    sender
	logger *slog.Logger
}

func New(repo *repository.MessageRepository, bot sender, logger *slog.Logger) *Worker {
	return &Worker{repo: repo, bot: bot, logger: logger}
}

func (w *Worker) Run(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for ctx.Err() == nil {
		processed, err := w.ProcessNext(ctx)
		if err != nil {
			w.logger.Error("delivery outbox operation failed")
		}
		if processed && err == nil {
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (w *Worker) ProcessNext(ctx context.Context) (bool, error) {
	item, err := w.repo.ClaimDelivery(ctx, leaseDuration)
	if err != nil || item == nil {
		return false, err
	}
	req := botclient.SendMessageRequest{
		RecipientTelegramID: item.RecipientTelegramID, SessionID: item.SessionID, ContentType: item.ContentType,
		FileID: item.FileID, Text: item.Content, SenderLabel: "Клиент:",
	}
	if item.SenderRole == "executor" {
		req.SenderLabel = "Менеджер:"
	}
	if item.Notice {
		req.SessionID = 0
		req.ContentType = "text"
		req.FileID = ""
		req.SenderLabel = ""
		req.Text = "Не удалось подтвердить доставку сообщения после нескольких попыток. Обратитесь к оператору."
	}
	sendCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	sendErr := w.bot.SendMessage(sendCtx, req)
	cancel()
	retry := time.Duration(5*(1<<item.Attempts)) * time.Second
	return true, w.repo.CompleteDelivery(ctx, item, sendErr == nil, retry, maxAttempts)
}
