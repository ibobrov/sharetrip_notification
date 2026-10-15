# sharetrip_notification

Локальный сервис уведомлений. Реализованы каркас, конфигурация окружения и инструменты разработки (задачи 01–04). HTTP, подключение приложения к БД и схема уведомлений ещё не реализованы; `make run` пока завершает работу без запуска сервера.

## Подготовка

Нужны Go 1.26.0 или совместимее, GNU Make и Docker с Compose v2. Добавьте Go и Make в PATH; запустите Docker.

1. Скопируйте `.env.example` в `.env`.
2. Задайте локальные учётные данные: согласуйте `DB_USER` с `POSTGRES_USER`, `DB_PASSWORD` с `POSTGRES_PASSWORD`, `DB_NAME` с `POSTGRES_DB`, `DB_PORT` с `POSTGRES_PORT`.
3. Заполните `GOOSE_DBSTRING` полным DSN той же БД, URL-кодируя специальные символы логина и пароля. `.env` не хранится в Git. Загрузка `.env` приложением будет подключена в задаче 10; Goose и Compose уже читают файл явно.
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

Сборка создаёт `bin/notification` (на Windows — `bin/notification.exe`). `make tidy` запускается только при осмысленном изменении зависимостей: сейчас он удалит требования, зарезервированные для следующих задач и ещё не импортируемые кодом.

Миграции запускаются отдельно. Начальная схема появится в задаче 05; пока следующие команды предназначены для работы после её добавления:

```text
make migrate-create name=create_notifications
make migrate-status
make migrate-up
```

Перед `make migrate-up`/`make migrate-down` проверьте целевую БД в локальном `.env`. `make migrate-down` откатывает последнюю миграцию и может удалить данные; проверяйте откаты на одноразовой БД. Приложение и Compose автоматически миграции не применяют.

[Спецификация](docs/spec.md) · [План и состояние задач](tasks/README.md)
