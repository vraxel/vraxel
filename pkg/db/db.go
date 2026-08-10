package db

import (
	"context"
	"fmt"
	"net/url"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"
	"vraxel.io/vraxel/pkg/db/generated"
)

// DB holds the database connection pool and query interface.
type DB struct {
	mu      sync.RWMutex
	Pool    *pgxpool.Pool
	Queries *generated.Queries
}

// NewDB creates a DB instance: connection pool + sqlc queries.
func NewDB(ctx context.Context, cfg Config) (*DB, error) {
	pool, err := NewPool(ctx, cfg)
	if err != nil {
		return nil, err
	}
	queries := generated.New(pool)
	return &DB{Pool: pool, Queries: queries}, nil
}

// Close closes the underlying connection pool.
func (d *DB) Close() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.Pool.Close()
}

// GetPool returns the current connection pool in a thread-safe manner.
func (d *DB) GetPool() *pgxpool.Pool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.Pool
}

// GetQueries returns the current queries instance in a thread-safe manner.
func (d *DB) GetQueries() *generated.Queries {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.Queries
}

// Reload creates a new connection pool with the given config, verifies it,
// then atomically replaces the old pool and queries. The old pool is closed
// after the swap.
func (d *DB) Reload(ctx context.Context, cfg Config) error {
	newPool, err := NewPool(ctx, cfg)
	if err != nil {
		return fmt.Errorf("reload database: %w", err)
	}

	d.mu.Lock()
	oldPool := d.Pool
	d.Pool = newPool
	d.Queries = generated.New(newPool)
	d.mu.Unlock()

	oldPool.Close()
	return nil
}

// Config holds PostgreSQL connection parameters.
type Config struct {
	Host     string
	Port     int
	User     string
	Password string
	DBName   string
	SSLMode  string
	MaxConns int32
}

// DSN returns the PostgreSQL connection string with the password masked.
func (c Config) DSN() string {
	sslMode := c.SSLMode
	if sslMode == "" {
		sslMode = "disable"
	}
	port := c.Port
	if port == 0 {
		port = 5432
	}
	return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
		c.User, url.QueryEscape(c.Password), c.Host, port, c.DBName, sslMode)
}

// RedactedDSN returns the DSN with the password replaced for safe logging.
func (c Config) RedactedDSN() string {
	sslMode := c.SSLMode
	if sslMode == "" {
		sslMode = "disable"
	}
	port := c.Port
	if port == 0 {
		port = 5432
	}
	return fmt.Sprintf("postgres://%s:***@%s:%d/%s?sslmode=%s",
		c.User, c.Host, port, c.DBName, sslMode)
}

// NewPool creates a new pgx connection pool and verifies connectivity.
func NewPool(ctx context.Context, cfg Config) (*pgxpool.Pool, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.DSN())
	if err != nil {
		return nil, fmt.Errorf("parse pool config: %w", err)
	}
	if cfg.MaxConns > 0 {
		poolCfg.MaxConns = cfg.MaxConns
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("create connection pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return pool, nil
}
