# Meeting Room Booking Service

Сервис бронирования переговорок (Go + PostgreSQL + Docker Compose) с DDD-структурой.

## Архитектура
- `app/entities/` — сущности, ошибки, инварианты, порты.
- `app/usecase/` — use cases (rooms, schedules, bookings, auth).
- `app/repository/` — репозитории (`memory` и `postgres`).
- `app/api/` — HTTP handlers, JWT middleware, route wiring.
- `app/di/` — dependency injection и сборка приложения.
- `deploy/postgres/init` — SQL-инициализация схемы.

## Основные файлы
- HTTP: `app/api/http.go`
- Бизнес-логика: `app/usecase/create.go`, `app/usecase/find.go`, `app/usecase/delete.go`, `app/usecase/service.go`, `app/usecase/conference.go`
- Доменные контракты: `app/entities/entities.go`, `app/entities/errors.go`, `app/entities/ports.go`, `app/entities/slot.go`
- Репозитории: `app/repository/memory.go`, `app/repository/postgres.go`
- DI: `app/di/container.go`

## API
Сервис поддерживает контракт из `api.yaml`.

Основные эндпоинты:
- `GET /_info`
- `POST /dummyLogin`
- `POST /register`
- `POST /login`
- `GET /rooms/list`
- `POST /rooms/create`
- `POST /rooms/{roomId}/schedule/create`
- `GET /rooms/{roomId}/slots/list?date=YYYY-MM-DD`
- `POST /bookings/create`
- `GET /bookings/list`
- `GET /bookings/my`
- `POST /bookings/{bookingId}/cancel`

Также оставлены alias-маршруты для обратной совместимости (`/rooms`, `/bookings` и т.д.).

## Бизнес-правила
- Длительность слота фиксирована: 30 минут.
- Один слот может иметь только одну активную бронь.
- Без расписания переговорка недоступна для бронирования.
- Все `date-time` — в UTC.
- Бронировать может только роль `user`.
- Отмена брони идемпотентна.
- Бронь слота в прошлом запрещена (`400`).
- `/bookings/my` возвращает только будущие брони.

## Генерация слотов
Гибридная стратегия:
1. Предгенерация на 30 дней при создании расписания.
2. Lazy-generation на дату при запросе, если слоты еще не созданы.

## Conference link
`POST /bookings/create` поддерживает опциональный флаг:

```json
{
  "slotId": "...",
  "createConferenceLink": true
}
```

Если внешний провайдер недоступен, бронь не откатывается: сохраняется `conferenceStatus=failed`.

## Тесты
- Unit: `app/usecase/service_test.go`, `app/entities/slot_test.go`
- API E2E: `app/api/apitest/http_e2e_test.go`
- Postgres repository tests: `repository/postgrestest/postgres_repository_test.go`

## Запуск
```bash
make up
make seed
```

Проверка:
```bash
curl http://localhost:8080/_info
```

## Полезные команды
- `make test`
- `make swagger`
- `make loadtest`
- `make down`

## Docker
- Сервис: `localhost:8080`
- PostgreSQL: контейнер `db`
- Приложение: контейнер `app`

Если менялась схема и старый volume мешает:
```bash
docker compose down -v
docker compose up -d db
```
