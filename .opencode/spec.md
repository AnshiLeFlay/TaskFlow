# TaskFlow business specification

## Identities and roles

Authentication belongs to Keycloak. Every command receives a verified actor with
a stable subject (`sub`) and realm roles. Authorization inside a project uses
`project_members`: `admin` manages membership/workflow/tasks, `member` manages
tasks and comments, and `viewer` reads only. A creator becomes owner and admin.

## Aggregates

- **Project**: name, optional description, immutable owner, members and boards.
- **Board**: project-owned name, ordered statuses and a directed rule graph.
- **Task**: board/status, title, optional description/assignee/deadline,
  immutable author and comments.
- **TransitionRule**: board-local from/to edge and an AND-list of configurable
  predicates documented in `docs/workflow.md`.

## Required scenarios

1. An authenticated user creates a project and is immediately able to manage it.
2. Its owner adds another Keycloak subject as `admin`, `member` or `viewer`.
3. An admin creates one or more boards and can create, rename, reorder and remove
   unreferenced statuses.
4. An admin creates or updates directed transition rules. Cross-board edges and
   unknown predicates are rejected.
5. An admin/member creates and edits a task with title, description, optional
   assignee and deadline; its initial status belongs to the board.
6. A transition is rejected when the edge is not configured or any predicate
   fails. On success the task is committed and a domain event is broadcast over
   project-scoped WebSocket and gRPC streams.
7. Adding a comment and changing assignment produce realtime task events; the UI
   displays incoming events as toasts and refreshes the active board.
8. A viewer can list accessible projects/boards/tasks but cannot mutate them.

## Acceptance checks

- Compose starts PostgreSQL, imported Keycloak, Go REST/WebSocket/gRPC backend,
  and the built Vue application with one command.
- Swagger is served below `/swagger`; proto source is under `backend/proto`.
- Unit tests cover domain predicates and use cases, integration tests exercise a
  real PostgreSQL repository, and Playwright contains at least the workflow and
  two-client realtime scenarios.
- Repository structure and dependencies reflect Domain, Application,
  Infrastructure and Interfaces layers.

