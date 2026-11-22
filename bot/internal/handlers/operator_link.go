package handlers

import (
	"context"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func (h *OperatorHandler) HandleLink(ctx context.Context, message *tgbotapi.Message) {
	if message.Chat == nil || message.From == nil || !message.Chat.IsPrivate() || message.Chat.ID != message.From.ID {
		return
	}
	code := strings.TrimSpace(message.CommandArguments())
	if code == "" || len(code) > 64 {
		h.sendMessage(message.Chat.ID, tr(message.From, "Получите код в панели и отправьте его командой /link.", "Get a code in the panel and send it with /link."))
		return
	}
	confirmation, err := h.backend.ClaimTelegramLink(ctx, message.From.ID, code)
	if err != nil || confirmation == "" {
		h.sendMessage(message.Chat.ID, tr(message.From, "Код недоступен. Выпустите новый код в панели.", "The code is unavailable. Generate a new code in the panel."))
		return
	}
	h.sendMessage(message.Chat.ID, tr(message.From, "Введите в панели код подтверждения:\n", "Enter this confirmation code in the panel:\n")+confirmation)
}
