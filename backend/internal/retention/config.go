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
	maxRetentionMonths            = (1<<63 - 1) / int64(daysPerMonth*24*time.Hour)
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
		if err != nil || months <= 0 || int64(months) > maxRetentionMonths {
			return Config{}, fmt.Errorf("RETENTION_PII_MONTHS must be an integer between 1 and %d", maxRetentionMonths)
		}
		cfg.PIIRetentionPeriod = time.Duration(months) * daysPerMonth * 24 * time.Hour
	}

	if v := os.Getenv("RETENTION_MESSAGE_MONTHS"); v != "" {
		months, err := strconv.Atoi(v)
		if err != nil || months <= 0 || int64(months) > maxRetentionMonths {
			return Config{}, fmt.Errorf("RETENTION_MESSAGE_MONTHS must be an integer between 1 and %d", maxRetentionMonths)
		}
		cfg.MessageRetentionPeriod = time.Duration(months) * daysPerMonth * 24 * time.Hour
	}

	return cfg, nil
}
