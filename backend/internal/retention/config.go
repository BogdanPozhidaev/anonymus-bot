package retention

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	// PIIRetentionPeriod — через сколько времени после закрытия сессии
	// анонимизируются PII участников (username, first_name, telegram_id).
	PIIRetentionPeriod time.Duration

	// MessageRetentionPeriod — через сколько времени после закрытия сессии
	// удаляются сообщения и медиафайлы.
	MessageRetentionPeriod time.Duration
}

const (
	defaultPIIRetentionMonths     = 3
	defaultMessageRetentionMonths = 12
	daysPerMonth                  = 30
)

func DefaultConfig() Config {
	return Config{
		PIIRetentionPeriod:     defaultPIIRetentionMonths * daysPerMonth * 24 * time.Hour,
		MessageRetentionPeriod: defaultMessageRetentionMonths * daysPerMonth * 24 * time.Hour,
	}
}

// LoadConfig читает периоды retention из переменных окружения
// RETENTION_PII_MONTHS и RETENTION_MESSAGE_MONTHS. Если переменная
// не задана или не парсится как положительное целое число месяцев,
// используется значение по умолчанию (см. DefaultConfig).
func LoadConfig() (Config, error) {
	cfg := DefaultConfig()

	if v := os.Getenv("RETENTION_PII_MONTHS"); v != "" {
		months, err := strconv.Atoi(v)
		if err != nil || months <= 0 {
			return Config{}, fmt.Errorf("invalid RETENTION_PII_MONTHS %q: must be a positive integer", v)
		}
		cfg.PIIRetentionPeriod = time.Duration(months) * daysPerMonth * 24 * time.Hour
	}

	if v := os.Getenv("RETENTION_MESSAGE_MONTHS"); v != "" {
		months, err := strconv.Atoi(v)
		if err != nil || months <= 0 {
			return Config{}, fmt.Errorf("invalid RETENTION_MESSAGE_MONTHS %q: must be a positive integer", v)
		}
		cfg.MessageRetentionPeriod = time.Duration(months) * daysPerMonth * 24 * time.Hour
	}

	return cfg, nil
}
