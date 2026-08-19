# Configurable workflow

Each board owns an ordered set of statuses and a directed graph of transition
rules. A rule identifies exactly one source and target status on the same board
and carries a condition object. The graph and conditions are persisted
in PostgreSQL; there are no hard-coded status names in transition behavior.

Example:

```json
{
  "from_status_id": "7f8b6dd1-...",
  "to_status_id": "dc931d3e-...",
  "conditions": {
    "author_only": false,
    "assignee_only": false,
    "requires_comment": true,
    "project_owner_only": false,
    "allowed_roles": ["admin", "member"]
  }
}
```

Supported condition fields:

| Field | Passes when enabled / non-empty |
| --- | --- |
| `author_only` | JWT `sub` equals the task's immutable `author_id`. |
| `assignee_only` | JWT `sub` equals the current non-empty `assignee_id`. |
| `requires_comment` | At least one persisted comment exists for the task. |
| `project_owner_only` | JWT `sub` equals the project's immutable `owner_id`. |
| `allowed_roles` | The actor's project membership role is in `roles`. |

All enabled conditions use logical AND. An object with all flags false and no
`allowed_roles` allows any project
`admin` or `member` to traverse that configured edge. A `viewer` cannot mutate a
task even if a condition would otherwise pass.

## Default board preset

A board created without explicit statuses gets a preset that is usable
immediately, rather than columns with no way to move between them:

| Column | Meaning |
| --- | --- |
| `Backlog` | arrived, nobody has committed to it yet |
| `To Do` | accepted, waiting to start |
| `In Progress` | being worked on |
| `Done` | finished |
| `Canceled` | dropped: a duplicate, a mistake, or no longer needed |

`Backlog` is separate from `To Do` because the move between them is the moment
work was accepted. It is the only record that separates how long a task waited
from how long it took, and it cannot be reconstructed later, so tasks are
created in `Backlog`. `Canceled` exists so that abandoned work has an ending
other than `Done`; without it, `Done` silently comes to mean "no longer on the
board" and stops being a measure of anything.

Ten rules are created, none with conditions: each column to the next and back
again, `Backlog`, `To Do` and `In Progress` to `Canceled`, and `Canceled` back
to `Backlog`.

Two absences are deliberate rather than oversights:

- **No `Done -> Canceled`.** Finished work cannot be un-finished. Deciding
  afterwards that it was unnecessary is a new task, not an undo.
- **No `Canceled` into a working column.** A revived task re-enters through
  `Backlog`, so it passes the same acceptance point as everything else and its
  timings stay comparable.

Supplying statuses explicitly creates them with no rules at all. That caller is
designing a workflow, and inventing transitions for it would be guesswork.

The preset applies only at creation. Boards that already exist are untouched,
and boards created before it still need their rules added by hand.

## Evaluation algorithm

1. Authenticate the actor and load their project membership.
2. Reject a viewer or non-member before disclosing task details.
3. Load the task, its current status and the requested target.
4. Reject targets from another board and no-op transitions.
5. Look up the directed `(board, current status, target status)` rule. **No rule
   means denied**; workflows are default-deny.
6. Load only the facts needed by conditions (project owner and comment
   existence), evaluate every condition, and return a stable error code if one
   fails.
7. Update with a compare-and-swap on the original status; reject a concurrent
   move as a conflict.
8. Create and publish `task.transitioned` with event ID, task/board/project
   IDs, old/new statuses, actor and timestamp.

Rule-management endpoints validate that both statuses belong to the route's
board, reject duplicate directed edges and reject unknown conditions. Status
positions must be non-negative; clients keep the settled board order unique.
Deleting a referenced
status returns a conflict instead of deleting dependent tasks.

The example flows from the assignment are represented without special cases:

- `To Do -> In Progress`: `{"author_only":true}`
- `In Progress -> Review`: `{"requires_comment":true}`
- `Review -> Done`: `{"project_owner_only":true}`
