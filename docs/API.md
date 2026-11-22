# Обзор API

Backend предоставляет два семейства эндпоинтов:

- `/internal/*` — сервисный API бэкенда. Каждый запрос бота содержит один
  заголовок `X-Internal-Token`. Отсутствующий, повторный или неверный заголовок
  даёт `401` до вызова обработчика. Настройка файла токена описана в
  [ENV_VARS.md](ENV_VARS.md).
- `/api/*` — публичный API для веб-панели, требует авторизации
  (Bearer-токен сессии), кроме `/api/auth/login` и `/api/auth/totp/verify`

Базовый Compose не публикует порт бэкенда. Nginx не проксирует
`/api/internal` и его подмаршруты. Dev override публикует порт бэкенда
только на `127.0.0.1:8080`. Для Swagger и k6 используйте локальное тестовое
окружение. Для k6 укажите loopback-адрес явно через `BASE_URL` и путь к
тестовому токену через `INTERNAL_API_TOKEN_FILE`. Изолированный запуск описан
в [Operations](OPERATIONS.md#isolated-k6-relay-check).

## Аутентификация

| Метод | Путь | Описание |
|---|---|---|
| POST | `/api/auth/login` | Логин/пароль → totp_required или totp_setup_required |
| POST | `/api/auth/totp/verify` | Подтверждение TOTP-кода → session_token |
| POST | `/api/auth/logout` | Отзыв текущей сессии |

За 5 минут разрешено до 5 попыток входа на пару логин/IP и до 30
попыток на IP. Для TOTP разрешено до 5 проверок на промежуточный токен
в пределах его срока действия и до 10 проверок на оператора за 5 минут.
При превышении лимита API возвращает `429` и `Retry-After` в секундах.
При недоступном ограничителе Redis запрос получает `503`.
Неверный пароль, неизвестный логин и неактивный аккаунт получают
одинаковый `401` на этапе ввода пароля.
После изменения роли или статуса прежние сессии и промежуточные токены
оператора перестают действовать. Повторное включение оператора их не восстанавливает.

## Готовность сервиса

`GET /api/health` доступен через Nginx. При доступных PostgreSQL и Redis
возвращает `200` и `status: ok`. При недоступной зависимости возвращает
`503` с именем и состоянием зависимости без деталей подключения.
`GET /health` остаётся доступен напрямую на бэкенде для существующих проверок.

## Сессии

| Метод | Путь | Доступ |
|---|---|---|
| GET | `/api/sessions` | Все авторизованные (admin видит всё, operator — свои) |
| GET | `/api/sessions/:id` | Владелец сессии или admin |
| POST | `/api/sessions` | Все авторизованные |
| POST | `/api/sessions/:id/invites` | Владелец сессии или admin. Принимает `role`, возвращает `token` и `expires_at` при выдаче |
| DELETE | `/api/sessions/:id/invites/:role` | Владелец сессии или admin. Отзывает неиспользованное приглашение |
| PATCH | `/api/sessions/:id/status` | Владелец сессии или admin |
| PATCH | `/api/sessions/:id/owner` | Владелец сессии или admin |
| GET | `/api/sessions/:id/messages` | Владелец сессии или admin |
| POST | `/api/sessions/:id/send-as` | Владелец сессии или admin (режим active) |

### Приглашения

Новая ссылка действует 24 часа и используется один раз. Перевыпуск отзывает
предыдущую ссылку на ту же роль. Выпустить и принять приглашение можно для
свободного места в активной сессии. Ссылку следует передавать получателю лично.
Бот принимает токен в личном чате и посылает его в теле
`POST /internal/sessions/bind` вместе с данными пользователя. Старые ссылки
с ID и ролью заменяются новыми. Уже подключённые участники могут открыть
сессию через `/sessions`.

## Операторы (только admin)

| Метод | Путь |
|---|---|
| GET | `/api/operators` |
| POST | `/api/operators` |
| PATCH | `/api/operators/:id` |

`PATCH /api/operators/:id` accepts the `admin` and `operator` roles.
A role change invalidates the affected operator's existing authentication.
The API returns HTTP 409 if a role or status change would leave no active
administrator.

## Входящие обращения

| Метод | Путь |
|---|---|
| GET | `/api/incoming-requests` |
| PATCH | `/api/incoming-requests/:id/status` |

## Аудит

Доступен только администраторам. Записи возвращают `actor_type`.
CSV содержит выбранную страницу с теми же фильтрами.

| Метод | Путь | Параметры запроса |
|---|---|---|
| GET | `/api/audit-log` | `actor_id`, `action`, `target_type`, `date_from`, `date_to`, `format` (json\|csv), `limit` (1–200, по умолчанию 100), `offset` (по умолчанию 0). JSON возвращает `has_more`. Даты принимаются в ISO `YYYY-MM-DD` или RFC 3339. |

## Полная автогенерируемая документация

Для локальной разработки Swagger UI можно включить переменной
`ENABLE_SWAGGER=true` и открыть `/swagger/index.html`. По умолчанию он отключён.
## Message delivery

`GET /api/sessions/:id/messages` returns `delivered` and
`delivery_status`. New messages are `queued`, `retrying`, `delivered`, or
`failed`. `legacy_unknown` identifies history recorded before the outbox.
Its historical boolean alone does not prove delivery. The protected relay
and bind calls no longer return a recipient Telegram identifier or pending
message contents to the bot. The delivery worker uses the protected
backend-to-bot endpoint for each attempt.

The backend saves the message and its outbox entry in one transaction.
Work waits for the counterpart to join and is eligible while the session
is active. After a transport or Telegram error, the worker retries at
5, 10, 20 and 40 seconds. After the fifth failed attempt, history shows
`failed`. For participant messages, a separate retryable notice is sent
privately to the sender. Messages sent on behalf of a participant by an
operator report the failure in the panel.

Successful Telegram API acceptance is the delivery confirmation. It
does not report whether the recipient read the message. Workers lease
work for 90 seconds and a completed entry is not sent again. If Telegram
accepts a send and its response is lost, or the database cannot record
that success, a later retry may send a duplicate. Telegram does not offer
an idempotency key for these send calls.

## Session transcript download

`GET /api/sessions/:id/transcript?format=csv` or `?format=pdf` downloads
the retained history in chronological order using the same session access
rules as the panel. It requires the operator Bearer token. The assigned
operator and administrators can download from the session detail screen.
The export includes the UTC timestamp, sender role, content type, delivery
status, operator send-as attribution, message text and media captions.
It omits attachment files, file references, participant identifiers and
session titles. Text written in the conversation is included in the export.

CSV uses UTF-8 with a BOM and prefixes spreadsheet formula-like text with
an apostrophe. PDF embeds a font with Cyrillic support and displays
unsupported characters using `[U+XXXX]` notation. Use CSV when the original
Unicode text is needed.

Downloads are attachments with `Cache-Control: no-store`. Each successful
export requires a `session_transcript_exported` audit record with operator,
session, format and message count. If the audit write fails, the endpoint
returns HTTP 503. The limit is 2,000 messages and 1 MiB of source text per
export, plus 250 pages for PDF. Oversized exports return HTTP 422 without
a partial file. If all four exporter slots are in use, the endpoint
returns HTTP 503.

## Request limits and paging

The backend limits API bodies to 64 KiB and returns HTTP 413 when a body
exceeds the limit. Media upload from Telegram uses the separate bot limit.
`GET /api/sessions` and `GET /api/sessions/:id/messages` accept `limit`
(1–200, default 50) and `offset`. Both return `has_more`. Sessions can be
filtered by `status` and searched by title or exact ID with `q`. History
pages start with the newest messages and display each page in chronological
order. Both endpoints apply the existing session access rules.

The same page parameters and `has_more` field apply to public operator
and incoming-request lists and protected internal incoming-request and
session lists. The internal participant list places the selected session
first so `/current` works even when it is older than the newest sessions.
Results use a stable ID tie-breaker. Telegram list menus request ten
rows per page.

An internal incoming-request submission reuses an open request for the
same sender or a request created within the past ten minutes. A new
record returns HTTP 201 and sends an operator alert. A reused record
returns HTTP 200 with the existing ID. The first private text sent to
the bot without a selected session fills the open request's empty
first-message field. Later text leaves that field intact.

## Password change

`POST /api/auth/password` requires an authenticated operator session,
the current password and a replacement. The replacement must contain
at least 12 characters, fit in 72 UTF-8 bytes, contain no control
characters and differ from the current password. Up to five current
password checks per operator are allowed in five minutes, separately
from login.
HTTP 204 indicates success. Existing sessions and pending sign-ins are
then invalidated. The next sign-in still requires TOTP.

After verifying TOTP for an account with a temporary password,
`POST /api/auth/totp/verify` returns `password_change_required: true`.
The accompanying five-minute bearer token works only for password
change and logout. After a successful change, sign in again with
the permanent password and the same authenticator.

`POST /api/operators/:id/password/reset` requires an administrator
session and resets another active or invited operator's password.
The response returns the generated password once with
`Cache-Control: no-store`. The backend saves the hash, marks the
password for mandatory change and invalidates older sessions and
pending sign-ins in the same update. Existing TOTP enrollment is
preserved. The audit entry records the administrator and the target.

## TOTP recovery

- `POST /api/auth/totp/recovery-codes` requires a full operator session,
  the current password and a valid TOTP. It issues eight codes once in
  a no-store response. PostgreSQL stores only their digests. Reissuing
  codes invalidates earlier codes, sessions and pending sign-ins.
- `POST /api/auth/totp/recover` requires a pending token from the password
  step of sign-in and one recovery code. It consumes the password
  challenge even when the submitted recovery code is invalid. On
  success it consumes the code and returns a fresh five-minute TOTP
  enrollment challenge without granting panel access. The login and
  recovery code submission endpoints use the existing attempt limits.
- Confirm the new authenticator using `POST /api/auth/totp/verify`.
  Successful confirmation replaces the encrypted secret and revokes
  earlier sessions, pending sign-ins and unused recovery codes.
  The previous authenticator remains usable until confirmation.
  An unused code is needed to restart an interrupted recovery.

## Operator Telegram linking

| Method | Path | Required access | Success |
|---|---|---|---|
| GET | `/api/account/telegram-link` | Operator session | 200 with `linked` |
| POST | `/api/account/telegram-link` | Operator session | 201 with `code` |
| POST | `/internal/operators/telegram-link/claim` | Bot service token | 200 with `confirmation_code` |
| POST | `/api/account/telegram-link/confirm` | Initiating operator account | 204 |

The bot sends `code` and the private sender's `telegram_id` in the claim
body. The panel sends the bot's confirmation code as `code`. Codes are
single-use and share the original five-minute expiration. Issuing a new
code replaces the pending challenge. Auth version changes invalidate
pending challenges. A missing or expired challenge returns 404 and an
account already linked to another operator returns 409. Responses
include `Cache-Control: no-store`. The audit event
`operator_telegram_linked` records the operator's internal ID.

The internal user lookup returns `language_code`. The internal operator
lookup returns `language`. The language update endpoint supports
`ru` and `en`. It updates an existing participant profile or a linked
active operator profile and returns 404 when neither exists.

## Operator audit

The audit records completed TOTP sign-in, sign-out, rejected credentials
within the attempt budget, session creation, incoming request status
updates, successful media access, and bot stop or switch actions. Entries
use internal actor and target identifiers. Authentication audit omits
credential input, tokens and TOTP codes. An unknown login has no actor ID.
Media range requests are separate HTTP accesses. Audit writes are
best-effort, with errors logged by the backend.

Session pause and close trigger neutral notices to the connected
participants. Delivery of these notices is best-effort and independent
of the persisted status.
