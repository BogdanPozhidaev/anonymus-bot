package repository_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/golang-migrate/migrate/v4"
)

func TestAllMigrationsRoundTrip(t *testing.T) {
	ctx := context.Background()
	pool := setupTestDB(t)
	m, err := migrate.New("file://"+filepath.ToSlash(migrationsPath()), pool.Config().ConnString())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = m.Close() })
	latest, dirty, err := m.Version()
	if err != nil || dirty || latest == 0 {
		t.Fatalf("initial version=%d dirty=%v: %v", latest, dirty, err)
	}
	if err := m.Down(); err != nil {
		t.Fatalf("migrate down: %v", err)
	}
	if _, dirty, err := m.Version(); dirty || !errors.Is(err, migrate.ErrNilVersion) {
		t.Fatalf("expected empty schema version, dirty=%v: %v", dirty, err)
	}
	var tables int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM pg_tables WHERE schemaname = 'public' AND tablename <> 'schema_migrations'`).Scan(&tables); err != nil || tables != 0 {
		t.Fatalf("down left %d application tables: %v", tables, err)
	}
	if err := m.Up(); err != nil {
		t.Fatalf("second migrate up: %v", err)
	}
	version, dirty, err := m.Version()
	if err != nil || dirty || version != latest {
		t.Fatalf("restored version=%d want=%d dirty=%v: %v", version, latest, dirty, err)
	}
}

func TestP0MigrationUpgradesExistingData(t *testing.T) {
	ctx := context.Background()
	pool := setupTestDB(t)
	migration, err := migrate.New("file://"+filepath.ToSlash(migrationsPath()), pool.Config().ConnString())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = migration.Close() })
	if err := migration.Migrate(4); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO operators (name,login,role,status) VALUES ('fixture','fixture-legacy','admin','active');
  INSERT INTO audit_log (actor_id,action) VALUES (1,'session_stop_requested'),(1,'operator_updated'),(1,'legacy_action');
  INSERT INTO audit_log (action) VALUES ('pii_anonymized');`); err != nil {
		t.Fatal(err)
	}
	if err := migration.Steps(1); err != nil {
		t.Fatal(err)
	}
	var version int
	var dirty bool
	if err := pool.QueryRow(ctx, "SELECT version,dirty FROM schema_migrations").Scan(&version, &dirty); err != nil || version != 5 || dirty {
		t.Fatalf("migration version=%d dirty=%v error=%v", version, dirty, err)
	}
	var classified string
	if err := pool.QueryRow(ctx, "SELECT string_agg(actor_type, ',' ORDER BY id) FROM audit_log").Scan(&classified); err != nil || classified != "user,operator,unknown,system" {
		t.Fatalf("unexpected actor types %q, %v", classified, err)
	}
	var authVersion int
	if err := pool.QueryRow(ctx, "SELECT auth_version FROM operators WHERE login='fixture-legacy'").Scan(&authVersion); err != nil || authVersion != 1 {
		t.Fatalf("initial auth version=%d, %v", authVersion, err)
	}
	if err := migration.Steps(-1); err != nil {
		t.Fatal(err)
	}
	var remaining int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM audit_log").Scan(&remaining); err != nil || remaining != 4 {
		t.Fatalf("rollback retained %d records, err=%v", remaining, err)
	}
}
