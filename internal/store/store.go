// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package store

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"net/url"

	"modernc.org/sqlite"
	sqlitelib "modernc.org/sqlite/lib"
)

//go:embed schema.sql
var schema string

const (
	schemaVersion   = 1
	primaryCodeMask = 0xff
)

var (
	ErrNotFound       = errors.New("not found")
	ErrExists         = errors.New("already exists")
	ErrLimit          = errors.New("credential limit reached")
	ErrLastCredential = errors.New("last credential")
)

type Credential struct {
	ID     []byte
	UserID string
	Data   []byte
}

type User struct {
	ID          string
	CredCount   int
	EnrollHash  []byte
	EnrollUntil int64
}

type Store struct {
	db *sql.DB
}

func Open(ctx context.Context, path string) (*Store, error) {
	dsn := url.URL{
		Scheme: "file",
		Path:   path,
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

func affected(result sql.Result, err error) (int64, error) {
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func exactlyOne(result sql.Result, err error, otherwise error) error {
	n, err := affected(result, err)
	if err != nil {
		return err
	}
	if n != 1 {
		return otherwise
	}
	return nil
}

func isConstraint(err error) bool {
	var sqliteErr *sqlite.Error
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
		err := tx.QueryRowContext(ctx,
			"DELETE FROM challenges WHERE slot = ? RETURNING data", slot).Scan(&data)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	})
	return data, err
}

func insertCredential(ctx context.Context, tx *sql.Tx, credential Credential) error {
	_, err := tx.ExecContext(ctx,
		"INSERT INTO credentials (id, user_id, data) VALUES (?, ?, ?)",
		credential.ID, credential.UserID, credential.Data)
	if isConstraint(err) {
		return ErrExists
	}
	return err
}

func (s *Store) CreateUser(ctx context.Context, credential Credential, recoveryMAC []byte, now int64) error {
	return s.write(ctx, now, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			"INSERT INTO users (id, cred_count, recovery_mac) VALUES (?, 1, ?)",
			credential.UserID, recoveryMAC)
		if isConstraint(err) {
			return ErrExists
		}
		if err != nil {
			return err
		}
		return insertCredential(ctx, tx, credential)
	})
}

func (s *Store) AddCredential(ctx context.Context, credential Credential, limit int, now int64) error {
	return s.write(ctx, now, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx,
			"UPDATE users SET cred_count = cred_count + 1 WHERE id = ? AND cred_count > 0 AND cred_count < ?",
			credential.UserID, limit)
		if err := exactlyOne(result, err, ErrLimit); err != nil {
			return err
		}
		return insertCredential(ctx, tx, credential)
	})
}

func (s *Store) Enroll(ctx context.Context, credential Credential, tokenHash, recoveryMAC []byte, now int64) error {
	return s.write(ctx, now, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx,
			"UPDATE users SET cred_count = 1, recovery_mac = ?, enroll_hash = NULL, enroll_expires = NULL "+
				"WHERE id = ? AND cred_count = 0 AND enroll_hash = ? AND enroll_expires > ?",
			recoveryMAC, credential.UserID, tokenHash, now)
		if err := exactlyOne(result, err, ErrNotFound); err != nil {
			return err
		}
		return insertCredential(ctx, tx, credential)
	})
}

func (s *Store) RemoveCredential(ctx context.Context, userID string, credentialID []byte, now int64) error {
	return s.write(ctx, now, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx,
			"DELETE FROM credentials WHERE id = ? AND user_id = ?", credentialID, userID)
		if err := exactlyOne(result, err, ErrNotFound); err != nil {
			return err
		}
		result, err = tx.ExecContext(ctx,
			"UPDATE users SET cred_count = cred_count - 1 WHERE id = ? AND cred_count > 1", userID)
		return exactlyOne(result, err, ErrLastCredential)
	})
}

func (s *Store) UpdateCredential(ctx context.Context, id, data []byte, now int64) error {
	return s.write(ctx, now, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, "UPDATE credentials SET data = ? WHERE id = ?", data, id)
		return exactlyOne(result, err, ErrNotFound)
	})
}

func (s *Store) Credentials(ctx context.Context, userID string) ([]Credential, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT id, data FROM credentials WHERE user_id = ? ORDER BY rowid", userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []Credential
	for rows.Next() {
		credential := Credential{UserID: userID}
		if err := rows.Scan(&credential.ID, &credential.Data); err != nil {
			return nil, err
		}
		out = append(out, credential)
	}
	return out, rows.Err()
}

func (s *Store) User(ctx context.Context, id string) (User, error) {
	user := User{ID: id}
	var hash []byte
	var until sql.NullInt64
	err := s.db.QueryRowContext(ctx,
		"SELECT cred_count, enroll_hash, enroll_expires FROM users WHERE id = ?", id).
		Scan(&user.CredCount, &hash, &until)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, err
	}
	user.EnrollHash = hash
	user.EnrollUntil = until.Int64
	return user, nil
}

func (s *Store) ReplaceRecovery(ctx context.Context, userID string, oldMAC, newMAC []byte, now int64) error {
	return s.write(ctx, now, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx,
			"UPDATE users SET recovery_mac = ? WHERE id = ? AND recovery_mac = ? AND cred_count > 0",
			newMAC, userID, oldMAC)
		return exactlyOne(result, err, ErrNotFound)
	})
}

func (s *Store) RotateRecovery(ctx context.Context, userID string, newMAC []byte, now int64) error {
	return s.write(ctx, now, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx,
			"UPDATE users SET recovery_mac = ? WHERE id = ? AND cred_count > 0", newMAC, userID)
		return exactlyOne(result, err, ErrNotFound)
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
		return exactlyOne(result, err, ErrNotFound)
	})
}

func (s *Store) DeleteUser(ctx context.Context, userID string, now int64) error {
	return s.write(ctx, now, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, "DELETE FROM users WHERE id = ?", userID)
		return exactlyOne(result, err, ErrNotFound)
	})
}

func (s *Store) CreateSession(ctx context.Context, hash []byte, userID string, expires, now int64) error {
	return s.write(ctx, now, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			"INSERT INTO sessions (hash, user_id, expires) VALUES (?, ?, ?)", hash, userID, expires)
		return err
	})
}

func (s *Store) Session(ctx context.Context, hash []byte, now int64) (string, int64, error) {
	var userID string
	var expires int64
	err := s.db.QueryRowContext(ctx,
		"SELECT user_id, expires FROM sessions WHERE hash = ? AND expires > ?", hash, now).
		Scan(&userID, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return "", 0, ErrNotFound
	}
	return userID, expires, err
}

func (s *Store) ExtendSession(ctx context.Context, hash []byte, expires, now int64) error {
	return s.write(ctx, now, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx,
			"UPDATE sessions SET expires = ? WHERE hash = ? AND expires > ? AND expires < ?",
			expires, hash, now, expires)
		return exactlyOne(result, err, ErrNotFound)
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
