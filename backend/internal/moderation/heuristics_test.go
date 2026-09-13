package moderation_test

import (
	"testing"

	"github.com/fastcheck/anonymus_bot/backend/internal/moderation"
)

func TestContainsSuspiciousPhrase_Detected(t *testing.T) {
	cases := []string{
		"давайте созвонимся завтра",
		"Напиши мне в вотсап",
		"вот мой номер, звони",
		"скинь контакт пожалуйста",
		"давай напрямую договоримся",
		"ДАВАЙ НАПРЯМУЮ", // проверка регистронезависимости
	}

	for _, text := range cases {
		found, _ := moderation.ContainsSuspiciousPhrase(text)
		if !found {
			t.Errorf("expected suspicious phrase detection in %q", text)
		}
	}
}

func TestContainsSuspiciousPhrase_CleanText(t *testing.T) {
	cases := []string{
		"Здравствуйте, когда будет готов заказ?",
		"Спасибо за оперативность",
		"Хорошо, договорились на завтра",
		"Можете перезвонить позже?",
	}

	for _, text := range cases {
		found, phrase := moderation.ContainsSuspiciousPhrase(text)
		if found {
			t.Errorf("unexpected detection in clean text %q, matched phrase: %q", text, phrase)
		}
	}
}
