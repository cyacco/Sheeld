package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sheeld/sheeld/internal/db/generated"
)

// fakeRow implements pgx.Row by scanning fields from a slice of pointers.
type fakeRow struct {
	values []any
	err    error
}

func (r *fakeRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	if len(dest) != len(r.values) {
		return errors.New("fakeRow: dest length mismatch")
	}
	for i, d := range dest {
		switch dst := d.(type) {
		case *uuid.UUID:
			*dst = r.values[i].(uuid.UUID)
		case *string:
			*dst = r.values[i].(string)
		case *time.Time:
			*dst = r.values[i].(time.Time)
		case *pgtype.Timestamptz:
			*dst = r.values[i].(pgtype.Timestamptz)
		default:
			return errors.New("fakeRow: unsupported dest type")
		}
	}
	return nil
}

// fakeDBTX implements generated.DBTX. Only QueryRow is exercised by the
// ValidateAPIKey path; the other methods are stubs.
type fakeDBTX struct {
	queryRow func(ctx context.Context, sql string, args ...any) pgx.Row
}

func (f *fakeDBTX) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, errors.New("not implemented")
}
func (f *fakeDBTX) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeDBTX) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return f.queryRow(ctx, sql, args...)
}

// makeStoredKey builds the prefix + hash for a given raw key, matching
// CreateAPIKey's logic. Uses the same apiKeyPrefixLen constant that
// CreateAPIKey and ValidateAPIKey share.
func makeStoredKey(rawKey string) (prefix, hash string) {
	prefix = rawKey[:apiKeyPrefixLen]
	sum := sha256.Sum256([]byte(rawKey))
	hash = hex.EncodeToString(sum[:])
	return
}

func TestValidateAPIKey(t *testing.T) {
	orgID := uuid.New()
	keyID := uuid.New()

	// Use a deterministic raw key matching the "shld_" + 64 hex char format.
	rawKey := "shld_" + strings.Repeat("a", 64)
	storedPrefix, storedHash := makeStoredKey(rawKey)

	rowFor := func(prefix, hash string) *fakeRow {
		return &fakeRow{values: []any{
			keyID,
			orgID,
			"test-key",
			hash,
			prefix,
			time.Now(),
			pgtype.Timestamptz{},
		}}
	}

	tests := []struct {
		name      string
		submitted string
		row       pgx.Row
		wantOrg   uuid.UUID
		wantErr   bool
	}{
		{
			name:      "valid key authenticates",
			submitted: rawKey,
			row:       rowFor(storedPrefix, storedHash),
			wantOrg:   orgID,
			wantErr:   false,
		},
		{
			name:      "wrong full key with matching prefix is rejected",
			submitted: "shld_" + strings.Repeat("a", 8) + strings.Repeat("b", 56),
			// Stored row matches the prefix (first 13 chars are "shld_aaaaaaaa")
			// but the stored hash is for the original rawKey. The submitted key
			// shares the prefix but its sha256 differs, so constant-time compare
			// must fail.
			row:     rowFor(storedPrefix, storedHash),
			wantOrg: uuid.Nil,
			wantErr: true,
		},
		{
			name:      "no row returned is rejected",
			submitted: rawKey,
			row:       &fakeRow{err: pgx.ErrNoRows},
			wantOrg:   uuid.Nil,
			wantErr:   true,
		},
		{
			name:      "submitted key shorter than prefix is rejected",
			submitted: "shld_",
			row:       nil, // should never be queried
			wantOrg:   uuid.Nil,
			wantErr:   true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			db := &fakeDBTX{
				queryRow: func(ctx context.Context, sql string, args ...any) pgx.Row {
					if tc.row == nil {
						t.Fatalf("unexpected QueryRow call")
					}
					return tc.row
				},
			}
			svc := NewAuthService(generated.New(db), "test-secret", time.Hour)

			gotOrg, err := svc.ValidateAPIKey(context.Background(), tc.submitted)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if gotOrg != tc.wantOrg {
				t.Errorf("org id: got %s want %s", gotOrg, tc.wantOrg)
			}
		})
	}
}
