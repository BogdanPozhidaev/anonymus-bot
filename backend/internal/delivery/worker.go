package delivery

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/fastcheck/anonymus_bot/backend/internal/botclient"
	"github.com/fastcheck/anonymus_bot/backend/internal/repository"
)

const leaseDuration = 90 * time.Second
const maxAttempts = 5
const sendTimeout = 45 * time.Second
const completionTimeout = 10 * time.Second

type sender interface {
	SendMessage(context.Context, botclient.SendMessageRequest) error
}

type Worker struct {
	repo           *repository.MessageRepository
	bot            sender
	logger         *slog.Logger
	observeAttempt func(notice, success bool)
}

func New(repo *repository.MessageRepository, bot sender, logger *slog.Logger, observeAttempt func(notice, success bool)) *Worker {
	return &Worker{repo: repo, bot: bot, logger: logger, observeAttempt: observeAttempt}
}

func (w *Worker) Run(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	// A claimed item keeps its own send and completion deadlines during shutdown.
	attemptCtx := context.WithoutCancel(ctx)
	for ctx.Err() == nil {
		processed, err := w.processNext(ctx, attemptCtx)
		if err != nil && !errors.Is(err, ctx.Err()) {
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
	return w.processNext(ctx, ctx)
}

func (w *Worker) processNext(claimCtx, attemptCtx context.Context) (bool, error) {
	item, err := w.repo.ClaimDelivery(claimCtx, leaseDuration)
	if err != nil || item == nil {
		return false, err
	}
	req := botclient.SendMessageRequest{
		RecipientTelegramID: item.RecipientTelegramID, SessionID: item.SessionID, ContentType: item.ContentType,
		FileID: item.FileID, Text: item.Content, SenderLabel: "Клиент:", Language: item.Language,
	}
	if item.SenderRole == "executor" {
		req.SenderLabel = "Менеджер:"
	}
	if item.Language == "en" {
		req.SenderLabel = "Client:"
		if item.SenderRole == "executor" {
			req.SenderLabel = "Manager:"
		}
	}
	if item.Notice {
		req.SessionID = 0
		req.ContentType = "text"
		req.FileID = ""
		req.SenderLabel = ""
		req.Text = "Не удалось подтвердить доставку сообщения после нескольких попыток. Обратитесь к оператору."
		if item.Language == "en" {
			req.Text = "We could not confirm delivery after several attempts. Contact an operator."
		}
	}
	sendCtx, cancelSend := context.WithTimeout(attemptCtx, sendTimeout)
	sendErr := w.bot.SendMessage(sendCtx, req)
	cancelSend()
	if w.observeAttempt != nil {
		w.observeAttempt(item.Notice, sendErr == nil)
	}
	retry := time.Duration(5*(1<<item.Attempts)) * time.Second
	completeCtx, cancelCompletion := context.WithTimeout(attemptCtx, completionTimeout)
	defer cancelCompletion()
	return true, w.repo.CompleteDelivery(completeCtx, item, sendErr == nil, retry, maxAttempts)
}
