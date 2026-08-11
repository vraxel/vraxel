// Package dbtest provisions a throwaway PostgreSQL database per test:
// CREATE DATABASE, run the embedded migrations, DROP on cleanup. Tests
// exercise the real store layer -- sqlc queries, partial unique indexes,
// FK cascades -- instead of mocks.
//
// Connection comes from TEST_DB_HOST / TEST_DB_PORT / TEST_DB_USER /
// TEST_DB_PASSWORD (defaults match deployment/docker-compose.yaml:
// 127.0.0.1:5432 vraxel/vraxel). When nothing listens there the suite
// skips with an actionable message, so `go test ./...` stays green on
// machines without PostgreSQL; CI provides a service container and runs
// everything for real.
package dbtest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"vraxel.io/vraxel/pkg/db"
	"vraxel.io/vraxel/pkg/db/migrations"
)

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

var (
	host     = env("TEST_DB_HOST", "127.0.0.1")
	port     = env("TEST_DB_PORT", "5432")
	user     = env("TEST_DB_USER", "vraxel")
	password = env("TEST_DB_PASSWORD", "vraxel")

	probeOnce sync.Once
	probeErr  error
)

// adminDSN targets the always-present "postgres" maintenance database,
// used only for CREATE/DROP DATABASE.
func adminDSN() string {
	return fmt.Sprintf("postgres://%s:%s@%s:%s/postgres?sslmode=disable", user, password, host, port)
}

// probe dials PostgreSQL once per test binary so an absent server costs
// one connection timeout, not one per test.
func probe() error {
	probeOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		conn, err := pgx.Connect(ctx, adminDSN())
		if err != nil {
			probeErr = err
			return
		}
		_ = conn.Close(ctx)
	})
	return probeErr
}

// New returns a *db.DB bound to a freshly created, fully migrated
// database. The database is dropped on test cleanup. Skips the test when
// no PostgreSQL is reachable.
func New(t *testing.T) *db.DB {
	t.Helper()
	if err := probe(); err != nil {
		t.Skipf("no PostgreSQL at %s:%s (start deployment/docker-compose.yaml or set TEST_DB_*): %v", host, port, err)
	}

	name := "vraxel_test_" + randHex(6)
	ctx := context.Background()

	admin, err := pgx.Connect(ctx, adminDSN())
	if err != nil {
		t.Fatalf("dbtest: connect admin: %v", err)
	}
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		_ = admin.Close(ctx)
		t.Fatalf("dbtest: create database: %v", err)
	}
	_ = admin.Close(ctx)

	portN, _ := strconv.Atoi(port)
	database, err := db.NewDB(ctx, db.Config{
		Host: host, Port: portN, User: user, Password: password,
		DBName: name, SSLMode: "disable", MaxConns: 4,
	})
	if err != nil {
		t.Fatalf("dbtest: open %s: %v", name, err)
	}
	if err := db.Migrate(ctx, database.GetPool(), migrations.FS); err != nil {
		database.Close()
		t.Fatalf("dbtest: migrate: %v", err)
	}

	t.Cleanup(func() {
		database.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		admin, err := pgx.Connect(ctx, adminDSN())
		if err != nil {
			t.Logf("dbtest: drop %s: %v", name, err)
			return
		}
		defer admin.Close(ctx)
		if _, err := admin.Exec(ctx, "DROP DATABASE "+name+" WITH (FORCE)"); err != nil {
			t.Logf("dbtest: drop %s: %v", name, err)
		}
	})
	return database
}

func randHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
