# Развёртывание TaskFlow на сервере

Скрипты в этом каталоге разворачивают стек на удалённом Linux-сервере с
доменом и автоматическим TLS (Let's Encrypt). **Исходный код репозитория не
изменяется**: всё, что зависит от конкретного сервера, генерируется в
`deploy/generated/` и подкладывается через `docker-compose.override.yml`.

## Что нужно подготовить

1. **Сервер**: Linux (Ubuntu 22.04/24.04, Debian 12, Rocky — проверяется
   apt/dnf/yum), **2 vCPU, 4 ГБ RAM**, ~15 ГБ диска, доступ по SSH под root
   или пользователем с `sudo`. Docker поставится автоматически, если его нет.
2. **Локально (Windows)**: Git Bash — там есть `bash`, `ssh`, `scp`, `tar`.
3. Выбрать режим — см. ниже.

## Два режима: `domain` и `ip`

Одного сервера достаточно в обоих случаях. Разница только в TLS.

### `DEPLOY_MODE=domain` — HTTPS (рекомендуется)

Caddy на 80/443 сам получает сертификаты Let's Encrypt. Нужны **две записи
DNS** на IP сервера: `DOMAIN` (UI, `/api`, `/ws`, `/mcp`, `/swagger`) и
`AUTH_DOMAIN` (Keycloak). Открытые порты: 80/tcp, 443/tcp, 443/udp.

**Своего домена нет — это не помеха.** Сервис `sslip.io` резолвит любое имя
вида `1-2-3-4.sslip.io` в IP `1.2.3.4`, регистрировать и настраивать ничего не
надо, а Let's Encrypt выдаёт на такие имена обычный доверенный сертификат.
Для сервера `203.0.113.10`:

```sh
DEPLOY_MODE=domain
DOMAIN=203-0-113-10.sslip.io
AUTH_DOMAIN=auth.203-0-113-10.sslip.io
```

Обратная сторона: имя некрасивое, а работоспособность зависит от чужого
DNS-сервиса. Когда появится свой домен — поменяйте эти две строки и запустите
деплой заново.

### `DEPLOY_MODE=ip` — HTTP по IP, без TLS

Caddy не поднимается, сервисы публикуются напрямую:

| Адрес | Что |
| --- | --- |
| `http://IP` | UI (и `/api`, `/ws` через nginx фронтенда) |
| `http://IP:8080` | REST API, `/mcp`, `/swagger/` |
| `http://IP:8082` | Keycloak |

Чтобы это вообще заработало, скрипт дополнительно ставит
`MCP_INSECURE_HTTP_HOSTS=<IP>` (иначе бэкенд не стартует: он требует HTTPS для
`MCP_PUBLIC_URL`) и меняет `sslRequired` realm на `none` (иначе Keycloak
ответит «HTTPS required» на попытку логина).

**Чем вы платите:** пароль при входе в Keycloak, access-токены и содержимое
задач идут по сети открытым текстом. Любой, кто видит трафик между браузером и
сервером, может прочитать данные и угнать сессию. Это приемлемо для закрытого
стенда или демо и неприемлемо для реальных данных. Если сомневаетесь —
берите `sslip.io` и режим `domain`, он не сложнее.

## Запуск

```sh
cp deploy/deploy.conf.example deploy/deploy.conf
# отредактируйте deploy.conf: SSH_TARGET, DEPLOY_MODE и адреса, логины админов
bash deploy/deploy.sh
```

Скрипт: проверит SSH → зальёт исходники (tar по SSH, без `.git` и
`node_modules`) → на сервере поставит Docker/jq → сгенерирует секреты, `.env`,
оверлей, `Caddyfile` и прод-realm → соберёт образы → поднимет стек и дождётся
healthcheck'ов → приведёт Keycloak к нужной конфигурации → зарегистрирует
MCP-клиент агента → напечатает реквизиты доступа.

Первый прогон — 5–15 минут (сборка Go и Vite).

## Остальные команды

```sh
bash deploy/deploy.sh --sync-only   # только залить исходники
bash deploy/deploy.sh --logs        # логи backend/frontend/keycloak/caddy
bash deploy/deploy.sh --secrets     # показать сгенерированные пароли
bash deploy/deploy.sh --down        # остановить стек
```

Повторный `bash deploy/deploy.sh` — это обновление: секреты и пароли
переиспользуются, база и Keycloak не пересоздаются.

## Что делает прод-оверлей (режим `domain`)

| Изменение | Зачем |
| --- | --- |
| `postgres`, `keycloak`, `frontend`, `backend`: `ports: !reset []` | наружу не торчит ничего, кроме Caddy. Из dev-compose иначе бы публиковались 5432, 8080, 8081, 8082, 9000 |
| `backend`: `127.0.0.1:50051` | gRPC доступен только с самого сервера (через SSH-туннель) |
| `keycloak`: `start` вместо `start-dev`, `KC_PROXY_HEADERS=xforwarded` | production-режим за TLS-терминатором |
| сервис `caddy` | единственная точка входа, авто-TLS, HTTP/3 |
| свой `realm-taskflow.json` | прод-URL, случайный client secret, без демо-юзеров |

Маршрутизация Caddy на основном домене: `/api/*`, `/ws`, `/mcp*`,
`/.well-known/oauth-protected-resource*`, `/swagger*` → backend, всё
остальное → frontend (nginx со сборкой SPA). `auth.DOMAIN` → Keycloak.

В режиме `ip` оверлей проще: Caddy нет, `postgres` по-прежнему не публикуется,
а `frontend`/`backend`/`keycloak` вешаются на 80/8080/8082. Keycloak остаётся
в `start-dev` — production-режим по plain HTTP на публичном адресе только
добавил бы проблем.

## Учётные записи

Демо-пользователи `alice`/`bob`/`vera` не создаются (а если остались от
прошлой установки — удаляются). Вместо них создаются два администратора с
realm-ролью `admin`, логины задаются в `deploy.conf`. Пароли (и пароль консоли
Keycloak) генерируются случайно и лежат на сервере в
`deploy/generated/CREDENTIALS.txt` (0600).

Password grant (`directAccessGrantsEnabled`) у `taskflow-web` выключен —
получить токен можно только через браузерный Authorization Code + PKCE.
Саморегистрация в realm тоже отключена.

### Роль `superadmin`

Обоим администраторам по умолчанию выдаётся realm-роль `superadmin`
(`OWNER_SUPERADMIN` / `AGENT_SUPERADMIN` в `deploy.conf`). Она даёт доступ ко
всем проектам, минуя `project_members`: пользователь видит их в списке,
получает realtime-события и действует как project admin, не будучи
участником.

Роль читается из access-токена, поэтому снятие её в консоли Keycloak
действует со следующего токена и не оставляет за собой данных для чистки —
в отличие от «раздать членство во всех проектах».

Чего роль **не** делает:

- не обходит условия переходов (`author_only`, `assignee_only`,
  `project_owner_only`, `requires_comment`) — это процесс проекта, а не
  контроль доступа, и суперадмин ему подчиняется;
- не делает пользователя назначаемым исполнителем: у него нет строки в
  `project_members`, а исполнителем может быть только участник;
- не меняет владельца проекта.

## MCP для агента (Афина)

Скрипт регистрирует public-клиент `AGENT_MCP_CLIENT_ID` со scope
`taskflow:mcp` и вашим `redirect_uri`. Конфигурация клиента:

```yaml
mcp_servers:
  taskflow:
    url: "https://taskflow.example.com/mcp"
    auth: oauth
    oauth:
      client_id: "athena-taskflow"
      scope: "taskflow:mcp"
      redirect_uri: "http://localhost:8642/api/mcp/oauth/callback/taskflow"
```

Агент логинится под своим пользователем Keycloak и получает ровно его права в
проектах — членство в проекте регистрация клиента не даёт, добавьте агента в
нужные проекты через UI.

Добавить ещё одного агента можно на сервере:

```sh
cd /opt/taskflow && docker compose exec -T \
  -e MCP_CLIENT_ID=another-agent \
  -e MCP_REDIRECT_URIS=https://agent.example.com/callback \
  keycloak sh /opt/keycloak/bin/register-mcp-client.sh
```

## Эксплуатация

```sh
# на сервере, из /opt/taskflow
docker compose ps
docker compose logs -f backend
docker compose restart backend

# бэкап БД
docker compose exec -T postgres pg_dumpall -U taskflow > /root/taskflow-$(date +%F).sql

# gRPC с локальной машины
ssh -L 50051:127.0.0.1:50051 user@server
```

Данные Postgres лежат в volume `taskflow_postgres-data`, сертификаты — в
`taskflow_caddy-data`. `docker compose down` их не удаляет,
`docker compose down -v` — удаляет.

## Известные ограничения

- **Смена адреса после установки** (в том числе переход `ip` → `domain`).
  `.env`, оверлей, Caddyfile и клиент `taskflow-web` скрипт обновит, но `iss`
  в уже выданных токенах — нет; пользователям нужно перелогиниться. База и
  пароли при этом сохраняются.
- **MCP в режиме `ip`.** Многие MCP-клиенты отказываются ходить на
  OAuth-ресурс по plain HTTP независимо от настроек сервера. Если Афина не
  подключается — это первое, что стоит проверить, и повод перейти на `domain`.
- **Realm импортируется только при первом старте Keycloak.** Поэтому
  изменения в `keycloak/realm-taskflow.json` при повторных деплоях
  применяются не импортом, а через `kcadm` (шаг 9 `remote-install.sh`) — и
  только для того, что там перечислено. Новые сущности в realm-файле надо
  либо добавлять в этот шаг, либо импортировать вручную.
- **Сборка идёт на сервере.** На 2 ГБ RAM `npm ci` может уйти в OOM; тогда
  собирайте образы локально и пушьте в registry.
- **Swagger UI открыт публично** на `/swagger/`. Если это нежелательно —
  уберите блок `handle /swagger*` из `deploy/generated/Caddyfile` и
  перезапустите Caddy (учтите: файл перегенерируется при каждом деплое).
