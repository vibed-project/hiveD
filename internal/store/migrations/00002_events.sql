-- +goose Up
CREATE TABLE events (
  id               bigserial PRIMARY KEY,
  resource_version bigint NOT NULL,
  colony           text   NOT NULL,
  run              text   NOT NULL,
  seq              bigint NOT NULL,
  type             text   NOT NULL,
  ts               timestamptz NOT NULL DEFAULT now(),
  payload          jsonb  NOT NULL,
  UNIQUE (colony, run, seq)
);

CREATE INDEX events_run_idx ON events (colony, run, seq);
CREATE INDEX events_rv_idx ON events (resource_version);

-- +goose Down
DROP TABLE events;
