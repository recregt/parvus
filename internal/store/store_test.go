// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package store_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/recregt/parvus/internal/store"
)

const (
	now      = 1_800_000_000
	userID   = "alice"
	someData = "data"
)

func credential(id string) store.Credential {
	return store.Credential{ID: []byte(id), UserID: userID, Data: []byte(someData)}
}

func TestReopenKeepsData(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "parvus.db")
	first, err := store.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if err := first.CreateUser(ctx, credential("c1"), []byte("mac"), now); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	second, err := store.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = second.Close() }()
	user, err := second.User(ctx, userID)
	if err != nil || user.CredCount != 1 {
		t.Fatalf("user = %+v, %v", user, err)
	}
}

func TestDatabasePathIsUsedAsGiven(t *testing.T) {
	t.Chdir(t.TempDir())
	for _, path := range []string{"relative.db", "with space?and=query.db"} {
		db, err := store.Open(context.Background(), path)
		if err != nil {
			t.Fatalf("%q: %v", path, err)
		}
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("%q was not created: %v", path, err)
		}
	}
}

func TestOpenRefusesUnknownSchemaVersion(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "parvus.db")
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.ExecContext(ctx, "PRAGMA user_version = 99"); err != nil {
		t.Fatal(err)
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Open(ctx, path); err == nil {
		t.Fatal("unknown schema version accepted")
	}
}

func TestDuplicateCredentialIDIsRejected(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "parvus.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if err := db.CreateUser(ctx, credential("c1"), []byte("mac"), now); err != nil {
		t.Fatal(err)
	}
	if err := db.CreateUser(ctx, store.Credential{ID: []byte("c2"), UserID: userID}, []byte("mac"), now); !errors.Is(err, store.ErrExists) {
		t.Fatalf("duplicate user err = %v", err)
	}
	if err := db.AddCredential(ctx, credential("c1"), 5, now); !errors.Is(err, store.ErrExists) {
		t.Fatalf("duplicate credential err = %v", err)
	}
	user, err := db.User(ctx, userID)
	if err != nil || user.CredCount != 1 {
		t.Fatalf("failed add changed the count: %+v, %v", user, err)
	}
}
