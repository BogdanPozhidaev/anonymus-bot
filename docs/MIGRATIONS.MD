# Работа с миграциями

⚠️ На Windows нативный `migrate.exe` не подключается к Postgres в Docker по неясной причине (сетевая проблема). Используем Docker-контейнер вместо нативного бинарника.

## Предварительно

Postgres должен быть поднят с пробросом порта:

\`\`\`powershell
cd infra
docker compose -f docker-compose.yml -f docker-compose.dev.yml up -d postgres
\`\`\`

Узнай имя Docker-сети:
\`\`\`powershell
docker network ls | findstr infra
\`\`\`

## Применить миграции

\`\`\`powershell
cd C:\anonymus_bot
docker run --rm -v "${PWD}/migrations:/migrations" --network infra_default migrate/migrate -path=/migrations/ -database "postgres://app_user:<пароль_из_.env>@postgres:5432/mediator_bot?sslmode=disable" up
\`\`\`

## Откатить миграции

Замени `up` на `down -all` в команде выше.

## Создать новую миграцию

\`\`\`powershell
migrate create -ext sql -dir migrations -seq <название_миграции>
\`\`\`

(эта команда работает локально нормально, без Docker — проблема была только в подключении к БД)