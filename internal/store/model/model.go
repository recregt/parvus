// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package model

import (
	"bytes"
	"context"
	"slices"
	"sync"

	"github.com/recregt/parvus/internal/store"
)

var _ store.Store = (*Store)(nil)

type Store struct {
	mu          sync.Mutex
	users       map[string]*user
	credentials []store.Credential
	sessions    map[string]session
	challenges  map[string]challenge
}

type user struct {
	recovery    []byte
	enrollHash  []byte
	enrollUntil int64
}

type session struct {
	userID  string
	expires int64
}

type challenge struct {
	data    []byte
	expires int64
}

func New() *Store {
	return &Store{
		users:      map[string]*user{},
		sessions:   map[string]session{},
		challenges: map[string]challenge{},
	}
}

func (s *Store) PutChallenge(_ context.Context, slot string, data []byte, expires, _ int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.challenges[slot] = challenge{data: bytes.Clone(data), expires: expires}
	return nil
}

func (s *Store) TakeChallenge(_ context.Context, slot string, now int64) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.challenges[slot]
	delete(s.challenges, slot)
	if !ok || c.expires <= now {
		return nil, store.ErrNotFound
	}
	return c.data, nil
}

func (s *Store) CreateUser(_ context.Context, credential store.Credential, recoveryMAC []byte, _ int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.users[credential.UserID]; exists || s.credentialExists(credential.ID) {
		return store.ErrExists
	}
	s.users[credential.UserID] = &user{recovery: bytes.Clone(recoveryMAC)}
	s.add(credential)
	return nil
}

func (s *Store) User(_ context.Context, id string, now int64) (store.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.users[id]
	if !ok {
		return store.User{}, store.ErrNotFound
	}
	out := store.User{ID: id, CredCount: s.count(id)}
	if u.enrollHash != nil && u.enrollUntil > now {
		out.EnrollHash = bytes.Clone(u.enrollHash)
		out.EnrollUntil = u.enrollUntil
	}
	return out, nil
}

func (s *Store) DeleteUser(_ context.Context, userID string, _ int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.users[userID]; !ok {
		return store.ErrNotFound
	}
	delete(s.users, userID)
	s.forget(userID)
	return nil
}

func (s *Store) AddCredential(_ context.Context, credential store.Credential, limit int, _ int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	count := s.count(credential.UserID)
	switch {
	case s.users[credential.UserID] == nil || count == 0:
		return store.ErrNotFound
	case count >= limit:
		return store.ErrLimit
	case s.credentialExists(credential.ID):
		return store.ErrExists
	}
	s.add(credential)
	return nil
}

func (s *Store) Credentials(_ context.Context, userID string) ([]store.Credential, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []store.Credential
	for _, c := range s.credentials {
		if c.UserID == userID {
			out = append(out, clone(c))
		}
	}
	return out, nil
}

func (s *Store) UpdateCredential(_ context.Context, id, data []byte, _ int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.credentials {
		if bytes.Equal(s.credentials[i].ID, id) {
			s.credentials[i].Data = bytes.Clone(data)
			return nil
		}
	}
	return store.ErrNotFound
}

func (s *Store) RemoveCredential(_ context.Context, userID string, credentialID []byte, _ int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	index := slices.IndexFunc(s.credentials, func(c store.Credential) bool {
		return c.UserID == userID && bytes.Equal(c.ID, credentialID)
	})
	switch {
	case index < 0:
		return store.ErrNotFound
	case s.count(userID) == 1:
		return store.ErrLastCredential
	}
	s.credentials = slices.Delete(s.credentials, index, index+1)
	return nil
}

func (s *Store) ReplaceRecovery(_ context.Context, userID string, oldMAC, newMAC []byte, _ int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.users[userID]
	if !ok || s.count(userID) == 0 || !bytes.Equal(u.recovery, oldMAC) {
		return store.ErrNotFound
	}
	u.recovery = bytes.Clone(newMAC)
	return nil
}

func (s *Store) RotateRecovery(_ context.Context, userID string, newMAC []byte, _ int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.users[userID]
	if !ok || s.count(userID) == 0 {
		return store.ErrNotFound
	}
	u.recovery = bytes.Clone(newMAC)
	return nil
}

func (s *Store) Reset(_ context.Context, userID string, tokenHash []byte, until, _ int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.users[userID]
	if !ok {
		return store.ErrNotFound
	}
	s.forget(userID)
	*u = user{enrollHash: bytes.Clone(tokenHash), enrollUntil: until}
	return nil
}

func (s *Store) Enroll(_ context.Context, credential store.Credential, tokenHash, recoveryMAC []byte, now int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.users[credential.UserID]
	switch {
	case !ok || s.count(credential.UserID) != 0 || u.enrollHash == nil || u.enrollUntil <= now ||
		!bytes.Equal(u.enrollHash, tokenHash):
		return store.ErrNotFound
	case s.credentialExists(credential.ID):
		return store.ErrExists
	}
	*u = user{recovery: bytes.Clone(recoveryMAC)}
	s.add(credential)
	return nil
}

func (s *Store) CreateSession(_ context.Context, hash []byte, userID string, expires, now int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.users[userID]; !ok {
		return store.ErrNotFound
	}
	if existing, ok := s.sessions[string(hash)]; ok && existing.expires > now {
		return store.ErrExists
	}
	s.sessions[string(hash)] = session{userID: userID, expires: expires}
	return nil
}

func (s *Store) Session(_ context.Context, hash []byte, now int64) (string, int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	found, ok := s.sessions[string(hash)]
	if !ok || found.expires <= now {
		return "", 0, store.ErrNotFound
	}
	return found.userID, found.expires, nil
}

func (s *Store) ExtendSession(_ context.Context, hash []byte, expires, now int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	found, ok := s.sessions[string(hash)]
	if !ok || found.expires <= now || found.expires >= expires {
		return store.ErrNotFound
	}
	found.expires = expires
	s.sessions[string(hash)] = found
	return nil
}

func (s *Store) DeleteSession(_ context.Context, hash []byte, _ int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, string(hash))
	return nil
}

func (s *Store) DeleteSessions(_ context.Context, userID string, _ int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.forgetSessions(userID)
	return nil
}

func (s *Store) count(userID string) int {
	n := 0
	for _, c := range s.credentials {
		if c.UserID == userID {
			n++
		}
	}
	return n
}

func (s *Store) credentialExists(id []byte) bool {
	return slices.ContainsFunc(s.credentials, func(c store.Credential) bool { return bytes.Equal(c.ID, id) })
}

func (s *Store) add(credential store.Credential) {
	s.credentials = append(s.credentials, clone(credential))
}

func (s *Store) forget(userID string) {
	s.credentials = slices.DeleteFunc(s.credentials, func(c store.Credential) bool { return c.UserID == userID })
	s.forgetSessions(userID)
}

func (s *Store) forgetSessions(userID string) {
	for hash, found := range s.sessions {
		if found.userID == userID {
			delete(s.sessions, hash)
		}
	}
}

func clone(c store.Credential) store.Credential {
	return store.Credential{ID: bytes.Clone(c.ID), UserID: c.UserID, Data: bytes.Clone(c.Data)}
}
