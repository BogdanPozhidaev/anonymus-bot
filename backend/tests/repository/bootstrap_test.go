package repository_test

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"

	"github.com/fastcheck/anonymus_bot/backend/internal/auth"
	"github.com/fastcheck/anonymus_bot/backend/internal/repository"
	"github.com/golang-migrate/migrate/v4"
)

func TestBootstrapAdminIsAtomicAndOneTime(t *testing.T) {
	pool := setupTestDB(t)
	ctx := context.Background()
	operators := repository.NewOperatorRepository(pool)
	hash, err := auth.HashPassword("fixture")
	if err != nil {
		t.Fatal(err)
	}
	type result struct {
		login   string
		created bool
		err     error
	}
	results := make(chan result, 2)
	var wg sync.WaitGroup
	for _, login := range []string{"first", "second"} {
		wg.Go(func() {
			created, err := operators.BootstrapAdmin(ctx, login, login, hash)
			results <- result{login, created, err}
		})
	}
	wg.Wait()
	close(results)
	winner := ""
	denied := 0
	for result := range results {
		if result.created && result.err == nil {
			winner = result.login
		} else if errors.Is(result.err, repository.ErrBootstrapInitialized) {
			denied++
		} else {
			t.Fatalf("unexpected bootstrap result: %v", result.err)
		}
	}
	if winner == "" || denied != 1 {
		t.Fatal("expected one administrator")
	}
	if created, err := operators.BootstrapAdmin(ctx, winner, "changed", "fake-hash"); err != nil || created {
		t.Fatal("repeat was not a no-op")
	}
	admin, err := operators.GetByLogin(ctx, winner)
	if err != nil || admin.Name != winner || admin.Role != "admin" || admin.PasswordHash == nil || *admin.PasswordHash != hash {
		t.Fatal("bootstrap account changed")
	}
	if _, err := pool.Exec(ctx, "DELETE FROM operators WHERE id=$1", admin.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := operators.BootstrapAdmin(ctx, "third", "third", hash); !errors.Is(err, repository.ErrBootstrapInitialized) {
		t.Fatal("removing operators reopened bootstrap")
	}
}

func TestBootstrapUpgradeKeepsExistingDatabaseClosed(t *testing.T) {
	pool := setupTestDB(t)
	ctx := context.Background()
	migration, err := migrate.New("file://"+filepath.ToSlash(migrationsPath()), pool.Config().ConnString())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = migration.Close() }()
	if err := migration.Migrate(5); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO operators (name, login, role) VALUES ('Fixture', 'existing', 'operator')"); err != nil {
		t.Fatal(err)
	}
	if err := migration.Migrate(6); err != nil {
		t.Fatal(err)
	}
	operators := repository.NewOperatorRepository(pool)
	if _, err := operators.BootstrapAdmin(ctx, "new", "Fixture", "fixture-hash"); !errors.Is(err, repository.ErrBootstrapInitialized) {
		t.Fatal("an existing operator must prevent bootstrap")
	}
	if _, err := pool.Exec(ctx, "DELETE FROM operators WHERE login='existing'"); err != nil {
		t.Fatal(err)
	}
	if _, err := operators.BootstrapAdmin(ctx, "new", "Fixture", "fixture-hash"); !errors.Is(err, repository.ErrBootstrapInitialized) {
		t.Fatal("migration did not persist the initialized state")
	}
}
