# Настройка первого администратора и группы операторов

## Создание первого администратора

Система не может создать первого оператора через API — создание оператора
требует уже существующего авторизованного admin. Поэтому первый admin
создаётся вручную через SQL.

### Шаг 1 — сгенерируй хэш пароля

Временно создай файл `backend/cmd/hashpass/main.go`:

\`\`\`go
package main

import (
	"fmt"
	"os"

	"github.com/fastcheck/anonymus_bot/backend/internal/auth"
)

func main() {
	hash, err := auth.HashPassword(os.Args[1])
	if err != nil {
		panic(err)
	}
	fmt.Println(hash)
}
\`\`\`

\`\`\`powershell
cd backend
go run ./cmd/hashpass "ВашНадёжныйПароль123!"
\`\`\`

Скопируй полученный хэш. Удали временный файл после использования:
\`\`\`powershell
Remove-Item -Recurse cmd\hashpass
\`\`\`

### Шаг 2 — вставь оператора в БД

\`\`\`powershell
cd infra
docker compose exec postgres psql -U app_user -d mediator_bot -c "INSERT INTO operators (name, login, role, status, password_hash, language) VALUES ('Admin', 'admin', 'admin', 'active', '<вставь_хэш>', 'ru');"
\`\`\`

### Шаг 3 — войди в веб-панель

Открой веб-панель, войди с логином `admin` и заданным паролем. При первом
входе будет предложено настроить TOTP 2FA — отсканируй QR-код в
приложении-аутентификаторе (Google Authenticator, Authy и т.п.).

После этого первого admin можно использовать для создания остальных
операторов через раздел "Операторы" в панели.

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