package workspaceconfig

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cyacco/Sheeld/internal/shared/domain"
)

func TestComputeVersion(t *testing.T) {
	orgID := uuid.New()
	cfg := &domain.WorkspaceConfig{
		Organizations: []domain.OrgConfig{{
			ID:      orgID,
			APIKeys: []domain.APIKeyConfig{{KeyHash: "abc"}},
		}},
	}

	v1, err := computeVersion(cfg)
	if err != nil {
		t.Fatalf("computeVersion: %v", err)
	}

	// Version and GeneratedAt must not affect the hash.
	cfg.Version = v1
	cfg.GeneratedAt = time.Now()
	v2, err := computeVersion(cfg)
	if err != nil {
		t.Fatalf("computeVersion: %v", err)
	}
	if v1 != v2 {
		t.Errorf("version changed after setting Version/GeneratedAt: %s != %s", v1, v2)
	}

	// Content changes must change the hash.
	cfg.Organizations[0].APIKeys[0].KeyHash = "def"
	v3, err := computeVersion(cfg)
	if err != nil {
		t.Fatalf("computeVersion: %v", err)
	}
	if v3 == v1 {
		t.Error("version unchanged after content change")
	}
}
