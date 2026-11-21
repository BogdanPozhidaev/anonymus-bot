package moderation

import (
	"regexp"
	"strings"
	"unicode"
)

type patternEntry struct {
	name    string
	pattern *regexp.Regexp
}

// patterns упорядочены от наиболее специфичной причины к наименее специфичной.
// Порядок важен: если текст совпадает с несколькими паттернами одновременно
// (например, email содержит и "@", похожий на username, и цифры в домене),
// возвращается самая информативная причина, а не первая случайно найденная.
var patterns = []patternEntry{
	{
		name:    "email",
		pattern: regexp.MustCompile(`[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}`),
	},
	{
		name:    "messenger_link",
		pattern: regexp.MustCompile(`(?i)(t\.me|telegram\.me|wa\.me|whatsapp\.com|viber\.com|signal\.(?:me|org))/\S+`),
	},
	{
		name:    "username",
		pattern: regexp.MustCompile(`(?:^|[^a-zA-Z0-9_.%+-])@[a-zA-Z0-9_]{5,32}(?:$|[^a-zA-Z0-9_])`),
	},
	{
		name:    "phone",
		pattern: regexp.MustCompile(`(?:^|[^[:alnum:]_])(?:\+[1-9](?:[ \t().-]{0,3}\d){7,14}|[78]\d{10}|[78][ -]?(?:\(\d{3}\)|\d{3})[ -]?\d{3}[ -]\d{2}[ -]\d{2}|\(\d{3}\)[ ]?\d{3}-\d{4})(?:$|[^[:alnum:]_])`),
	},
}

var (
	dot = regexp.MustCompile(`(?i)[ \t]*(?:\[\s*dot\s*\]|\(\s*dot\s*\)|\.)[ \t]*`)
	at  = regexp.MustCompile(`(?i)[ \t]*(?:\[\s*at\s*\]|\(\s*at\s*\)|@)[ \t]*`)
)

func normalizeWidth(text string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case unicode.Is(unicode.Cf, r):
			return -1
		case r >= '！' && r <= '～':
			return r - '！' + '!'
		case unicode.IsSpace(r) && r != '\n':
			return ' '
		default:
			return r
		}
	}, text)
}

// ContainsContactInfo проверяет текст на наличие любого признака утечки контактов.
// Проверяются ВСЕ паттерны; если совпало несколько, возвращается наиболее
// специфичная причина согласно порядку в patterns (email важнее username и т.д.).
func ContainsContactInfo(text string) (bool, string) {
	return matchContact(text, nil)
}

func matchContact(text string, disabled map[string]bool) (bool, string) {
	text = dot.ReplaceAllString(normalizeWidth(text), ".")
	// Preserve the word boundary before a handle while allowing obfuscated email.
	emailText := at.ReplaceAllString(text, "@")
	for _, p := range patterns {
		candidate := text
		if p.name == "email" {
			candidate = emailText
		}
		if !disabled[p.name] && p.pattern.MatchString(candidate) {
			return true, p.name
		}
	}

	return false, ""
}
