package handlers_test

import (
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/fastcheck/anonymus_bot/bot/internal/handlers"
)

func TestDetectBlockedType(t *testing.T) {
	cases := []struct {
		name     string
		message  *tgbotapi.Message
		expected string
	}{
		{"sticker", &tgbotapi.Message{Sticker: &tgbotapi.Sticker{}}, "sticker"},
		{"video", &tgbotapi.Message{Video: &tgbotapi.Video{}}, "video"},
		{"video_note", &tgbotapi.Message{VideoNote: &tgbotapi.VideoNote{}}, "video_note"},
		{"document", &tgbotapi.Message{Document: &tgbotapi.Document{}}, "document"},
		{"location", &tgbotapi.Message{Location: &tgbotapi.Location{}}, "location"},
		{"venue", &tgbotapi.Message{Venue: &tgbotapi.Venue{}}, "venue"},
		{"contact", &tgbotapi.Message{Contact: &tgbotapi.Contact{}}, "contact"},
		{"poll", &tgbotapi.Message{Poll: &tgbotapi.Poll{}}, "poll"},
		{"dice", &tgbotapi.Message{Dice: &tgbotapi.Dice{}}, "dice"},
		{"passport", &tgbotapi.Message{PassportData: &tgbotapi.PassportData{}}, "passport"},
		{"animation", &tgbotapi.Message{Animation: &tgbotapi.Animation{}}, "animation"},
		{"audio", &tgbotapi.Message{Audio: &tgbotapi.Audio{}}, "audio"},
		{"nothing_blocked", &tgbotapi.Message{Text: "hello"}, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := handlers.DetectBlockedType(tc.message)
			if result != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, result)
			}
		})
	}
}
