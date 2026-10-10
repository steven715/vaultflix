package repository

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// openTestPool connects to the migrated Postgres of the integration stack
// (docker-compose.test.yml's go-test service sets VAULTFLIX_TEST_DATABASE_URL).
// Without it the test is skipped, so the native `task verify` gate stays
// DB-free; VAULTFLIX_REQUIRE_DB=1 turns that skip into a failure where the
// database is the whole point of running. See ADR-0012.
func openTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("VAULTFLIX_TEST_DATABASE_URL")
	if dsn == "" {
		if os.Getenv("VAULTFLIX_REQUIRE_DB") == "1" {
			t.Fatal("VAULTFLIX_TEST_DATABASE_URL not set but VAULTFLIX_REQUIRE_DB=1")
		}
		t.Skip("VAULTFLIX_TEST_DATABASE_URL not set; runs inside task test-integration")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect test database: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}
