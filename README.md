# TaskFlow

TaskFlow is a full-stack MVP for projects, configurable Kanban workflows, and
real-time task updates. A project has members and one or more boards; boards
own ordered statuses and transition rules that gate how tasks move between
them. Status changes fan out live over WebSocket and gRPC.

## Tech stack

- **Backend**: Go, PostgreSQL, gRPC, WebSocket, Keycloak (OIDC) auth
- **Frontend**: Vue 3, TypeScript, Vite
- **Infra**: Docker Compose, Keycloak, PostgreSQL
- **Tests**: Go (`go test`, race detector), Vitest/component tests, Playwright e2e

## Repository layout

```
backend/    Go service: cmd/server, internal/{domain,application,infrastructure,interfaces}
frontend/   Vue 3 + TypeScript SPA (src/)
e2e/        Playwright end-to-end specs
docs/       Detailed documentation (architecture, ERD, workflow, gRPC)
docker/     Compose-mounted init scripts (e.g. Postgres)
keycloak/   Imported realm configuration
swagger/    Reviewed OpenAPI 3 contract and generated Swagger output
.opencode/  AI coding agent instructions, spec, and plugins
```

## Services and ports

| Service | URL / address |
| --- | --- |
| Vue application (UI) | <http://localhost:8081> |
| REST API | <http://localhost:8080/api/v1> |
| MCP (OAuth-protected Streamable HTTP) | <http://localhost:8080/mcp> |
| Swagger UI | <http://localhost:8080/swagger/> |
| gRPC | `localhost:50051` |
| Keycloak | <http://localhost:8082> |

## Quick start

```sh
cp .env.example .env
docker compose up --build
```

Wait until `docker compose ps` reports all services healthy, then open the UI
at <http://localhost:8081>.

## Tests

```sh
make env                 # copy .env.example to .env if missing
make test                # backend unit tests + frontend typecheck/build
make test-backend-docker # backend tests inside the golang:1.25-alpine test stage
make test-integration    # repository tests against PostgreSQL
make test-all            # test + test-integration
make e2e                 # Playwright e2e (stack must already be running)
make e2e-full            # bring the stack up and run e2e end to end, headless
```

Run `make help` for the full target list.

## Documentation

See [docs/README.md](docs/README.md) for credentials, authentication, REST,
realtime and MCP interfaces, and operational notes. Further detail:

- [docs/ERD.md](docs/ERD.md) — relational model
- [docs/architecture.md](docs/architecture.md) — layers and trust boundaries
- [docs/workflow.md](docs/workflow.md) — transition rule model
- [docs/grpc.md](docs/grpc.md) — notification stream contract
