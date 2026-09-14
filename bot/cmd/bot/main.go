package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/fastcheck/anonymus_bot/bot/internal/backendclient"
	"github.com/fastcheck/anonymus_bot/bot/internal/config"
	"github.com/fastcheck/anonymus_bot/bot/internal/dispatcher"
	"github.com/fastcheck/anonymus_bot/bot/internal/handlers"
	"github.com/fastcheck/anonymus_bot/bot/internal/server"
)

const updateConcurrency = 10

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
	startHandler := handlers.NewStartHandler(bot, backend, cfg.PhotosDir, cfg.VoiceDir, logger)
	helpHandler := handlers.NewHelpHandler(bot, logger)
	languageHandler := handlers.NewLanguageHandler(bot, backend, logger)
	operatorHandler := handlers.NewOperatorHandler(bot, backend, logger)
	messageRelayHandler := handlers.NewMessageRelayHandler(bot, backend, logger)
	photoHandler := handlers.NewPhotoHandler(bot, backend, cfg.PhotosDir, logger)
	voiceHandler := handlers.NewVoiceHandler(bot, backend, cfg.VoiceDir, logger)
	blockedContentHandler := handlers.NewBlockedContentHandler(bot, logger)
	sessionsListHandler := handlers.NewSessionsListHandler(bot, backend, logger)
	stopHandler := handlers.NewStopHandler(bot, backend, logger)
	alertCallbackHandler := handlers.NewAlertCallbackHandler(bot, backend, cfg.PanelBaseURL, logger)

	handleUpdate := func(ctx context.Context, update tgbotapi.Update) {
		if update.CallbackQuery != nil {
			switch {
			case strings.HasPrefix(update.CallbackQuery.Data, "set_lang:"):
				languageHandler.HandleCallback(ctx, update.CallbackQuery)
			case strings.HasPrefix(update.CallbackQuery.Data, "switch_session:"):
				sessionsListHandler.HandleSwitchCallback(ctx, update.CallbackQuery)
			case strings.HasPrefix(update.CallbackQuery.Data, "open_session:"),
				strings.HasPrefix(update.CallbackQuery.Data, "take_request:"),
				strings.HasPrefix(update.CallbackQuery.Data, "close_session:"):
				alertCallbackHandler.Handle(ctx, update.CallbackQuery)
			case strings.HasPrefix(update.CallbackQuery.Data, "op_menu:"):
				operatorHandler.HandleMenuCallback(ctx, update.CallbackQuery)
			default:
				callbackConfig := tgbotapi.NewCallback(update.CallbackQuery.ID, "")
				if _, err := bot.Request(callbackConfig); err != nil {
					logger.Error("failed to answer callback", "error", err)
				}
			}
			return
		}

		internalServer := server.New(bot, cfg.OperatorGroupChatID, logger)
		go func() {
			if err := internalServer.Start(ctx, ":9090"); err != nil && err != http.ErrServerClosed {
				logger.Error("internal server failed", "error", err)
			}
		}()

		if update.Message == nil {
			return
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
			case "sessions":
				sessionsListHandler.HandleSessionsCommand(ctx, update.Message)
			case "current":
				sessionsListHandler.HandleCurrentCommand(ctx, update.Message)
			case "stop":
				stopHandler.Handle(ctx, update.Message)
			}
			return
		}

		if blockedType := handlers.DetectBlockedType(update.Message); blockedType != "" {
			blockedContentHandler.Handle(update.Message, blockedType)
			return
		}

		if update.Message.Photo != nil {
			photoHandler.Handle(ctx, update.Message)
			return
		}

		if update.Message.Voice != nil {
			voiceHandler.Handle(ctx, update.Message)
			return
		}

		if update.Message.Text != "" {
			messageRelayHandler.HandleText(ctx, update.Message)
			return
		}

		logger.Info("received unsupported message type", "chat_id", update.Message.Chat.ID)
	}

	updateConfig := tgbotapi.NewUpdate(0)
	updateConfig.Timeout = 30

	updates := bot.GetUpdatesChan(updateConfig)

	logger.Info("bot started, listening for updates", "concurrency", updateConcurrency)

	ctx := context.Background()
	d := dispatcher.New(handleUpdate, updateConcurrency, logger)
	d.Run(ctx, updates)
}
