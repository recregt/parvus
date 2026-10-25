// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package store

import (
	"context"
	"errors"
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

type Store interface {
	PutChallenge(ctx context.Context, slot string, data []byte, expires, now int64) error
	TakeChallenge(ctx context.Context, slot string, now int64) ([]byte, error)

	CreateUser(ctx context.Context, credential Credential, recoveryMAC []byte, now int64) error
	User(ctx context.Context, id string, now int64) (User, error)
	DeleteUser(ctx context.Context, userID string, now int64) error

	AddCredential(ctx context.Context, credential Credential, limit int, now int64) error
	Credentials(ctx context.Context, userID string) ([]Credential, error)
	UpdateCredential(ctx context.Context, id, data []byte, now int64) error
	RemoveCredential(ctx context.Context, userID string, credentialID []byte, now int64) error

	ReplaceRecovery(ctx context.Context, userID string, oldMAC, newMAC []byte, now int64) error
	RotateRecovery(ctx context.Context, userID string, newMAC []byte, now int64) error

	Reset(ctx context.Context, userID string, tokenHash []byte, until, now int64) error
	Enroll(ctx context.Context, credential Credential, tokenHash, recoveryMAC []byte, now int64) error

	CreateSession(ctx context.Context, hash []byte, userID string, expires, now int64) error
	Session(ctx context.Context, hash []byte, now int64) (string, int64, error)
	ExtendSession(ctx context.Context, hash []byte, expires, now int64) error
	DeleteSession(ctx context.Context, hash []byte, now int64) error
	DeleteSessions(ctx context.Context, userID string, now int64) error
}
