# Anonymus Bot — Telegram-бот-посредник с анонимизацией сессий

MVP-версия Telegram-бота и веб-панели для организации анонимных двусторонних сессий между Заказчиком и Исполнителем через компанию-посредника.

## Стек

- **Bot:** Go 1.25+, go-telegram-bot-api
- **Backend:** Go 1.25+, Gin
- **БД:** PostgreSQL 15+
- **FSM/rate limiting:** Redis
- **Web Panel:** React, TypeScript (в разработке)
- **Инфраструктура:** Docker Compose, Nginx

## Структура репозитория

\`\`\`
├── bot/            # Telegram Bot Service
├── backend/        # Backend API
├── webpanel/       # Веб-панель (React, в разработке)
├── migrations/     # SQL-миграции
├── infra/          # Docker Compose, Nginx конфиги
└── docs/           # Документация проекта
\`\`\`

## Запуск локально

1. Склонируй репозиторий
2. Скопируй пример конфига:
   \`\`\`
   cp infra/.env.example infra/.env
   \`\`\`
3. Заполни `infra/.env` своими значениями (для локальной разработки можно оставить дефолтные)
4. Подними инфраструктуру:
   \`\`\`
   cd infra
   docker compose up --build
   \`\`\`
5. Проверь, что всё работает:
   \`\`\`
   curl http://localhost/api/health
   \`\`\`
   Ожидаемый ответ: `{"status":"ok","postgres":"ok","redis":"ok"}`

## Миграции БД

См. [docs/MIGRATIONS.md](docs/MIGRATIONS.md)

## Требования к окружению

- Docker Desktop
- Go 1.25+ (для локальной разработки без Docker)
- Git

## Типовые проблемы и решения

| Проблема | Решение |
|---|---|
| `variable is not set. Defaulting to a blank string` | `.env` должен лежать в папке `infra/`, не в корне проекта |
| Postgres просит пароль, который не подходит | Пароль "зашит" в volume при первом запуске. Удали volume: `docker volume rm infra_postgres_data`, пересоздай контейнер |
| `migrate.exe` на Windows не подключается к БД | Используй Docker-версию migrate вместо нативного бинарника — см. [docs/MIGRATIONS.md](docs/MIGRATIONS.md) |
| golangci-lint падает с ошибкой версии конфига | Убедись, что в `.golangci.yml` указано `version: "2"` |

## CI/CD

GitHub Actions прогоняет на каждый PR: сборку, `go vet`, линтер (golangci-lint v2), тесты с `-race`. Мердж в `main` заблокирован без прохождения всех проверок.

## Статус проекта

🚧 MVP в разработке. Текущий этап: инфраструктура и модель данных.