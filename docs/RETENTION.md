# Retention — политика хранения данных

Согласно ТЗ (раздел 11): PII анонимизируется через 3 месяца после закрытия сессии,
сообщения/медиа удаляются через 12 месяцев. Обе операции необратимы.

## Автоматический запуск

Сервис `retention` в docker-compose запускается ежедневно в час, заданный
переменной `RETENTION_RUN_HOUR` (по умолчанию 03:00 UTC).

## Ручной запуск (dry-run) — ОБЯЗАТЕЛЕН перед первым включением в проде

Перед тем как полагаться на автоматический запуск, обязательно прогоните
dry-run вручную и проверьте вывод:

\`\`\`powershell
docker compose run --rm --build --entrypoint /retention retention -dry-run
\`\`\`

Это покажет, сколько сессий/пользователей/сообщений было бы затронуто,
БЕЗ реального изменения данных.

## Ручной реальный запуск (вне расписания)

\`\`\`powershell
docker compose run --rm --build --entrypoint /retention retention
\`\`\`

## Мониторинг

Retention-процесс завершается с кодом выхода 1, если были ошибки —
это должно быть настроено как алерт в Uptime Kuma/аналогичном мониторинге
(Спринт 10/при развёртывании на VPS).

## Изменение периодов retention

Периоды заданы в `backend/internal/retention/config.go` (`DefaultConfig`).
Для MVP не вынесены в переменные окружения — при необходимости кастомизации
для конкретного клиента потребуется либо пересобрать образ, либо (в будущем)
вынести в env.

## Настройка периодов через переменные окружения

По умолчанию: PII анонимизируется через 3 месяца, сообщения удаляются
через 12 месяцев. Изменить можно через `.env`:

\`\`\`env
RETENTION_PII_MONTHS=3
RETENTION_MESSAGE_MONTHS=12
\`\`\`

Изменения применяются при следующем запуске retention-контейнера.
# Incoming request retention

Incoming requests are deleted by creation time after the PII retention
period (`RETENTION_PII_MONTHS`, 90 days by default), including unprocessed
requests. Dry runs report the eligible count without deleting anything.
People in any non-closed session are excluded from anonymization.
If a media file cannot be safely removed, its message record remains for
inspection and retry.

## Published media lifecycle

The bot owns publication into `bot_photos` and `bot_voice`. It writes
downloaded media before registering the message, removes temporary photo
originals and incomplete conversions, and can discard a file when the
backend confirms the message was rejected without being stored. Successful
immediate and deferred Telegram sends leave the published file in place.

Backend mounts both volumes read-only and serves authorized previews from
`/data/photos/<uuid>.jpg` and `/data/voice/<uuid>.ogg` via the message media
API. The panel consumes that API. Retention is the only component that
unlinks published files. It uses the existing closed-session cutoff
(12 months by default) and preserves a shared legacy file until its last
reference expires. Failed removal keeps the database record for retry.
Previously missing files allow row cleanup. Dry-run counts are estimates
and may count multiple references to a shared file.

The download limit remains 20 MiB with a 20-second combined deadline.
Photos still pass through ImageMagick metadata removal. The voice path
retains the existing Telegram Ogg contract without additional MIME or codec
rejection. A relay response error can leave an unreferenced file because the
write may already have succeeded. Retention only processes files referenced
by stored messages. Files deleted by older versions cannot be recovered here.
