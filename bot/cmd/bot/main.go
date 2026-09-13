package main

import (
	"context"
	"log/slog"
	"os"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/fastcheck/anonymus_bot/bot/internal/backendclient"
	"github.com/fastcheck/anonymus_bot/bot/internal/config"
	"github.com/fastcheck/anonymus_bot/bot/internal/handlers"
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

	backend := backendclient.New(cfg.BackendURL)
	startHandler := handlers.NewStartHandler(bot, backend, cfg.PhotosDir, logger)
	helpHandler := handlers.NewHelpHandler(bot, logger)
	languageHandler := handlers.NewLanguageHandler(bot, backend, logger)
	operatorHandler := handlers.NewOperatorHandler(bot, backend, logger)
	messageRelayHandler := handlers.NewMessageRelayHandler(bot, backend, logger)
	photoHandler := handlers.NewPhotoHandler(bot, backend, cfg.PhotosDir, logger)

	updateConfig := tgbotapi.NewUpdate(0)
	updateConfig.Timeout = 30

	updates := bot.GetUpdatesChan(updateConfig)

	logger.Info("bot started, listening for updates")

	ctx := context.Background()

	for update := range updates {
		if update.CallbackQuery != nil {
			if strings.HasPrefix(update.CallbackQuery.Data, "set_lang:") {
				languageHandler.HandleCallback(ctx, update.CallbackQuery)
			} else {
				callbackConfig := tgbotapi.NewCallback(update.CallbackQuery.ID, "")
				if _, err := bot.Request(callbackConfig); err != nil {
					logger.Error("failed to answer callback", "error", err)
				}
			}
			continue
		}

		if update.Message == nil {
			continue
		}

		if update.Message.IsCommand() {
			switch update.Message.Command() {
			case "start":
				startHandler.Handle(ctx, update.Message)
			case "help":
				helpHandler.Handle(update.Message)
			case "language":
				languageHandler.HandleCommand(update.Message)
			case "operator":
				operatorHandler.Handle(ctx, update.Message)
			}
			continue
		}

		if update.Message.Photo != nil {
			photoHandler.Handle(ctx, update.Message)
			continue
		}

		if update.Message.Text != "" {
			messageRelayHandler.HandleText(ctx, update.Message)
			continue
		}

		logger.Info("received unsupported message type", "chat_id", update.Message.Chat.ID)
	}
}
