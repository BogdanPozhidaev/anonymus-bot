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
	handler   HandlerFunc
	logger    *slog.Logger
	semaphore chan struct{}
	wg        sync.WaitGroup
}

func New(handler HandlerFunc, concurrency int, logger *slog.Logger) *Dispatcher {
	return &Dispatcher{
		handler:   handler,
		logger:    logger,
		semaphore: make(chan struct{}, concurrency),
	}
}

// Run читает апдейты из канала до его закрытия, распределяя обработку
// по пулу горутин. Блокирует вызывающего до тех пор, пока канал updates
// не будет закрыт и все запущенные обработчики не завершатся.
func (d *Dispatcher) Run(ctx context.Context, updates tgbotapi.UpdatesChannel) {
	var order sync.Mutex
	tails := make(map[int64]chan struct{})
	for update := range updates {
		update := update

		d.semaphore <- struct{}{}
		d.wg.Add(1)
		id := updateUserID(update)
		var previous <-chan struct{}
		done := make(chan struct{})
		if id != 0 {
			order.Lock()
			previous, tails[id] = tails[id], done
			order.Unlock()
		}

		go func() {
			defer func() {
				if id != 0 {
					order.Lock()
					if tails[id] == done {
						delete(tails, id)
					}
					close(done)
					order.Unlock()
				}
				<-d.semaphore
				d.wg.Done()

				if recover() != nil {
					d.logger.Error("recovered from panic in update handler")
				}
			}()

			if previous != nil {
				<-previous
			}
			d.handler(ctx, update)
		}()
	}

	d.wg.Wait()
}

func updateUserID(update tgbotapi.Update) int64 {
	if update.CallbackQuery != nil && update.CallbackQuery.From != nil {
		return update.CallbackQuery.From.ID
	}
	if update.Message != nil && update.Message.From != nil {
		return update.Message.From.ID
	}
	return 0
}
