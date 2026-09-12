package handlers

import (
	"log/slog"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type HelpHandler struct {
	bot    *tgbotapi.BotAPI
	logger *slog.Logger
}

func NewHelpHandler(bot *tgbotapi.BotAPI, logger *slog.Logger) *HelpHandler {
	return &HelpHandler{bot: bot, logger: logger}
}

const helpTextRU = `Доступные команды:
/start — начать работу
/help — это сообщение
/sessions — список ваших активных сессий
/current — текущая активная сессия
/language — сменить язык

Если у вас возник вопрос — просто напишите его текстом, и мы передадим его менеджеру.`

const helpTextEN = `Available commands:
/start — begin
/help — this message
/sessions — list of your active sessions
/current — current active session
/language — change language

If you have a question — just type it, and we'll pass it to the manager.`

func (h *HelpHandler) Handle(message *tgbotapi.Message) {
	text := helpTextRU
	if message.From.LanguageCode == "en" {
		text = helpTextEN
	}

	msg := tgbotapi.NewMessage(message.Chat.ID, text)
	if _, err := h.bot.Send(msg); err != nil {
		h.logger.Error("failed to send help message", "error", err, "chat_id", message.Chat.ID)
	}
}
