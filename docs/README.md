# TaskFlow documentation

TaskFlow is a deliberately small but complete project-management service. A
project contains members and one or more Kanban boards. Boards own ordered
statuses and configurable transition rules; tasks can move only when the rule
for that exact status pair exists and every configured condition passes.

## Quick start

Prerequisites: Docker Engine with Docker Compose v2 and at least 4 GB of free
memory.

```sh
cp .env.example .env
docker compose up --build
```

The first start imports the `taskflow` realm and runs PostgreSQL migrations.
Wait until `docker compose ps` reports all services healthy.

| Service | URL / address |
| --- | --- |
| Vue application | <http://localhost:8081> |
| REST API | <http://localhost:8080/api/v1> |
| MCP Streamable HTTP | <http://localhost:8080/mcp> |
| Swagger UI (annotation-generated contract) | <http://localhost:8080/swagger/> |
| Reviewed OpenAPI 3 YAML | <http://localhost:8080/swagger/openapi.yaml> |
| WebSocket | `ws://localhost:8080/ws` |
| gRPC | `localhost:50051` |
| Keycloak | <http://localhost:8082> |
| Backend liveness | <http://localhost:8080/healthz> |
| Backend readiness | <http://localhost:8080/readyz> |

Development credentials imported with the realm:

| Purpose | Username | Password | Realm role / stable subject |
| --- | --- | --- | --- |
| Project owner | `alice` | `alice` | `admin` / `11111111-1111-4111-8111-111111111111` |
| Member | `bob` | `bob` | `member` / `22222222-2222-4222-8222-222222222222` |
| Viewer | `vera` | `vera` | `viewer` / `33333333-3333-4333-8333-333333333333` |
| Keycloak console | `admin` | `admin` | bootstrap administrator |

These credentials and direct access grants are development conveniences. Change
them, disable password grants and enable TLS before deploying anywhere shared.

## Authentication and project authorization

The browser uses Keycloak Authorization Code + PKCE. The backend accepts only a
Bearer access token whose signature, issuer, audience/client, expiry and claims
are valid. WebSocket clients supply the same token as the `token` query
parameter. The Vue shell opens one global stream immediately after login;
membership is re-checked as events are delivered, so notifications continue on
the Projects page and membership changes do not require a reconnect. Realm roles identify the general type of account, while the
authoritative role inside a particular project is stored in
`project_members` (`admin`, `member`, or `viewer`). A project creator is inserted
as its `admin` and remains its owner. The backend uses the least-privilege
`taskflow-backend` Keycloak service account (`query-users`, `view-users`) to
populate the user selector; project membership itself is still stored only in
TaskFlow.

For command-line exploration, obtain a development token:

```sh
curl -s -X POST \
  http://localhost:8082/realms/taskflow/protocol/openid-connect/token \
  -H 'Content-Type: application/x-www-form-urlencoded' \
  -d 'client_id=taskflow-web' \
  -d 'grant_type=password' \
  -d 'username=alice' \
  -d 'password=alice'
```

Use the returned `access_token` as `Authorization: Bearer <token>`. All API
errors use a JSON error body and an appropriate HTTP status.

## MCP agents

TaskFlow exposes a stateless Streamable HTTP MCP server at `/mcp`. It uses the
same application services, project RBAC and workflow rules as the REST API, but
has its own OAuth audience. Protected-resource discovery is available at
`/.well-known/oauth-protected-resource/mcp`; an unauthenticated MCP request
links to it from the `WWW-Authenticate` challenge.

Register every agent as its own public Authorization Code + PKCE client. Use
the exact callback URI reported by that agent:

```sh
make mcp-client \
  CLIENT_ID=codex-taskflow \
  REDIRECT_URIS=http://127.0.0.1:1455/callback

make mcp-client \
  CLIENT_ID=claude-taskflow \
  REDIRECT_URIS=http://127.0.0.1:1456/callback
```

Multiple callback URIs may be comma-separated. Only HTTPS callbacks and
localhost loopback callbacks are accepted. Configure the MCP client with URL
`http://localhost:8080/mcp`, the registered client ID, and requested scope
`taskflow:mcp`.

### MCP client in another Docker container

`localhost` inside a client container is that container, not the Windows host.
For Docker Desktop agents, start TaskFlow with the checked-in, secret-free
Docker-agent development profile:

```sh
docker compose --env-file .env.docker-agent.example up --build -d
# Equivalent convenience target:
make up-docker-agent
```

This profile uses `host.docker.internal` as the single canonical hostname for
both browser-facing Keycloak and MCP discovery. Register Hermes after the stack
is healthy:

```sh
make mcp-client \
  CLIENT_ID=hermes-taskflow \
  REDIRECT_URIS=http://localhost:8642/api/mcp/oauth/callback/taskflow
```

The callback above is handled by the Hermes dashboard published on Windows
port `8642`; `taskflow` is the MCP server key in Hermes configuration. Start
the login from that dashboard so it can bridge the browser callback back into
the container.

```yaml
mcp_servers:
  taskflow:
    url: "http://host.docker.internal:8080/mcp"
    auth: oauth
    oauth:
      client_id: "hermes-taskflow"
      scope: "taskflow:mcp"
      redirect_uri: "http://localhost:8642/api/mcp/oauth/callback/taskflow"
```

The protected resource URL is `http://host.docker.internal:8080/mcp`; the
issuer is `http://host.docker.internal:8082/realms/taskflow`. Both URLs must be
used exactly: the resource URL is also the access-token audience, and the
issuer must match Keycloak discovery and the token `iss` claim. The backend
still fetches signing keys over the isolated Compose network through
`http://keycloak:8080`, deliberately separate from the canonical issuer.

`MCP_INSECURE_HTTP_HOSTS=host.docker.internal` is a narrow development-only
exception. It permits plaintext only for that exact hostname. Never put a
shared or production hostname there; use a trusted HTTPS endpoint instead.
The regular `docker compose up --build` flow keeps the original localhost
defaults. When the Docker-agent profile is active, host-side MCP clients should
also use `http://host.docker.internal:8080/mcp` so all clients agree on one
resource identifier.

The access token identifies the interactive Keycloak user. The agent therefore
has exactly that user's project permissions; viewer/member/admin restrictions
and workflow conditions are not bypassed. Registering an OAuth client does not
grant project membership.

For non-local deployment, set `MCP_PUBLIC_URL` and `MCP_AUDIENCE` to the same
public HTTPS URL and restrict `MCP_ALLOWED_ORIGINS`. Plain HTTP is accepted only
for localhost, loopback addresses, or exact development hosts explicitly named
in `MCP_INSECURE_HTTP_HOSTS`. Set `MCP_ALLOWED_ORIGINS` to an empty value when
only native (non-browser) MCP clients should be accepted.

The server exposes explicit tools for users, projects, membership, boards,
statuses, workflow rules, tasks, assignments, comments and transitions. It
does not invent delete operations for tasks, projects, boards or members,
because those operations do not exist in the TaskFlow domain.

Tool arguments use `snake_case` and successful results are structured JSON.
Business failures are returned as MCP tool errors with
`{"error":{"code":"...","message":"...","details":{}}}`; token and origin
failures remain HTTP `401`/`403` responses and are not exposed as tool output.

## API and realtime interfaces

The reviewed contract is [../swagger/openapi.yaml](../swagger/openapi.yaml).
Handlers also contain `swag` annotations; `make swagger` regenerates the
canonical Swagger UI input as `backend/swagger/generated/swagger.{yaml,json}`
and copies those artifacts to `swagger/generated/`. The UI deliberately loads
that generated JSON, while the more detailed OpenAPI 3 YAML remains available
for client generation and review.
The REST API covers the Keycloak user directory, projects and membership, boards, status CRUD and ordering,
transition-rule CRUD, tasks, persisted comment history, comments, and task
transitions.

Successful task status changes are committed first and then published as a
`task.transitioned` domain event. The in-process event bus fans the event out
to:

- authorized WebSocket connections subscribed to the affected project;
- subscribers to `taskflow.v1.TaskEvents/Subscribe` on
  port `50051`.

Inspect the protobuf contract at
[../backend/proto/taskflowv1/task_events.proto](../backend/proto/taskflowv1/task_events.proto). With
`grpcurl` installed, reflection can be queried using:

```sh
grpcurl -plaintext localhost:50051 list
grpcurl -plaintext -H 'authorization: Bearer <token>' \
  -d '{"project_id":"<project UUID>"}' \
  localhost:50051 taskflow.v1.TaskEvents/Subscribe
```

The gRPC stream is a live feed, not a durable queue. See [grpc.md](grpc.md) for
the wire contract and [architecture.md](architecture.md) for delivery semantics.

## Tests and development

```sh
make env                 # copy .env.example to .env if missing
make test-backend        # unit and application tests, race detector
make test-backend-docker # same suite, built and run inside golang:1.25-alpine (no local Go)
make test-integration    # repository tests against PostgreSQL
make test-frontend       # typecheck, component tests when present, production build
make test-all            # test-backend + test-frontend + test-integration
make e2e                 # four Playwright scenarios, including real MCP OAuth PKCE
make e2e-headed          # same scenarios with a visible browser
make e2e-full            # bring the stack up (build + wait) and run e2e headless
make swagger             # regenerate Swagger output from Go annotations
```

Run the stack before Playwright tests (`make e2e-full` does this for you). The
e2e suite uses the imported Keycloak users and creates unique project names, so
parallel/repeated runs do not depend on a clean application database.
Integration tests are guarded by the `integration` build tag and use
`TEST_DATABASE_URL` when it is set.

With the Docker-agent profile active, the MCP test can additionally execute an
authenticated `initialize` request from a running client container. On
PowerShell:

```powershell
$env:MCP_E2E_ISSUER = 'http://host.docker.internal:8082/realms/taskflow'
$env:MCP_E2E_URL = 'http://host.docker.internal:8080/mcp'
$env:MCP_E2E_CLIENT_CONTAINER = 'platform-hermes-1'
cd e2e
.\node_modules\.bin\playwright.cmd test specs/mcp.spec.ts
```

The test completes Authorization Code + PKCE in Chromium, verifies the token
issuer/audience/scope, then passes that token over stdin to a `docker exec`
probe. The token is neither committed nor printed.

For a fast local loop without Compose, run PostgreSQL and Keycloak from Compose,
then start `backend` and `frontend` with the variables from `.env.example`. Keep
in mind that the issuer in browser-issued tokens is
`http://localhost:8082/realms/taskflow`; the backend intentionally uses a
separate internal JWKS URL inside Docker.

## Operational notes

- `GET /healthz` checks the process; `GET /readyz` also checks PostgreSQL.
- Migrations are applied once at backend startup and are safe to re-run.
- The backend shuts HTTP and gRPC listeners down gracefully on SIGINT/SIGTERM.
- Set `CORS_ALLOWED_ORIGINS` explicitly in non-local environments.
- WebSocket and gRPC fan-out is currently at-most-once and process-local. A
  transactional outbox plus a durable broker is the natural production
  extension.
- Do not expose PostgreSQL, the Keycloak development mode, or the gRPC stream to
  an untrusted network without TLS and additional gRPC authorization.

## More detail

- [ERD.md](ERD.md) — relational model and table responsibilities.
- [architecture.md](architecture.md) — layers, trust boundaries and data flows.
- [workflow.md](workflow.md) — rule model and deterministic transition checks.
- [grpc.md](grpc.md) — notification stream contract and client example.
