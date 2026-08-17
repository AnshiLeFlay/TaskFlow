#!/bin/sh
set -eu

: "${MCP_CLIENT_ID:?MCP_CLIENT_ID is required}"
: "${MCP_REDIRECT_URIS:?MCP_REDIRECT_URIS is required (comma-separated)}"

MCP_REALM="${KEYCLOAK_REALM:-taskflow}"
MCP_SERVER="${KEYCLOAK_SERVER_URL:-http://localhost:8080}"
MCP_ADMIN_REALM="${KEYCLOAK_ADMIN_REALM:-master}"
MCP_ADMIN_USER="${KEYCLOAK_ADMIN:-${KC_BOOTSTRAP_ADMIN_USERNAME:-admin}}"
MCP_ADMIN_SECRET="${KEYCLOAK_ADMIN_PASSWORD:-${KC_BOOTSTRAP_ADMIN_PASSWORD:-admin}}"
MCP_RESOURCE_AUDIENCE="${MCP_AUDIENCE:-http://localhost:8080/mcp}"
KCADM=/opt/keycloak/bin/kcadm.sh

redirect_json=""
old_ifs=$IFS
IFS=','
for redirect_uri in $MCP_REDIRECT_URIS; do
  case "$redirect_uri" in
    https://*|http://localhost:*|http://127.0.0.1:*) ;;
    *) echo "redirect URI must use HTTPS or a localhost loopback address: $redirect_uri" >&2; exit 2 ;;
  esac
  [ -z "$redirect_json" ] || redirect_json="$redirect_json,"
  redirect_json="${redirect_json}\"${redirect_uri}\""
done
IFS=$old_ifs
redirect_json="[$redirect_json]"

$KCADM config credentials --server "$MCP_SERVER" --realm "$MCP_ADMIN_REALM" --user "$MCP_ADMIN_USER" --password "$MCP_ADMIN_SECRET" >/dev/null

scope_id=$($KCADM get client-scopes -r "$MCP_REALM" --fields id,name --format csv --noquotes | grep ',taskflow:mcp$' | cut -d, -f1 | sed -n '1p')
if [ -z "$scope_id" ]; then
  scope_id=$($KCADM create client-scopes -r "$MCP_REALM" -i \
    -s name='taskflow:mcp' \
    -s description='Access TaskFlow through its MCP protected resource' \
    -s protocol='openid-connect' \
    -s 'attributes={"include.in.token.scope":"true","display.on.consent.screen":"true","consent.screen.text":"Manage TaskFlow through an AI agent"}')
fi

mapper_id=$($KCADM get "client-scopes/$scope_id/protocol-mappers/models" -r "$MCP_REALM" --fields id,name --format csv --noquotes | grep ',taskflow-mcp-audience$' | cut -d, -f1 | sed -n '1p')
if [ -z "$mapper_id" ]; then
  mapper_id=$($KCADM create "client-scopes/$scope_id/protocol-mappers/models" -r "$MCP_REALM" -i \
    -s name='taskflow-mcp-audience' \
    -s protocol='openid-connect' \
    -s protocolMapper='oidc-audience-mapper' \
    -s 'consentRequired=false' \
    -s "config.\"included.custom.audience\"=$MCP_RESOURCE_AUDIENCE" \
    -s 'config."id.token.claim"=false' \
    -s 'config."access.token.claim"=true')
else
  $KCADM update "client-scopes/$scope_id/protocol-mappers/models/$mapper_id" -r "$MCP_REALM" \
    -s name='taskflow-mcp-audience' \
    -s protocol='openid-connect' \
    -s protocolMapper='oidc-audience-mapper' \
    -s 'consentRequired=false' \
    -s "config.\"included.custom.audience\"=$MCP_RESOURCE_AUDIENCE" \
    -s 'config."id.token.claim"=false' \
    -s 'config."access.token.claim"=true' >/dev/null
fi

client_id=$($KCADM get clients -r "$MCP_REALM" -q "clientId=$MCP_CLIENT_ID" --fields id --format csv --noquotes | sed -n '1p')
if [ -z "$client_id" ]; then
  client_id=$($KCADM create clients -r "$MCP_REALM" -i \
    -s "clientId=$MCP_CLIENT_ID" \
    -s enabled=true \
    -s protocol=openid-connect \
    -s publicClient=true \
    -s serviceAccountsEnabled=false \
    -s standardFlowEnabled=true \
    -s directAccessGrantsEnabled=false \
    -s implicitFlowEnabled=false \
    -s "redirectUris=$redirect_json" \
    -s 'attributes."pkce.code.challenge.method"=S256')
else
  $KCADM update "clients/$client_id" -r "$MCP_REALM" \
    -s enabled=true \
    -s protocol=openid-connect \
    -s publicClient=true \
    -s serviceAccountsEnabled=false \
    -s standardFlowEnabled=true \
    -s directAccessGrantsEnabled=false \
    -s implicitFlowEnabled=false \
    -s "redirectUris=$redirect_json" \
    -s 'attributes."pkce.code.challenge.method"=S256' >/dev/null
fi

$KCADM update "clients/$client_id/optional-client-scopes/$scope_id" -r "$MCP_REALM" -n >/dev/null
echo "MCP OAuth client '$MCP_CLIENT_ID' is ready for: $MCP_REDIRECT_URIS"
