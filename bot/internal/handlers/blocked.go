package handlers

import (
	"log/slog"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type BlockedContentHandler struct {
	bot    *tgbotapi.BotAPI
	logger *slog.Logger
}

func NewBlockedContentHandler(bot *tgbotapi.BotAPI, logger *slog.Logger) *BlockedContentHandler {
	return &BlockedContentHandler{bot: bot, logger: logger}
}

const blockedContentText = "Этот тип сообщений временно не поддерживается. Отправьте текстом, голосовым или фото."

// DetectBlockedType проверяет апдейт на неподдерживаемые типы контента
// и возвращает название типа для логирования, либо пустую строку, если
// сообщение не подпадает ни под одну из известных блокируемых категорий.
func DetectBlockedType(message *tgbotapi.Message) string {
	switch {
	case message.Sticker != nil:
		return "sticker"
	case message.Video != nil:
		return "video"
	case message.VideoNote != nil:
		return "video_note"
	case message.Document != nil:
		return "document"
	case message.Location != nil:
		return "location"
	case message.Venue != nil:
		return "venue"
	case message.Contact != nil:
		return "contact"
	case message.Poll != nil:
		return "poll"
	case message.Dice != nil:
		return "dice"
	case message.Game != nil:
		return "game"
	case message.PassportData != nil:
		return "passport"
	case message.Animation != nil:
		return "animation"
	case message.Audio != nil:
		return "audio"
	default:
		return ""
	}
}

func (h *BlockedContentHandler) Handle(message *tgbotapi.Message, blockedType string) {
	h.logger.Info("blocked unsupported content type", "content_type", blockedType, "chat_id", message.Chat.ID)

	msg := tgbotapi.NewMessage(message.Chat.ID, blockedContentText)
	if _, err := h.bot.Send(msg); err != nil {
		h.logger.Error("failed to send blocked content notice", "error", err, "chat_id", message.Chat.ID)
	}
}
