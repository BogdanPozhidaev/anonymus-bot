package handlers

import (
	"strconv"

	"github.com/fastcheck/anonymus_bot/bot/internal/backendclient"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func parsePageOffset(value string) (int, bool) {
	offset, err := strconv.Atoi(value)
	return offset, err == nil && offset >= 0 && offset <= 1000000 && offset%backendclient.MenuPageSize == 0
}

func pageButtons(prefix string, offset int, hasMore bool) []tgbotapi.InlineKeyboardButton {
	const size = backendclient.MenuPageSize
	var buttons []tgbotapi.InlineKeyboardButton
	if offset >= size {
		buttons = append(buttons, tgbotapi.NewInlineKeyboardButtonData("Назад", prefix+strconv.Itoa(offset-size)))
	}
	if hasMore && offset <= 1000000-size {
		buttons = append(buttons, tgbotapi.NewInlineKeyboardButtonData("Вперёд", prefix+strconv.Itoa(offset+size)))
	}
	return buttons
}

func menuExcerpt(value string) string {
	const maxRunes = 80
	runes := []rune(value)
	if len(runes) > maxRunes {
		return string(runes[:maxRunes]) + "…"
	}
	return value
}
