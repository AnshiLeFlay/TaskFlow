# Instructions for AI coding agents

## Working agreement

1. Read `.opencode/spec.md`, `docs/architecture.md` and
   `docs/workflow.md` before changing behavior.
2. Preserve inward dependency direction: domain <- application <- adapters.
   Domain code must not import SQL, HTTP, WebSocket, gRPC or Keycloak packages.
3. Put business rules in domain/application services, never only in a handler or
   Vue component. Treat the JWT `sub` as the actor identity.
4. Add or update a failing test before changing an invariant. Use table-driven Go
   tests for workflow predicates and application fakes for use cases.
5. Keep REST JSON in `snake_case`, timestamps in RFC 3339 UTC and identifiers as
   UUID strings. Keep public API changes synchronized with
   `swagger/openapi.yaml`, TypeScript types and e2e tests.
6. Never weaken default-deny transitions, project membership checks, issuer/
   signature validation or WebSocket authorization to make a test pass.
7. Return typed domain/application errors and translate them at interfaces. Do
   not expose SQL or token-validation internals to clients.
8. Use parameterized SQL and transactional state changes. Publish status events
   only after the database commit succeeds.

## Style and checks

- Go: `gofmt`, small interfaces owned by consumers, context propagation, wrapped
  errors and race-safe concurrency. Run `go test -race ./...`.
- Vue/TypeScript: Composition API with `<script setup>`, explicit API types,
  accessible labels and keyboard behavior. Run `npm run typecheck` and
  `npm run build` in `frontend`.
- Integration: run `make test-integration` with PostgreSQL available.
- End to end: run `make e2e`; use `make e2e-headed` only for interactive
  debugging.
- Infrastructure: validate with `docker compose config`, then exercise health,
  login, one successful transition and one rejected transition.

Do not commit `.env`, database volumes, access tokens, generated test reports or
dependency directories. Development realm credentials are fixtures, not secrets.

