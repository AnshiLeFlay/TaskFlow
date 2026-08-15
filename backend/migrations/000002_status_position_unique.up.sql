-- Deduplicate any pre-existing (board_id, position) collisions before adding
-- the unique index below. Without this, a database that already has
-- duplicate positions (e.g. seeded before this migration existed) would fail
-- to apply this migration, leaving the schema_migrations table dirty and the
-- backend crash-looping on startup.
WITH ranked AS (
    SELECT id, row_number() OVER (PARTITION BY board_id ORDER BY position, created_at, id) - 1 AS rn
    FROM statuses
)
UPDATE statuses s SET position = ranked.rn, updated_at = now()
FROM ranked WHERE s.id = ranked.id AND s.position <> ranked.rn;

CREATE UNIQUE INDEX statuses_board_position_uniq ON statuses (board_id, position);
