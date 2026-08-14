package db

import (
	"context"
	"fmt"
	"hash/fnv"

	"github.com/jackc/pgx/v5"
	"vraxel.io/vraxel/pkg/db/generated"
)

// WithTx runs fn inside a database transaction scoped to the given
// context. fn receives a Queries handle bound to the transaction, so
// every call it makes participates in the same tx. The transaction is
// committed when fn returns nil, and rolled back on any other outcome
// (error, panic).
//
// This is the ONE way store implementations should start transactions.
// It removes the need for each store to hand-write begin/defer-rollback/
// commit boilerplate, and it keeps *pgx.Tx confined to the store-
// implementation layer (pkg/apis/*/store/*.go): fn operates on a
// *generated.Queries, not on the raw tx, so no pgx types can leak to
// callers.
//
// Nested calls are not supported. If fn needs to span multiple stores,
// every participating store must expose a Querier-accepting helper
// (e.g. CreateHostTx(ctx, q Querier, ...)) so the outer WithTx caller
// can chain them on the same qtx.
func (d *DB) WithTx(ctx context.Context, fn func(ctx context.Context, q *generated.Queries) error) (err error) {
	return d.WithTxOpts(ctx, pgx.TxOptions{}, fn)
}

// WithTxOpts is the explicit-isolation variant. Pass
// `pgx.TxOptions{IsoLevel: pgx.Serializable}` for transactions whose
// invariants depend on serializability (e.g. "pick the next version
// number for this name"). The opts are forwarded to pgx.BeginTx
// unchanged; everything else (commit / rollback / panic recovery)
// mirrors WithTx.
func (d *DB) WithTxOpts(ctx context.Context, opts pgx.TxOptions, fn func(ctx context.Context, q *generated.Queries) error) (err error) {
	tx, err := d.GetPool().BeginTx(ctx, opts)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback(ctx)
			panic(p)
		}
		if err != nil {
			_ = tx.Rollback(ctx)
			return
		}
		if cErr := tx.Commit(ctx); cErr != nil {
			err = fmt.Errorf("commit tx: %w", cErr)
		}
	}()
	q := d.GetQueries().WithTx(tx)
	return fn(ctx, q)
}

// WithTxReturning is the generic variant of WithTx that returns a
// value alongside the error. Keeps the committed value only.
func WithTxReturning[T any](ctx context.Context, d *DB, fn func(ctx context.Context, q *generated.Queries) (T, error)) (T, error) {
	var zero T
	tx, err := d.GetPool().Begin(ctx)
	if err != nil {
		return zero, fmt.Errorf("begin tx: %w", err)
	}
	q := d.GetQueries().WithTx(tx)
	out, err := fn(ctx, q)
	if err != nil {
		_ = tx.Rollback(ctx)
		return zero, err
	}
	if cErr := tx.Commit(ctx); cErr != nil {
		return zero, fmt.Errorf("commit tx: %w", cErr)
	}
	return out, nil
}

// AdvisoryLockMode controls whether AdvisoryLock blocks or returns
// immediately when the key is held by another session.
type AdvisoryLockMode int

const (
	// AdvisoryLockBlocking blocks (pg_advisory_xact_lock) until the
	// key is free. Used when correctness requires serialization.
	AdvisoryLockBlocking AdvisoryLockMode = iota
	// AdvisoryLockTry returns immediately with acquired=false if the
	// key is already held (pg_try_advisory_xact_lock). Used for
	// opportunistic leader election / skip-if-busy paths.
	AdvisoryLockTry
)

// AdvisoryLock takes a PostgreSQL advisory lock keyed by a single
// int64. The lock is scoped to the caller's transaction (WithTx
// provides one) and is released automatically on commit/rollback.
//
// Concurrency rule: advisory locks are the only cross-instance lock
// mechanism the codebase permits, because in-process locks
// (sync.Mutex) are useless when vraxel-server is horizontally scaled.
// Both instances connect to the same Postgres; Postgres advisory
// locks give them a single coordination point.
func AdvisoryLock(ctx context.Context, q *generated.Queries, key int64, mode AdvisoryLockMode) (acquired bool, err error) {
	// pgx txs expose Conn() but a Queries bound via WithTx hides it;
	// we use a raw tag with the underlying connection stored in the
	// Queries' db field. To avoid reflecting into sqlc internals we
	// accept a pgx.Tx-aware override via LockConn.
	return lockWithQueries(ctx, q, key, mode)
}

// LockConn is the pgx-scoped executor that AdvisoryLock actually uses.
// It must receive a handle whose queries run inside the caller's tx.
// Exposed as a free function so store implementations can provide
// either a Queries (for typed SQL) or pass the raw tx directly when
// using pgx.Tx.QueryRow.
func LockConn(ctx context.Context, tx pgx.Tx, key int64, mode AdvisoryLockMode) (bool, error) {
	switch mode {
	case AdvisoryLockBlocking:
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", key); err != nil {
			return false, fmt.Errorf("advisory_xact_lock(%d): %w", key, err)
		}
		return true, nil
	case AdvisoryLockTry:
		var acquired bool
		if err := tx.QueryRow(ctx, "SELECT pg_try_advisory_xact_lock($1)", key).Scan(&acquired); err != nil {
			return false, fmt.Errorf("try_advisory_xact_lock(%d): %w", key, err)
		}
		return acquired, nil
	default:
		return false, fmt.Errorf("unknown AdvisoryLockMode: %d", mode)
	}
}

// lockWithQueries is an internal shim: pgx.Tx is the true carrier of
// advisory-lock scope, and the Queries object doesn't expose it. Store
// implementations that need AdvisoryLock call LockConn directly with
// the tx they already hold; the Queries-based entry point is here to
// cover the rare case where a store only has a Queries pointer (e.g.
// recovery_scanner in advisory mode at startup). In that case we
// temporarily use the generated package's non-transactional queries
// interface which run on the underlying pool connection.
func lockWithQueries(ctx context.Context, q *generated.Queries, key int64, mode AdvisoryLockMode) (bool, error) {
	// The generated.Queries embeds a DBTX; all queries serialize on
	// the same connection scope. For tx-bound queries (via WithTx)
	// this is the tx itself. We use the q.TryAdvisoryLock /
	// AdvisoryLock sqlc helpers when present, otherwise we fail —
	// callers must wire one.
	switch mode {
	case AdvisoryLockBlocking:
		return true, q.AcquireAdvisoryLock(ctx, key)
	case AdvisoryLockTry:
		return q.TryAdvisoryLock(ctx, key)
	default:
		return false, fmt.Errorf("unknown AdvisoryLockMode: %d", mode)
	}
}

// HashLockKey turns a natural key ("agentgw.bind:<uuid>") into the int64
// an advisory lock takes.
//
// Advisory locks share ONE int64 key space across the entire database, so
// every caller must namespace its own strings before hashing -- two
// subsystems that both lock on "42" would silently serialise against each
// other.
//
// A 64-bit collision between two namespaced keys means two unrelated
// operations occasionally wait on one another. That costs latency, never
// correctness, which is why a hash is acceptable here at all.
func HashLockKey(key string) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(key))
	return int64(h.Sum64())
}
