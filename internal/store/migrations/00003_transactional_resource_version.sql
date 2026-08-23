-- Replace the hived_resource_version SEQUENCE with a transactional counter.
--
-- Sequences are deliberately non-transactional: nextval() takes effect
-- immediately and never rolls back. That breaks the watch contract in
-- meta.proto ("a client can List then Watch(since=resource_version) without
-- missing an event"), because resource_version order does not have to match
-- commit order. Writer A can take rv=878 and commit slowly while writer B
-- takes rv=879 and commits first; a watcher polling in between delivers 879,
-- advances its cursor past 878, and never sees A's row.
--
-- A single-row counter updated inside the writing transaction fixes this.
-- UPDATE ... RETURNING holds the row lock until commit, so a transaction
-- cannot obtain rv=N+1 until the holder of rv=N has committed. A watcher that
-- observes N+1 is therefore guaranteed N is already visible. Rollback also
-- releases the value, where nextval() would have burned it.
--
-- Trade-off: all resource_version allocation now serializes on one row. A
-- single-row UPDATE is sub-millisecond, so the ceiling is far above what a
-- control plane needs, but if it ever binds, shard the counter by kind and
-- track a per-shard watermark rather than reverting to a sequence.

-- +goose Up
CREATE TABLE hived_resource_version_counter (
  only_row boolean PRIMARY KEY DEFAULT true CHECK (only_row),
  value    bigint  NOT NULL
);

-- Seed from the sequence so watch cursors held by live clients stay valid
-- across the upgrade. A never-called sequence reports last_value = 1 with
-- is_called = false, which means "0 allocated so far".
INSERT INTO hived_resource_version_counter (only_row, value)
VALUES (true, (
  SELECT CASE WHEN is_called THEN last_value ELSE last_value - 1 END
  FROM hived_resource_version
));

-- +goose StatementBegin
CREATE FUNCTION hived_next_resource_version() RETURNS bigint
LANGUAGE sql
AS $$
  UPDATE hived_resource_version_counter SET value = value + 1 RETURNING value;
$$;
-- +goose StatementEnd

DROP SEQUENCE hived_resource_version;

-- +goose Down
CREATE SEQUENCE hived_resource_version;
SELECT setval('hived_resource_version',
  GREATEST((SELECT value FROM hived_resource_version_counter), 1));
DROP FUNCTION hived_next_resource_version();
DROP TABLE hived_resource_version_counter;
