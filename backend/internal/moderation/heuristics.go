package moderation

import "strings"

// suspiciousPhrases — фразы, указывающие на попытку договориться об обходе
// платформы. Проверка регистронезависимая, по вхождению подстроки.
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
}

// ContainsSuspiciousPhrase проверяет текст на наличие фраз, типичных для
// попытки договориться об обходе платформы. Проверка регистронезависимая.
func ContainsSuspiciousPhrase(text string) (bool, string) {
	lowered := strings.ToLower(text)

	for _, phrase := range suspiciousPhrases {
		if strings.Contains(lowered, phrase) {
			return true, phrase
		}
	}

	return false, ""
}
