// Command init-admin creates the admin user in the database.
//
// It inserts a user with username "admin" and a bcrypt-hashed password
// so that the server's SeedRBAC can bind the platform-admin role on startup.
//
// Usage:
//
//	go run ./cmd/init-admin [flags]
//
// Environment variables (override defaults):
//
//	DB_HOST, DB_PORT, DB_USER, DB_PASSWORD, DB_NAME, DB_SSL_MODE
//	ADMIN_USERNAME, ADMIN_PASSWORD, ADMIN_EMAIL, ADMIN_PHONE
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

func main() {
	// Database connection flags (same defaults as config.yaml)
	dbHost := flag.String("db-host", envOrDefault("DB_HOST", "localhost"), "database host")
	dbPort := flag.String("db-port", envOrDefault("DB_PORT", "5432"), "database port")
	dbUser := flag.String("db-user", envOrDefault("DB_USER", "vraxel"), "database user")
	dbPass := flag.String("db-password", envOrDefault("DB_PASSWORD", "vraxel"), "database password")
	dbName := flag.String("db-name", envOrDefault("DB_NAME", "vraxel"), "database name")
	dbSSL := flag.String("db-ssl-mode", envOrDefault("DB_SSL_MODE", "disable"), "database SSL mode")

	// Admin user flags
	username := flag.String("username", envOrDefault("ADMIN_USERNAME", "admin"), "admin username")
	password := flag.String("password", envOrDefault("ADMIN_PASSWORD", "Admin123!"), "admin password")
	email := flag.String("email", envOrDefault("ADMIN_EMAIL", "admin@vraxel.io"), "admin email")
	phone := flag.String("phone", envOrDefault("ADMIN_PHONE", "13800000000"), "admin phone")
	displayName := flag.String("display-name", envOrDefault("ADMIN_DISPLAY_NAME", "Admin"), "admin display name")

	flag.Parse()

	dsn := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s",
		*dbUser, *dbPass, *dbHost, *dbPort, *dbName, *dbSSL)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to connect to database: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close(ctx)

	// Hash password
	hash, err := bcrypt.GenerateFromPassword([]byte(*password), 10)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to hash password: %v\n", err)
		os.Exit(1)
	}

	// Insert admin user (skip if already exists)
	var id int64
	err = conn.QueryRow(ctx,
		`INSERT INTO users (username, email, display_name, phone, status, password_hash)
		 VALUES ($1, $2, $3, $4, 'active', $5)
		 ON CONFLICT (username) DO UPDATE SET
		   password_hash = EXCLUDED.password_hash,
		   email = EXCLUDED.email,
		   display_name = EXCLUDED.display_name,
		   phone = EXCLUDED.phone,
		   status = 'active',
		   updated_at = now()
		 RETURNING id`,
		*username, *email, *displayName, *phone, string(hash),
	).Scan(&id)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to upsert admin user: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("admin user ready: id=%d username=%s email=%s\n", id, *username, *email)
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
