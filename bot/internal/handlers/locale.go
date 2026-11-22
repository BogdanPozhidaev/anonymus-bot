package handlers

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/fastcheck/anonymus_bot/bot/internal/backendclient"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func normalizeLanguage(value string) string {
	tag, _, _ := strings.Cut(strings.ToLower(value), "-")
	if tag == "en" || tag == "ru" {
		return tag
	}
	return ""
}

func languageOf(user *tgbotapi.User) string {
	if user != nil && normalizeLanguage(user.LanguageCode) == "en" {
		return "en"
	}
	return "ru"
}

func tr(user *tgbotapi.User, ru, en string) string {
	if languageOf(user) == "en" {
		return en
	}
	return ru
}

func ResolveLanguage(ctx context.Context, backend *backendclient.Client, telegramID int64, fallback string) string {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	user, err := backend.GetUserByTelegramID(ctx, telegramID)
	if err == nil {
		if code := normalizeLanguage(user.LanguageCode); code != "" {
			return code
		}
	} else if errors.Is(err, backendclient.ErrNotFound) {
		operator, err := backend.GetOperatorByTelegramID(ctx, telegramID)
		if err == nil && isActiveOperator(operator) {
			if code := normalizeLanguage(operator.Language); code != "" {
				return code
			}
		}
	}
	return languageOf(&tgbotapi.User{LanguageCode: fallback})
}

// LocalizeUpdate copies the sender before replacing Telegram's language hint.
func LocalizeUpdate(ctx context.Context, backend *backendclient.Client, update tgbotapi.Update) tgbotapi.Update {
	if c := update.CallbackQuery; c != nil && c.Message != nil && c.Message.Chat != nil && c.Message.Chat.IsPrivate() && c.From != nil {
		callback, sender := *c, *c.From
		sender.LanguageCode = ResolveLanguage(ctx, backend, sender.ID, sender.LanguageCode)
		callback.From = &sender
		update.CallbackQuery = &callback
	} else if m := update.Message; m != nil && m.Chat != nil && m.Chat.IsPrivate() && m.From != nil {
		message, sender := *m, *m.From
		sender.LanguageCode = ResolveLanguage(ctx, backend, sender.ID, sender.LanguageCode)
		message.From = &sender
		update.Message = &message
	}
	return update
}
