package dispatcher

import (
	"context"
	"log/slog"
	"sync"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// HandlerFunc обрабатывает один апдейт. Реализация должна быть безопасна
// для конкурентного вызова из разных горутин одновременно.
type HandlerFunc func(ctx context.Context, update tgbotapi.Update)

// Dispatcher распределяет входящие апдейты по ограниченному пулу горутин.
type Dispatcher struct {
	handler     HandlerFunc
	logger      *slog.Logger
	concurrency int
	semaphore   chan struct{}
	wg          sync.WaitGroup
}

func New(handler HandlerFunc, concurrency int, logger *slog.Logger) *Dispatcher {
	return &Dispatcher{
		handler:     handler,
		logger:      logger,
		concurrency: concurrency,
		semaphore:   make(chan struct{}, concurrency),
	}
}

// Run читает апдейты из канала до его закрытия, распределяя обработку
// по пулу горутин. Блокирует вызывающего до тех пор, пока канал updates
// не будет закрыт и все запущенные обработчики не завершатся.
func (d *Dispatcher) Run(ctx context.Context, updates tgbotapi.UpdatesChannel) {
	for update := range updates {
		update := update

		d.semaphore <- struct{}{}
		d.wg.Add(1)

		go func() {
			defer func() {
				<-d.semaphore
				d.wg.Done()

				if r := recover(); r != nil {
					d.logger.Error("recovered from panic in update handler", "panic", r)
				}
			}()

			d.handler(ctx, update)
		}()
	}

	d.wg.Wait()
}
