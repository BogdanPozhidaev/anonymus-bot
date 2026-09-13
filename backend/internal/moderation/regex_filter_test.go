package moderation_test

import (
	"testing"

	"github.com/fastcheck/anonymus_bot/backend/internal/moderation"
)

func TestContainsContactInfo_Phone(t *testing.T) {
	cases := []string{
		"+7 999 123 45 67",
		"89991234567",
		"+7(999)123-45-67",
		"8-999-123-45-67",
	}

	for _, text := range cases {
		found, _ := moderation.ContainsContactInfo(text)
		if !found {
			t.Errorf("expected phone detection in %q", text)
		}
	}
}

func TestContainsContactInfo_Username(t *testing.T) {
	found, reason := moderation.ContainsContactInfo("напиши мне @ivan_petrov")
	if !found {
		t.Error("expected username detection")
	}
	if reason != "username" {
		t.Errorf("expected reason 'username', got %q", reason)
	}
}

func TestContainsContactInfo_MessengerLink(t *testing.T) {
	cases := []string{
		"t.me/ivan_petrov",
		"https://wa.me/79991234567",
		"whatsapp.com/send?phone=79991234567",
	}

	for _, text := range cases {
		found, _ := moderation.ContainsContactInfo(text)
		if !found {
			t.Errorf("expected messenger link detection in %q", text)
		}
	}
}

func TestContainsContactInfo_Email(t *testing.T) {
	found, reason := moderation.ContainsContactInfo("пишите на ivan@example.com")
	if !found {
		t.Error("expected email detection")
	}
	if reason != "email" {
		t.Errorf("expected reason 'email', got %q", reason)
	}
}

func TestContainsContactInfo_CleanText(t *testing.T) {
	cases := []string{
		"Здравствуйте, когда будет готов заказ?",
		"Спасибо большое за помощь!",
		"Хорошо, договорились",
		"Стоимость услуги 5000 рублей", // короткое число, не должно попасть под 6-12 цифр
	}

	for _, text := range cases {
		found, reason := moderation.ContainsContactInfo(text)
		if found {
			t.Errorf("unexpected detection in clean text %q, reason: %s", text, reason)
		}
	}
}

func TestContainsContactInfo_LongDigits(t *testing.T) {
	found, reason := moderation.ContainsContactInfo("код заказа 123456789")
	if !found {
		t.Error("expected detection of suspicious digit sequence")
	}
	// Девятизначная последовательность цифр допустимо распознаётся
	// либо как потенциальный телефон, либо как длинная цифровая
	// последовательность — оба варианта корректны с точки зрения
	// цели теста: убедиться, что подозрительные цифры блокируются.
	if reason != "phone" && reason != "long_digits" {
		t.Errorf("expected reason 'phone' or 'long_digits', got %q", reason)
	}
}
