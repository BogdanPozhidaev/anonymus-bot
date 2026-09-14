# Переменные окружения

| Переменная | Описание | Пример |
|---|---|---|
| `POSTGRES_USER` | Имя пользователя Postgres | `app_user` |
| `POSTGRES_PASSWORD` | Пароль Postgres | — (секрет) |
| `POSTGRES_DB` | Имя базы данных | `mediator_bot` |
| `POSTGRES_PORT` | Порт Postgres | `5432` |
| `REDIS_PORT` | Порт Redis | `6379` |
| `BACKEND_PORT` | Порт Backend API | `8080` |
| `TELEGRAM_BOT_TOKEN` | Токен Telegram-бота от @BotFather | — (секрет) |
| `PII_ENCRYPTION_KEY` | Ключ AES-256 для шифрования PII, base64, 32 байта | — (секрет, генерируется через `openssl rand -base64 32`) |
| `RETENTION_RUN_HOUR` | Час запуска retention-джобы (0-23, UTC) | `03` |

**Важно:** `.env` никогда не коммитится в git. Используй `.env.example` как шаблон.