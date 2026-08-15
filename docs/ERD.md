# Entity relationship model

Keycloak is the identity source. TaskFlow deliberately stores its immutable
`sub` claim as `user_id` instead of copying credentials or profiles into the
application database.

```mermaid
erDiagram
    PROJECTS ||--o{ PROJECT_MEMBERS : has
    PROJECTS ||--o{ BOARDS : contains
    BOARDS ||--o{ STATUSES : defines
    BOARDS ||--o{ TRANSITION_RULES : configures
    BOARDS ||--o{ TASKS : holds
    STATUSES ||--o{ TRANSITION_RULES : from_status
    STATUSES ||--o{ TRANSITION_RULES : to_status
    STATUSES ||--o{ TASKS : current_status
    TASKS ||--o{ COMMENTS : receives

    PROJECTS {
        uuid id PK
        text name
        text description
        text owner_id "Keycloak sub"
        timestamptz created_at
        timestamptz updated_at
    }
    PROJECT_MEMBERS {
        uuid project_id PK,FK
        text user_id PK "Keycloak sub"
        text role "admin|member|viewer"
        timestamptz created_at
    }
    BOARDS {
        uuid id PK
        uuid project_id FK
        text name
        timestamptz created_at
        timestamptz updated_at
    }
    STATUSES {
        uuid id PK
        uuid board_id FK
        text name
        integer position
        timestamptz created_at
        timestamptz updated_at
    }
    TRANSITION_RULES {
        uuid id PK
        uuid board_id FK
        uuid from_status_id FK
        uuid to_status_id FK
        jsonb conditions
        timestamptz created_at
        timestamptz updated_at
    }
    TASKS {
        uuid id PK
        uuid board_id FK
        uuid status_id FK
        text title
        text description
        text author_id "Keycloak sub"
        text assignee_id "nullable Keycloak sub"
        timestamptz deadline "nullable"
        timestamptz created_at
        timestamptz updated_at
    }
    COMMENTS {
        uuid id PK
        uuid task_id FK
        text author_id "Keycloak sub"
        text body
        timestamptz created_at
    }
```

## Table responsibilities

- `projects` is the aggregate root for authorization. `owner_id` is immutable;
  ownership is stronger than a mutable membership role.
- `project_members` is the project-scoped access-control list. Its composite
  key prevents duplicate membership.
- `boards` partitions independent workflows inside a project.
- `statuses` stores user-visible columns. `position` is the primary display
  order; IDs provide a deterministic tie-break while two reorder PATCH requests
  are in flight.
- `transition_rules` stores one directed edge per `(board, from, to)`. Conditions
  are JSON so new predicates can be added without changing the graph schema.
- `tasks` references a status on its own board; database and application checks
  prevent cross-board status assignments.
- `comments` supplies the audit-friendly discussion history and the
  `requires_comment` predicate.

Foreign keys cascade for aggregate-owned rows. A status referenced by a task or
a rule cannot be removed until those references are changed, so deleting a
column cannot silently orphan work.
