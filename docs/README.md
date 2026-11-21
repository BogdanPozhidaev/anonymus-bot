# Anonymus Bot — Telegram-бот-посредник с анонимизацией сессий

MVP-версия Telegram-бота и веб-панели для организации анонимных двусторонних сессий между Заказчиком и Исполнителем через компанию-посредника. Обе стороны общаются через одного бота, не видя Telegram ID, username, номер телефона друг друга.

## Статус проекта

✅ MVP полностью реализован (Спринты 0-10). Все функциональные требования из ТЗ покрыты, включая четыре зоны повышенного риска: маппинг сообщений, фильтры модерации, переключение сессий, retention.

⚠️ Security-чеклист (15 пунктов) должен быть пройден вручную перед деплоем на прод — см. `docs/SECURITY_CHECKLIST.md`.

## Стек

- **Bot:** Go 1.26, go-telegram-bot-api/v5
- **Backend:** Go 1.26, Gin
- **БД:** PostgreSQL 15
- **FSM/rate limiting/сессии оператора:** Redis 7
- **Web Panel:** React 18, TypeScript, Vite, Mantine, TanStack Query, Zustand
- **Инфраструктура:** Docker Compose, Nginx
- **Обработка фото:** ImageMagick

## Структура репозитория
├── bot/ # Telegram Bot Service
├── backend/ # Backend API + retention CLI
├── webpanel/ # Веб-панель (React)
├── migrations/ # SQL-миграции
├── infra/ # Docker Compose, Nginx конфиги
└── docs/ # Документация проекта


## Функциональность

### Для клиента/исполнителя (через Telegram-бота)
- Онбординг через deeplink с автоматическим определением роли
- Обмен текстом, фото (метаданные вырезаются автоматически), голосовыми сообщениями
- Автоматическая блокировка попыток обмена контактами (регулярные выражения + эвристика)
- Переключение между несколькими активными сессиями (`/sessions`, `/current`)
- Запрос на завершение диалога (`/stop`)
- Двуязычный интерфейс (RU/EN — определение языка, полная локализация не завершена)

### Для оператора (веб-панель + Telegram)
- Аутентификация: логин/пароль + обязательная TOTP 2FA
- Управление сессиями: создание, привязка сторон через deeplink, статусы, просмотр переписки
- Отправка сообщений от имени клиента/исполнителя (режим active) с полной модерацией
- Входящие обращения — создание сессии в один клик
- Управление операторами (только admin) — создание, статусы, немедленный отзыв доступа
- Аудит действий с фильтрами и экспортом в CSV
- Уведомления в группу операторов о модерации, новых обращениях, запросах на завершение

### Автоматические процессы
- Retention: анонимизация PII через 3 месяца после закрытия сессии, удаление сообщений/медиа через 12 месяцев
- Ежедневный автозапуск retention с dry-run режимом для предпросмотра

## Local startup

For the production panel image, the Nginx routes and owner-provided TLS
certificates, follow [DEPLOYMENT.md](DEPLOYMENT.md).

1. Copy `infra/.env.example` to `infra/.env`. Set PostgreSQL and Redis
   passwords, a base64 PII key and both service-token files. See
   [ENV_VARS.md](ENV_VARS.md).
2. From the repository root, start dependencies, migrate, and start backend:

   ```powershell
   docker compose -f infra/docker-compose.yml -f infra/docker-compose.dev.yml up -d --build backend
   docker compose -f infra/docker-compose.yml -f infra/docker-compose.dev.yml run --rm -T migrate migrate status
   ```

3. Check `http://127.0.0.1:8080/health` and create the first admin using
   [OPERATOR_SETUP.md](OPERATOR_SETUP.md).
4. In `webpanel`, run `npm.cmd ci` and `npm.cmd run dev`. Open
   `http://localhost:5173` to sign in and complete TOTP.
5. Start `bot` separately when its Telegram settings are ready. Start the
   scheduled retention service separately after inspecting a dry run.

## Retention

См. [docs/RETENTION.md](docs/RETENTION.md) — обязательно прогнать `-dry-run` перед первым включением автоматического запуска в проде.

## Требования к окружению

- Docker Desktop
- Go 1.26+ (для локальной разработки без Docker)
- Node.js 20.19+ или 22.12+ (для веб-панели)
- Git

## Типовые проблемы и решения

| Проблема | Решение |
|---|---|
| `variable is not set. Defaulting to a blank string` | `.env` должен лежать в папке `infra/`, не в корне проекта |
| Postgres просит пароль, который не подходит | Пароль "зашит" в volume при первом запуске. Удали volume: `docker volume rm infra_postgres_data`, пересоздай контейнер |
| `migrate.exe` на Windows не подключается к БД | Используй Docker-версию migrate — см. `docs/MIGRATIONS.md` |
| Конфликт портов на 5432/6379 | Проверь, нет ли нативно установленного Postgres/Redis как службы Windows: `Get-Service \| Where-Object { $_.Name -like "*postgres*" }` |
| Vite слушает только IPv6, браузер не подключается | Убедись, что в `vite.config.ts` указано `server.host: true` |
| API возвращает данные, но веб-панель показывает `undefined` | Проверь наличие JSON-тегов в Go-моделях (`backend/internal/models/`) |

## CI/CD

GitHub Actions прогоняет на каждый PR: сборку, `go vet`, линтер (golangci-lint v2), тесты с `-race` (включая testcontainers для repository/retention). Мердж в `main` заблокирован без прохождения всех проверок.

## Документация

<<<<<<< HEAD
- [docs/ENV_VARS.md](docs/ENV_VARS.md) — переменные окружения
- [docs/MIGRATIONS.md](docs/MIGRATIONS.md) — работа с миграциями
- [docs/RETENTION.md](docs/RETENTION.md) — политика хранения данных
- [docs/OPERATOR_SETUP.md](docs/OPERATOR_SETUP.md) — создание первого администратора, настройка группы операторов
- [docs/API.md](docs/API.md) — обзор API-эндпоинтов (либо Swagger, см. Тикет 11.2)

## Известные ограничения MVP

- Локализация: сохранённый выбор языка пользователя не используется хендлерами (только язык профиля Telegram)
- Смена пароля при первом входе оператора не реализована (только настройка 2FA)
- Email/Telegram-приглашения операторов не реализованы (только ручное создание с временным паролем)
- Фото/голос в веб-панели отображаются как текстовые плейсхолдеры, без превью
- Периоды retention захардкожены в коде, не настраиваются через переменные окружения

