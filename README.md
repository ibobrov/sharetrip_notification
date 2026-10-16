# sharetrip_notification

Локальный сервис уведомлений. Он принимает уведомление через HTTP, сохраняет его в собственной PostgreSQL и возвращает по идентификатору. Доставка во внешние каналы в первую версию не входит.

## Подготовка

Нужны Go 1.26.0 или совместимее, GNU Make и Docker с Compose v2. Добавьте Go и Make в PATH; запустите Docker.

1. Скопируйте `.env.example` в `.env`.
2. Задайте локальные учётные данные: согласуйте `DB_USER` с `POSTGRES_USER`, `DB_PASSWORD` с `POSTGRES_PASSWORD`, `DB_NAME` с `POSTGRES_DB`, `DB_PORT` с `POSTGRES_PORT`.
3. Заполните `GOOSE_DBSTRING` полным DSN той же БД, URL-кодируя специальные символы логина и пароля. `.env` не хранится в Git. Приложение загружает его перед проверкой конфигурации; Goose и Compose читают файл явно.
4. Выполните `make deps`, затем `make up`.

Compose создаёт проект `sharetrip-notification`, PostgreSQL 16.6 и именованный volume `sharetrip-notification_postgres_data`. БД доступна только на loopback: `127.0.0.1:6544`; внутри контейнера используется порт 5432. `make up` ожидает успешный healthcheck, `make down` сохраняет данные. Изменение `POSTGRES_*` после инициализации volume не меняет существующие учётные данные PostgreSQL.

## Команды

`make help` выводит все команды. `make deps` скачивает модули и устанавливает golangci-lint v2.11.3 и Goose v3.28.0 в `.tools/`. Версия Goose CLI совпадает с библиотекой в `go.mod`.

```text
make fmt
make lint
make build
make test
```

Тесты задач 07, 08, 09.5 и 10 прошли до приёмки и затем были удалены.

Сборка создаёт `bin/notification` (на Windows — `bin/notification.exe`). `make run` загружает `.env`, подключается к PostgreSQL и запускает HTTP на `127.0.0.1:8081` по умолчанию. По SIGINT/SIGTERM сервер даёт активным запросам до пяти секунд и закрывает пул. HTTP использует Fiber v2.52.15 и govalidator v12.0.0.

Каждый запрос получает `X-Request-Id`. В stdout записывается один JSON-лог с операцией, HTTP-кодом, длительностью, request ID и notification ID при наличии; payload, пароль и DSN не выводятся.

Миграции запускаются отдельно. Начальная миграция notifications уже добавлена:

```text
make migrate-status
make migrate-up
```

Перед `make migrate-up`/`make migrate-down` проверьте целевую БД в локальном `.env`. `make migrate-down` откатывает последнюю миграцию и может удалить данные; проверяйте откаты на одноразовой БД. Приложение и Compose автоматически миграции не применяют.

## Проверка API и сохранения

После `make migrate-up` запустите приложение командой `make run`.

Создайте уведомление:

```text
curl -i -X POST http://127.0.0.1:8081/api/v1/notifications \
  -H "Content-Type: application/json" \
  -d '{"recipient_id":"client-123","type":"trip_published","payload":{"trip_id":"trip-456"}}'
```

Ожидается HTTP 201 и объект уведомления напрямую:

```json
{
  "id": "e7e8bcc8-97c4-4abf-9d71-c7c5a19841d6",
  "recipient_id": "client-123",
  "type": "trip_published",
  "status": "created",
  "payload": {"trip_id": "trip-456"},
  "created_at": "2026-10-17T00:00:00Z"
}
```

Скопируйте фактический `id` из ответа и получите запись:

```text
curl -i http://127.0.0.1:8081/api/v1/notifications/<id>
```

Ожидается HTTP 200 и тот же объект. Для дополнительной проверки прочитайте строку напрямую из локальной БД; пользователь и имя БД в команде должны совпадать с `.env`:

```text
docker compose --env-file .env -f deploy/docker-compose.yml exec postgres \
  psql -U notification -d sharetrip_notification \
  -c "SELECT id, recipient_id, type, status, payload, created_at FROM notifications WHERE id = '<id>';"
```

Остановите `make run`, запустите его снова и повторите GET: запись должна сохраниться. Команды `make down` и затем `make up` также сохраняют запись, потому что обычный `down` не удаляет именованный volume. После завершения проверки выполните `make down`.

POST возвращает 400 для некорректного тела. GET возвращает 400 для некорректного UUID, 404 для корректного отсутствующего UUID и 500 для внутренней ошибки. Формат ошибок отдельно не задан.

[Спецификация](docs/spec.md) · [План и состояние задач](tasks/README.md)
