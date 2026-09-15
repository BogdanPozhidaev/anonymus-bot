# Обзор API

Backend предоставляет два семейства эндпоинтов:

- `/internal/*` — только для внутреннего общения bot ↔ backend, без
  авторизации оператора (защищено на уровне сети — недоступно снаружи
  через Nginx в продакшн-конфигурации)
- `/api/*` — публичный API для веб-панели, требует авторизации
  (Bearer-токен сессии), кроме `/api/auth/login` и `/api/auth/totp/verify`

## Аутентификация

| Метод | Путь | Описание |
|---|---|---|
| POST | `/api/auth/login` | Логин/пароль → totp_required или totp_setup_required |
| POST | `/api/auth/totp/verify` | Подтверждение TOTP-кода → session_token |
| POST | `/api/auth/logout` | Отзыв текущей сессии |

## Сессии

| Метод | Путь | Доступ |
|---|---|---|
| GET | `/api/sessions` | Все авторизованные (admin видит всё, operator — свои) |
| GET | `/api/sessions/:id` | Владелец сессии или admin |
| POST | `/api/sessions` | Все авторизованные |
| PATCH | `/api/sessions/:id/status` | Владелец сессии или admin |
| PATCH | `/api/sessions/:id/owner` | Владелец сессии или admin |
| GET | `/api/sessions/:id/messages` | Владелец сессии или admin |
| POST | `/api/sessions/:id/send-as` | Владелец сессии или admin (режим active) |

## Операторы (только admin)

| Метод | Путь |
|---|---|
| GET | `/api/operators` |
| POST | `/api/operators` |
| PATCH | `/api/operators/:id` |

## Входящие обращения

| Метод | Путь |
|---|---|
| GET | `/api/incoming-requests` |
| PATCH | `/api/incoming-requests/:id/status` |

## Аудит

| Метод | Путь | Параметры запроса |
|---|---|---|
| GET | `/api/audit-log` | `actor_id`, `action`, `target_type`, `date_from`, `date_to`, `format` (json\|csv) |

## Полная автогенерируемая документация

См. Swagger UI по адресу `/swagger/index.html` при запущенном backend
(настройка — см. Тикет 11.2).