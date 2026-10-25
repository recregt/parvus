// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package main

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
)

func TestRestartAfterACrashReplacesAStaleSocketButNeverALiveOne(t *testing.T) {
	path := filepath.Join(t.TempDir(), "parvus.sock")
	crashed, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	crashed.(*net.UnixListener).SetUnlinkOnClose(false)
	if err := crashed.Close(); err != nil {
		t.Fatal(err)
	}

	restarted, err := listenUnix(path)
	if err != nil {
		t.Fatalf("stale socket blocked the restart: %v", err)
	}
	defer func() { _ = restarted.Close() }()

	if _, err := listenUnix(path); err == nil {
		t.Fatal("a socket served by a live process was taken over")
	}
}

func TestTheDatabaseIsPrivateToParvus(t *testing.T) {
	restrictNewFiles()
	dir := t.TempDir()
	db, err := openStore(context.Background(), filepath.Join(dir, "parvus.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if err := db.PutChallenge(context.Background(), "slot", []byte("x"), 1, 0); err != nil {
		t.Fatal(err)
	}
	files, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		info, err := file.Info()
		if err != nil {
			t.Fatal(err)
		}
		if mode := info.Mode().Perm(); mode&0o077 != 0 {
			t.Fatalf("%s is mode %o, readable beyond its owner", file.Name(), mode)
		}
	}
}

func TestSocketAdmitsItsGroupAndNobodyElse(t *testing.T) {
	path := filepath.Join(t.TempDir(), "parvus.sock")
	listener, err := listenUnix(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listener.Close() }()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0o660 {
		t.Fatalf("socket mode is %o, want 660", mode)
	}
}
