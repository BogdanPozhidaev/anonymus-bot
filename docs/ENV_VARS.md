# Переменные окружения

| Переменная | Описание | Пример |
|---|---|---|
| `POSTGRES_USER` | Имя пользователя Postgres | `app_user` |
| `POSTGRES_PASSWORD` | Пароль Postgres | — (секрет) |
| `POSTGRES_DB` | Имя базы данных | `mediator_bot` |
| `POSTGRES_PORT` | Порт Postgres | `5432` |
| `REDIS_PORT` | Порт Redis | `6379` |
| `REDIS_PASSWORD` | Секрет Compose Redis, от 32 символов из букв ASCII, цифр, `_` и `-` | Обязателен для Compose |
| `BACKEND_PORT` | Порт Backend API при запуске вне Compose. Compose использует внутренний 8080 | `8080` |
| `ENABLE_SWAGGER` | Включить Swagger на бэкенде | `false` |
| `AUTH_PROXY_SUBNET` | Подсеть Compose для связи Nginx и бэкенда | `172.31.247.0/24` |
| `AUTH_PROXY_IP_RANGE` | Диапазон динамических адресов в подсети прокси | `172.31.247.128/25` |
| `AUTH_PROXY_IP` | Фиксированный адрес Nginx в этой подсети | `172.31.247.2` |
| `INTERNAL_API_TOKEN_FILE` | Абсолютный путь к файлу сервисного токена вне репозитория | Обязателен для Compose |
| `BOT_INTERNAL_TOKEN_FILE` | Абсолютный путь к отдельному токену для вызовов бэкенд → бот | Обязателен для Compose |
| `TELEGRAM_BOT_TOKEN` | Токен Telegram-бота от @BotFather | — (секрет) |
| `OPERATOR_GROUP_CHAT_ID` | Ненулевой числовой ID группы операторов | Обязателен при запуске бота |
| `PANEL_BASE_URL` | Публичный HTTP(S) origin панели для ссылок бота | `http://localhost` |
| `PANEL_HTTP_BIND` | Адрес публикации HTTP в базовом Compose | `0.0.0.0` |
| `PANEL_HTTP_PORT` | Порт публикации HTTP в базовом Compose | `80` |
| `PANEL_HTTPS_PORT` | Порт публикации HTTPS с TLS override | `443` |
| `TLS_CERT_DIR` | Каталог сертификата и ключа, обязателен для TLS override | См. [DEPLOYMENT.md](DEPLOYMENT.md) |
| `PII_ENCRYPTION_KEY` | Ключ AES-256 для шифрования PII, base64, 32 байта | — (секрет, генерируется через `openssl rand -base64 32`) |
| `RETENTION_RUN_HOUR` | Час запуска retention-джобы (0-23, UTC) | `03` |
| `RETENTION_PII_MONTHS` | Через сколько месяцев после закрытия сессии анонимизируется PII | `3` |
| `RETENTION_MESSAGE_MONTHS` | Через сколько месяцев после закрытия сессии удаляются сообщения/медиа | `12` |

**Важно:** `.env` никогда не коммитится в git. Используй `.env.example` как шаблон.

On existing local Compose installations, add the Redis password before
recreating Redis and backend together. Expect operators to sign in again
if Redis loses its in-memory sessions. Preserve the existing PostgreSQL
credentials and PII encryption key when reusing volumes. For standalone
launches, set `DATABASE_URL` explicitly and supply Redis credentials in
`REDIS_URL` or via `REDIS_PASSWORD`. Use `docker compose config --quiet`
for validation so interpolated secrets are not printed.

### Internal API token

Generate two independent tokens, one per direction. For each token generate
32 cryptographically random bytes and encode them as 64 hexadecimal
characters. Write each value directly to a separate file outside the repository with
access limited to the deployment account and the Docker service that reads it.
For example, on a POSIX host, `(umask 077; openssl rand -hex 32 > /absolute/path/internal-api-token)`.
On Windows, restrict access with file ACLs. The environment file contains only
the absolute path. Do not print the token or enable HTTP header dumps.

Docker Compose mounts `INTERNAL_API_TOKEN_FILE` at `/run/secrets/internal_api_token`
for bot → backend calls and `BOT_INTERNAL_TOKEN_FILE` at
`/run/secrets/bot_internal_token` for backend → bot calls. Both processes
read both files at startup. On a standalone launch, set both file paths
for each process. Keep HTTP traffic on the trusted local Docker network.

For rotation, prepare the new file with the same permissions and recreate
backend and bot together so both reload it. For example, run
`docker compose up -d --no-deps --force-recreate backend bot` from `infra`
with the deployment environment loaded. A short interruption is expected.

### Authentication client IP

Compose gives Nginx a fixed address and passes that address to the backend
as `TRUSTED_PROXY_IP`. Nginx overwrites `X-Real-IP` on auth requests.
If the default subnet overlaps another network, set `AUTH_PROXY_SUBNET`
and adjust `AUTH_PROXY_IP_RANGE` and `AUTH_PROXY_IP` to fit it. Keep the
fixed address outside the dynamic range. For standalone deployments leave
`TRUSTED_PROXY_IP` unset for direct access, or set it to the single address
of a reverse proxy that overwrites `X-Real-IP`.

### Moderation and panel branding

- `MODERATION_DISABLED_RULES` is empty by default. Set a comma-separated
  list of rules to exclude only when the deployment owner has chosen
  that policy. See [Moderation](MODERATION.md).
- `MODERATION_AUTO_PAUSE` defaults to `false`. The backend accepts
  `true` for the optional pause policy.
- `PANEL_BOT_USERNAME` is the bot username without the leading `@`.
  Set it to enable invitation links. It is exposed as a public
  identification value in the panel configuration.
- `PANEL_PRODUCT_NAME` sets the panel title at container startup.
  For Vite development, use `VITE_BOT_USERNAME` and `VITE_PRODUCT_NAME`.
