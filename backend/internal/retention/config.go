package retention

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	// PIIRetentionPeriod is the minimum time after session closure before anonymization.
	PIIRetentionPeriod time.Duration

	// MessageRetentionPeriod is the minimum time after closure before deleting messages and media.
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

// LoadConfig reads the retention periods from RETENTION_PII_MONTHS and
// RETENTION_MESSAGE_MONTHS. Unset values use the defaults and invalid values fail.
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
