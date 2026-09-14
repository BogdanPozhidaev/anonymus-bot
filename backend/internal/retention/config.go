package retention

import "time"

type Config struct {
	// PIIRetentionPeriod — через сколько времени после закрытия сессии
	// анонимизируются PII участников (username, first_name, telegram_id).
	PIIRetentionPeriod time.Duration

	// MessageRetentionPeriod — через сколько времени после закрытия сессии
	// удаляются сообщения и медиафайлы.
	MessageRetentionPeriod time.Duration
}

func DefaultConfig() Config {
	return Config{
		PIIRetentionPeriod:     3 * 30 * 24 * time.Hour,  // M = 3 месяца (приближённо)
		MessageRetentionPeriod: 12 * 30 * 24 * time.Hour, // N = 12 месяцев (приближённо)
	}
}
