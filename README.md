# Task Manager Service

REST API для управления задачами внутри команд: ролевая модель, история изменений, комментарии,
кеширование списка задач в Redis и SQL-отчет по команде.

Стек: Go 1.26, MySQL 8.4, Redis 7, Docker Compose, OpenAPI 3.

## Быстрый старт

Нужны Docker с Compose v2, `make` и `openssl`. Запуск одной командой:

```bash
make up
```

Если `.env` еще нет, команда создает его из `.env.example` и генерирует секреты `MYSQL_PASSWORD`,
`MYSQL_ROOT_PASSWORD` и `JWT_SECRET`. Существующий `.env` не перезаписывается, поэтому свои значения можно
задать заранее. Дальше Compose собирает образ, поднимает MySQL и Redis, дожидается их healthcheck, применяет
миграции одноразовым сервисом `migrate` и запускает приложение. Команда завершается, когда все сервисы готовы,
и печатает адрес Swagger UI.

| Команда | Действие |
| --- | --- |
| `make up` | собрать и запустить стек (повторный вызов пересобирает приложение) |
| `make logs` | логи приложения |
| `make down` | остановить стек, данные MySQL сохраняются в volume |
| `docker compose down -v` | остановить и удалить данные |

Если порты `8080`, `3306` или `6379` заняты, поменяйте `HTTP_PUBLISHED_PORT`, `MYSQL_PUBLISHED_PORT`
и `REDIS_PUBLISHED_PORT` в `.env`.

После запуска сервис доступен на `http://localhost:8080`.

| URL | Назначение |
| --- | --- |
| `http://localhost:8080/swagger/` | Swagger UI |
| `http://localhost:8080/swagger/openapi.yaml` | OpenAPI-спецификация (`api/openapi.yaml`) |
| `http://localhost:8080/healthz` | liveness |
| `http://localhost:8080/readyz` | readiness (доступность MySQL) |
| `http://localhost:8080/metrics` | метрики Prometheus |

### Локальный запуск без контейнера приложения

```bash
make .env
docker compose up -d mysql redis
set -a; source .env; set +a
make migrate
go run ./cmd/server
```

## Конфигурация

Все параметры задаются переменными окружения, полный список с значениями по умолчанию — в `.env.example`.
Секреты значений по умолчанию не имеют: без них сервис не стартует и сообщает, какой переменной не хватает.

| Переменная | По умолчанию | Описание |
| --- | --- | --- |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `HTTP_ADDR` | `:8080` | адрес HTTP-сервера |
| `HTTP_READ_TIMEOUT` / `HTTP_WRITE_TIMEOUT` / `HTTP_IDLE_TIMEOUT` | `10s` / `15s` / `60s` | таймауты сервера |
| `HTTP_REQUEST_TIMEOUT` | `10s` | дедлайн контекста запроса (распространяется на MySQL и Redis) |
| `HTTP_SHUTDOWN_TIMEOUT` | `15s` | время на graceful shutdown |
| `MYSQL_HOST`, `MYSQL_PORT` | —, `3306` | адрес MySQL |
| `MYSQL_USER`, `MYSQL_PASSWORD`, `MYSQL_DATABASE` | обязательные | доступ к БД |
| `MYSQL_MAX_OPEN_CONNS` / `MYSQL_MAX_IDLE_CONNS` / `MYSQL_CONN_MAX_LIFETIME` | `20` / `10` / `30m` | пул соединений |
| `REDIS_ADDR` | обязательная | адрес Redis |
| `REDIS_PASSWORD`, `REDIS_DB` | пусто, `0` | доступ к Redis |
| `REDIS_TIMEOUT` | `300ms` | таймаут операций Redis |
| `REDIS_BREAKER_FAILURES` / `REDIS_BREAKER_COOLDOWN` | `5` / `15s` | circuit breaker: сколько ошибок подряд размыкают цепь и через сколько пробовать снова |
| `JWT_SECRET` | обязательная | ключ подписи HS256, не короче 32 байт |
| `JWT_TTL` | `24h` | время жизни токена |
| `CACHE_TASKS_TTL` | `5m` | TTL кеша списка задач |
| `RATE_LIMIT_REQUESTS` / `RATE_LIMIT_WINDOW` | `100` / `1m` | лимит запросов к API с одного IP за окно |
| `MYSQL_ROOT_PASSWORD`, `*_PUBLISHED_PORT` | — | используются только docker-compose |

## Миграции

SQL-миграции лежат в `migrations/`, встраиваются в бинарник и применяются через [goose](https://github.com/pressly/goose)
утилитой `cmd/migrate`:

```bash
go run ./cmd/migrate up
```

Доступные команды: `up`, `down` (откат одной миграции), `status`, `version`. В Compose миграции применяются
автоматически сервисом `migrate` перед стартом приложения. Вручную в контейнере:

```bash
docker compose run --rm migrate
```

## Права доступа

| Действие | owner | admin | member |
| --- | --- | --- | --- |
| Видеть команду, участников, задачи, историю, комментарии | да | да | да |
| Создавать задачи, комментировать | да | да | да |
| Приглашать участников (роль admin или member) | да | да | нет |
| Менять роли участников | да | нет | нет |
| Редактировать любую задачу команды | да | да | нет |
| Редактировать свою задачу (название, описание, статус, исполнитель) | да | да | создатель задачи |
| Менять статус назначенной задачи | да | да | исполнитель |
| Смотреть `/teams/{id}/stats` | да | да | нет |

Ресурсы чужих команд не раскрываются: для пользователя вне команды команда и ее задачи отвечают `404`.
Исполнитель задачи должен состоять в той же команде, иначе `422`. Роль owner через приглашение не выдается
и не может быть изменена.

## Надежность

- **Транзакции.** Создание команды и добавление создателя в участники, создание или обновление задачи вместе
  с записью истории выполняются атомарно (`TxManager.WithinTx`).
- **Конкурентные обновления.** `PUT /tasks/{id}` читает задачу через `SELECT ... FOR UPDATE`, поэтому
  параллельные изменения сериализуются, а история строится от актуального состояния. Дополнительно
  `UPDATE` проверяет `version`, а клиент может передать ожидаемую `version` и получить `409`, если задачу
  уже изменили.
- **Кеш.** Список задач кешируется в Redis на 5 минут по ключу из команды и всех фильтров. Инвалидация —
  через счетчик поколения команды: любое изменение задач увеличивает `tasks:team:{id}:gen`, и старые ключи
  перестают читаться и истекают по TTL. Чтение, начатое до инвалидации, сохраняет результат под старым
  поколением и не может «отравить» кеш.
- **Деградация Redis.** Все обращения к Redis идут через circuit breaker. Если Redis недоступен, список
  задач читается напрямую из MySQL, а rate limiter пропускает запросы.
- **Rate limiting.** Fixed window в Redis по IP-адресу клиента; при превышении `429` и `Retry-After`.
- **Наблюдаемость.** JSON-логи `log/slog` с `request_id` (заголовок `X-Request-ID`), метрика
  `http_request_duration_seconds` по маршруту и статусу.

## SQL-отчет

`GET /api/v1/teams/{id}/stats` считается одним запросом с CTE (`internal/storage/mysql/stats.go`):
задачи по статусам, топ-3 исполнителя по задачам, закрытым за последние 30 дней (`ROW_NUMBER()`),
среднее время закрытия (`TIMESTAMPDIFF`) и количество комментариев по задачам команды. Все подзапросы
ограничены `team_id`, доступ проверяется в сервисном слое.

## Примеры запросов

```bash
curl -s localhost:8080/api/v1/register -d '{"email":"alice@example.com","password":"password123","name":"Alice"}'
curl -s localhost:8080/api/v1/register -d '{"email":"bob@example.com","password":"password123","name":"Bob"}'

TOKEN=$(curl -s localhost:8080/api/v1/login -d '{"email":"alice@example.com","password":"password123"}' | jq -r .access_token)
AUTH="Authorization: Bearer $TOKEN"

curl -s localhost:8080/api/v1/teams -H "$AUTH" -d '{"name":"Backend"}'
curl -s localhost:8080/api/v1/teams -H "$AUTH"
curl -s localhost:8080/api/v1/teams/1/invite -H "$AUTH" -d '{"email":"bob@example.com","role":"member"}'
curl -s localhost:8080/api/v1/teams/1/members -H "$AUTH"
curl -s -X PATCH localhost:8080/api/v1/teams/1/members/2 -H "$AUTH" -d '{"role":"admin"}'

curl -s localhost:8080/api/v1/tasks -H "$AUTH" -d '{"team_id":1,"title":"Implement login","assignee_id":2}'
curl -s "localhost:8080/api/v1/tasks?team_id=1&status=todo&assignee_id=2&limit=20&offset=0" -H "$AUTH"
curl -s "localhost:8080/api/v1/tasks?team_id=1&limit=20&cursor=<next_cursor>" -H "$AUTH"
curl -s -X PUT localhost:8080/api/v1/tasks/1 -H "$AUTH" -d '{"status":"in_progress","version":1}'
curl -s -X PUT localhost:8080/api/v1/tasks/1 -H "$AUTH" -d '{"assignee_id":null}'
curl -s localhost:8080/api/v1/tasks/1/history -H "$AUTH"

curl -s localhost:8080/api/v1/tasks/1/comments -H "$AUTH" -d '{"content":"Looks good"}'
curl -s localhost:8080/api/v1/tasks/1/comments -H "$AUTH"

curl -s localhost:8080/api/v1/teams/1/stats -H "$AUTH"
```

Ошибки возвращаются в едином формате:

```json
{"error": "forbidden: assignee can change only task status"}
```

| Код | Когда |
| --- | --- |
| `400` | некорректное тело или параметры запроса |
| `401` | нет токена, токен недействителен, неверный логин или пароль |
| `403` | недостаточно прав в команде |
| `404` | ресурс не существует или недоступен пользователю |
| `409` | email занят, пользователь уже в команде, конфликт версии задачи |
| `422` | исполнитель не состоит в команде |
| `429` | превышен лимит запросов |

## Тесты и линтер

```bash
make test
```

Unit-тесты правил доступа: `internal/domain` (политики) и `internal/service` (сценарии с фейками репозиториев).

```bash
make test-integration
```

Интеграционный тест SQL-отчета (`internal/storage/mysql/stats_integration_test.go`) поднимает MySQL
через testcontainers, поэтому нужен запущенный Docker.

```bash
make lint
```

## Структура

```
cmd/server             точка входа, сборка зависимостей, graceful shutdown
cmd/migrate            применение миграций
api/                   OpenAPI-спецификация
internal/config        конфигурация из env
internal/domain        сущности, правила доступа, доменные ошибки
internal/service       use cases: auth, teams, tasks
internal/storage/mysql репозитории, транзакции, SQL-отчет
internal/storage/redis кеш задач, rate limiter, circuit breaker
internal/token         выпуск и проверка JWT
internal/transport/http роутинг, middleware, DTO
internal/logging       slog с request_id
migrations/            SQL-миграции
```
