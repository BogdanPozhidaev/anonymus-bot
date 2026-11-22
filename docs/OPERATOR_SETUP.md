# Настройка первого администратора и группы операторов

## Bootstrap the first administrator

Start backend and verify migration status using [MIGRATIONS.MD](MIGRATIONS.MD).
Save a unique password on one line in a restricted UTF-8 file outside Git.
Use at least 12 characters and at most 72 UTF-8 bytes excluding the
newline. Control characters and entirely blank passwords are rejected.
From the repository root:

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

## Привязка аккаунта для команды /operator

1. Войдите в панель с постоянным паролем и TOTP. Откройте
   **Аккаунт Telegram** и нажмите **Начать привязку**.
2. Отправьте команду с кодом из панели в личном чате с ботом.
3. Введите код из ответа бота в панель и нажмите **Подтвердить привязку**.
4. Отправьте боту `/operator`, чтобы открыть меню.

На оба шага с кодами даётся пять минут с момента их выпуска в панели.
При истечении срока создайте новый код. Держите оба кода в личном чате
и панели. Для смены аккаунта повторите процедуру через
**Привязать другой аккаунт**. Предыдущая привязка действует до
подтверждения новой. Каждый аккаунт Telegram можно привязать
только к одному оператору.

Команда `/language` сохраняет русский или английский язык личного
диалога после подключения к сессии или привязки аккаунта оператора.
У пользователя, который уже участвовал в сессии и затем привязал
аккаунт оператора, применяется язык профиля участника.
Уведомления в общей группе операторов остаются на русском.
