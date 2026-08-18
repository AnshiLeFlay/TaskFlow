#!/usr/bin/env bash
# ---------------------------------------------------------------------------
# TaskFlow — деплой с Windows (Git Bash) / Linux / macOS на удалённый сервер.
#
#   bash deploy/deploy.sh                 # синхронизировать и развернуть
#   bash deploy/deploy.sh --sync-only     # только залить исходники
#   bash deploy/deploy.sh --logs          # смотреть логи на сервере
#   bash deploy/deploy.sh --down          # остановить стек
#   bash deploy/deploy.sh --secrets       # показать сгенерированные пароли
#
# Исходный код репозитория не изменяется: всё, что зависит от сервера
# (.env, docker-compose.override.yml, Caddyfile, realm), генерируется
# на сервере в deploy/generated/.
# ---------------------------------------------------------------------------
set -euo pipefail

# Git Bash: не превращать /opt/taskflow в C:/Program Files/...
export MSYS_NO_PATHCONV=1
export MSYS2_ARG_CONV_EXCL='*'

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
CONF_FILE="$SCRIPT_DIR/deploy.conf"
ACTION=deploy

while [ $# -gt 0 ]; do
  case "$1" in
    --config)    CONF_FILE="$2"; shift 2 ;;
    --sync-only) ACTION=sync;    shift ;;
    --logs)      ACTION=logs;    shift ;;
    --down)      ACTION=down;    shift ;;
    --secrets)   ACTION=secrets; shift ;;
    -h|--help)   sed -n '2,16p' "$0"; exit 0 ;;
    *) echo "Неизвестный аргумент: $1" >&2; exit 2 ;;
  esac
done

log()  { printf '\033[1;36m==>\033[0m %s\n' "$*"; }
die()  { printf '\033[1;31mОШИБКА:\033[0m %s\n' "$*" >&2; exit 1; }

[ -f "$CONF_FILE" ] || die "нет файла $CONF_FILE. Скопируйте deploy/deploy.conf.example и заполните."
# shellcheck disable=SC1090
. "$CONF_FILE"

DEPLOY_MODE="${DEPLOY_MODE:-domain}"
case "$DEPLOY_MODE" in
  domain) REQUIRED="SSH_TARGET REMOTE_DIR DOMAIN AUTH_DOMAIN ACME_EMAIL OWNER_USERNAME AGENT_USERNAME" ;;
  ip)     REQUIRED="SSH_TARGET REMOTE_DIR SERVER_IP OWNER_USERNAME AGENT_USERNAME" ;;
  *)      die "DEPLOY_MODE должен быть 'domain' или 'ip' (сейчас: $DEPLOY_MODE)" ;;
esac
for var in $REQUIRED; do
  [ -n "${!var:-}" ] || die "в $CONF_FILE не задан $var (режим $DEPLOY_MODE)"
done
SSH_PORT="${SSH_PORT:-22}"

SSH_OPTS=(-p "$SSH_PORT" -o StrictHostKeyChecking=accept-new)
remote() { ssh "${SSH_OPTS[@]}" "$SSH_TARGET" "$@"; }
# Выполнить команду на сервере от root: напрямую, если уже root, иначе через sudo.
SUDO_PREFIX='if [ "$(id -u)" -eq 0 ]; then SUDO=""; else SUDO="sudo"; fi;'

command -v ssh >/dev/null || die "не найден ssh (в Windows используйте Git Bash)"

case "$ACTION" in
  logs)    exec ssh -t "${SSH_OPTS[@]}" "$SSH_TARGET" \
             "$SUDO_PREFIX cd '$REMOTE_DIR' && \$SUDO docker compose logs -f --tail=200 backend frontend keycloak caddy" ;;
  down)    remote "$SUDO_PREFIX cd '$REMOTE_DIR' && \$SUDO docker compose down"; log "стек остановлен"; exit 0 ;;
  secrets) remote "$SUDO_PREFIX \$SUDO cat '$REMOTE_DIR/deploy/generated/CREDENTIALS.txt'"; exit 0 ;;
esac

# --- 1. проверка доступности сервера ---------------------------------------
log "Проверяю SSH-доступ к $SSH_TARGET…"
remote 'echo "хост: $(hostname), пользователь: $(id -un)"' || die "не могу подключиться по SSH"

# --- 2. заливка исходников --------------------------------------------------
log "Заливаю исходники в $SSH_TARGET:$REMOTE_DIR …"
EXCLUDES=(
  --exclude=./.git
  --exclude=./.env
  --exclude=./deploy/deploy.conf
  --exclude=./deploy/generated
  --exclude=./frontend/node_modules
  --exclude=./frontend/dist
  --exclude=./e2e/node_modules
  --exclude=./e2e/test-results
  --exclude=./e2e/playwright-report
  --exclude=./e2e/blob-report
  --exclude=./backend/bin
  --exclude=./backend/coverage.out
  --exclude='./backend;C'
  --exclude=./postgres-data
)
tar -czf - -C "$ROOT_DIR" "${EXCLUDES[@]}" . \
  | remote "mkdir -p '$REMOTE_DIR' && tar -xzf - -C '$REMOTE_DIR'"

# deploy.conf передаём отдельно (он исключён из архива, чтобы не затирался)
scp -P "$SSH_PORT" -o StrictHostKeyChecking=accept-new -q \
    "$CONF_FILE" "$SSH_TARGET:$REMOTE_DIR/deploy/deploy.conf"

if [ "$ACTION" = sync ]; then
  log "Готово (--sync-only): исходники залиты, стек не пересобирался."
  exit 0
fi

# --- 3. установка на сервере ------------------------------------------------
log "Запускаю установку на сервере (сборка образов может занять 5–15 минут)…"
ssh -t "${SSH_OPTS[@]}" "$SSH_TARGET" \
  "cd '$REMOTE_DIR' && if [ \"\$(id -u)\" -eq 0 ]; then bash deploy/remote-install.sh; else sudo -E bash deploy/remote-install.sh; fi"

log "Деплой завершён."
if [ "$DEPLOY_MODE" = domain ]; then
  printf '    UI:       https://%s\n' "$DOMAIN"
  printf '    Keycloak: https://%s\n' "$AUTH_DOMAIN"
else
  printf '    UI:       http://%s\n'      "$SERVER_IP"
  printf '    Keycloak: http://%s:8082\n' "$SERVER_IP"
  printf '    API/MCP:  http://%s:8080\n' "$SERVER_IP"
fi
printf '    Пароли:   bash deploy/deploy.sh --secrets\n'
