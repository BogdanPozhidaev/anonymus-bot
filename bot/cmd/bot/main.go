package main

import (
	"log/slog"
	"os"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/fastcheck/anonymus_bot/bot/internal/config"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	cfg, err := config.Load()
	if err != nil {
		logger.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	bot, err := tgbotapi.NewBotAPI(cfg.TelegramBotToken)
	if err != nil {
		logger.Error("failed to create bot", "error", err)
		os.Exit(1)
	}

	logger.Info("bot authorized", "username", bot.Self.UserName)

	updateConfig := tgbotapi.NewUpdate(0)
	updateConfig.Timeout = 30

	updates := bot.GetUpdatesChan(updateConfig)

	logger.Info("bot started, listening for updates")

	for update := range updates {
		if update.Message == nil {
			continue
		}

		logger.Info("received message",
			"from_id", update.Message.From.ID,
			"text", update.Message.Text,
		)

		msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Получено: "+update.Message.Text)
		if _, err := bot.Send(msg); err != nil {
			logger.Error("failed to send message", "error", err)
		}
	}
}
