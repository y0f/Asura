package storage

import (
	"context"
	"strings"
	"time"
)

func (s *SQLiteStore) InsertAudit(ctx context.Context, entry *AuditEntry) error {
	now := formatTime(time.Now())
	_, err := s.writeDB.ExecContext(ctx,
		`INSERT INTO audit_log (action, entity, entity_id, api_key_name, detail, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		entry.Action, entry.Entity, entry.EntityID, entry.APIKeyName, entry.Detail, now)
	return err
}

// --- Sessions ---

// --- TOTP Keys ---

func (s *SQLiteStore) CreateTOTPKey(ctx context.Context, key *TOTPKey) error {
	now := formatTime(time.Now())
	res, err := s.writeDB.ExecContext(ctx,
		`INSERT INTO totp_keys (api_key_name, secret, created_at) VALUES (?, ?, ?)`,
		key.APIKeyName, key.Secret, now)
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	key.ID = id
	key.CreatedAt = parseTime(now)
	return nil
}

func (s *SQLiteStore) GetTOTPKey(ctx context.Context, apiKeyName string) (*TOTPKey, error) {
	var key TOTPKey
	var createdAt string
	err := s.readDB.QueryRowContext(ctx,
		`SELECT id, api_key_name, secret, created_at FROM totp_keys WHERE api_key_name=?`, apiKeyName).
		Scan(&key.ID, &key.APIKeyName, &key.Secret, &createdAt)
	if err != nil {
		return nil, err
	}
	key.CreatedAt = parseTime(createdAt)
	return &key, nil
}

func (s *SQLiteStore) DeleteTOTPKey(ctx context.Context, apiKeyName string) error {
	_, err := s.writeDB.ExecContext(ctx, "DELETE FROM totp_keys WHERE api_key_name=?", apiKeyName)
	return err
}

// --- Sessions ---

func (s *SQLiteStore) CreateSession(ctx context.Context, sess *Session) error {
	now := formatTime(time.Now())
	res, err := s.writeDB.ExecContext(ctx,
		`INSERT INTO sessions (token_hash, api_key_name, key_hash, ip_address, created_at, expires_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		sess.TokenHash, sess.APIKeyName, sess.KeyHash, sess.IPAddress, now, formatTime(sess.ExpiresAt))
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	sess.ID = id
	sess.CreatedAt = parseTime(now)
	return nil
}

func (s *SQLiteStore) GetSessionByTokenHash(ctx context.Context, tokenHash string) (*Session, error) {
	var sess Session
	var createdAt, expiresAt string
	err := s.readDB.QueryRowContext(ctx,
		`SELECT id, token_hash, api_key_name, key_hash, ip_address, created_at, expires_at
		 FROM sessions WHERE token_hash=?`, tokenHash).
		Scan(&sess.ID, &sess.TokenHash, &sess.APIKeyName, &sess.KeyHash, &sess.IPAddress, &createdAt, &expiresAt)
	if err != nil {
		return nil, err
	}
	sess.CreatedAt = parseTime(createdAt)
	sess.ExpiresAt = parseTime(expiresAt)
	return &sess, nil
}

func (s *SQLiteStore) DeleteSession(ctx context.Context, tokenHash string) error {
	_, err := s.writeDB.ExecContext(ctx, "DELETE FROM sessions WHERE token_hash=?", tokenHash)
	return err
}

func (s *SQLiteStore) ExtendSession(ctx context.Context, tokenHash string, newExpiry time.Time) error {
	_, err := s.writeDB.ExecContext(ctx,
		"UPDATE sessions SET expires_at=? WHERE token_hash=?",
		formatTime(newExpiry), tokenHash)
	return err
}

func (s *SQLiteStore) DeleteSessionsByAPIKeyName(ctx context.Context, apiKeyName string) (int64, error) {
	res, err := s.writeDB.ExecContext(ctx, "DELETE FROM sessions WHERE api_key_name=?", apiKeyName)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s *SQLiteStore) DeleteSessionsExceptKeyNames(ctx context.Context, validNames []string) (int64, error) {
	if len(validNames) == 0 {
		res, err := s.writeDB.ExecContext(ctx, "DELETE FROM sessions")
		if err != nil {
			return 0, err
		}
		return res.RowsAffected()
	}
	placeholders := make([]string, len(validNames))
	args := make([]any, len(validNames))
	for i, name := range validNames {
		placeholders[i] = "?"
		args[i] = name
	}
	query := "DELETE FROM sessions WHERE api_key_name NOT IN (" + strings.Join(placeholders, ",") + ")"
	res, err := s.writeDB.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s *SQLiteStore) DeleteExpiredSessions(ctx context.Context) (int64, error) {
	now := formatTime(time.Now())
	res, err := s.writeDB.ExecContext(ctx, "DELETE FROM sessions WHERE expires_at < ?", now)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// --- Retention ---

func (s *SQLiteStore) PurgeOldData(ctx context.Context, before time.Time) (int64, error) {
	ts := formatTime(before)
	var totalDeleted int64

	res, err := s.writeDB.ExecContext(ctx, "DELETE FROM check_results WHERE created_at < ?", ts)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	totalDeleted += n

	res, err = s.writeDB.ExecContext(ctx,
		`DELETE FROM incident_events WHERE incident_id IN
		 (SELECT id FROM incidents WHERE status='resolved' AND resolved_at < ?)`, ts)
	if err != nil {
		return totalDeleted, err
	}
	n, _ = res.RowsAffected()
	totalDeleted += n

	res, err = s.writeDB.ExecContext(ctx, "DELETE FROM incidents WHERE status='resolved' AND resolved_at < ?", ts)
	if err != nil {
		return totalDeleted, err
	}
	n, _ = res.RowsAffected()
	totalDeleted += n

	res, err = s.writeDB.ExecContext(ctx, "DELETE FROM content_changes WHERE created_at < ?", ts)
	if err != nil {
		return totalDeleted, err
	}
	n, _ = res.RowsAffected()
	totalDeleted += n

	res, err = s.writeDB.ExecContext(ctx, "DELETE FROM audit_log WHERE created_at < ?", ts)
	if err != nil {
		return totalDeleted, err
	}
	n, _ = res.RowsAffected()
	totalDeleted += n

	expired, err := s.DeleteExpiredSessions(ctx)
	if err != nil {
		return totalDeleted, err
	}
	totalDeleted += expired

	return totalDeleted, nil
}
