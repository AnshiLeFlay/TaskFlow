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
as its `admin` and remains its owner.

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

## API and realtime interfaces

The reviewed contract is [../swagger/openapi.yaml](../swagger/openapi.yaml).
Handlers also contain `swag` annotations; `make swagger` regenerates the
canonical Swagger UI input as `backend/swagger/generated/swagger.{yaml,json}`
and copies those artifacts to `swagger/generated/`. The UI deliberately loads
that generated JSON, while the more detailed OpenAPI 3 YAML remains available
for client generation and review.
The REST API covers projects and membership, boards, status CRUD and ordering,
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
make test-backend       # unit and application tests, race detector
make test-integration   # repository tests against PostgreSQL
make test-frontend      # typecheck, component tests when present, production build
make e2e                # two Playwright scenarios, headless
make e2e-headed         # same scenarios with a visible browser
make swagger            # regenerate Swagger output from Go annotations
```

Run the stack before Playwright tests. The e2e suite uses the imported Keycloak
users and creates unique project names, so parallel/repeated runs do not depend
on a clean application database. Integration tests are guarded by the
`integration` build tag and use `TEST_DATABASE_URL` when it is set.

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
