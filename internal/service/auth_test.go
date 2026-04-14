//go:build integration

// Tests in this file require a real PostgreSQL database because
// AuthService depends on the concrete *generated.Queries type from sqlc
// (not an interface) — there is no straightforward way to mock query
// methods without significant refactoring. The tests run under the
// `integration` build tag and spin up a throwaway Postgres container.
//
// Run with:
//
//	go test -tags=integration ./internal/service/...

package service

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/sheeld/sheeld/internal/db"
	"github.com/sheeld/sheeld/internal/db/generated"
)

var (
	authTestPool *pgxpool.Pool
	authTestCtr  *postgres.PostgresContainer
)

func TestMain(m *testing.M) {
	ctx := context.Background()

	var err error
	authTestCtr, err = postgres.Run(ctx, "postgres:16-alpine",
		postgres.WithDatabase("sheeld_test"),
		postgres.WithUsername("sheeld"),
		postgres.WithPassword("sheeld_test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to start postgres container: %v\n", err)
		os.Exit(1)
	}

	connStr, err := authTestCtr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to get connection string: %v\n", err)
		os.Exit(1)
	}

	authTestPool, err = pgxpool.New(ctx, connStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to connect to database: %v\n", err)
		os.Exit(1)
	}

	if err := db.RunMigrations(ctx, authTestPool); err != nil {
		fmt.Fprintf(os.Stderr, "failed to run migrations: %v\n", err)
		os.Exit(1)
	}

	code := m.Run()

	authTestPool.Close()
	authTestCtr.Terminate(ctx)

	os.Exit(code)
}

// TestValidateAPIKey_RejectsRevoked verifies that ValidateAPIKey refuses
// to authenticate an API key after it has been revoked, and that the
// returned error matches the not-found path so revocation state is not
// leaked to callers.
func TestValidateAPIKey_RejectsRevoked(t *testing.T) {
	ctx := context.Background()
	queries := generated.New(authTestPool)
	svc := NewAuthService(queries, "test-jwt-secret", time.Hour)

	// 1. Register a user/org so we have an organization_id to attach
	// the API key to.
	reg, err := svc.Register(ctx, "Revoke Test Org", "revoke@example.com", "strongpassword123")
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	// 2. Create an API key.
	created, err := svc.CreateAPIKey(ctx, reg.Organization.ID, "revoke-test-key")
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}
	rawKey := created.RawKey

	// 3. Validate it — should succeed and return the org ID.
	gotOrgID, err := svc.ValidateAPIKey(ctx, rawKey)
	if err != nil {
		t.Fatalf("validate before revoke: unexpected error: %v", err)
	}
	if gotOrgID != reg.Organization.ID {
		t.Fatalf("validate before revoke: org ID mismatch: got %s, want %s", gotOrgID, reg.Organization.ID)
	}

	// 4. Revoke the API key.
	if err := svc.RevokeAPIKey(ctx, reg.Organization.ID, created.APIKey.ID); err != nil {
		t.Fatalf("revoke api key: %v", err)
	}

	// 5. Validate again — should fail with the same "invalid API key"
	// error returned by the not-found path. Use a bogus key to capture
	// the canonical not-found error message and assert the revoked path
	// matches it exactly.
	_, notFoundErr := svc.ValidateAPIKey(ctx, "shld_definitely_not_a_real_key")
	if notFoundErr == nil {
		t.Fatal("validate with bogus key: expected error, got nil")
	}

	_, revokedErr := svc.ValidateAPIKey(ctx, rawKey)
	if revokedErr == nil {
		t.Fatal("validate after revoke: expected error, got nil")
	}
	if revokedErr.Error() != notFoundErr.Error() {
		t.Fatalf("validate after revoke: error message %q does not match not-found error %q (must be indistinguishable to avoid leaking revocation state)",
			revokedErr.Error(), notFoundErr.Error())
	}
}
