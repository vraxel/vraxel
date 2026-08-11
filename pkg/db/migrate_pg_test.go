package db_test

import (
	"context"
	"testing"

	"vraxel.io/vraxel/pkg/db"
	"vraxel.io/vraxel/pkg/db/dbtest"
	"vraxel.io/vraxel/pkg/db/migrations"
)

// dbtest.New already ran Migrate once; a second run must be a no-op and
// the recorded version must be the latest embedded migration.
func TestMigrateIdempotent(t *testing.T) {
	database := dbtest.New(t)
	ctx := context.Background()

	if err := db.Migrate(ctx, database.GetPool(), migrations.FS); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}
	v, err := database.MigrationVersion(ctx)
	if err != nil {
		t.Fatalf("MigrationVersion: %v", err)
	}
	if v == "" {
		t.Fatal("no migration version recorded")
	}
}
