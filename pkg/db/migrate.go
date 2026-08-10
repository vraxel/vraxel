package db

import (
	"context"
	"embed"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// migrationLockID is a fixed advisory lock ID for migration coordination.
const migrationLockID = 1000000

// pendingMigration is one discovered, not-yet-applied migration file.
// Promoted from a Migrate-local type so the discover/apply helpers can
// share it.
type pendingMigration struct {
	version  int64
	filename string
}

// Migrate runs all pending migrations from the embedded filesystem.
// It uses a PostgreSQL advisory lock to ensure only one instance runs
// migrations at a time (safe for horizontal scaling).
func Migrate(ctx context.Context, pool *pgxpool.Pool, fsys embed.FS) error {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire connection: %w", err)
	}
	defer conn.Release()

	// Advisory lock — blocks until acquired, auto-released when connection returns to pool.
	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock($1)", migrationLockID); err != nil {
		return fmt.Errorf("acquire migration lock: %w", err)
	}
	defer conn.Exec(ctx, "SELECT pg_advisory_unlock($1)", migrationLockID)

	if err := migrateEnsureSchemaTable(ctx, conn); err != nil {
		return err
	}

	applied, err := migrateLoadApplied(ctx, conn)
	if err != nil {
		return err
	}

	if err := migrateBaselineIfNeeded(ctx, conn, fsys, applied); err != nil {
		return err
	}

	pending, err := migrateDiscoverPending(fsys, applied)
	if err != nil {
		return err
	}

	if len(pending) == 0 {
		return nil
	}

	// Apply each pending migration in a transaction.
	for _, m := range pending {
		if err := migrateApplyOne(ctx, conn, fsys, m); err != nil {
			return err
		}
	}

	return nil
}

// migrateEnsureSchemaTable creates the schema_migrations bookkeeping table
// if it does not already exist.
func migrateEnsureSchemaTable(ctx context.Context, conn *pgxpool.Conn) error {
	if _, err := conn.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version    BIGINT      PRIMARY KEY,
			filename   TEXT        NOT NULL,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)
	`); err != nil {
		return fmt.Errorf("create schema_migrations table: %w", err)
	}
	return nil
}

// migrateLoadApplied loads the set of already-applied migration versions
// from schema_migrations.
func migrateLoadApplied(ctx context.Context, conn *pgxpool.Conn) (map[int64]bool, error) {
	applied := make(map[int64]bool)
	rows, err := conn.Query(ctx, "SELECT version FROM schema_migrations")
	if err != nil {
		return nil, fmt.Errorf("query applied versions: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var v int64
		if err := rows.Scan(&v); err != nil {
			return nil, fmt.Errorf("scan version: %w", err)
		}
		applied[v] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate versions: %w", err)
	}
	return applied, nil
}

// migrateBaselineIfNeeded performs baseline detection: if schema_migrations
// is empty but the database already has business tables (pre-migration
// era), mark the initial migration as applied so it won't try to CREATE
// TABLE on an existing schema. The initial migration is discovered from
// the embedded FS (lowest version) rather than hard-coded, so renaming
// or squashing it cannot silently break baselining. Mutates applied in
// place.
func migrateBaselineIfNeeded(ctx context.Context, conn *pgxpool.Conn, fsys embed.FS, applied map[int64]bool) error {
	if len(applied) != 0 {
		return nil
	}
	var exists bool
	err := conn.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_schema='public' AND table_name='users')",
	).Scan(&exists)
	if err != nil {
		return fmt.Errorf("check existing schema: %w", err)
	}
	if !exists {
		return nil
	}
	version, filename, err := initialMigration(fsys)
	if err != nil {
		return err
	}
	// Pre-existing database — mark the initial migration as applied.
	if _, err := conn.Exec(ctx,
		"INSERT INTO schema_migrations (version, filename) VALUES ($1, $2)",
		version, filename,
	); err != nil {
		return fmt.Errorf("baseline initial migration: %w", err)
	}
	applied[version] = true
	return nil
}

// initialMigration returns the lowest-versioned .up.sql in the embedded
// migration FS.
func initialMigration(fsys embed.FS) (int64, string, error) {
	entries, err := fsys.ReadDir(".")
	if err != nil {
		return 0, "", fmt.Errorf("read migration directory: %w", err)
	}
	var version int64
	var filename string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".up.sql") {
			continue
		}
		v, err := parseVersion(e.Name())
		if err != nil {
			return 0, "", fmt.Errorf("parse migration filename %q: %w", e.Name(), err)
		}
		if filename == "" || v < version {
			version, filename = v, e.Name()
		}
	}
	if filename == "" {
		return 0, "", fmt.Errorf("no .up.sql migrations embedded")
	}
	return version, filename, nil
}

// migrateDiscoverPending reads the embedded migration directory, rejects
// duplicate version prefixes, filters out already-applied versions, and
// returns the remaining migrations sorted ascending by version.
func migrateDiscoverPending(fsys embed.FS, applied map[int64]bool) ([]pendingMigration, error) {
	entries, err := fsys.ReadDir(".")
	if err != nil {
		return nil, fmt.Errorf("read migration directory: %w", err)
	}

	var pending []pendingMigration
	seen := make(map[int64]string) // version → filename, detect duplicates
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".up.sql") {
			continue
		}
		version, err := parseVersion(e.Name())
		if err != nil {
			return nil, fmt.Errorf("parse migration filename %q: %w", e.Name(), err)
		}
		if prev, dup := seen[version]; dup {
			return nil, fmt.Errorf("duplicate migration version %d: %q and %q", version, prev, e.Name())
		}
		seen[version] = e.Name()
		if !applied[version] {
			pending = append(pending, pendingMigration{version: version, filename: e.Name()})
		}
	}
	sort.Slice(pending, func(i, j int) bool { return pending[i].version < pending[j].version })
	return pending, nil
}

// migrateApplyOne applies a single pending migration inside its own
// transaction: run the SQL, then record the version in schema_migrations.
// On any step failure the tx is rolled back and the error wrapped.
func migrateApplyOne(ctx context.Context, conn *pgxpool.Conn, fsys embed.FS, m pendingMigration) error {
	sql, err := fsys.ReadFile(m.filename)
	if err != nil {
		return fmt.Errorf("read migration %q: %w", m.filename, err)
	}

	tx, err := conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx for %q: %w", m.filename, err)
	}

	if _, err := tx.Exec(ctx, string(sql)); err != nil {
		tx.Rollback(ctx)
		return fmt.Errorf("execute migration %q: %w", m.filename, err)
	}

	if _, err := tx.Exec(ctx,
		"INSERT INTO schema_migrations (version, filename) VALUES ($1, $2)",
		m.version, m.filename,
	); err != nil {
		tx.Rollback(ctx)
		return fmt.Errorf("record migration %q: %w", m.filename, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit migration %q: %w", m.filename, err)
	}
	return nil
}

// MigrationVersion returns the highest applied migration version recorded in
// schema_migrations. Returns pgx.ErrNoRows if no migrations have been applied.
func (d *DB) MigrationVersion(ctx context.Context) (string, error) {
	var version string
	err := d.GetPool().QueryRow(ctx,
		"SELECT version FROM schema_migrations ORDER BY version DESC LIMIT 1",
	).Scan(&version)
	return version, err
}

// parseVersion extracts the numeric version prefix from a migration filename.
// e.g. "000001_initial.up.sql" → 1
func parseVersion(filename string) (int64, error) {
	parts := strings.SplitN(filename, "_", 2)
	if len(parts) < 2 {
		return 0, fmt.Errorf("invalid migration filename: %s", filename)
	}
	return strconv.ParseInt(parts[0], 10, 64)
}
