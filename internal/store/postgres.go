package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresStore is the Keeper's production ResourceStore + EventStore,
// backed by a single "resources" table (Colony/Agent/AgentVersion/Run) and
// a single "events" table sharing the same hived_resource_version sequence
// as their watch-cursor space. See migrations/00001_init.sql and
// migrations/00002_events.sql.
type PostgresStore struct {
	pool *pgxpool.Pool
}

func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{pool: pool}
}

const resourceColumns = `uid, kind, api_version, colony, name, generation, resource_version,
	labels, annotations, spec, status, created_at, updated_at, deleted_at`

func scanResource(row pgx.Row) (Resource, error) {
	var (
		r          Resource
		labels     json.RawMessage
		annotations json.RawMessage
		spec       json.RawMessage
		status     json.RawMessage
	)
	err := row.Scan(&r.UID, &r.Kind, &r.APIVersion, &r.Colony, &r.Name, &r.Generation, &r.ResourceVersion,
		&labels, &annotations, &spec, &status, &r.CreatedAt, &r.UpdatedAt, &r.DeletedAt)
	if err != nil {
		return Resource{}, err
	}
	if err := json.Unmarshal(labels, &r.Labels); err != nil {
		return Resource{}, fmt.Errorf("store: unmarshal labels: %w", err)
	}
	if err := json.Unmarshal(annotations, &r.Annotations); err != nil {
		return Resource{}, fmt.Errorf("store: unmarshal annotations: %w", err)
	}
	r.Spec = []byte(spec)
	r.Status = []byte(status)
	return r, nil
}

func jsonbMap(m map[string]string) json.RawMessage {
	if m == nil {
		m = map[string]string{}
	}
	b, _ := json.Marshal(m)
	return b
}

func jsonbBytes(b []byte) json.RawMessage {
	if len(b) == 0 {
		return json.RawMessage("{}")
	}
	return json.RawMessage(b)
}

func (s *PostgresStore) Apply(ctx context.Context, r Resource, ifResourceVersion *int64) (Resource, error) {
	if IsImmutableKind(r.Kind) {
		return s.applyImmutable(ctx, r)
	}
	return s.applyMutable(ctx, r, ifResourceVersion)
}

func (s *PostgresStore) applyImmutable(ctx context.Context, r Resource) (Resource, error) {
	existing, err := s.Get(ctx, r.Kind, r.Colony, r.Name)
	switch {
	case err == nil:
		if jsonEqual(existing.Spec, r.Spec) {
			return existing, nil
		}
		return Resource{}, fmt.Errorf("%w: %s/%s/%s", ErrImmutable, r.Kind, r.Colony, r.Name)
	case errors.Is(err, ErrNotFound):
		// fall through to create
	default:
		return Resource{}, err
	}

	apiVersion := r.APIVersion
	if apiVersion == "" {
		apiVersion = DefaultAPIVersion
	}
	row := s.pool.QueryRow(ctx, `
		INSERT INTO resources (kind, api_version, colony, name, generation, resource_version, labels, annotations, spec, status)
		VALUES ($1, $2, $3, $4, 1, nextval('hived_resource_version'), $5, $6, $7, '{}'::jsonb)
		RETURNING `+resourceColumns,
		r.Kind, apiVersion, r.Colony, r.Name, jsonbMap(r.Labels), jsonbMap(r.Annotations), jsonbBytes(r.Spec))
	return scanResource(row)
}

func (s *PostgresStore) applyMutable(ctx context.Context, r Resource, ifResourceVersion *int64) (Resource, error) {
	apiVersion := r.APIVersion
	if apiVersion == "" {
		apiVersion = DefaultAPIVersion
	}
	row := s.pool.QueryRow(ctx, `
		INSERT INTO resources (kind, api_version, colony, name, generation, resource_version, labels, annotations, spec, status)
		VALUES ($1, $2, $3, $4, 1, nextval('hived_resource_version'), $5, $6, $7, $8)
		ON CONFLICT (kind, colony, name) DO UPDATE SET
			generation = CASE WHEN resources.spec IS DISTINCT FROM EXCLUDED.spec
				THEN resources.generation + 1 ELSE resources.generation END,
			resource_version = nextval('hived_resource_version'),
			labels = EXCLUDED.labels,
			annotations = EXCLUDED.annotations,
			spec = EXCLUDED.spec,
			updated_at = now(),
			deleted_at = NULL
		WHERE resources.deleted_at IS NULL
			AND ($9::bigint IS NULL OR resources.resource_version = $9)
		RETURNING `+resourceColumns,
		r.Kind, apiVersion, r.Colony, r.Name, jsonbMap(r.Labels), jsonbMap(r.Annotations), jsonbBytes(r.Spec), jsonbBytes(r.Status), ifResourceVersion)

	res, err := scanResource(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Resource{}, fmt.Errorf("%w: %s/%s/%s", ErrConflict, r.Kind, r.Colony, r.Name)
		}
		return Resource{}, err
	}
	return res, nil
}

func (s *PostgresStore) Get(ctx context.Context, kind, colony, name string) (Resource, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT `+resourceColumns+`
		FROM resources
		WHERE kind = $1 AND colony = $2 AND name = $3 AND deleted_at IS NULL`,
		kind, colony, name)
	r, err := scanResource(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Resource{}, fmt.Errorf("%w: %s/%s/%s", ErrNotFound, kind, colony, name)
		}
		return Resource{}, err
	}
	return r, nil
}

func (s *PostgresStore) List(ctx context.Context, kind string, opts ListOptions) (ListResult, error) {
	var rv int64
	if err := s.pool.QueryRow(ctx, `SELECT last_value FROM hived_resource_version`).Scan(&rv); err != nil {
		return ListResult{}, fmt.Errorf("store: snapshot resource version: %w", err)
	}

	pageSize := opts.PageSize
	if pageSize <= 0 {
		pageSize = 500
	}

	rows, err := s.pool.Query(ctx, `
		SELECT `+resourceColumns+`
		FROM resources
		WHERE kind = $1
			AND ($2 = '' OR colony = $2)
			AND deleted_at IS NULL
			AND ($3 = '' OR name > $3)
			AND labels @> $4::jsonb
		ORDER BY name
		LIMIT $5`,
		kind, opts.Colony, opts.PageToken, jsonbMap(opts.LabelSelector), pageSize+1)
	if err != nil {
		return ListResult{}, err
	}
	defer rows.Close()

	var items []Resource
	for rows.Next() {
		r, err := scanResource(rows)
		if err != nil {
			return ListResult{}, err
		}
		items = append(items, r)
	}
	if err := rows.Err(); err != nil {
		return ListResult{}, err
	}

	var nextPageToken string
	if int32(len(items)) > pageSize {
		nextPageToken = items[pageSize-1].Name
		items = items[:pageSize]
	}

	return ListResult{Items: items, NextPageToken: nextPageToken, ResourceVersion: rv}, nil
}

func (s *PostgresStore) ApplyStatus(ctx context.Context, r Resource) (Resource, error) {
	row := s.pool.QueryRow(ctx, `
		UPDATE resources
		SET status = $4, resource_version = nextval('hived_resource_version'), updated_at = now()
		WHERE kind = $1 AND colony = $2 AND name = $3 AND deleted_at IS NULL
		RETURNING `+resourceColumns,
		r.Kind, r.Colony, r.Name, jsonbBytes(r.Status))
	res, err := scanResource(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Resource{}, fmt.Errorf("%w: %s/%s/%s", ErrNotFound, r.Kind, r.Colony, r.Name)
		}
		return Resource{}, err
	}
	return res, nil
}

func (s *PostgresStore) Watch(ctx context.Context, kind string, opts ListOptions, since int64) (<-chan WatchEvent, error) {
	out := make(chan WatchEvent, 16)
	go func() {
		defer close(out)
		cursor := since
		seen := make(map[string]bool)
		ticker := time.NewTicker(pollInterval)
		defer ticker.Stop()
		bookmark := time.NewTicker(bookmarkInterval)
		defer bookmark.Stop()

		poll := func() bool {
			rows, err := s.pool.Query(ctx, `
				SELECT `+resourceColumns+`
				FROM resources
				WHERE kind = $1 AND ($2 = '' OR colony = $2) AND resource_version > $3
				ORDER BY resource_version
				LIMIT 500`,
				kind, opts.Colony, cursor)
			if err != nil {
				return ctx.Err() == nil
			}
			defer rows.Close()

			for rows.Next() {
				r, err := scanResource(rows)
				if err != nil {
					continue
				}
				select {
				case out <- WatchEvent{Type: classifyWatchEvent(r, seen), ResourceVersion: r.ResourceVersion, Object: r}:
				case <-ctx.Done():
					return false
				}
				if r.ResourceVersion > cursor {
					cursor = r.ResourceVersion
				}
			}
			return true
		}

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if !poll() {
					return
				}
			case <-bookmark.C:
				select {
				case out <- WatchEvent{Type: WatchBookmark, ResourceVersion: cursor}:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return out, nil
}

func (s *PostgresStore) Append(ctx context.Context, e Event) (Event, error) {
	row := s.pool.QueryRow(ctx, `
		INSERT INTO events (resource_version, colony, run, seq, type, payload)
		VALUES (nextval('hived_resource_version'), $1, $2,
			COALESCE((SELECT MAX(seq) FROM events WHERE colony = $1 AND run = $2), 0) + 1,
			$3, $4)
		RETURNING id, resource_version, colony, run, seq, type, ts, payload`,
		e.Colony, e.Run, e.Type, jsonbBytes(e.Payload))

	var (
		id      int64
		payload json.RawMessage
	)
	if err := row.Scan(&id, &e.ResourceVersion, &e.Colony, &e.Run, &e.Seq, &e.Type, &e.TS, &payload); err != nil {
		return Event{}, err
	}
	e.Payload = []byte(payload)
	return e, nil
}

func (s *PostgresStore) ListEvents(ctx context.Context, colony, run string, sinceSeq int64, limit int32) ([]Event, error) {
	if limit <= 0 {
		limit = 500
	}
	rows, err := s.pool.Query(ctx, `
		SELECT resource_version, colony, run, seq, type, ts, payload
		FROM events
		WHERE colony = $1 AND run = $2 AND seq > $3
		ORDER BY seq
		LIMIT $4`,
		colony, run, sinceSeq, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Event
	for rows.Next() {
		var (
			e       Event
			payload json.RawMessage
		)
		if err := rows.Scan(&e.ResourceVersion, &e.Colony, &e.Run, &e.Seq, &e.Type, &e.TS, &payload); err != nil {
			return nil, err
		}
		e.Payload = []byte(payload)
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *PostgresStore) WatchEvents(ctx context.Context, colony, run string, since int64) (<-chan Event, error) {
	out := make(chan Event, 16)
	go func() {
		defer close(out)
		cursor := since
		ticker := time.NewTicker(pollInterval)
		defer ticker.Stop()

		poll := func() bool {
			rows, err := s.pool.Query(ctx, `
				SELECT resource_version, colony, run, seq, type, ts, payload
				FROM events
				WHERE colony = $1 AND run = $2 AND resource_version > $3
				ORDER BY resource_version
				LIMIT 500`,
				colony, run, cursor)
			if err != nil {
				return ctx.Err() == nil
			}
			defer rows.Close()

			for rows.Next() {
				var (
					e       Event
					payload json.RawMessage
				)
				if err := rows.Scan(&e.ResourceVersion, &e.Colony, &e.Run, &e.Seq, &e.Type, &e.TS, &payload); err != nil {
					continue
				}
				e.Payload = []byte(payload)
				select {
				case out <- e:
				case <-ctx.Done():
					return false
				}
				if e.ResourceVersion > cursor {
					cursor = e.ResourceVersion
				}
			}
			return true
		}

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if !poll() {
					return
				}
			}
		}
	}()
	return out, nil
}

var (
	_ ResourceStore = (*PostgresStore)(nil)
	_ EventStore    = (*PostgresStore)(nil)
)
