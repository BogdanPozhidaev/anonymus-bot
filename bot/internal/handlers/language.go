package handlers

import (
	"context"
	"log/slog"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/fastcheck/anonymus_bot/bot/internal/backendclient"
)

type LanguageHandler struct {
	bot     *tgbotapi.BotAPI
	backend *backendclient.Client
	logger  *slog.Logger
}

func NewLanguageHandler(bot *tgbotapi.BotAPI, backend *backendclient.Client, logger *slog.Logger) *LanguageHandler {
	return &LanguageHandler{bot: bot, backend: backend, logger: logger}
}

const (
	languageCallbackPrefix = "set_lang:"
)

func (h *LanguageHandler) HandleCommand(message *tgbotapi.Message) {
	msg := tgbotapi.NewMessage(message.Chat.ID, "Выберите язык / Choose language:")

	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Русский", languageCallbackPrefix+"ru"),
			tgbotapi.NewInlineKeyboardButtonData("English", languageCallbackPrefix+"en"),
		),
	)
	msg.ReplyMarkup = keyboard

	if _, err := h.bot.Send(msg); err != nil {
		h.logger.Error("failed to send language selection", "error", err, "chat_id", message.Chat.ID)
	}
}

func (h *LanguageHandler) HandleCallback(ctx context.Context, callback *tgbotapi.CallbackQuery) {
	langCode := callback.Data[len(languageCallbackPrefix):]

	if langCode != "ru" && langCode != "en" {
		h.logger.Warn("unknown language code in callback", "lang_code", langCode)
		return
	}

	if err := h.backend.UpdateUserLanguage(ctx, callback.From.ID, langCode); err != nil {
		h.logger.Error("failed to update user language", "error", err, "telegram_id", callback.From.ID)
	}

	callbackConfig := tgbotapi.NewCallback(callback.ID, "")
	if _, err := h.bot.Request(callbackConfig); err != nil {
		h.logger.Error("failed to answer callback", "error", err)
	}

	confirmText := "Язык изменён на русский"
	if langCode == "en" {
		confirmText = "Language changed to English"
	}

	edit := tgbotapi.NewEditMessageText(callback.Message.Chat.ID, callback.Message.MessageID, confirmText)
	if _, err := h.bot.Send(edit); err != nil {
		h.logger.Error("failed to edit message", "error", err)
	}
}
