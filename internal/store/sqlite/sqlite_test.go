// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package sqlite_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/recregt/parvus/internal/store"
	"github.com/recregt/parvus/internal/store/contract"
	"github.com/recregt/parvus/internal/store/model"
	"github.com/recregt/parvus/internal/store/sqlite"
)

func open(t *testing.T) store.Store {
	t.Helper()
	db, err := sqlite.Open(context.Background(), filepath.Join(t.TempDir(), "parvus.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestContract(t *testing.T) {
	contract.Run(t, open)
}

func FuzzAgreesWithTheModel(f *testing.F) {
	for _, seed := range contract.Seeds() {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, script []byte) {
		contract.Agree(t, script, model.New(), open(t))
	})
}

func TestReopenKeepsData(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "parvus.db")
	first, err := sqlite.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	credential := store.Credential{ID: []byte("c1"), UserID: "alice", Data: []byte("data")}
	if err := first.CreateUser(ctx, credential, []byte("mac"), 0); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	second, err := sqlite.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = second.Close() }()
	user, err := second.User(ctx, "alice", 0)
	if err != nil || user.CredCount != 1 {
		t.Fatalf("user = %+v, %v", user, err)
	}
}

func TestDatabasePathIsUsedAsGiven(t *testing.T) {
	t.Chdir(t.TempDir())
	for _, path := range []string{"relative.db", "with space?and=query.db"} {
		db, err := sqlite.Open(context.Background(), path)
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
	if _, err := sqlite.Open(ctx, path); err == nil {
		t.Fatal("unknown schema version accepted")
	}
}
