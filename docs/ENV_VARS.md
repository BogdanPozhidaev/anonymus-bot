# Переменные окружения

| Переменная | Описание | Пример |
|---|---|---|
| `POSTGRES_USER` | Владелец БД в Compose для миграций, резервного копирования и первичной настройки администратора | `app_user` |
| `POSTGRES_PASSWORD` | Пароль владельца БД | — (секрет) |
| `POSTGRES_RUNTIME_PASSWORD` | Отдельный пароль роли `anonymus_runtime` для backend и retention. Не менее 32 символов из букв ASCII, цифр, `_` и `-`, отличается от пароля владельца | — (секрет, обязателен для Compose) |
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
| `PANEL_HTTPS_BIND` | Адрес публикации HTTPS с TLS override | `0.0.0.0` |
| `TLS_CERT_DIR` | Каталог сертификата и ключа, обязателен для TLS override | См. [DEPLOYMENT.md](DEPLOYMENT.md) |
| `PII_ENCRYPTION_KEY` | Ключ AES-256 версии 1, base64, 32 байта. Нужен для чтения ранее сохранённых данных без версии | Секрет, обязателен при активной версии 1 |
| `PII_ENCRYPTION_ACTIVE_VERSION` | Версия ключа для новых записей PII и TOTP. Из этого ключа также выводится отдельный ключ HMAC для retention | `1` |
| `PII_ENCRYPTION_KEYS` | Дополнительные ключи через запятую в формате `версия:base64`. Версии от 2, по 32 случайных байта на ключ | Пусто. Порядок применения описан в [Operations](OPERATIONS.md#encryption-key-rotation) |
| `RETENTION_RUN_HOUR` | Час запуска retention-джобы (две цифры от `00` до `23`, UTC) | `03` |
| `RETENTION_PII_MONTHS` | Через сколько месяцев после закрытия сессии анонимизируется PII | `3` |
| `RETENTION_MESSAGE_MONTHS` | Через сколько месяцев после закрытия сессии удаляются сообщения/медиа | `12` |

Для расчёта сроков хранения один месяц равен 30 дням. См. [Retention](RETENTION.md).

**Важно:** `.env` никогда не коммитится в git. Используй `.env.example` как шаблон.

On existing local Compose installations, add the Redis password before
recreating Redis and backend together. Expect operators to sign in again
if Redis loses its in-memory sessions. Preserve the existing PostgreSQL
credentials and PII encryption key when reusing volumes. For standalone
backend launches, set `DATABASE_URL` explicitly with the runtime login
and supply Redis credentials in `REDIS_URL` or via `REDIS_PASSWORD`.
The bot needs only its backend connection settings for these operations.
Use `docker compose config --quiet` for validation so interpolated secrets
are not printed.

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

### White-label settings

All settings below are optional. Panel settings are public and are served
through `/panel-config.json`. The panel renders configured text as plain text.

| Compose variable | Behavior |
|---|---|
| `PANEL_PRIMARY_COLOR` | Mantine accent palette. Choose `blue` (default), `cyan`, `teal`, `green`, `grape`, `violet`, or `indigo`. |
| `PANEL_COLOR_SCHEME` | Default scheme. Choose `light` (default), `dark`, or `auto` to match the browser. |
| `PANEL_LOGIN_SUBTITLE_RU`, `PANEL_LOGIN_SUBTITLE_EN` | Optional text below the product name on the sign-in page. An empty value hides it for that language. |
| `BOT_ORGANIZATION_NAME` | Name used by the built-in `/start` greetings. The configuration accepts up to 120 UTF-8 bytes. |
| `BOT_JOINED_TEXT_RU`, `BOT_JOINED_TEXT_EN` | Override the greeting sent on successful invitation redemption. |
| `BOT_REQUEST_RECEIVED_TEXT_RU`, `BOT_REQUEST_RECEIVED_TEXT_EN` | Override the reply to a new request or the neutral fallback greeting. |

Each bot text accepts up to 1,000 UTF-8 bytes. Bot values are trimmed at
the edges. Empty overrides use the built-in greeting for the selected
language, including the organization name when configured. Set Vite
counterparts with the `VITE_` prefix for the four new panel variables.

Example `infra/.env` values for a single deployment:

```dotenv
PANEL_PRODUCT_NAME='North & Co Panel'
PANEL_PRIMARY_COLOR=teal
PANEL_COLOR_SCHEME=dark
PANEL_LOGIN_SUBTITLE_RU='Вход сотрудников North & Co'
PANEL_LOGIN_SUBTITLE_EN='North & Co staff sign-in'
BOT_ORGANIZATION_NAME='North & Co'
```

### Optional monitoring

These values apply when adding `infra/docker-compose.monitoring.yml`.

| Compose variable | Behavior |
|---|---|
| `METRICS_TOKEN_FILE` | Absolute path to a dedicated 64-character hex token file shared by backend and Prometheus. Generate it independently of the two internal API tokens. |
| `GRAFANA_ADMIN_PASSWORD_FILE` | Absolute path to the initial Grafana administrator password file. |
| `GRAFANA_PORT` | Loopback port for Grafana, default `3000`. |

Keep both files outside the repository and grant access to the relevant
container users. See [Monitoring](OPERATIONS.md#monitoring).

### Optional operator SSO

Use `docker-compose.oidc.yml` with the TLS deployment. An empty
`OIDC_ISSUER_URL` disables SSO in a standalone backend. With the OIDC
override, all five settings are required.

| Variable | Value |
|---|---|
| `OIDC_ISSUER_URL` | Exact HTTPS issuer from the provider's discovery document. |
| `OIDC_CLIENT_ID` | Confidential web client registered at that issuer. |
| `OIDC_REDIRECT_URL` | Public panel origin over HTTPS followed by `/api/auth/oidc/callback`. |
| `OIDC_CLIENT_SECRET_FILE` | Absolute host path to the client secret file. |
| `OIDC_OPERATOR_MAP_FILE` | Absolute host path to a JSON array of unique `subject` and `operator_id` pairs. |

See [operator SSO operations](OPERATIONS.md#operator-sso-with-openid-connect)
for setup, file permissions and revocation.

### Optional Telegram webhook

The base Compose file uses polling. Add `docker-compose.webhook.yml`
after the TLS override to select webhook mode.

| Variable | Behavior |
|---|---|
| `BOT_UPDATE_MODE` | For standalone bot runs, `polling` (default) or `webhook`. The Compose files set this value for you. |
| `TELEGRAM_WEBHOOK_URL` | Public HTTPS URL ending in `/telegram/webhook`. Use a Telegram-supported HTTPS port and keep the URL free of credentials, query strings and fragments. |
| `TELEGRAM_WEBHOOK_SECRET_FILE` | Absolute path to a dedicated 64-character hex secret file. Keep it outside the repository and readable by bot UID 10001. |

See [Telegram webhook operations](OPERATIONS.md#telegram-webhook).
