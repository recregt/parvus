// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package postgres

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/recregt/parvus/internal/store"
)

//go:embed schema.sql
var schema string

const (
	schemaVersion       = 1
	uniqueViolation     = "23505"
	foreignKeyViolation = "23503"
)

var _ store.Store = (*Store)(nil)

type Store struct {
	db *sql.DB
}

func Open(ctx context.Context, url string) (*Store, error) {
	db, err := sql.Open("pgx", url)
	if err != nil {
		return nil, err
	}
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
	if _, err := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(hashtext('parvus.schema'))"); err != nil {
		return err
	}
	var exists bool
	if err := tx.QueryRowContext(ctx, "SELECT to_regclass('parvus_schema') IS NOT NULL").Scan(&exists); err != nil {
		return err
	}
	if !exists {
		if _, err := tx.ExecContext(ctx, schema); err != nil {
			return err
		}
		return tx.Commit()
	}
	var version int
	if err := tx.QueryRowContext(ctx, "SELECT version FROM parvus_schema").Scan(&version); err != nil {
		return err
	}
	if version != schemaVersion {
		return fmt.Errorf("unsupported schema version %d", version)
	}
	return tx.Commit()
}

func (s *Store) write(ctx context.Context, now int64, fn func(*sql.Tx) error) error {
	if err := s.sweep(ctx, now); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) sweep(ctx context.Context, now int64) error {
	statements := []string{
		"DELETE FROM challenges WHERE slot IN " +
			"(SELECT slot FROM challenges WHERE expires <= $1 FOR UPDATE SKIP LOCKED)",
		"DELETE FROM sessions WHERE hash IN " +
			"(SELECT hash FROM sessions WHERE expires <= $1 FOR UPDATE SKIP LOCKED)",
		"UPDATE users SET enroll_hash = NULL, enroll_expires = NULL WHERE id IN " +
			"(SELECT id FROM users WHERE enroll_expires <= $1 FOR UPDATE SKIP LOCKED)",
	}
	for _, statement := range statements {
		if _, err := s.db.ExecContext(ctx, statement, now); err != nil {
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

func meaning(err error) error {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return err
	}
	switch pgErr.Code {
	case uniqueViolation:
		return store.ErrExists
	case foreignKeyViolation:
		return store.ErrNotFound
	default:
		return err
	}
}

func (s *Store) PutChallenge(ctx context.Context, slot string, data []byte, expires, now int64) error {
	return s.write(ctx, now, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			"INSERT INTO challenges (slot, data, expires) VALUES ($1, $2, $3) "+
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
			"DELETE FROM challenges WHERE slot = $1 RETURNING data, expires", slot).Scan(&data, &expires)
		if errors.Is(err, sql.ErrNoRows) || err == nil && expires <= now {
			return store.ErrNotFound
		}
		return err
	})
	return data, err
}

func insertCredential(ctx context.Context, tx *sql.Tx, credential store.Credential) error {
	_, err := tx.ExecContext(ctx,
		"INSERT INTO credentials (id, user_id, data) VALUES ($1, $2, $3)",
		credential.ID, credential.UserID, credential.Data)
	return meaning(err)
}

func (s *Store) CreateUser(ctx context.Context, credential store.Credential, recoveryMAC []byte, now int64) error {
	return s.write(ctx, now, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			"INSERT INTO users (id, cred_count, recovery_mac) VALUES ($1, 1, $2)",
			credential.UserID, recoveryMAC)
		if err != nil {
			return meaning(err)
		}
		return insertCredential(ctx, tx, credential)
	})
}

func (s *Store) User(ctx context.Context, id string, now int64) (store.User, error) {
	user := store.User{ID: id}
	var hash []byte
	var until sql.NullInt64
	err := s.db.QueryRowContext(ctx,
		"SELECT cred_count, enroll_hash, enroll_expires FROM users WHERE id = $1", id).
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
		result, err := tx.ExecContext(ctx, "DELETE FROM users WHERE id = $1", userID)
		return exactlyOne(result, err, store.ErrNotFound)
	})
}

func (s *Store) AddCredential(ctx context.Context, credential store.Credential, limit int, now int64) error {
	return s.write(ctx, now, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx,
			"UPDATE users SET cred_count = cred_count + 1 WHERE id = $1 AND cred_count > 0 AND cred_count < $2",
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
	err := tx.QueryRowContext(ctx, "SELECT cred_count FROM users WHERE id = $1", userID).Scan(&count)
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
		"SELECT id, data FROM credentials WHERE user_id = $1 ORDER BY seq", userID)
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
		result, err := tx.ExecContext(ctx, "UPDATE credentials SET data = $1 WHERE id = $2", data, id)
		return exactlyOne(result, err, store.ErrNotFound)
	})
}

func (s *Store) RemoveCredential(ctx context.Context, userID string, credentialID []byte, now int64) error {
	return s.write(ctx, now, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx,
			"DELETE FROM credentials WHERE id = $1 AND user_id = $2", credentialID, userID)
		if err := exactlyOne(result, err, store.ErrNotFound); err != nil {
			return err
		}
		result, err = tx.ExecContext(ctx,
			"UPDATE users SET cred_count = cred_count - 1 WHERE id = $1 AND cred_count > 1", userID)
		return exactlyOne(result, err, store.ErrLastCredential)
	})
}

func (s *Store) ReplaceRecovery(ctx context.Context, userID string, oldMAC, newMAC []byte, now int64) error {
	return s.write(ctx, now, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx,
			"UPDATE users SET recovery_mac = $1 WHERE id = $2 AND recovery_mac = $3 AND cred_count > 0",
			newMAC, userID, oldMAC)
		return exactlyOne(result, err, store.ErrNotFound)
	})
}

func (s *Store) RotateRecovery(ctx context.Context, userID string, newMAC []byte, now int64) error {
	return s.write(ctx, now, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx,
			"UPDATE users SET recovery_mac = $1 WHERE id = $2 AND cred_count > 0", newMAC, userID)
		return exactlyOne(result, err, store.ErrNotFound)
	})
}

func (s *Store) Reset(ctx context.Context, userID string, tokenHash []byte, until, now int64) error {
	return s.write(ctx, now, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, "DELETE FROM credentials WHERE user_id = $1", userID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM sessions WHERE user_id = $1", userID); err != nil {
			return err
		}
		result, err := tx.ExecContext(ctx,
			"UPDATE users SET cred_count = 0, recovery_mac = NULL, enroll_hash = $1, enroll_expires = $2 WHERE id = $3",
			tokenHash, until, userID)
		return exactlyOne(result, err, store.ErrNotFound)
	})
}

func (s *Store) Enroll(ctx context.Context, credential store.Credential, tokenHash, recoveryMAC []byte, now int64) error {
	return s.write(ctx, now, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx,
			"UPDATE users SET cred_count = 1, recovery_mac = $1, enroll_hash = NULL, enroll_expires = NULL "+
				"WHERE id = $2 AND cred_count = 0 AND enroll_hash = $3 AND enroll_expires > $4",
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
			"INSERT INTO sessions (hash, user_id, expires) SELECT $1, id, $2 FROM users WHERE id = $3",
			hash, expires, userID)
		return exactlyOne(result, meaning(err), store.ErrNotFound)
	})
}

func (s *Store) Session(ctx context.Context, hash []byte, now int64) (string, int64, error) {
	var userID string
	var expires int64
	err := s.db.QueryRowContext(ctx,
		"SELECT user_id, expires FROM sessions WHERE hash = $1 AND expires > $2", hash, now).
		Scan(&userID, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return "", 0, store.ErrNotFound
	}
	return userID, expires, err
}

func (s *Store) ExtendSession(ctx context.Context, hash []byte, expires, now int64) error {
	return s.write(ctx, now, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx,
			"UPDATE sessions SET expires = $1 WHERE hash = $2 AND expires > $3 AND expires < $1",
			expires, hash, now)
		return exactlyOne(result, err, store.ErrNotFound)
	})
}

func (s *Store) DeleteSession(ctx context.Context, hash []byte, now int64) error {
	return s.write(ctx, now, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "DELETE FROM sessions WHERE hash = $1", hash)
		return err
	})
}

func (s *Store) DeleteSessions(ctx context.Context, userID string, now int64) error {
	return s.write(ctx, now, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "DELETE FROM sessions WHERE user_id = $1", userID)
		return err
	})
}
