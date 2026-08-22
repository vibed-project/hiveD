-- +goose Up
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE SEQUENCE hived_resource_version;

CREATE TABLE resources (
  uid              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  kind             text   NOT NULL,
  api_version      text   NOT NULL DEFAULT 'hived/v1alpha1',
  colony           text   NOT NULL,
  name             text   NOT NULL,
  generation       bigint NOT NULL DEFAULT 1,
  resource_version bigint NOT NULL,
  labels           jsonb  NOT NULL DEFAULT '{}',
  annotations      jsonb  NOT NULL DEFAULT '{}',
  spec             jsonb  NOT NULL,
  status           jsonb  NOT NULL DEFAULT '{}',
  created_at       timestamptz NOT NULL DEFAULT now(),
  updated_at       timestamptz NOT NULL DEFAULT now(),
  deleted_at       timestamptz,
  UNIQUE (kind, colony, name)
);

CREATE INDEX resources_rv_idx ON resources (resource_version);
CREATE INDEX resources_list_idx ON resources (kind, colony, name) WHERE deleted_at IS NULL;
CREATE INDEX resources_labels_idx ON resources USING gin (labels);

-- +goose Down
DROP TABLE resources;
DROP SEQUENCE hived_resource_version;
