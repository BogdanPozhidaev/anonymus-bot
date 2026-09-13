package moderation

import "regexp"

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
		pattern: regexp.MustCompile(`@[a-zA-Z0-9_]{5,32}`),
	},
	{
		name:    "phone",
		pattern: regexp.MustCompile(`(?:\+?\d{1,3}[\s\-]?)?\(?\d{3,4}\)?[\s\-]?\d{2,3}[\s\-]?\d{2}[\s\-]?\d{2,4}`),
	},
	{
		name:    "long_digits",
		pattern: regexp.MustCompile(`\d{6,12}`),
	},
}

// ContainsContactInfo проверяет текст на наличие любого признака утечки контактов.
// Проверяются ВСЕ паттерны; если совпало несколько, возвращается наиболее
// специфичная причина согласно порядку в patterns (email важнее username и т.д.).
func ContainsContactInfo(text string) (bool, string) {
	for _, p := range patterns {
		if p.pattern.MatchString(text) {
			return true, p.name
		}
	}

	return false, ""
}
