// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package sqlite

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"net/url"

	driver "modernc.org/sqlite"
	sqlitelib "modernc.org/sqlite/lib"

	"github.com/recregt/parvus/internal/store"
)

//go:embed schema.sql
var schema string

const (
	schemaVersion   = 1
	primaryCodeMask = 0xff
)

var _ store.Store = (*Store)(nil)

type Store struct {
	db *sql.DB
}

func Open(ctx context.Context, path string) (*Store, error) {
	dsn := url.URL{
		Scheme:   "file",
		OmitHost: true,
		Path:     path,
		RawQuery: url.Values{"_pragma": {
			"foreign_keys(1)",
			"journal_mode(WAL)",
			"locking_mode(EXCLUSIVE)",
		}}.Encode(),
	}
	db, err := sql.Open("sqlite", dsn.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if err := s.migrate(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var version int
	if err := tx.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	switch version {
	case schemaVersion:
		return tx.Commit()
	case 0:
		if _, err := tx.ExecContext(ctx, schema); err != nil {
			return err
		}
		return tx.Commit()
	default:
		return fmt.Errorf("unsupported schema version %d", version)
	}
}

func (s *Store) write(ctx context.Context, now int64, fn func(*sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := sweep(ctx, tx, now); err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}

func sweep(ctx context.Context, tx *sql.Tx, now int64) error {
	statements := []string{
		"DELETE FROM challenges WHERE expires <= ?",
		"DELETE FROM sessions WHERE expires <= ?",
		"UPDATE users SET enroll_hash = NULL, enroll_expires = NULL WHERE enroll_expires <= ?",
	}
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement, now); err != nil {
			return err
		}
	}
	return nil
}

func exactlyOne(result sql.Result, err error, otherwise error) error {
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return otherwise
	}
	return nil
}

func isConstraint(err error) bool {
	var sqliteErr *driver.Error
	return errors.As(err, &sqliteErr) && sqliteErr.Code()&primaryCodeMask == sqlitelib.SQLITE_CONSTRAINT
}

func (s *Store) PutChallenge(ctx context.Context, slot string, data []byte, expires, now int64) error {
	return s.write(ctx, now, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			"INSERT INTO challenges (slot, data, expires) VALUES (?, ?, ?) "+
				"ON CONFLICT (slot) DO UPDATE SET data = excluded.data, expires = excluded.expires",
			slot, data, expires)
		return err
	})
}

func (s *Store) TakeChallenge(ctx context.Context, slot string, now int64) ([]byte, error) {
	var data []byte
	err := s.write(ctx, now, func(tx *sql.Tx) error {
		var expires int64
		err := tx.QueryRowContext(ctx,
			"DELETE FROM challenges WHERE slot = ? RETURNING data, expires", slot).Scan(&data, &expires)
		if errors.Is(err, sql.ErrNoRows) || err == nil && expires <= now {
			return store.ErrNotFound
		}
		return err
	})
	return data, err
}

func insertCredential(ctx context.Context, tx *sql.Tx, credential store.Credential) error {
	_, err := tx.ExecContext(ctx,
		"INSERT INTO credentials (id, user_id, data) VALUES (?, ?, ?)",
		credential.ID, credential.UserID, credential.Data)
	if isConstraint(err) {
		return store.ErrExists
	}
	return err
}

func (s *Store) CreateUser(ctx context.Context, credential store.Credential, recoveryMAC []byte, now int64) error {
	return s.write(ctx, now, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			"INSERT INTO users (id, cred_count, recovery_mac) VALUES (?, 1, ?)",
			credential.UserID, recoveryMAC)
		if isConstraint(err) {
			return store.ErrExists
		}
		if err != nil {
			return err
		}
		return insertCredential(ctx, tx, credential)
	})
}

func (s *Store) User(ctx context.Context, id string, now int64) (store.User, error) {
	user := store.User{ID: id}
	var hash []byte
	var until sql.NullInt64
	err := s.db.QueryRowContext(ctx,
		"SELECT cred_count, enroll_hash, enroll_expires FROM users WHERE id = ?", id).
		Scan(&user.CredCount, &hash, &until)
	if errors.Is(err, sql.ErrNoRows) {
		return store.User{}, store.ErrNotFound
	}
	if err != nil {
		return store.User{}, err
	}
	if until.Valid && until.Int64 > now {
		user.EnrollHash = hash
		user.EnrollUntil = until.Int64
	}
	return user, nil
}

func (s *Store) DeleteUser(ctx context.Context, userID string, now int64) error {
	return s.write(ctx, now, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, "DELETE FROM users WHERE id = ?", userID)
		return exactlyOne(result, err, store.ErrNotFound)
	})
}

func (s *Store) AddCredential(ctx context.Context, credential store.Credential, limit int, now int64) error {
	return s.write(ctx, now, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx,
			"UPDATE users SET cred_count = cred_count + 1 WHERE id = ? AND cred_count > 0 AND cred_count < ?",
			credential.UserID, limit)
		if err != nil {
			return err
		}
		updated, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if updated == 0 {
			return whyNotAdded(ctx, tx, credential.UserID)
		}
		return insertCredential(ctx, tx, credential)
	})
}

func whyNotAdded(ctx context.Context, tx *sql.Tx, userID string) error {
	var count int
	err := tx.QueryRowContext(ctx, "SELECT cred_count FROM users WHERE id = ?", userID).Scan(&count)
	switch {
	case errors.Is(err, sql.ErrNoRows) || err == nil && count == 0:
		return store.ErrNotFound
	case err != nil:
		return err
	default:
		return store.ErrLimit
	}
}

func (s *Store) Credentials(ctx context.Context, userID string) ([]store.Credential, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT id, data FROM credentials WHERE user_id = ? ORDER BY rowid", userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []store.Credential
	for rows.Next() {
		credential := store.Credential{UserID: userID}
		if err := rows.Scan(&credential.ID, &credential.Data); err != nil {
			return nil, err
		}
		out = append(out, credential)
	}
	return out, rows.Err()
}

func (s *Store) UpdateCredential(ctx context.Context, id, data []byte, now int64) error {
	return s.write(ctx, now, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, "UPDATE credentials SET data = ? WHERE id = ?", data, id)
		return exactlyOne(result, err, store.ErrNotFound)
	})
}

func (s *Store) RemoveCredential(ctx context.Context, userID string, credentialID []byte, now int64) error {
	return s.write(ctx, now, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx,
			"DELETE FROM credentials WHERE id = ? AND user_id = ?", credentialID, userID)
		if err := exactlyOne(result, err, store.ErrNotFound); err != nil {
			return err
		}
		result, err = tx.ExecContext(ctx,
			"UPDATE users SET cred_count = cred_count - 1 WHERE id = ? AND cred_count > 1", userID)
		return exactlyOne(result, err, store.ErrLastCredential)
	})
}

func (s *Store) ReplaceRecovery(ctx context.Context, userID string, oldMAC, newMAC []byte, now int64) error {
	return s.write(ctx, now, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx,
			"UPDATE users SET recovery_mac = ? WHERE id = ? AND recovery_mac = ? AND cred_count > 0",
			newMAC, userID, oldMAC)
		return exactlyOne(result, err, store.ErrNotFound)
	})
}

func (s *Store) RotateRecovery(ctx context.Context, userID string, newMAC []byte, now int64) error {
	return s.write(ctx, now, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx,
			"UPDATE users SET recovery_mac = ? WHERE id = ? AND cred_count > 0", newMAC, userID)
		return exactlyOne(result, err, store.ErrNotFound)
	})
}

func (s *Store) Reset(ctx context.Context, userID string, tokenHash []byte, until, now int64) error {
	return s.write(ctx, now, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, "DELETE FROM credentials WHERE user_id = ?", userID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM sessions WHERE user_id = ?", userID); err != nil {
			return err
		}
		result, err := tx.ExecContext(ctx,
			"UPDATE users SET cred_count = 0, recovery_mac = NULL, enroll_hash = ?, enroll_expires = ? WHERE id = ?",
			tokenHash, until, userID)
		return exactlyOne(result, err, store.ErrNotFound)
	})
}

func (s *Store) Enroll(ctx context.Context, credential store.Credential, tokenHash, recoveryMAC []byte, now int64) error {
	return s.write(ctx, now, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx,
			"UPDATE users SET cred_count = 1, recovery_mac = ?, enroll_hash = NULL, enroll_expires = NULL "+
				"WHERE id = ? AND cred_count = 0 AND enroll_hash = ? AND enroll_expires > ?",
			recoveryMAC, credential.UserID, tokenHash, now)
		if err := exactlyOne(result, err, store.ErrNotFound); err != nil {
			return err
		}
		return insertCredential(ctx, tx, credential)
	})
}

func (s *Store) CreateSession(ctx context.Context, hash []byte, userID string, expires, now int64) error {
	return s.write(ctx, now, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx,
			"INSERT INTO sessions (hash, user_id, expires) SELECT ?, id, ? FROM users WHERE id = ?",
			hash, expires, userID)
		if isConstraint(err) {
			return store.ErrExists
		}
		return exactlyOne(result, err, store.ErrNotFound)
	})
}

func (s *Store) Session(ctx context.Context, hash []byte, now int64) (string, int64, error) {
	var userID string
	var expires int64
	err := s.db.QueryRowContext(ctx,
		"SELECT user_id, expires FROM sessions WHERE hash = ? AND expires > ?", hash, now).
		Scan(&userID, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return "", 0, store.ErrNotFound
	}
	return userID, expires, err
}

func (s *Store) ExtendSession(ctx context.Context, hash []byte, expires, now int64) error {
	return s.write(ctx, now, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx,
			"UPDATE sessions SET expires = ? WHERE hash = ? AND expires > ? AND expires < ?",
			expires, hash, now, expires)
		return exactlyOne(result, err, store.ErrNotFound)
	})
}

func (s *Store) DeleteSession(ctx context.Context, hash []byte, now int64) error {
	return s.write(ctx, now, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "DELETE FROM sessions WHERE hash = ?", hash)
		return err
	})
}

func (s *Store) DeleteSessions(ctx context.Context, userID string, now int64) error {
	return s.write(ctx, now, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "DELETE FROM sessions WHERE user_id = ?", userID)
		return err
	})
}
