// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package contract

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/recregt/parvus/internal/store"
)

var (
	users         = []string{"a", "b"}
	credentialIDs = []string{"c1", "c2", "c3"}
	secrets       = []string{"m1", "m2", "t1", "t2"}
	hashes        = []string{"s1", "s2"}
	slots         = []string{"x", "y"}
	offsets       = []int64{-1, 0, 1, 5}
	limits        = []int{1, 2, 3}
)

type step struct {
	name string
	run  func(s store.Store) string
}

type operation struct {
	name  string
	build func(r *reader, now int64) step
}

var operations = []operation{
	{"Tick", nil},
	{"PutChallenge", putChallenge},
	{"TakeChallenge", takeChallenge},
	{"CreateUser", createUser},
	{"User", readUser},
	{"DeleteUser", deleteUser},
	{"AddCredential", addCredential},
	{"Credentials", readCredentials},
	{"UpdateCredential", updateCredential},
	{"RemoveCredential", removeCredential},
	{"ReplaceRecovery", replaceRecovery},
	{"RotateRecovery", rotateRecovery},
	{"Reset", reset},
	{"Enroll", enroll},
	{"CreateSession", createSession},
	{"Session", readSession},
	{"ExtendSession", extendSession},
	{"DeleteSession", deleteSession},
	{"DeleteSessions", deleteSessions},
}

func Agree(t *testing.T, script []byte, reference, candidate store.Store) {
	t.Helper()
	steps := parse(script)
	for i, s := range steps {
		want, got := s.run(reference), s.run(candidate)
		if want != got || strings.HasPrefix(want, "unexpected") {
			t.Fatalf("step %d %s: reference %q, candidate %q\nscript:\n%s", i, s.name, want, got, describe(steps[:i+1]))
		}
	}
}

func Seed(lines ...string) []byte {
	var out []byte
	for _, line := range lines {
		fields := strings.Fields(line)
		index := slices.IndexFunc(operations, func(o operation) bool { return o.name == fields[0] })
		if index < 0 {
			panic("unknown operation " + fields[0])
		}
		out = append(out, byte(index))
		for _, field := range fields[1:] {
			n, err := strconv.Atoi(field)
			if err != nil {
				panic(err)
			}
			out = append(out, byte(n))
		}
	}
	return out
}

func Seeds() [][]byte {
	return [][]byte{
		Seed("CreateUser 0 0 0", "AddCredential 0 1 2", "RemoveCredential 0 0", "RemoveCredential 0 1", "Credentials 0"),
		Seed("CreateUser 0 0 0", "ReplaceRecovery 0 0 1", "ReplaceRecovery 0 0 1", "RotateRecovery 0 2", "User 0"),
		Seed("CreateUser 0 0 0", "CreateSession 0 0 3", "Reset 0 2 2", "Enroll 0 1 3 1", "Enroll 0 1 2 1", "Tick 1", "Enroll 0 2 2 1"),
		Seed("CreateUser 0 0 0", "CreateSession 0 0 2", "ExtendSession 0 3", "Tick 0", "Session 0", "DeleteSessions 0"),
		Seed("PutChallenge 0 0 2", "Tick 0", "TakeChallenge 0", "PutChallenge 1 0 3", "PutChallenge 1 1 3", "TakeChallenge 1"),
		Seed("CreateUser 0 0 0", "CreateUser 1 0 0", "DeleteUser 0", "CreateUser 1 0 0", "UpdateCredential 0 3"),
	}
}

type reader struct {
	data []byte
	at   int
}

func (r *reader) done() bool {
	return r.at >= len(r.data)
}

func (r *reader) next(n int) int {
	if r.done() {
		return 0
	}
	value := int(r.data[r.at]) % n
	r.at++
	return value
}

func pick[T any](r *reader, from []T) T {
	return from[r.next(len(from))]
}

func parse(script []byte) []step {
	r := &reader{data: script}
	now := int64(10)
	var steps []step
	for !r.done() {
		o := pick(r, operations)
		if o.build == nil {
			now += int64(r.next(len(offsets))) + 1
			continue
		}
		steps = append(steps, o.build(r, now))
	}
	return append(steps, snapshot(now)...)
}

func snapshot(now int64) []step {
	var steps []step
	for _, user := range users {
		steps = append(steps, userStep(user, now), credentialsStep(user))
	}
	for _, hash := range hashes {
		steps = append(steps, sessionStep(hash, now))
	}
	for _, slot := range slots {
		steps = append(steps, takeStep(slot, now))
	}
	return steps
}

func describe(steps []step) string {
	var out strings.Builder
	for _, s := range steps {
		fmt.Fprintln(&out, " ", s.name)
	}
	return out.String()
}

func outcome(err error, value string) string {
	for _, known := range []error{store.ErrNotFound, store.ErrExists, store.ErrLimit, store.ErrLastCredential} {
		if errors.Is(err, known) {
			return known.Error()
		}
	}
	if err != nil {
		return "unexpected: " + err.Error()
	}
	return "ok " + value
}

func credentialOf(user, id string) store.Credential {
	return store.Credential{ID: []byte(id), UserID: user, Data: []byte(id)}
}

func putChallenge(r *reader, now int64) step {
	slot, data, expires := pick(r, slots), pick(r, secrets), now+pick(r, offsets)
	return step{fmt.Sprintf("PutChallenge %s %s until %d at %d", slot, data, expires, now), func(s store.Store) string {
		return outcome(s.PutChallenge(ctx, slot, []byte(data), expires, now), "")
	}}
}

func takeChallenge(r *reader, now int64) step {
	return takeStep(pick(r, slots), now)
}

func takeStep(slot string, now int64) step {
	return step{fmt.Sprintf("TakeChallenge %s at %d", slot, now), func(s store.Store) string {
		data, err := s.TakeChallenge(ctx, slot, now)
		return outcome(err, string(data))
	}}
}

func createUser(r *reader, now int64) step {
	user, id, mac := pick(r, users), pick(r, credentialIDs), pick(r, secrets)
	return step{fmt.Sprintf("CreateUser %s %s %s", user, id, mac), func(s store.Store) string {
		return outcome(s.CreateUser(ctx, credentialOf(user, id), []byte(mac), now), "")
	}}
}

func readUser(r *reader, now int64) step {
	return userStep(pick(r, users), now)
}

func userStep(user string, now int64) step {
	return step{fmt.Sprintf("User %s at %d", user, now), func(s store.Store) string {
		u, err := s.User(ctx, user, now)
		return outcome(err, fmt.Sprintf("%d credentials, token %q until %d", u.CredCount, u.EnrollHash, u.EnrollUntil))
	}}
}

func deleteUser(r *reader, now int64) step {
	user := pick(r, users)
	return step{"DeleteUser " + user, func(s store.Store) string {
		return outcome(s.DeleteUser(ctx, user, now), "")
	}}
}

func addCredential(r *reader, now int64) step {
	user, id, limit := pick(r, users), pick(r, credentialIDs), pick(r, limits)
	return step{fmt.Sprintf("AddCredential %s %s limit %d", user, id, limit), func(s store.Store) string {
		return outcome(s.AddCredential(ctx, credentialOf(user, id), limit, now), "")
	}}
}

func readCredentials(r *reader, _ int64) step {
	return credentialsStep(pick(r, users))
}

func credentialsStep(user string) step {
	return step{"Credentials " + user, func(s store.Store) string {
		credentials, err := s.Credentials(ctx, user)
		var listed []string
		for _, c := range credentials {
			listed = append(listed, fmt.Sprintf("%s=%s", c.ID, c.Data))
		}
		return outcome(err, strings.Join(listed, ","))
	}}
}

func updateCredential(r *reader, now int64) step {
	id, data := pick(r, credentialIDs), pick(r, secrets)
	return step{fmt.Sprintf("UpdateCredential %s %s", id, data), func(s store.Store) string {
		return outcome(s.UpdateCredential(ctx, []byte(id), []byte(data), now), "")
	}}
}

func removeCredential(r *reader, now int64) step {
	user, id := pick(r, users), pick(r, credentialIDs)
	return step{fmt.Sprintf("RemoveCredential %s %s", user, id), func(s store.Store) string {
		return outcome(s.RemoveCredential(ctx, user, []byte(id), now), "")
	}}
}

func replaceRecovery(r *reader, now int64) step {
	user, old, next := pick(r, users), pick(r, secrets), pick(r, secrets)
	return step{fmt.Sprintf("ReplaceRecovery %s %s with %s", user, old, next), func(s store.Store) string {
		return outcome(s.ReplaceRecovery(ctx, user, []byte(old), []byte(next), now), "")
	}}
}

func rotateRecovery(r *reader, now int64) step {
	user, next := pick(r, users), pick(r, secrets)
	return step{fmt.Sprintf("RotateRecovery %s to %s", user, next), func(s store.Store) string {
		return outcome(s.RotateRecovery(ctx, user, []byte(next), now), "")
	}}
}

func reset(r *reader, now int64) step {
	user, token, until := pick(r, users), pick(r, secrets), now+pick(r, offsets)
	return step{fmt.Sprintf("Reset %s token %s until %d at %d", user, token, until, now), func(s store.Store) string {
		return outcome(s.Reset(ctx, user, []byte(token), until, now), "")
	}}
}

func enroll(r *reader, now int64) step {
	user, id, token, mac := pick(r, users), pick(r, credentialIDs), pick(r, secrets), pick(r, secrets)
	return step{fmt.Sprintf("Enroll %s %s token %s code %s at %d", user, id, token, mac, now), func(s store.Store) string {
		return outcome(s.Enroll(ctx, credentialOf(user, id), []byte(token), []byte(mac), now), "")
	}}
}

func createSession(r *reader, now int64) step {
	hash, user, expires := pick(r, hashes), pick(r, users), now+pick(r, offsets)
	return step{fmt.Sprintf("CreateSession %s %s until %d at %d", hash, user, expires, now), func(s store.Store) string {
		return outcome(s.CreateSession(ctx, []byte(hash), user, expires, now), "")
	}}
}

func readSession(r *reader, now int64) step {
	return sessionStep(pick(r, hashes), now)
}

func sessionStep(hash string, now int64) step {
	return step{fmt.Sprintf("Session %s at %d", hash, now), func(s store.Store) string {
		user, expires, err := s.Session(ctx, []byte(hash), now)
		return outcome(err, fmt.Sprintf("%s until %d", user, expires))
	}}
}

func extendSession(r *reader, now int64) step {
	hash, expires := pick(r, hashes), now+pick(r, offsets)
	return step{fmt.Sprintf("ExtendSession %s until %d at %d", hash, expires, now), func(s store.Store) string {
		return outcome(s.ExtendSession(ctx, []byte(hash), expires, now), "")
	}}
}

func deleteSession(r *reader, now int64) step {
	hash := pick(r, hashes)
	return step{"DeleteSession " + hash, func(s store.Store) string {
		return outcome(s.DeleteSession(ctx, []byte(hash), now), "")
	}}
}

func deleteSessions(r *reader, now int64) step {
	user := pick(r, users)
	return step{"DeleteSessions " + user, func(s store.Store) string {
		return outcome(s.DeleteSessions(ctx, user, now), "")
	}}
}
