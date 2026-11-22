package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"

	"github.com/fastcheck/anonymus_bot/backend/internal/auth"
	"github.com/fastcheck/anonymus_bot/backend/internal/config"
	"github.com/fastcheck/anonymus_bot/backend/internal/db"
	"github.com/fastcheck/anonymus_bot/backend/internal/repository"
)

var validLogin = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,64}$`)

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string, in io.Reader, out io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: admin migrate up|status or admin create-admin --login LOGIN [--name NAME]")
	}
	switch args[0] {
	case "migrate":
		if len(args) != 2 || (args[1] != "up" && args[1] != "status") {
			return errors.New("usage: admin migrate up|status")
		}
		dsn, err := config.DatabaseURL()
		if err != nil {
			return err
		}
		if args[1] == "status" {
			return databaseStatus(dsn, out)
		}
		sourceDir := os.Getenv("MIGRATIONS_DIR")
		if sourceDir == "" {
			sourceDir = "/migrations"
		}
		return migrateDatabase(sourceDir, dsn, out)
	case "create-admin":
		flags := flag.NewFlagSet("create-admin", flag.ContinueOnError)
		flags.SetOutput(io.Discard)
		login := flags.String("login", "", "")
		name := flags.String("name", "Administrator", "")
		if err := flags.Parse(args[1:]); err != nil || flags.NArg() != 0 || !validLogin.MatchString(*login) ||
			strings.TrimSpace(*name) == "" || len(*name) > 128 || strings.ContainsAny(*name, "\x00\r\n") {
			return errors.New("provide --login (1–64 letters, digits, dots, hyphens or underscores) and a valid --name")
		}
		dsn, err := config.DatabaseURL()
		if err != nil {
			return err
		}
		password, err := readPassword(in)
		if err != nil {
			return err
		}
		if err := auth.ValidateNewPassword(password); err != nil {
			return err
		}
		hash, err := auth.HashPassword(password)
		if err != nil {
			return errors.New("could not hash bootstrap password")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		pool, err := db.NewPool(ctx, dsn)
		if err != nil {
			return errors.New("cannot connect to PostgreSQL; check database settings and availability")
		}
		defer pool.Close()
		// Refuse to bootstrap on a database with unapplied or failed migrations.
		var version uint
		var dirty bool
		if err := pool.QueryRow(ctx, "SELECT version, dirty FROM schema_migrations").Scan(&version, &dirty); err != nil || dirty || version < 6 {
			return errors.New("database is not ready; apply migrations and verify their status first")
		}
		created, err := repository.NewOperatorRepository(pool).BootstrapAdmin(ctx, *login, *name, hash)
		if errors.Is(err, repository.ErrBootstrapInitialized) {
			return err
		}
		if err != nil {
			return errors.New("bootstrap database operation failed")
		}
		if created {
			_, _ = fmt.Fprintln(out, "First administrator created.")
		} else {
			_, _ = fmt.Fprintln(out, "Administrator already exists; no changes made.")
		}
		return nil
	default:
		return errors.New("unknown admin command; use migrate or create-admin")
	}
}

func readPassword(in io.Reader) (string, error) {
	data, err := io.ReadAll(io.LimitReader(bufio.NewReader(in), 75))
	password := strings.TrimSuffix(strings.TrimSuffix(string(data), "\n"), "\r")
	if err != nil || len(password) == 0 || len(password) > 72 || strings.TrimSpace(password) == "" || strings.ContainsAny(password, "\x00\r\n") {
		return "", errors.New("provide a single non-empty password of at most 72 bytes on stdin")
	}
	return password, nil
}

func databaseStatus(dsn string, out io.Writer) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := db.NewPool(ctx, dsn)
	if err != nil {
		return errors.New("cannot connect to PostgreSQL; check database settings and availability")
	}
	defer pool.Close()
	var exists bool
	if err := pool.QueryRow(ctx, "SELECT to_regclass('schema_migrations') IS NOT NULL").Scan(&exists); err != nil {
		return errors.New("cannot read migration state")
	}
	if !exists {
		return errors.New("no migrations applied; run migrate up")
	}
	var version int
	var dirty bool
	if err := pool.QueryRow(ctx, "SELECT version, dirty FROM schema_migrations").Scan(&version, &dirty); err != nil {
		return errors.New("no readable migration state; apply migrations or inspect the database")
	}
	return reportMigrationState(out, version, dirty)
}

func migrateDatabase(dir, dsn string, out io.Writer) error {
	m, err := migrate.New("file://"+dir, dsn)
	if err != nil {
		return errors.New("cannot initialize migrations; check the directory and PostgreSQL configuration")
	}
	defer func() { _, _ = m.Close() }()
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		var dirty migrate.ErrDirty
		if errors.As(err, &dirty) {
			return fmt.Errorf("migration version %d is dirty; inspect the database before retrying", dirty.Version)
		}
		return errors.New("migration failed; run migrate status and inspect the migration before retrying")
	}
	version, dirty, err := m.Version()
	if errors.Is(err, migrate.ErrNilVersion) {
		return errors.New("no migrations applied; run migrate up")
	}
	if err != nil {
		return errors.New("cannot read migration state")
	}
	return reportMigrationState(out, int(version), dirty)
}

func reportMigrationState(out io.Writer, version int, dirty bool) error {
	_, _ = fmt.Fprintf(out, "version=%d dirty=%t\n", version, dirty)
	if dirty {
		return errors.New("database migration is dirty")
	}
	return nil
}
