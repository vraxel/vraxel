// dev-token generates a long-lived JWT access token for development/testing.
//
// It connects to the database, loads the OIDC signing key, finds the admin
// user, and issues a token with a configurable TTL (default 30 days).
//
// The server authenticates via the vraxel_at HttpOnly cookie (BFF pattern), so
// curl must pass the token as a cookie rather than an Authorization header.
// Non-idempotent requests additionally require the X-CSRF-Token header --
// dev-token bypasses CSRF by not setting an vraxel_csrf cookie, so POST/PUT/etc.
// from curl using this token work without a CSRF header.
//
// Usage:
//
//	go run ./cmd/dev-token
//	go run ./cmd/dev-token -ttl 365d
//	TOKEN=$(go run ./cmd/dev-token)
//	curl --cookie "vraxel_at=$TOKEN" http://localhost:8088/api/iam/v1/users
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"vraxel.io/vraxel/lib/oidc"
	"vraxel.io/vraxel/pkg/db/generated"
)

func main() {
	dbHost := flag.String("db-host", envOrDefault("DB_HOST", "localhost"), "database host")
	dbPort := flag.String("db-port", envOrDefault("DB_PORT", "5432"), "database port")
	dbUser := flag.String("db-user", envOrDefault("DB_USER", "vraxel"), "database user")
	dbPass := flag.String("db-password", envOrDefault("DB_PASSWORD", "vraxel"), "database password")
	dbName := flag.String("db-name", envOrDefault("DB_NAME", "vraxel"), "database name")
	dbSSL := flag.String("db-ssl-mode", envOrDefault("DB_SSL_MODE", "disable"), "database SSL mode")
	username := flag.String("user", "admin", "username to generate token for")
	ttl := flag.String("ttl", "30d", "token TTL (e.g. 1h, 7d, 30d, 365d)")
	algorithm := flag.String("algorithm", "EdDSA", "signing algorithm")
	issuer := flag.String("issuer", "http://localhost:8088", "token issuer")
	flag.Parse()

	duration, err := parseDuration(*ttl)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid TTL %q: %v\n", *ttl, err)
		os.Exit(1)
	}

	dsn := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s",
		*dbUser, *dbPass, *dbHost, *dbPort, *dbName, *dbSSL)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "connect to database: %v\n", err)
		os.Exit(1)
	}
	defer pool.Close()

	queries := generated.New(pool)

	// Load signing key
	keyStore := oidc.NewDBKeyStore(pool, queries)
	keySet, err := keyStore.LoadOrGenerate(*algorithm)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load signing key: %v\n", err)
		os.Exit(1)
	}

	// Find user
	var userID int64
	err = pool.QueryRow(ctx, "SELECT id FROM users WHERE username = $1 AND status = 'active'", *username).Scan(&userID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "find user %q: %v\n", *username, err)
		os.Exit(1)
	}

	// Issue token
	ts := oidc.NewTokenService(keySet, *issuer, duration)
	token, err := ts.IssueAccessToken(userID, "dev-cli", []string{"openid", "profile", "email"})
	if err != nil {
		fmt.Fprintf(os.Stderr, "issue token: %v\n", err)
		os.Exit(1)
	}

	fmt.Print(token)
}

func parseDuration(s string) (time.Duration, error) {
	if strings.HasSuffix(s, "d") {
		days, err := strconv.Atoi(strings.TrimSuffix(s, "d"))
		if err != nil {
			return 0, err
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}
	return time.ParseDuration(s)
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
