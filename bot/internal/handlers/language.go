package handlers

import (
	"context"
	"errors"
	"log/slog"
	"strings"

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
	if message.Chat == nil || !message.Chat.IsPrivate() {
		return
	}
	msg := tgbotapi.NewMessage(message.Chat.ID, "Выберите язык / Choose language:")

	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Русский", languageCallbackPrefix+"ru"),
			tgbotapi.NewInlineKeyboardButtonData("English", languageCallbackPrefix+"en"),
		),
	)
	msg.ReplyMarkup = keyboard

	if _, err := h.bot.Send(msg); err != nil {
		h.logger.Error("failed to send language selection")
	}
}

func (h *LanguageHandler) HandleCallback(ctx context.Context, callback *tgbotapi.CallbackQuery) {
	if callback.Message == nil || callback.Message.Chat == nil || !callback.Message.Chat.IsPrivate() || callback.From == nil {
		return
	}
	langCode, ok := strings.CutPrefix(callback.Data, languageCallbackPrefix)

	if !ok || (langCode != "ru" && langCode != "en") {
		return
	}

	if err := h.backend.UpdateUserLanguage(ctx, callback.From.ID, langCode); err != nil {
		text := tr(callback.From, "Не удалось сохранить язык. Попробуйте позже.", "Unable to save the language. Try again later.")
		if errors.Is(err, backendclient.ErrNotFound) {
			text = tr(callback.From, "Сначала подключитесь к сессии или привяжите аккаунт оператора.",
				"Join a session or link your operator account first.")
		} else {
			h.logger.Error("failed to update user language")
		}
		alert := tgbotapi.NewCallbackWithAlert(callback.ID, text)
		if _, err := h.bot.Request(alert); err != nil {
			h.logger.Error("failed to answer callback")
		}
		return
	}

	callbackConfig := tgbotapi.NewCallback(callback.ID, "")
	if _, err := h.bot.Request(callbackConfig); err != nil {
		h.logger.Error("failed to answer callback")
	}

	confirmText := "Язык изменён на русский"
	if langCode == "en" {
		confirmText = "Language changed to English"
	}

	edit := tgbotapi.NewEditMessageText(callback.Message.Chat.ID, callback.Message.MessageID, confirmText)
	if _, err := h.bot.Send(edit); err != nil {
		h.logger.Error("failed to edit message")
	}
}
