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
		VALUES ($1, $2, $3, $4, 1, hived_next_resource_version(), $5, $6, $7, '{}'::jsonb)
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
		VALUES ($1, $2, $3, $4, 1, hived_next_resource_version(), $5, $6, $7, $8)
		ON CONFLICT (kind, colony, name) DO UPDATE SET
			generation = CASE WHEN resources.spec IS DISTINCT FROM EXCLUDED.spec
				THEN resources.generation + 1 ELSE resources.generation END,
			-- Only advance the watch cursor when something actually changed.
			-- An unconditional bump meant a controller that re-applies an
			-- identical spec on every reconcile emitted a MODIFIED event on
			-- every reconcile; one that reconciles on its own watch is then a
			-- self-sustaining hot loop.
			resource_version = CASE WHEN resources.spec IS DISTINCT FROM EXCLUDED.spec
					OR resources.labels IS DISTINCT FROM EXCLUDED.labels
					OR resources.annotations IS DISTINCT FROM EXCLUDED.annotations
				THEN hived_next_resource_version() ELSE resources.resource_version END,
			labels = EXCLUDED.labels,
			annotations = EXCLUDED.annotations,
			spec = EXCLUDED.spec,
			updated_at = CASE WHEN resources.spec IS DISTINCT FROM EXCLUDED.spec
					OR resources.labels IS DISTINCT FROM EXCLUDED.labels
					OR resources.annotations IS DISTINCT FROM EXCLUDED.annotations
				THEN now() ELSE resources.updated_at END,
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
	// The counter is transactional, so this reads the highest resource_version
	// that is actually committed and visible. A sequence's last_value could
	// report a value whose row had not committed yet (and reported 1 on a
	// never-called sequence, which made Watch(since=1) skip the very first
	// resource ever created).
	var rv int64
	if err := s.pool.QueryRow(ctx, `SELECT value FROM hived_resource_version_counter`).Scan(&rv); err != nil {
		return ListResult{}, fmt.Errorf("store: snapshot resource version: %w", err)
	}

	pageSize := clampPageSize(opts.PageSize)

	afterColony, afterName, err := decodePageToken(opts.PageToken)
	if err != nil {
		return ListResult{}, err
	}

	// Paginate on (colony, name), not name alone: name is only unique per
	// (kind, colony), so a name-only cursor skips every same-named resource
	// in the colonies that sort after the one the cursor landed in.
	//
	// pageSize+1 is computed in int64: pageSize is int32, so MaxInt32+1 used
	// to wrap negative and Postgres rejected the LIMIT outright.
	rows, err := s.pool.Query(ctx, `
		SELECT `+resourceColumns+`
		FROM resources
		WHERE kind = $1
			AND ($2 = '' OR colony = $2)
			AND deleted_at IS NULL
			AND ($3::boolean IS NOT TRUE OR (colony, name) > ($4, $5))
			AND labels @> $6::jsonb
		ORDER BY colony, name
		LIMIT $7`,
		kind, opts.Colony, opts.PageToken != "", afterColony, afterName,
		jsonbMap(opts.LabelSelector), int64(pageSize)+1)
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
		last := items[pageSize-1]
		nextPageToken = encodePageToken(last.Colony, last.Name)
		items = items[:pageSize]
	}

	return ListResult{Items: items, NextPageToken: nextPageToken, ResourceVersion: rv}, nil
}

func (s *PostgresStore) ApplyStatus(ctx context.Context, r Resource) (Resource, error) {
	row := s.pool.QueryRow(ctx, `
		UPDATE resources
		SET status = $4, resource_version = hived_next_resource_version(), updated_at = now()
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
	// seq is assigned as MAX(seq)+1, a read-modify-write. Under READ
	// COMMITTED, concurrent appenders to the same run all read the same MAX
	// and all compute the same next value; UNIQUE (colony, run, seq) then
	// rejects every one but the winner. Nothing retried, so a Run whose
	// Worker reported from more than one goroutine silently lost the
	// majority of its events.
	//
	// A transaction-scoped advisory lock keyed on (colony, run) serializes
	// appends per run without taking a table lock and without a retry loop.
	// Appends to different runs never contend.
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Event{}, fmt.Errorf("store: begin append: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// The two-argument form takes a pair of int4 keys, so colony and run are
	// hashed separately: no delimiter to escape and no way for one field's
	// content to spill into the other's. A hash collision would only make two
	// unrelated runs share a lock, never corrupt a sequence.
	if _, err := tx.Exec(ctx,
		`SELECT pg_advisory_xact_lock(hashtext($1), hashtext($2))`,
		e.Colony, e.Run); err != nil {
		return Event{}, fmt.Errorf("store: lock run event log: %w", err)
	}

	// ts was omitted from the column list entirely, so every event silently
	// took the column default (ingestion time) and a caller-supplied
	// timestamp was discarded. NULL still falls back to now().
	row := tx.QueryRow(ctx, `
		INSERT INTO events (resource_version, colony, run, seq, type, ts, payload)
		VALUES (hived_next_resource_version(), $1, $2,
			COALESCE((SELECT MAX(seq) FROM events WHERE colony = $1 AND run = $2), 0) + 1,
			$3, COALESCE($4, now()), $5)
		RETURNING id, resource_version, colony, run, seq, type, ts, payload`,
		e.Colony, e.Run, e.Type, nullableTime(e.TS), jsonbBytes(e.Payload))

	var (
		id      int64
		payload json.RawMessage
	)
	if err := row.Scan(&id, &e.ResourceVersion, &e.Colony, &e.Run, &e.Seq, &e.Type, &e.TS, &payload); err != nil {
		return Event{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Event{}, fmt.Errorf("store: commit append: %w", err)
	}
	e.Payload = []byte(payload)
	return e, nil
}

// nullableTime maps the zero time to SQL NULL so the caller can leave TS
// unset and get the server clock.
func nullableTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

func (s *PostgresStore) ListEvents(ctx context.Context, colony, run string, sinceSeq int64, limit int32) ([]Event, error) {
	limit = clampPageSize(limit)
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
