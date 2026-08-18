#!/usr/bin/env bash
# ---------------------------------------------------------------------------
# Выполняется НА СЕРВЕРЕ из каталога проекта (обычно /opt/taskflow).
# Идемпотентен: повторный запуск = обновление стека.
#
# Ничего в исходниках репозитория не меняет. Все server-specific настройки
# генерируются заново при каждом запуске:
#   .env                        (в корне проекта)
#   docker-compose.override.yml (в корне проекта)
#   deploy/generated/Caddyfile
#   deploy/generated/realm-taskflow.json
#   deploy/generated/secrets.env, CREDENTIALS.txt
# ---------------------------------------------------------------------------
set -euo pipefail

log()  { printf '\033[1;36m==>\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33m[!]\033[0m %s\n' "$*"; }
die()  { printf '\033[1;31mОШИБКА:\033[0m %s\n' "$*" >&2; exit 1; }

[ "$(id -u)" -eq 0 ] || die "запускать нужно от root (или через sudo)"
PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$PROJECT_DIR"
GEN="$PROJECT_DIR/deploy/generated"
mkdir -p "$GEN"; chmod 700 "$GEN"

[ -f deploy/deploy.conf ] || die "нет deploy/deploy.conf"
# shellcheck disable=SC1091
. deploy/deploy.conf
DEPLOY_MODE="${DEPLOY_MODE:-domain}"
case "$DEPLOY_MODE" in
  domain)
    : "${DOMAIN:?не задан DOMAIN}" "${AUTH_DOMAIN:?не задан AUTH_DOMAIN}" "${ACME_EMAIL:?не задан ACME_EMAIL}"
    PUBLIC_HOST="$DOMAIN"
    ;;
  ip)
    : "${SERVER_IP:?не задан SERVER_IP}"
    PUBLIC_HOST="$SERVER_IP"
    ;;
  *) die "DEPLOY_MODE должен быть 'domain' или 'ip' (сейчас: $DEPLOY_MODE)" ;;
esac
: "${OWNER_USERNAME:?не задан OWNER_USERNAME}" "${AGENT_USERNAME:?не задан AGENT_USERNAME}"
OWNER_EMAIL="${OWNER_EMAIL:-$OWNER_USERNAME@taskflow.local}"
AGENT_EMAIL="${AGENT_EMAIL:-$AGENT_USERNAME@taskflow.local}"
OWNER_FIRSTNAME="${OWNER_FIRSTNAME:-$OWNER_USERNAME}"; OWNER_LASTNAME="${OWNER_LASTNAME:-Admin}"
AGENT_FIRSTNAME="${AGENT_FIRSTNAME:-$AGENT_USERNAME}"; AGENT_LASTNAME="${AGENT_LASTNAME:-Agent}"
LOG_LEVEL="${LOG_LEVEL:-info}"
EXPOSE_GRPC_LOCALHOST="${EXPOSE_GRPC_LOCALHOST:-true}"
SETUP_FIREWALL="${SETUP_FIREWALL:-false}"
OWNER_SUPERADMIN="${OWNER_SUPERADMIN:-true}"
AGENT_SUPERADMIN="${AGENT_SUPERADMIN:-true}"
ENABLE_AGENT_MCP_CLIENT="${ENABLE_AGENT_MCP_CLIENT:-true}"
AGENT_MCP_CLIENT_ID="${AGENT_MCP_CLIENT_ID:-athena-taskflow}"
AGENT_MCP_REDIRECT_URIS="${AGENT_MCP_REDIRECT_URIS:-}"

# ---------------------------------------------------------------------------
# 1. Системные зависимости
# ---------------------------------------------------------------------------
log "Проверяю системные зависимости…"
if command -v apt-get >/dev/null; then
  PKG_UPDATE="apt-get update -qq"; PKG_INSTALL="apt-get install -y -qq"
elif command -v dnf >/dev/null; then
  PKG_UPDATE=":"; PKG_INSTALL="dnf install -y -q"
elif command -v yum >/dev/null; then
  PKG_UPDATE=":"; PKG_INSTALL="yum install -y -q"
else
  PKG_UPDATE=":"; PKG_INSTALL=""
fi

need_pkgs=()
for bin in curl jq tar; do command -v "$bin" >/dev/null || need_pkgs+=("$bin"); done
if [ "${#need_pkgs[@]}" -gt 0 ]; then
  [ -n "$PKG_INSTALL" ] || die "установите вручную: ${need_pkgs[*]}"
  log "Ставлю пакеты: ${need_pkgs[*]}"
  $PKG_UPDATE >/dev/null; $PKG_INSTALL "${need_pkgs[@]}" >/dev/null
fi

if ! command -v docker >/dev/null; then
  log "Docker не найден — ставлю через get.docker.com…"
  curl -fsSL https://get.docker.com -o /tmp/get-docker.sh
  sh /tmp/get-docker.sh
  systemctl enable --now docker || true
fi
docker compose version >/dev/null 2>&1 || die "нет плагина 'docker compose' v2"

COMPOSE_VER="$(docker compose version --short | sed 's/^v//')"
ver_ge() { [ "$(printf '%s\n%s\n' "$2" "$1" | sort -V | head -1)" = "$2" ]; }
ver_ge "$COMPOSE_VER" "2.24.0" \
  || die "нужен docker compose >= 2.24.0 (сейчас $COMPOSE_VER): в оверлее используются теги !reset/!override"

MEM_MB="$(awk '/MemTotal/ {print int($2/1024)}' /proc/meminfo)"
[ "$MEM_MB" -ge 3500 ] || warn "на сервере ${MEM_MB} МБ RAM; Keycloak + Postgres + сборка требуют ~4 ГБ. Возможен OOM при npm/go build."

# ---------------------------------------------------------------------------
# 2. Секреты (генерируются один раз и переиспользуются)
# ---------------------------------------------------------------------------
# Роль superadmin выдаёт доступ ко всем проектам в обход project_members.
OWNER_ROLES_JSON='["admin","member"]'; OWNER_ROLES="admin member"
AGENT_ROLES_JSON='["admin","member"]'; AGENT_ROLES="admin member"
if [ "$OWNER_SUPERADMIN" = "true" ]; then
  OWNER_ROLES_JSON='["admin","member","superadmin"]'; OWNER_ROLES="admin member superadmin"
fi
if [ "$AGENT_SUPERADMIN" = "true" ]; then
  AGENT_ROLES_JSON='["admin","member","superadmin"]'; AGENT_ROLES="admin member superadmin"
fi

gen_secret() { LC_ALL=C tr -dc 'A-Za-z0-9' </dev/urandom | head -c 32; }
if [ ! -f "$GEN/secrets.env" ]; then
  log "Генерирую секреты…"
  ( umask 077
    cat > "$GEN/secrets.env" <<SECRETS
POSTGRES_PASSWORD=$(gen_secret)
KEYCLOAK_ADMIN_PASSWORD=$(gen_secret)
KEYCLOAK_DIRECTORY_CLIENT_SECRET=$(gen_secret)
OWNER_PASSWORD=$(gen_secret)
AGENT_PASSWORD=$(gen_secret)
SECRETS
  )
else
  log "Использую существующие секреты из deploy/generated/secrets.env"
fi
chmod 600 "$GEN/secrets.env"
# shellcheck disable=SC1091
. "$GEN/secrets.env"

if [ "$DEPLOY_MODE" = domain ]; then
  APP_URL="https://$DOMAIN"
  AUTH_URL="https://$AUTH_DOMAIN"
  MCP_URL="$APP_URL/mcp"
  WS_URL="wss://$DOMAIN/ws"
  INSECURE_HOSTS=""
  SSL_REQUIRED="external"
else
  # Без TLS Keycloak на 8082, API/MCP на 8080, UI на 80 — всё по IP.
  APP_URL="http://$SERVER_IP"
  AUTH_URL="http://$SERVER_IP:8082"
  MCP_URL="http://$SERVER_IP:8080/mcp"
  WS_URL="ws://$SERVER_IP/ws"
  # Бэкенд иначе откажется стартовать: MCP_PUBLIC_URL по HTTP разрешён
  # только для localhost или явно перечисленных хостов.
  INSECURE_HOSTS="$SERVER_IP"
  # Realm по умолчанию требует HTTPS для внешних запросов — снимаем,
  # иначе Keycloak ответит "HTTPS required" на логин по IP.
  SSL_REQUIRED="none"
  warn "режим 'ip': TLS нет, пароли и токены идут по сети открытым текстом."
fi
# Backend отдаёт discovery и Swagger на своём хосте — в режиме domain это
# основной домен, в режиме ip — порт 8080.
BACKEND_BASE="${MCP_URL%/mcp}"
WELLKNOWN_URL="$BACKEND_BASE/.well-known/oauth-protected-resource/mcp"
SWAGGER_URL="$BACKEND_BASE/swagger/"

# ---------------------------------------------------------------------------
# 3. .env
# ---------------------------------------------------------------------------
log "Генерирую .env…"
( umask 077
  cat > "$PROJECT_DIR/.env" <<ENVFILE
# СГЕНЕРИРОВАНО deploy/remote-install.sh — правки будут перезаписаны.
POSTGRES_USER=taskflow
POSTGRES_PASSWORD=$POSTGRES_PASSWORD
POSTGRES_DB=taskflow
DATABASE_URL=postgres://taskflow:$POSTGRES_PASSWORD@postgres:5432/taskflow?sslmode=disable

KEYCLOAK_ADMIN=admin
KEYCLOAK_ADMIN_PASSWORD=$KEYCLOAK_ADMIN_PASSWORD
KEYCLOAK_CLIENT_ID=taskflow-web
KEYCLOAK_PUBLIC_URL=$AUTH_URL
KEYCLOAK_ISSUER_URL=$AUTH_URL/realms/taskflow
KEYCLOAK_JWKS_URL=http://keycloak:8080/realms/taskflow/protocol/openid-connect/certs
KEYCLOAK_ADMIN_URL=http://keycloak:8080
KEYCLOAK_REALM=taskflow
KEYCLOAK_DIRECTORY_CLIENT_ID=taskflow-backend
KEYCLOAK_DIRECTORY_CLIENT_SECRET=$KEYCLOAK_DIRECTORY_CLIENT_SECRET
KEYCLOAK_SKIP_AUDIENCE=false

MCP_ENABLED=true
MCP_PUBLIC_URL=$MCP_URL
MCP_AUDIENCE=$MCP_URL
MCP_ALLOWED_ORIGINS=$APP_URL
MCP_INSECURE_HTTP_HOSTS=$INSECURE_HOSTS

HTTP_ADDR=:8080
GRPC_ADDR=:50051
CORS_ALLOWED_ORIGINS=$APP_URL
MIGRATIONS_URL=file://migrations
SHUTDOWN_TIMEOUT=10s
LOG_LEVEL=$LOG_LEVEL
HEALTHCHECK_URL=http://127.0.0.1:8080/healthz

VITE_API_URL=/api/v1
VITE_WS_URL=$WS_URL
VITE_KEYCLOAK_URL=$AUTH_URL
VITE_KEYCLOAK_REALM=taskflow
VITE_KEYCLOAK_CLIENT_ID=taskflow-web
ENVFILE
)
chmod 600 "$PROJECT_DIR/.env"

# ---------------------------------------------------------------------------
# 4. docker-compose.override.yml
# ---------------------------------------------------------------------------
log "Генерирую docker-compose.override.yml (режим: $DEPLOY_MODE)…"
GRPC_PORT=""
[ "$EXPOSE_GRPC_LOCALHOST" = "true" ] && GRPC_PORT=', "127.0.0.1:50051:50051"'

if [ "$DEPLOY_MODE" = ip ]; then
  # Без Caddy: сервисы публикуются напрямую. Keycloak остаётся в start-dev —
  # production-режим по plain HTTP на публичном адресе только добавит проблем.
  BACKEND_PORTS="ports: !override [\"8080:8080\"${GRPC_PORT}]"
  cat > "$PROJECT_DIR/docker-compose.override.yml" <<OVERRIDE_IP
# СГЕНЕРИРОВАНО deploy/remote-install.sh — правки будут перезаписаны.
# Режим 'ip': без TLS и без обратного прокси.
services:
  postgres:
    ports: !reset []

  keycloak:
    ports: !override ["8082:8080"]
    volumes: !override
      - ./deploy/generated/realm-taskflow.json:/opt/keycloak/data/import/realm-taskflow.json:ro
      - ./keycloak/register-mcp-client.sh:/opt/keycloak/bin/register-mcp-client.sh:ro

  backend:
    $BACKEND_PORTS

  frontend:
    ports: !override ["80:80"]
OVERRIDE_IP
  rm -f "$GEN/Caddyfile"
else

if [ "$EXPOSE_GRPC_LOCALHOST" = "true" ]; then
  BACKEND_PORTS='ports: !override ["127.0.0.1:50051:50051"]'
else
  BACKEND_PORTS='ports: !reset []'
fi
cat > "$PROJECT_DIR/docker-compose.override.yml" <<OVERRIDE
# СГЕНЕРИРОВАНО deploy/remote-install.sh — правки будут перезаписаны.
# Прод-оверлей: убирает публикацию внутренних портов наружу, переводит
# Keycloak в production-режим за прокси и добавляет Caddy с авто-TLS.
services:
  postgres:
    ports: !reset []

  keycloak:
    command: ["start", "--import-realm"]
    ports: !reset []
    environment:
      KC_PROXY_HEADERS: xforwarded
      KC_HTTP_ENABLED: "true"
    volumes: !override
      - ./deploy/generated/realm-taskflow.json:/opt/keycloak/data/import/realm-taskflow.json:ro
      - ./keycloak/register-mcp-client.sh:/opt/keycloak/bin/register-mcp-client.sh:ro

  backend:
    $BACKEND_PORTS

  frontend:
    ports: !reset []

  caddy:
    image: caddy:2-alpine
    restart: unless-stopped
    ports:
      - "80:80"
      - "443:443"
      - "443:443/udp"
    volumes:
      - ./deploy/generated/Caddyfile:/etc/caddy/Caddyfile:ro
      - caddy-data:/data
      - caddy-config:/config
    depends_on:
      frontend:
        condition: service_healthy
      keycloak:
        condition: service_healthy

volumes:
  caddy-data:
  caddy-config:
OVERRIDE

# ---------------------------------------------------------------------------
# 5. Caddyfile
# ---------------------------------------------------------------------------
log "Генерирую Caddyfile…"
cat > "$GEN/Caddyfile" <<CADDY
# СГЕНЕРИРОВАНО deploy/remote-install.sh — правки будут перезаписаны.
{
	email $ACME_EMAIL
}

$DOMAIN {
	encode zstd gzip

	handle /api/* {
		reverse_proxy backend:8080
	}
	handle /ws {
		reverse_proxy backend:8080
	}
	handle /mcp* {
		reverse_proxy backend:8080
	}
	handle /.well-known/oauth-protected-resource* {
		reverse_proxy backend:8080
	}
	handle /swagger* {
		reverse_proxy backend:8080
	}
	handle {
		reverse_proxy frontend:80
	}
}

$AUTH_DOMAIN {
	reverse_proxy keycloak:8080
}
CADDY
chmod 644 "$GEN/Caddyfile"

fi   # конец ветки DEPLOY_MODE=domain

# ---------------------------------------------------------------------------
# 6. realm-taskflow.json — прод-версия импортируемого realm:
#    прод-URL, сгенерированный client secret, без демо-пользователей,
#    вместо них два администратора.
# ---------------------------------------------------------------------------
log "Генерирую прод-realm для Keycloak…"
jq \
  --arg app "$APP_URL" \
  --arg mcpaud "$MCP_URL" \
  --arg besecret "$KEYCLOAK_DIRECTORY_CLIENT_SECRET" \
  --arg sslreq "$SSL_REQUIRED" \
  --arg ou "$OWNER_USERNAME" --arg oe "$OWNER_EMAIL" --arg of "$OWNER_FIRSTNAME" --arg ol "$OWNER_LASTNAME" --arg op "$OWNER_PASSWORD" \
  --arg au "$AGENT_USERNAME" --arg ae "$AGENT_EMAIL" --arg af "$AGENT_FIRSTNAME" --arg al "$AGENT_LASTNAME" --arg ap "$AGENT_PASSWORD" \
  '
  def mkadmin($u; $e; $f; $l; $p; $roles):
    { username: $u, email: $e, firstName: $f, lastName: $l,
      enabled: true, emailVerified: true,
      credentials: [{ type: "password", value: $p, temporary: false }],
      realmRoles: $roles };

  .roles.realm += [{ name: "superadmin",
                     description: "Unrestricted access to every project, bypassing project_members" }]
  | .registrationAllowed = false
  | .sslRequired = $sslreq
  | .clients = [ .clients[]
      | if .clientId == "taskflow-web" then
          .rootUrl = $app
          | .baseUrl = $app
          | .redirectUris = [$app + "/*"]
          | .webOrigins = [$app]
          | .directAccessGrantsEnabled = false
          | .attributes."post.logout.redirect.uris" = ($app + "/*")
        elif .clientId == "taskflow-backend" then
          .secret = $besecret
        else . end ]
  | .clientScopes = [ .clientScopes[]
      | if .name == "taskflow:mcp" then
          .protocolMappers = [ .protocolMappers[]
            | if .name == "taskflow-mcp-audience"
              then .config."included.custom.audience" = $mcpaud
              else . end ]
        else . end ]
  | .users = [ .users[] | select(has("serviceAccountClientId")) ]
              + [ mkadmin($ou; $oe; $of; $ol; $op; $oroles),
                  mkadmin($au; $ae; $af; $al; $ap; $aroles) ]
  ' keycloak/realm-taskflow.json > "$GEN/realm-taskflow.json"
chmod 644 "$GEN/realm-taskflow.json"
jq -e '(.users | length) == 3 and ([.users[].username] | index("alice") | not)' \
   "$GEN/realm-taskflow.json" >/dev/null \
  || die "не удалось сгенерировать realm — проверьте keycloak/realm-taskflow.json"

# ---------------------------------------------------------------------------
# 7. Firewall (опционально)
# ---------------------------------------------------------------------------
if [ "$SETUP_FIREWALL" = "true" ] && command -v ufw >/dev/null; then
  if [ "$DEPLOY_MODE" = domain ]; then
    FW_PORTS="22/tcp 80/tcp 443/tcp 443/udp"
  else
    FW_PORTS="22/tcp 80/tcp 8080/tcp 8082/tcp"
  fi
  log "Настраиваю ufw ($FW_PORTS)…"
  for p in $FW_PORTS; do ufw allow "$p" >/dev/null || true; done
  ufw --force enable >/dev/null || true
fi

# ---------------------------------------------------------------------------
# 8. Сборка и запуск
# ---------------------------------------------------------------------------
log "Собираю образы и поднимаю стек (первый запуск — долго)…"
docker compose build
if ! docker compose up -d --wait --wait-timeout 600; then
  warn "не все сервисы стали healthy — статус и последние логи ниже"
  docker compose ps
  docker compose logs --tail=60 backend keycloak caddy || true
  die "стек не поднялся"
fi
docker compose ps

# ---------------------------------------------------------------------------
# 9. Согласование realm через kcadm.
#    Импорт realm выполняется Keycloak только при первом старте, поэтому при
#    повторных деплоях (смена домена, новые админы) правим через Admin API.
# ---------------------------------------------------------------------------
log "Согласую конфигурацию Keycloak…"
kc() { docker compose exec -T keycloak /opt/keycloak/bin/kcadm.sh "$@"; }
kc config credentials --server http://localhost:8080 --realm master \
   --user admin --password "$KEYCLOAK_ADMIN_PASSWORD" >/dev/null \
  || die "не удалось войти в Keycloak как admin. Пароль bootstrap-администратора
       применяется только при ПЕРВОМ старте Keycloak: если deploy/generated/secrets.env
       был удалён или заменён, а том с БД сохранился, пароли разошлись.
       Восстановите старый secrets.env либо смените пароль admin в консоли Keycloak."

client_uuid() {
  kc get clients -r taskflow -q "clientId=$1" --fields id --format csv --noquotes 2>/dev/null | tr -d '\r' | sed -n '1p'
}
user_uuid() {
  kc get users -r taskflow -q "username=$1" -q exact=true --fields id --format csv --noquotes 2>/dev/null | tr -d '\r' | sed -n '1p'
}

kc update realms/taskflow -s "sslRequired=$SSL_REQUIRED" -s 'registrationAllowed=false' >/dev/null

WEB_ID="$(client_uuid taskflow-web)"
[ -n "$WEB_ID" ] || die "в realm 'taskflow' не найден клиент taskflow-web"
kc update "clients/$WEB_ID" -r taskflow \
  -s "rootUrl=$APP_URL" \
  -s "baseUrl=$APP_URL" \
  -s "redirectUris=[\"$APP_URL/*\"]" \
  -s "webOrigins=[\"$APP_URL\"]" \
  -s 'directAccessGrantsEnabled=false' \
  -s "attributes.\"post.logout.redirect.uris\"=$APP_URL/*" \
  -s 'attributes."pkce.code.challenge.method"=S256' >/dev/null

BE_ID="$(client_uuid taskflow-backend)"
if [ -n "$BE_ID" ]; then
  kc update "clients/$BE_ID" -r taskflow -s "secret=$KEYCLOAK_DIRECTORY_CLIENT_SECRET" >/dev/null
  kc add-roles -r taskflow --uusername service-account-taskflow-backend \
     --cclientid realm-management --rolename query-users --rolename view-users >/dev/null 2>&1 || true
fi

# Роль superadmin: импорт realm заводит её только при первом старте Keycloak,
# поэтому при обновлении уже существующей установки создаём её здесь.
if ! kc get roles/superadmin -r taskflow >/dev/null 2>&1; then
  log "  создаю realm-роль 'superadmin'"
  kc create roles -r taskflow \
    -s name=superadmin \
    -s 'description=Unrestricted access to every project, bypassing project_members' >/dev/null
fi

ensure_admin() { # username email firstName lastName password "role role …"
  local u="$1" e="$2" f="$3" l="$4" p="$5" roles="$6" id role args
  id="$(user_uuid "$u")"
  if [ -z "$id" ]; then
    log "  создаю администратора '$u' (роли: $roles)"
    kc create users -r taskflow -i \
      -s "username=$u" -s enabled=true -s emailVerified=true \
      -s "email=$e" -s "firstName=$f" -s "lastName=$l" >/dev/null
    kc set-password -r taskflow --username "$u" --new-password "$p" >/dev/null
  else
    log "  администратор '$u' уже существует — пароль не меняю"
  fi
  args=""
  for role in $roles; do args="$args --rolename $role"; done
  # shellcheck disable=SC2086
  kc add-roles -r taskflow --uusername "$u" $args >/dev/null 2>&1 || true
}
ensure_admin "$OWNER_USERNAME" "$OWNER_EMAIL" "$OWNER_FIRSTNAME" "$OWNER_LASTNAME" "$OWNER_PASSWORD" "$OWNER_ROLES"
ensure_admin "$AGENT_USERNAME" "$AGENT_EMAIL" "$AGENT_FIRSTNAME" "$AGENT_LASTNAME" "$AGENT_PASSWORD" "$AGENT_ROLES"

for demo in alice bob vera; do
  demo_id="$(user_uuid "$demo")"
  if [ -n "$demo_id" ]; then
    warn "удаляю демо-пользователя '$demo'"
    kc delete "users/$demo_id" -r taskflow >/dev/null 2>&1 || true
  fi
done

# ---------------------------------------------------------------------------
# 10. MCP-клиент агента (public OAuth + PKCE)
# ---------------------------------------------------------------------------
if [ "$ENABLE_AGENT_MCP_CLIENT" = "true" ] && [ -n "$AGENT_MCP_REDIRECT_URIS" ]; then
  log "Регистрирую MCP-клиент '$AGENT_MCP_CLIENT_ID'…"
  docker compose exec -T \
    -e MCP_CLIENT_ID="$AGENT_MCP_CLIENT_ID" \
    -e MCP_REDIRECT_URIS="$AGENT_MCP_REDIRECT_URIS" \
    keycloak sh /opt/keycloak/bin/register-mcp-client.sh \
    || warn "MCP-клиент не зарегистрирован (redirect URI должен быть https:// или http://localhost:* / http://127.0.0.1:*)"
elif [ "$ENABLE_AGENT_MCP_CLIENT" = "true" ]; then
  warn "AGENT_MCP_REDIRECT_URIS пуст — MCP-клиент агента не регистрировался"
fi

# ---------------------------------------------------------------------------
# 11. Проверки и итог
# ---------------------------------------------------------------------------
log "Проверяю публичные адреса (ожидается 200/302/401)…"
check() {
  local code
  code="$(curl -s -o /dev/null -w '%{http_code}' --max-time 25 "$1" || echo 000)"
  printf '    %-62s %s\n' "$1" "$code"
}
check "$APP_URL/"
check "$AUTH_URL/realms/taskflow/.well-known/openid-configuration"
check "$WELLKNOWN_URL"

( umask 077
  cat > "$GEN/CREDENTIALS.txt" <<CREDS
TaskFlow — реквизиты доступа ($(date -u '+%Y-%m-%d %H:%M UTC'))
================================================================
Режим развёртывания    $DEPLOY_MODE
Приложение (UI)        $APP_URL
Keycloak               $AUTH_URL
Swagger UI             $SWAGGER_URL
MCP endpoint           $MCP_URL

Консоль Keycloak ($AUTH_URL/admin)
  логин    admin
  пароль   $KEYCLOAK_ADMIN_PASSWORD

Администраторы TaskFlow (realm taskflow)
  $OWNER_USERNAME / $OWNER_PASSWORD   ($OWNER_EMAIL)
    realm-роли: $OWNER_ROLES
  $AGENT_USERNAME / $AGENT_PASSWORD   ($AGENT_EMAIL)
    realm-роли: $AGENT_ROLES

  Роль superadmin даёт доступ ко всем проектам без записи в project_members.
  Снять её можно в консоли Keycloak — подействует со следующего токена.

MCP-клиент агента
  url            $MCP_URL
  client_id      $AGENT_MCP_CLIENT_ID
  scope          taskflow:mcp
  redirect_uri   $AGENT_MCP_REDIRECT_URIS

Служебное
  PostgreSQL              taskflow / $POSTGRES_PASSWORD (доступен только внутри compose-сети)
  taskflow-backend secret $KEYCLOAK_DIRECTORY_CLIENT_SECRET
CREDS
)
chmod 600 "$GEN/CREDENTIALS.txt"

echo
cat "$GEN/CREDENTIALS.txt"
echo
log "Готово. Реквизиты сохранены в $GEN/CREDENTIALS.txt (права 0600)."
