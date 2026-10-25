// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package contract

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/recregt/parvus/internal/store"
)

type Opener func(t *testing.T) store.Store

type rule struct {
	name  string
	check func(t *testing.T, s store.Store)
}

func Run(t *testing.T, open Opener) {
	t.Helper()
	for _, r := range rules {
		t.Run(r.name, func(t *testing.T) { r.check(t, open(t)) })
	}
}

var rules = []rule{
	{"a challenge is returned once and only before it expires", challengeIsSingleUseAndExpires},
	{"a newer challenge replaces the pending one in its slot", challengeIsReplaced},
	{"a user starts with exactly one credential", userStartsWithOneCredential},
	{"a credential id belongs to exactly one user", credentialIDIsUnique},
	{"credentials are added up to the limit and listed in order", credentialsUpToTheLimit},
	{"credentials are added only to a user who already has one", credentialsNeedAnActiveUser},
	{"the last credential cannot be removed", lastCredentialStays},
	{"updating a credential replaces its data", credentialDataIsUpdated},
	{"a recovery code is replaced only by presenting the current one", recoveryNeedsTheCurrentCode},
	{"reset removes credentials, sessions and the recovery code", resetClearsTheAccount},
	{"enrollment needs the current token before it expires", enrollmentNeedsTheToken},
	{"sessions expire and are only ever extended forward", sessionsExpireAndExtendForward},
	{"sessions end one at a time or all together", sessionsEnd},
	{"deleting a user removes everything that belongs to them", deletionCascades},
	{"a challenge is taken by exactly one of two concurrent callers", challengeRace},
	{"two concurrent removals never remove the last credential", removalRace},
	{"a recovery code is used by exactly one of two concurrent callers", recoveryRace},
	{"two concurrent additions never pass the limit", additionRace},
}

const (
	alice = "alice"
	bob   = "bob"
	limit = 3
)

var ctx = context.Background()

func credential(user, id string) store.Credential {
	return store.Credential{ID: []byte(id), UserID: user, Data: []byte("data of " + id)}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func is(t *testing.T, err, want error) {
	t.Helper()
	if !errors.Is(err, want) {
		t.Fatalf("got %v, want %v", err, want)
	}
}

func ids(t *testing.T, s store.Store, user string) []string {
	t.Helper()
	credentials, err := s.Credentials(ctx, user)
	must(t, err)
	out := []string{}
	for _, c := range credentials {
		out = append(out, string(c.ID))
	}
	return out
}

func expectIDs(t *testing.T, s store.Store, user string, want ...string) {
	t.Helper()
	got := ids(t, s, user)
	if len(got) != len(want) {
		t.Fatalf("%s has credentials %q, want %q", user, got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("%s has credentials %q, want %q", user, got, want)
		}
	}
	u, err := s.User(ctx, user, 0)
	if err == nil && u.CredCount != len(want) {
		t.Fatalf("%s counts %d credentials but has %q", user, u.CredCount, got)
	}
}

func challengeIsSingleUseAndExpires(t *testing.T, s store.Store) {
	must(t, s.PutChallenge(ctx, "a", []byte("one"), 10, 0))
	data, err := s.TakeChallenge(ctx, "a", 5)
	must(t, err)
	if string(data) != "one" {
		t.Fatalf("took %q", data)
	}
	_, err = s.TakeChallenge(ctx, "a", 5)
	is(t, err, store.ErrNotFound)

	must(t, s.PutChallenge(ctx, "b", []byte("two"), 10, 5))
	_, err = s.TakeChallenge(ctx, "b", 10)
	is(t, err, store.ErrNotFound)
}

func challengeIsReplaced(t *testing.T, s store.Store) {
	must(t, s.PutChallenge(ctx, "a", []byte("old"), 10, 0))
	must(t, s.PutChallenge(ctx, "a", []byte("new"), 10, 0))
	data, err := s.TakeChallenge(ctx, "a", 0)
	must(t, err)
	if string(data) != "new" {
		t.Fatalf("took %q", data)
	}
}

func userStartsWithOneCredential(t *testing.T, s store.Store) {
	must(t, s.CreateUser(ctx, credential(alice, "c1"), []byte("m1"), 0))
	expectIDs(t, s, alice, "c1")

	is(t, s.CreateUser(ctx, credential(alice, "c2"), []byte("m1"), 0), store.ErrExists)
	expectIDs(t, s, alice, "c1")
}

func credentialIDIsUnique(t *testing.T, s store.Store) {
	must(t, s.CreateUser(ctx, credential(alice, "c1"), []byte("m1"), 0))
	is(t, s.CreateUser(ctx, credential(bob, "c1"), []byte("m1"), 0), store.ErrExists)
	_, err := s.User(ctx, bob, 0)
	is(t, err, store.ErrNotFound)

	must(t, s.CreateUser(ctx, credential(bob, "c2"), []byte("m1"), 0))
	is(t, s.AddCredential(ctx, credential(alice, "c2"), limit, 0), store.ErrExists)
	expectIDs(t, s, alice, "c1")
	expectIDs(t, s, bob, "c2")
}

func credentialsUpToTheLimit(t *testing.T, s store.Store) {
	must(t, s.CreateUser(ctx, credential(alice, "c1"), []byte("m1"), 0))
	must(t, s.AddCredential(ctx, credential(alice, "c2"), limit, 0))
	must(t, s.AddCredential(ctx, credential(alice, "c3"), limit, 0))
	is(t, s.AddCredential(ctx, credential(alice, "c4"), limit, 0), store.ErrLimit)
	expectIDs(t, s, alice, "c1", "c2", "c3")
}

func credentialsNeedAnActiveUser(t *testing.T, s store.Store) {
	is(t, s.AddCredential(ctx, credential(alice, "c1"), limit, 0), store.ErrNotFound)

	must(t, s.CreateUser(ctx, credential(alice, "c1"), []byte("m1"), 0))
	must(t, s.Reset(ctx, alice, []byte("t1"), 100, 0))
	is(t, s.AddCredential(ctx, credential(alice, "c2"), limit, 0), store.ErrNotFound)
	expectIDs(t, s, alice)
}

func lastCredentialStays(t *testing.T, s store.Store) {
	must(t, s.CreateUser(ctx, credential(alice, "c1"), []byte("m1"), 0))
	must(t, s.AddCredential(ctx, credential(alice, "c2"), limit, 0))
	must(t, s.CreateUser(ctx, credential(bob, "c3"), []byte("m1"), 0))

	must(t, s.RemoveCredential(ctx, alice, []byte("c1"), 0))
	is(t, s.RemoveCredential(ctx, alice, []byte("c2"), 0), store.ErrLastCredential)
	is(t, s.RemoveCredential(ctx, alice, []byte("c1"), 0), store.ErrNotFound)
	is(t, s.RemoveCredential(ctx, alice, []byte("c3"), 0), store.ErrNotFound)
	expectIDs(t, s, alice, "c2")
	expectIDs(t, s, bob, "c3")
}

func credentialDataIsUpdated(t *testing.T, s store.Store) {
	must(t, s.CreateUser(ctx, credential(alice, "c1"), []byte("m1"), 0))
	must(t, s.UpdateCredential(ctx, []byte("c1"), []byte("counter moved"), 0))
	is(t, s.UpdateCredential(ctx, []byte("c9"), []byte("x"), 0), store.ErrNotFound)

	credentials, err := s.Credentials(ctx, alice)
	must(t, err)
	if string(credentials[0].Data) != "counter moved" {
		t.Fatalf("data is %q", credentials[0].Data)
	}
}

func recoveryNeedsTheCurrentCode(t *testing.T, s store.Store) {
	must(t, s.CreateUser(ctx, credential(alice, "c1"), []byte("m1"), 0))
	is(t, s.ReplaceRecovery(ctx, alice, []byte("wrong"), []byte("m2"), 0), store.ErrNotFound)
	is(t, s.ReplaceRecovery(ctx, bob, []byte("m1"), []byte("m2"), 0), store.ErrNotFound)

	must(t, s.ReplaceRecovery(ctx, alice, []byte("m1"), []byte("m2"), 0))
	is(t, s.ReplaceRecovery(ctx, alice, []byte("m1"), []byte("m3"), 0), store.ErrNotFound)

	must(t, s.RotateRecovery(ctx, alice, []byte("m4"), 0))
	is(t, s.ReplaceRecovery(ctx, alice, []byte("m2"), []byte("m5"), 0), store.ErrNotFound)
	must(t, s.ReplaceRecovery(ctx, alice, []byte("m4"), []byte("m5"), 0))
	is(t, s.RotateRecovery(ctx, bob, []byte("m6"), 0), store.ErrNotFound)
}

func resetClearsTheAccount(t *testing.T, s store.Store) {
	must(t, s.CreateUser(ctx, credential(alice, "c1"), []byte("m1"), 0))
	must(t, s.CreateSession(ctx, []byte("s1"), alice, 100, 0))
	must(t, s.Reset(ctx, alice, []byte("t1"), 100, 0))

	expectIDs(t, s, alice)
	_, _, err := s.Session(ctx, []byte("s1"), 0)
	is(t, err, store.ErrNotFound)
	is(t, s.ReplaceRecovery(ctx, alice, []byte("m1"), []byte("m2"), 0), store.ErrNotFound)
	is(t, s.RotateRecovery(ctx, alice, []byte("m2"), 0), store.ErrNotFound)

	u, err := s.User(ctx, alice, 0)
	must(t, err)
	if !bytes.Equal(u.EnrollHash, []byte("t1")) || u.EnrollUntil != 100 {
		t.Fatalf("user after reset %+v", u)
	}
	is(t, s.Reset(ctx, bob, []byte("t1"), 100, 0), store.ErrNotFound)
}

func enrollmentNeedsTheToken(t *testing.T, s store.Store) {
	must(t, s.CreateUser(ctx, credential(alice, "c1"), []byte("m1"), 0))
	must(t, s.CreateUser(ctx, credential(bob, "c9"), []byte("m1"), 0))
	must(t, s.Reset(ctx, alice, []byte("t1"), 100, 0))

	is(t, s.Enroll(ctx, credential(alice, "c2"), []byte("wrong"), []byte("m2"), 10), store.ErrNotFound)
	is(t, s.Enroll(ctx, credential(alice, "c9"), []byte("t1"), []byte("m2"), 10), store.ErrExists)
	u, err := s.User(ctx, alice, 10)
	must(t, err)
	if u.EnrollHash == nil {
		t.Fatal("a failed enrollment used up the token")
	}

	must(t, s.Enroll(ctx, credential(alice, "c2"), []byte("t1"), []byte("m2"), 20))
	expectIDs(t, s, alice, "c2")
	is(t, s.Enroll(ctx, credential(alice, "c3"), []byte("t1"), []byte("m3"), 20), store.ErrNotFound)
	must(t, s.ReplaceRecovery(ctx, alice, []byte("m2"), []byte("m3"), 20))

	must(t, s.Reset(ctx, bob, []byte("t2"), 100, 20))
	if u, _ := s.User(ctx, bob, 100); u.EnrollHash != nil {
		t.Fatal("an expired token is still visible")
	}
	is(t, s.Enroll(ctx, credential(bob, "c4"), []byte("t2"), []byte("m2"), 100), store.ErrNotFound)
}

func sessionsExpireAndExtendForward(t *testing.T, s store.Store) {
	must(t, s.CreateUser(ctx, credential(alice, "c1"), []byte("m1"), 0))
	must(t, s.CreateSession(ctx, []byte("s1"), alice, 100, 0))
	is(t, s.CreateSession(ctx, []byte("s1"), alice, 100, 0), store.ErrExists)
	is(t, s.CreateSession(ctx, []byte("s2"), bob, 100, 0), store.ErrNotFound)

	user, expires, err := s.Session(ctx, []byte("s1"), 50)
	must(t, err)
	if user != alice || expires != 100 {
		t.Fatalf("session is %s until %d", user, expires)
	}

	must(t, s.ExtendSession(ctx, []byte("s1"), 200, 50))
	is(t, s.ExtendSession(ctx, []byte("s1"), 150, 50), store.ErrNotFound)
	_, _, err = s.Session(ctx, []byte("s1"), 199)
	must(t, err)

	_, _, err = s.Session(ctx, []byte("s1"), 200)
	is(t, err, store.ErrNotFound)
	is(t, s.ExtendSession(ctx, []byte("s1"), 300, 200), store.ErrNotFound)
}

func sessionsEnd(t *testing.T, s store.Store) {
	must(t, s.CreateUser(ctx, credential(alice, "c1"), []byte("m1"), 0))
	must(t, s.CreateUser(ctx, credential(bob, "c2"), []byte("m1"), 0))
	for _, hash := range []string{"s1", "s2"} {
		must(t, s.CreateSession(ctx, []byte(hash), alice, 100, 0))
	}
	must(t, s.CreateSession(ctx, []byte("s3"), bob, 100, 0))

	must(t, s.DeleteSession(ctx, []byte("s1"), 0))
	must(t, s.DeleteSession(ctx, []byte("s1"), 0))
	expectSessions(t, s, map[string]bool{"s1": false, "s2": true, "s3": true})

	must(t, s.DeleteSessions(ctx, alice, 0))
	expectSessions(t, s, map[string]bool{"s1": false, "s2": false, "s3": true})
}

func expectSessions(t *testing.T, s store.Store, alive map[string]bool) {
	t.Helper()
	for hash, want := range alive {
		_, _, err := s.Session(ctx, []byte(hash), 0)
		if got := err == nil; got != want {
			t.Fatalf("session %s alive %v, want %v", hash, got, want)
		}
	}
}

func deletionCascades(t *testing.T, s store.Store) {
	must(t, s.CreateUser(ctx, credential(alice, "c1"), []byte("m1"), 0))
	must(t, s.CreateSession(ctx, []byte("s1"), alice, 100, 0))
	must(t, s.DeleteUser(ctx, alice, 0))

	_, err := s.User(ctx, alice, 0)
	is(t, err, store.ErrNotFound)
	expectIDs(t, s, alice)
	expectSessions(t, s, map[string]bool{"s1": false})
	is(t, s.DeleteUser(ctx, alice, 0), store.ErrNotFound)
	must(t, s.CreateUser(ctx, credential(alice, "c1"), []byte("m1"), 0))
}

func concurrently(calls ...func() error) []error {
	errs := make([]error, len(calls))
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i, call := range calls {
		wg.Go(func() {
			<-start
			errs[i] = call()
		})
	}
	close(start)
	wg.Wait()
	return errs
}

func exactlyOneSucceeded(t *testing.T, errs []error, loser error) {
	t.Helper()
	succeeded := 0
	for _, err := range errs {
		switch {
		case err == nil:
			succeeded++
		case !errors.Is(err, loser):
			t.Fatalf("unexpected error %v", err)
		}
	}
	if succeeded != 1 {
		t.Fatalf("%d of %d concurrent calls succeeded", succeeded, len(errs))
	}
}

const simultaneousRaces = 16

func raceEveryUser(t *testing.T, setup func(user string), attempts func(user string) [2]func() error, loser error) {
	t.Helper()
	var calls []func() error
	for n := range simultaneousRaces {
		user := fmt.Sprintf("user%d", n)
		setup(user)
		pair := attempts(user)
		calls = append(calls, pair[:]...)
	}
	errs := concurrently(calls...)
	for n := range simultaneousRaces {
		exactlyOneSucceeded(t, errs[2*n:2*n+2], loser)
	}
}

func challengeRace(t *testing.T, s store.Store) {
	take := func(user string) func() error {
		return func() error {
			_, err := s.TakeChallenge(ctx, user, 0)
			return err
		}
	}
	raceEveryUser(t,
		func(user string) { must(t, s.PutChallenge(ctx, user, []byte("one"), 10, 0)) },
		func(user string) [2]func() error { return [2]func() error{take(user), take(user)} },
		store.ErrNotFound)
}

func removalRace(t *testing.T, s store.Store) {
	remove := func(user, id string) func() error {
		return func() error { return s.RemoveCredential(ctx, user, []byte(id), 0) }
	}
	raceEveryUser(t,
		func(user string) {
			must(t, s.CreateUser(ctx, credential(user, user+"-c1"), []byte("m1"), 0))
			must(t, s.AddCredential(ctx, credential(user, user+"-c2"), limit, 0))
		},
		func(user string) [2]func() error {
			return [2]func() error{remove(user, user+"-c1"), remove(user, user+"-c2")}
		},
		store.ErrLastCredential)
}

func recoveryRace(t *testing.T, s store.Store) {
	replace := func(user, next string) func() error {
		return func() error { return s.ReplaceRecovery(ctx, user, []byte("m1"), []byte(next), 0) }
	}
	raceEveryUser(t,
		func(user string) { must(t, s.CreateUser(ctx, credential(user, user+"-c1"), []byte("m1"), 0)) },
		func(user string) [2]func() error { return [2]func() error{replace(user, "m2"), replace(user, "m3")} },
		store.ErrNotFound)
}

func additionRace(t *testing.T, s store.Store) {
	add := func(user, id string) func() error {
		return func() error { return s.AddCredential(ctx, credential(user, id), 2, 0) }
	}
	raceEveryUser(t,
		func(user string) { must(t, s.CreateUser(ctx, credential(user, user+"-c1"), []byte("m1"), 0)) },
		func(user string) [2]func() error {
			return [2]func() error{add(user, user+"-c2"), add(user, user+"-c3")}
		},
		store.ErrLimit)
}
