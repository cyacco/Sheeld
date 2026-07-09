package backendconfig

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/cyacco/Sheeld/internal/shared/crypto"
	"github.com/cyacco/Sheeld/internal/shared/domain"
)

// Snapshotter persists the last-applied workspace config to disk, encrypted
// with AES-256-GCM, so a data plane restarted during a control-plane outage
// can resume serving from the cached config instead of 503ing until the
// control plane returns.
//
// The payload contains plaintext LLM keys, so it is never written
// unencrypted; the file is created 0600 and written atomically
// (tmp + rename).
type Snapshotter struct {
	path   string
	hexKey string
}

// NewSnapshotter creates a snapshotter writing to path with the given
// 64-char hex AES-256 key.
func NewSnapshotter(path, hexKey string) *Snapshotter {
	return &Snapshotter{path: path, hexKey: hexKey}
}

// Save encrypts and atomically writes the config.
func (s *Snapshotter) Save(cfg *domain.WorkspaceConfig) error {
	plain, err := json.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshaling config snapshot: %w", err)
	}
	sealed, err := crypto.Encrypt(string(plain), s.hexKey)
	if err != nil {
		return fmt.Errorf("encrypting config snapshot: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".config-snapshot-*")
	if err != nil {
		return fmt.Errorf("creating snapshot temp file: %w", err)
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return fmt.Errorf("restricting snapshot permissions: %w", err)
	}
	if _, err := tmp.WriteString(sealed); err != nil {
		tmp.Close()
		return fmt.Errorf("writing snapshot: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing snapshot: %w", err)
	}
	if err := os.Rename(tmp.Name(), s.path); err != nil {
		return fmt.Errorf("replacing snapshot: %w", err)
	}
	return nil
}

// Load reads and decrypts the snapshot. Returns os.ErrNotExist (wrapped) if
// no snapshot has been written yet.
func (s *Snapshotter) Load() (*domain.WorkspaceConfig, error) {
	sealed, err := os.ReadFile(s.path)
	if err != nil {
		return nil, fmt.Errorf("reading config snapshot: %w", err)
	}
	plain, err := crypto.Decrypt(string(sealed), s.hexKey)
	if err != nil {
		return nil, fmt.Errorf("decrypting config snapshot: %w", err)
	}
	var cfg domain.WorkspaceConfig
	if err := json.Unmarshal([]byte(plain), &cfg); err != nil {
		return nil, fmt.Errorf("decoding config snapshot: %w", err)
	}
	return &cfg, nil
}
