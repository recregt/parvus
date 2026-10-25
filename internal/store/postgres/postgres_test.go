// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

//go:build postgres

package postgres_test

import (
	"context"
	"crypto/rand"
	"database/sql"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/recregt/parvus/internal/store"
	"github.com/recregt/parvus/internal/store/contract"
	"github.com/recregt/parvus/internal/store/model"
	"github.com/recregt/parvus/internal/store/postgres"
)

func freshDatabase(t *testing.T) string {
	t.Helper()
	server := os.Getenv("PARVUS_TEST_POSTGRES")
	if server == "" {
		t.Fatal("PARVUS_TEST_POSTGRES must name a Postgres server, for example host=/run/postgresql user=parvus")
	}
	admin, err := sql.Open("pgx", server+" dbname=postgres")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close() })
	name := "parvus_" + strings.ToLower(rand.Text())
	if _, err := admin.ExecContext(context.Background(), "CREATE DATABASE "+name); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = admin.ExecContext(context.Background(), "DROP DATABASE "+name+" WITH (FORCE)")
	})
	return "postgres:///" + name + "?" + strings.ReplaceAll(server, " ", "&")
}

func open(t *testing.T, url string) *postgres.Store {
	t.Helper()
	db, err := postgres.Open(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestContract(t *testing.T) {
	contract.Run(t, func(t *testing.T) store.Store { return open(t, freshDatabase(t)) })
}

func FuzzAgreesWithTheModel(f *testing.F) {
	for _, seed := range contract.Seeds() {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, script []byte) {
		contract.Agree(t, script, model.New(), open(t, freshDatabase(t)))
	})
}

func TestReopenKeepsDataAndRefusesAnUnknownSchema(t *testing.T) {
	ctx := context.Background()
	url := freshDatabase(t)
	credential := store.Credential{ID: []byte("c1"), UserID: "alice", Data: []byte("data")}
	if err := open(t, url).CreateUser(ctx, credential, []byte("mac"), 0); err != nil {
		t.Fatal(err)
	}
	user, err := open(t, url).User(ctx, "alice", 0)
	if err != nil || user.CredCount != 1 {
		t.Fatalf("user after reopening = %+v, %v", user, err)
	}

	raw, err := sql.Open("pgx", url)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = raw.Close() }()
	if _, err := raw.ExecContext(ctx, "UPDATE parvus_schema SET version = 99"); err != nil {
		t.Fatal(err)
	}
	if _, err := postgres.Open(ctx, url); err == nil {
		t.Fatal("unknown schema version accepted")
	}
}

func TestEnginesStartingTogetherCreateTheSchemaOnce(t *testing.T) {
	url := freshDatabase(t)
	start := make(chan struct{})
	errs := make([]error, 2)
	var wg sync.WaitGroup
	for i := range errs {
		wg.Go(func() {
			<-start
			db, err := postgres.Open(context.Background(), url)
			if err == nil {
				err = db.Close()
			}
			errs[i] = err
		})
	}
	close(start)
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
}
