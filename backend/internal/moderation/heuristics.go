package moderation

import (
	"strings"
	"unicode"
)

// suspiciousPhrases contains substrings associated with bypassing the platform.
var suspiciousPhrases = []string{
	"созвонимся",
	"созвонимся в",
	"напиши мне в",
	"напишите мне в",
	"напиши в вотсап",
	"напиши в телеграм",
	"вот мой номер",
	"мой номер телефона",
	"давай напрямую",
	"давайте напрямую",
	"скинь контакт",
	"скиньте контакт",
	"скинь номер",
	"скиньте номер",
	"переходи в личку",
	"пиши в личку",
	"добавь меня в",
	"давай в обход",
	"без посредника",
	"без посредников",
	"contact me directly",
	"text me on whatsapp",
	"message me on telegram",
	"send me your number",
	"here is my phone number",
	"let's talk off platform",
}

// ContainsSuspiciousPhrase checks for suspicious substrings without regard to case.
func ContainsSuspiciousPhrase(text string) (bool, string) {
	words := strings.FieldsFunc(strings.ToLower(normalizeWidth(text)), func(r rune) bool {
		return r != '\'' && !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})
	lowered := " " + strings.Join(words, " ") + " "

	for _, phrase := range suspiciousPhrases {
		if strings.Contains(lowered, " "+phrase+" ") {
			return true, phrase
		}
	}

	return false, ""
}
