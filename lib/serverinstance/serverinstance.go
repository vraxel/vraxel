// Package serverinstance maintains this process's row in
// server_instances: a self-renewing lease recording which
// server instances are alive and how siblings reach them.
//
// It exists because several subsystems need instance identity for
// different reasons -- the agent gateway pins each control-channel
// socket to its owning instance and forwards across instances, and the
// playbook-run orchestrator marks runs whose driving instance died --
// and none of those reasons is agent-specific. BuildInstanceID and
// BuildInternalAddr contain nothing about agents at all.
//
// Sibling of lib/leaderlock: same layer (platform primitives over a
// pgxpool), opposite question. leaderlock answers "who is THE one",
// serverinstance answers "who is alive and at what address".
package serverinstance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	// RenewInterval is how often a live instance refreshes its lease.
	RenewInterval = 10 * time.Second
	// StaleAfter is when an instance is presumed dead. Three missed
	// renewals: long enough to ride out a GC pause or a brief DB
	// hiccup, short enough that failover is seconds not minutes.
	StaleAfter = 30 * time.Second
)

// Logger is the minimal logging surface, mirroring lib/leaderlock so
// callers can pass the same adapter.
type Logger interface {
	Infof(format string, args ...any)
	Warnf(format string, args ...any)
}

// Registry is the server_instances table.
//
// Raw SQL over the pool rather than sqlc: lib/ must not import
// pkg/db/generated's sibling business helpers, and five statements
// against one four-column table do not justify the generated surface.
type Registry struct {
	pool *pgxpool.Pool
}

// NewRegistry binds a registry to a connection pool.
func NewRegistry(pool *pgxpool.Pool) *Registry { return &Registry{pool: pool} }

// Register inserts or refreshes this instance's row.
func (r *Registry) Register(ctx context.Context, instanceID, internalAddr string) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO server_instances (instance_id, internal_addr)
		VALUES ($1, $2)
		ON CONFLICT (instance_id) DO UPDATE
		SET internal_addr = EXCLUDED.internal_addr,
		    started_at    = now(),
		    last_seen_at  = now()`, instanceID, internalAddr)
	if err != nil {
		return fmt.Errorf("register server instance %s: %w", instanceID, err)
	}
	return nil
}

// Touch renews the lease, re-creating the row if it has gone missing.
//
// An UPDATE would match nothing when the row is absent, and report no
// error for it -- which is exactly the state a failed Register or a
// mistaken sweep leaves behind. The instance would then stay invisible to
// every sibling for the rest of the process's life. Upserting makes each
// renewal self-healing. started_at is left alone so it keeps meaning
// "when this instance came up", not "when it last renewed".
func (r *Registry) Touch(ctx context.Context, instanceID, internalAddr string) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO server_instances (instance_id, internal_addr)
		VALUES ($1, $2)
		ON CONFLICT (instance_id) DO UPDATE
		SET internal_addr = EXCLUDED.internal_addr,
		    last_seen_at  = now()`, instanceID, internalAddr)
	if err != nil {
		return fmt.Errorf("renew server instance %s: %w", instanceID, err)
	}
	return nil
}

// Delete removes this instance's row on graceful shutdown.
func (r *Registry) Delete(ctx context.Context, instanceID string) error {
	_, err := r.pool.Exec(ctx,
		`DELETE FROM server_instances WHERE instance_id = $1`, instanceID)
	if err != nil {
		return fmt.Errorf("deregister server instance %s: %w", instanceID, err)
	}
	return nil
}

// DeleteStale drops rows whose lease has expired.
//
// The cutoff is computed from the DB clock, never the caller's:
// last_seen_at is written with now() on the server, so subtracting
// StaleAfter from a Go-side timestamp folds every instance's clock drift
// straight into the liveness window. With a 10s renewal and a 30s
// expiry, a machine whose clock runs 20s fast would delete the rows of
// instances that had just renewed -- and, before Touch became an upsert,
// they would never have come back.
func (r *Registry) DeleteStale(ctx context.Context) error {
	_, err := r.pool.Exec(ctx,
		`DELETE FROM server_instances
		 WHERE last_seen_at < now() - make_interval(secs => $1)`, StaleAfter.Seconds())
	if err != nil {
		return fmt.Errorf("sweep stale server instances: %w", err)
	}
	return nil
}

// ListAlive returns the instance ids whose lease has not expired. The
// orphan-run sweep uses it to find runs whose owner instance is gone
// (design §6.3).
func (r *Registry) ListAlive(ctx context.Context) ([]string, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT instance_id FROM server_instances
		 WHERE last_seen_at >= now() - make_interval(secs => $1)`, StaleAfter.Seconds())
	if err != nil {
		return nil, fmt.Errorf("list alive server instances: %w", err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan instance id: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// IsAlive reports whether an instance id currently holds a valid lease.
// Consumers that route work to a specific instance check this before
// dialling it.
func (r *Registry) IsAlive(ctx context.Context, instanceID string) (bool, error) {
	var ok bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM server_instances
			WHERE instance_id = $1
			  AND last_seen_at >= now() - make_interval(secs => $2)
		)`, instanceID, StaleAfter.Seconds()).Scan(&ok)
	if err != nil {
		return false, fmt.Errorf("check server instance %s liveness: %w", instanceID, err)
	}
	return ok, nil
}

// Lease owns one instance's registration for the lifetime of a context.
type Lease struct {
	registry     *Registry
	instanceID   string
	internalAddr string
	log          Logger

	// OnTick runs after each successful renewal. Subsystems hang their
	// own periodic reconciliation off the lease rather than starting a
	// second ticker with the same cadence; the agent gateway uses it to
	// sweep agents whose heartbeat expired.
	OnTick func(ctx context.Context)
}

// NewLease builds a lease. instanceID must be stable for the life of
// the process and unique across instances.
func NewLease(registry *Registry, instanceID, internalAddr string, log Logger) *Lease {
	return &Lease{registry: registry, instanceID: instanceID, internalAddr: internalAddr, log: log}
}

// InstanceID returns the id this lease registers.
func (l *Lease) InstanceID() string { return l.instanceID }

// Start registers the instance and renews it until ctx is done, then
// deregisters. Registration is synchronous so callers can rely on the
// row existing once Start returns.
func (l *Lease) Start(ctx context.Context) {
	if err := l.registry.Register(ctx, l.instanceID, l.internalAddr); err != nil {
		l.log.Warnf("serverinstance: %v", err)
	} else {
		l.log.Infof("serverinstance: %s registered at %s", l.instanceID, l.internalAddr)
	}
	go l.run(ctx)
}

func (l *Lease) run(ctx context.Context) {
	ticker := time.NewTicker(RenewInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			// ctx is already cancelled, so deregistration needs a fresh
			// deadline of its own; without it the DELETE never runs and
			// the row lingers until the stale sweep.
			cctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
			if err := l.registry.Delete(cctx, l.instanceID); err != nil {
				l.log.Warnf("serverinstance: %v", err)
			}
			cancel()
			return
		case <-ticker.C:
			if err := l.registry.Touch(ctx, l.instanceID, l.internalAddr); err != nil {
				l.log.Warnf("serverinstance: %v", err)
			}
			if err := l.registry.DeleteStale(ctx); err != nil {
				l.log.Warnf("serverinstance: %v", err)
			}
			if l.OnTick != nil {
				l.OnTick(ctx)
			}
		}
	}
}

// BuildInstanceID composes the server_instances primary key from the
// deployment name plus this process's host and pid.
//
// server.Name alone is not unique -- it names the deployment, and a
// deployment is precisely the thing that runs several instances.
// Hostname alone collides between two processes on one box. varchar(64)
// is the column width, so an over-long result is truncated rather than
// left to fail the INSERT at runtime.
func BuildInstanceID(serverName string) string {
	if serverName == "" {
		serverName = "vraxel"
	}
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "unknown"
	}
	id := fmt.Sprintf("%s-%s-%d", serverName, host, os.Getpid())
	if len(id) > 64 {
		// Truncating would cut the pid off the end -- exactly the part
		// that distinguishes two processes on one long-named host -- and
		// two instances sharing an id take over each other's leases. Keep
		// a prefix for readability and let a hash of the whole thing carry
		// the uniqueness.
		sum := sha256.Sum256([]byte(id))
		id = id[:47] + "-" + hex.EncodeToString(sum[:8])
	}
	return id
}

// InternalAddrEnv overrides the derived inter-instance address.
const InternalAddrEnv = "VRAXEL_INTERNAL_ADDR"

// BuildInternalAddr resolves the address sibling instances use to reach
// this one, from the address this process actually listens on.
//
// The env var wins because only the deployment knows the answer in
// general: a K8s pod IP, a second NIC, a mesh address. The fallback is
// this host's name with the listener's port, which is correct for
// bare-metal and single-node installs.
//
// The port must come from the listener and not from externalUrl. Behind a
// load balancer -- which is the only reason to run several instances --
// externalUrl carries the LB's port, so every sibling would advertise a
// port that reaches the balancer instead of itself; and two instances on
// one box (the standard dev setup) would advertise the same address while
// listening on different ones.
func BuildInternalAddr(listenAddr string) string {
	if v := strings.TrimSpace(os.Getenv(InternalAddrEnv)); v != "" {
		return v
	}
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "127.0.0.1"
	}
	// listenAddr is a net.Listen address (":9099", "0.0.0.0:9099"): the
	// host half is a bind wildcard, never something a sibling can dial, so
	// only the port is kept.
	if _, port, err := net.SplitHostPort(listenAddr); err == nil && port != "" {
		return net.JoinHostPort(host, port)
	}
	return host
}
