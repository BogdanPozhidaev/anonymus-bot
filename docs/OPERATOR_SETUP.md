# Настройка первого администратора и группы операторов

## Bootstrap the first administrator

Start backend and verify migration status using [MIGRATIONS.MD](MIGRATIONS.MD).
Save a unique password on one line in a restricted UTF-8 file outside Git.
The limit is 72 bytes excluding the newline. From the repository root:

```powershell
$OutputEncoding = [Text.UTF8Encoding]::new($false)
Get-Content -Raw -Encoding UTF8 'C:/private/bootstrap-password.txt' |
  docker compose -f infra/docker-compose.yml run --rm -T --no-deps migrate create-admin --login admin --name Administrator
```

The CLI receives the password on stdin. The same login is a no-op on repeat
even when supplied a different password. Another login is refused once
initialized, including after operators are removed. Additional operators
are created using the regular administrator interface.

Sign in using the selected password and complete the existing TOTP setup.
Remove the temporary password file when the credential is safely stored.

## Настройка группы операторов в Telegram

Бот отправляет алерты (срабатывание модерации, новые обращения, запросы
на завершение сессии) в отдельную Telegram-группу.

### Шаг 1 — создай группу

Создай новую группу в Telegram, добавь туда бота (`@ваш_бот`).

### Шаг 2 — узнай chat_id группы

Напиши любое сообщение в группу, затем открой в браузере:
\`\`\`
https://api.telegram.org/bot<ВАШ_ТОКЕН>/getUpdates
\`\`\`

Найди в JSON-ответе поле \`"chat":{"id":-100123456789,...}\` — это
отрицательное число и есть chat_id группы.

### Шаг 3 — пропиши в конфиг

В \`infra/.env\`:
\`\`\`
OPERATOR_GROUP_CHAT_ID=-100123456789
\`\`\`

Перезапусти бота:
\`\`\`powershell
docker compose up --build bot
\`\`\`

## Whitelist для команды /operator

Личный режим \`/operator\` в боте доступен только тем, кто уже является
активным оператором в системе (проверяется по telegram_id через таблицу
\`operators\`). Чтобы оператор мог использовать эту команду, у него должен
быть заполнен \`telegram_id\` в записи оператора — сейчас это делается
вручную через SQL:

\`\`\`sql
UPDATE operators SET telegram_id = <telegram_id_оператора> WHERE login = '<логин>';
\`\`\`

(автоматическое приглашение через Telegram с самостоятельной привязкой
telegram_id — не реализовано в MVP, см. "Известные ограничения" в README)