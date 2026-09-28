// Package pgtest provides isolated PostgreSQL databases for tests. Each call creates a fresh
// schema, applies migrations and drops the schema when the test finishes.
package pgtest

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"vodafone/store/internal/auth"
	"vodafone/store/internal/crm"
	"vodafone/store/internal/postgres"
)

// New returns a migrated database in a private schema. Tests are skipped when
// TEST_DATABASE_URL is unset, unless REQUIRE_DATABASE_TESTS=true (set in CI).
func New(t testing.TB) *postgres.DB {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		if os.Getenv("REQUIRE_DATABASE_TESTS") == "true" {
			t.Fatal("TEST_DATABASE_URL is required")
		}
		t.Skip("TEST_DATABASE_URL not set; skipping database test")
	}
	ctx := context.Background()
	schema := "test_" + strings.ReplaceAll(crm.NewID(), "-", "")[:16]
	admin, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	db, err := postgres.Open(ctx, url, map[string]string{"search_path": schema + ",public"})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() {
		db.Close()
		if _, err := admin.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Errorf("drop schema: %v", err)
		}
		admin.Close(context.Background())
	})
	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

// FastAuth returns an auth service with a low bcrypt cost, for tests only.
func FastAuth(db *postgres.DB) *auth.Service {
	cfg := auth.DefaultConfig()
	cfg.BcryptCost = 4
	return auth.NewService(db, cfg)
}

const Password = "correct-horse-battery"

// Store is a test store with two employees and a manager.
type Store struct {
	ID                 string
	Ioana, Andrei, Mgr crm.User
}

// NewStore creates a store and its accounts, all with Password.
func NewStore(t testing.TB, db *postgres.DB, a *auth.Service, name string) Store {
	t.Helper()
	ctx := context.Background()
	s := Store{ID: crm.NewID()}
	if err := db.InsertStore(ctx, crm.RetailStore{ID: s.ID, Name: name, Timezone: "Europe/Bucharest"}); err != nil {
		t.Fatal(err)
	}
	mk := func(first, role string) crm.User {
		u, err := a.CreateUser(ctx, auth.NewUser{StoreID: s.ID, Email: fmt.Sprintf("%s@%s.test", strings.ToLower(first), strings.ToLower(strings.ReplaceAll(name, " ", ""))), Name: first + " Test", Role: role, Password: Password})
		if err != nil {
			t.Fatal(err)
		}
		return u
	}
	s.Ioana, s.Andrei, s.Mgr = mk("Ioana", crm.RoleEmployee), mk("Andrei", crm.RoleEmployee), mk("Elena", crm.RoleManager)
	return s
}
