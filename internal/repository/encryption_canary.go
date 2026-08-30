package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// GetEncryptionCanary returns the sealed canary blob and whether the
// singleton row exists yet. It does not seal or open anything itself —
// callers (main.go, via envcrypt.VerifyOrInitCanary) own the sealer.
func (s *Store) GetEncryptionCanary(ctx context.Context) (sealed []byte, found bool, err error) {
	err = s.db.QueryRowContext(ctx, `SELECT canary_ct FROM encryption_canary WHERE id=1`).Scan(&sealed)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("get encryption canary: %w", err)
	}
	return sealed, true, nil
}

// SetEncryptionCanary seeds the singleton canary row on first boot. It is
// never called again after that — the canary is checked, not rewritten.
func (s *Store) SetEncryptionCanary(ctx context.Context, sealed []byte) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO encryption_canary(id, canary_ct, created_at) VALUES(1, ?, ?)`,
		sealed, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("set encryption canary: %w", err)
	}
	return nil
}
