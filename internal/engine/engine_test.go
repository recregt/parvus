// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package engine_test

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/recregt/parvus/internal/engine"
	"github.com/recregt/parvus/internal/softauth"
	"github.com/recregt/parvus/internal/store"
)

const (
	rpID           = "example.com"
	origin         = "https://example.com"
	ceremonyTTL    = time.Minute
	sessionTTL     = time.Hour
	credentialCap  = 3
	startingUnix   = 1_800_000_000
	pepperFixture  = "0123456789abcdef0123456789abcdef"
	userAlice      = "alice"
	slotOne        = "slot-1"
	slotTwo        = "slot-2"
	slotThree      = "slot-3"
	enrollmentSlot = "enroll"
)

type clock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *clock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

type harness struct {
	t      *testing.T
	ctx    context.Context
	engine *engine.Engine
	clock  *clock
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	ctx := context.Background()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "parvus.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	clk := &clock{now: time.Unix(startingUnix, 0)}
	eng, err := engine.New(db, engine.Config{
		RPID:           rpID,
		RPName:         "Example",
		Origins:        []string{origin},
		Pepper:         []byte(pepperFixture),
		MaxCredentials: credentialCap,
		Now:            clk.Now,
	})
	if err != nil {
		t.Fatal(err)
	}
	return &harness{t: t, ctx: ctx, engine: eng, clock: clk}
}

func (h *harness) register(userID, slot string) (*softauth.Authenticator, string) {
	h.t.Helper()
	device := softauth.New(h.t, rpID, origin)
	options, err := h.engine.BeginRegistration(h.ctx, engine.RegistrationInput{
		Slot: slot, UserID: userID, TTL: ceremonyTTL,
	})
	if err != nil {
		h.t.Fatal(err)
	}
	result, err := h.engine.FinishRegistration(h.ctx, slot, device.Register(options))
	if err != nil {
		h.t.Fatal(err)
	}
	return device, result.RecoveryCode
}

func (h *harness) login(device *softauth.Authenticator, slot, userID string) (engine.Session, error) {
	h.t.Helper()
	options, err := h.engine.BeginLogin(h.ctx, slot, userID, ceremonyTTL)
	if err != nil {
		h.t.Fatal(err)
	}
	return h.engine.FinishLogin(h.ctx, slot, device.Assert(options), sessionTTL)
}

func (h *harness) addDevice(userID string) (*softauth.Authenticator, error) {
	h.t.Helper()
	device := softauth.New(h.t, rpID, origin)
	options, err := h.engine.BeginAddCredential(h.ctx, userID, ceremonyTTL)
	if err != nil {
		return nil, err
	}
	return device, h.engine.FinishAddCredential(h.ctx, userID, device.Register(options))
}

func TestRegisterThenLoginWithoutUsername(t *testing.T) {
	h := newHarness(t)
	device, _ := h.register(userAlice, slotOne)
	session, err := h.login(device, slotTwo, "")
	if err != nil {
		t.Fatal(err)
	}
	if session.UserID != userAlice {
		t.Fatalf("session user = %q", session.UserID)
	}
	info, err := h.engine.Introspect(h.ctx, session.Token)
	if err != nil || info.UserID != userAlice {
		t.Fatalf("introspect = %+v, %v", info, err)
	}
}

func TestRegistrationOffersOnlyTheAllowedAlgorithms(t *testing.T) {
	h := newHarness(t)
	options, err := h.engine.BeginRegistration(h.ctx, engine.RegistrationInput{
		Slot: slotOne, UserID: userAlice, TTL: ceremonyTTL,
	})
	if err != nil {
		t.Fatal(err)
	}
	var parsed struct {
		PublicKey struct {
			Params []struct {
				Alg int `json:"alg"`
			} `json:"pubKeyCredParams"`
		} `json:"publicKey"`
	}
	if err := json.Unmarshal(options, &parsed); err != nil {
		t.Fatal(err)
	}
	var offered []int
	for _, param := range parsed.PublicKey.Params {
		offered = append(offered, param.Alg)
	}
	if want := []int{-7, -8, -257}; !slices.Equal(offered, want) {
		t.Fatalf("offered algorithms %v, want %v", offered, want)
	}
}

func TestLoginWithUserIDListsCredentials(t *testing.T) {
	h := newHarness(t)
	device, _ := h.register(userAlice, slotOne)
	options, err := h.engine.BeginLogin(h.ctx, slotTwo, userAlice, ceremonyTTL)
	if err != nil {
		t.Fatal(err)
	}
	var parsed struct {
		PublicKey struct {
			Allow []struct {
				ID         string   `json:"id"`
				Transports []string `json:"transports"`
			} `json:"allowCredentials"`
			Timeout int `json:"timeout"`
		} `json:"publicKey"`
	}
	if err := json.Unmarshal(options, &parsed); err != nil {
		t.Fatal(err)
	}
	if len(parsed.PublicKey.Allow) != 1 || parsed.PublicKey.Allow[0].ID != softauth.B64(device.Credential) {
		t.Fatalf("allowCredentials = %+v", parsed.PublicKey.Allow)
	}
	if len(parsed.PublicKey.Allow[0].Transports) != 0 {
		t.Fatalf("transports leaked into allowCredentials: %+v", parsed.PublicKey.Allow[0])
	}
	if parsed.PublicKey.Timeout != int(ceremonyTTL.Milliseconds()) {
		t.Fatalf("timeout = %d", parsed.PublicKey.Timeout)
	}
	if _, err := h.engine.FinishLogin(h.ctx, slotTwo, device.Assert(options), sessionTTL); err != nil {
		t.Fatal(err)
	}
}

func TestUnknownUserLooksLikeKnownUser(t *testing.T) {
	h := newHarness(t)
	options, err := h.engine.BeginLogin(h.ctx, slotOne, "nobody", ceremonyTTL)
	if err != nil {
		t.Fatal(err)
	}
	var parsed struct {
		PublicKey struct {
			Allow []struct{ ID string } `json:"allowCredentials"`
		} `json:"publicKey"`
	}
	if err := json.Unmarshal(options, &parsed); err != nil {
		t.Fatal(err)
	}
	if len(parsed.PublicKey.Allow) != 1 {
		t.Fatalf("allowCredentials = %+v", parsed.PublicKey.Allow)
	}
	stranger := softauth.New(t, rpID, origin)
	_, err = h.engine.FinishLogin(h.ctx, slotOne, stranger.Assert(options), sessionTTL)
	if !errors.Is(err, engine.ErrAuth) {
		t.Fatalf("err = %v", err)
	}
}

func TestSignCounterThatDoesNotAdvanceIsRejected(t *testing.T) {
	h := newHarness(t)
	device, _ := h.register(userAlice, slotOne)
	if _, err := h.login(device, slotTwo, ""); err != nil {
		t.Fatal(err)
	}
	device.Counter = 0
	if _, err := h.login(device, slotThree, ""); !errors.Is(err, engine.ErrAuth) {
		t.Fatalf("err = %v", err)
	}
}

func TestChallengeIsSingleUseAndExpires(t *testing.T) {
	h := newHarness(t)
	device, _ := h.register(userAlice, slotOne)
	options, err := h.engine.BeginLogin(h.ctx, slotTwo, "", ceremonyTTL)
	if err != nil {
		t.Fatal(err)
	}
	body := device.Assert(options)
	if _, err := h.engine.FinishLogin(h.ctx, slotTwo, body, sessionTTL); err != nil {
		t.Fatal(err)
	}
	if _, err := h.engine.FinishLogin(h.ctx, slotTwo, body, sessionTTL); !errors.Is(err, engine.ErrAuth) {
		t.Fatalf("replay err = %v", err)
	}
	options, err = h.engine.BeginLogin(h.ctx, slotThree, "", ceremonyTTL)
	if err != nil {
		t.Fatal(err)
	}
	h.clock.Advance(ceremonyTTL)
	if _, err := h.engine.FinishLogin(h.ctx, slotThree, device.Assert(options), sessionTTL); !errors.Is(err, engine.ErrAuth) {
		t.Fatalf("expired err = %v", err)
	}
}

func TestBeginReplacesPendingChallengeInSameSlot(t *testing.T) {
	h := newHarness(t)
	device, _ := h.register(userAlice, slotOne)
	first, err := h.engine.BeginLogin(h.ctx, slotTwo, "", ceremonyTTL)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.engine.BeginLogin(h.ctx, slotTwo, "", ceremonyTTL); err != nil {
		t.Fatal(err)
	}
	if _, err := h.engine.FinishLogin(h.ctx, slotTwo, device.Assert(first), sessionTTL); !errors.Is(err, engine.ErrAuth) {
		t.Fatalf("err = %v", err)
	}
}

func TestSecondRegistrationForExistingUserIsRefused(t *testing.T) {
	h := newHarness(t)
	h.register(userAlice, slotOne)
	_, err := h.engine.BeginRegistration(h.ctx, engine.RegistrationInput{
		Slot: slotTwo, UserID: userAlice, TTL: ceremonyTTL,
	})
	if !errors.Is(err, store.ErrExists) {
		t.Fatalf("err = %v", err)
	}
}

func TestRecoveryCodeIsSingleUseAndRotates(t *testing.T) {
	h := newHarness(t)
	_, code := h.register(userAlice, slotOne)
	loose := strings.ToLower(strings.ReplaceAll(code, "-", ""))
	result, err := h.engine.Recover(h.ctx, userAlice, loose, sessionTTL)
	if err != nil {
		t.Fatal(err)
	}
	if result.CredentialCount != 1 || result.RecoveryCode == code || result.UserID != userAlice {
		t.Fatalf("result = %+v", result)
	}
	if _, err := h.engine.Recover(h.ctx, userAlice, code, sessionTTL); !errors.Is(err, engine.ErrAuth) {
		t.Fatalf("reuse err = %v", err)
	}
	if _, err := h.engine.Recover(h.ctx, userAlice, result.RecoveryCode, sessionTTL); err != nil {
		t.Fatal(err)
	}
}

func TestRecoveryRejectsWrongUserAndGarbage(t *testing.T) {
	h := newHarness(t)
	_, code := h.register(userAlice, slotOne)
	for _, attempt := range []struct{ user, code string }{
		{"bob", code},
		{userAlice, "AAAA-AAAA-AAAA-AAAA"},
		{userAlice, "short"},
		{"", code},
	} {
		if _, err := h.engine.Recover(h.ctx, attempt.user, attempt.code, sessionTTL); !errors.Is(err, engine.ErrAuth) {
			t.Fatalf("%+v: err = %v", attempt, err)
		}
	}
}

func TestRotateRecoveryVoidsOldCode(t *testing.T) {
	h := newHarness(t)
	_, code := h.register(userAlice, slotOne)
	fresh, err := h.engine.RotateRecovery(h.ctx, userAlice)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.engine.Recover(h.ctx, userAlice, code, sessionTTL); !errors.Is(err, engine.ErrAuth) {
		t.Fatalf("old code err = %v", err)
	}
	if _, err := h.engine.Recover(h.ctx, userAlice, fresh, sessionTTL); err != nil {
		t.Fatal(err)
	}
}

func TestResetThenEnrollWithToken(t *testing.T) {
	h := newHarness(t)
	device, code := h.register(userAlice, slotOne)
	session, err := h.login(device, slotTwo, "")
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := h.engine.Reset(h.ctx, userAlice, sessionTTL)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.engine.Introspect(h.ctx, session.Token); !errors.Is(err, engine.ErrAuth) {
		t.Fatalf("session survived reset: %v", err)
	}
	if _, err := h.engine.Recover(h.ctx, userAlice, code, sessionTTL); !errors.Is(err, engine.ErrAuth) {
		t.Fatalf("code survived reset: %v", err)
	}
	if _, err := h.login(device, slotThree, ""); !errors.Is(err, engine.ErrAuth) {
		t.Fatalf("old credential survived reset: %v", err)
	}
	begin := func(enroll string) ([]byte, error) {
		return h.engine.BeginRegistration(h.ctx, engine.RegistrationInput{
			Slot: enrollmentSlot, UserID: userAlice, TTL: ceremonyTTL, EnrollToken: enroll,
		})
	}
	for _, bad := range []string{"", "wrong"} {
		if _, err := begin(bad); !errors.Is(err, engine.ErrAuth) {
			t.Fatalf("token %q err = %v", bad, err)
		}
	}
	options, err := begin(token)
	if err != nil {
		t.Fatal(err)
	}
	fresh := softauth.New(t, rpID, origin)
	result, err := h.engine.FinishRegistration(h.ctx, enrollmentSlot, fresh.Register(options))
	if err != nil {
		t.Fatal(err)
	}
	if result.RecoveryCode == "" || result.RecoveryCode == code {
		t.Fatalf("result = %+v", result)
	}
	if _, err := h.login(fresh, slotOne, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := begin(token); !errors.Is(err, store.ErrExists) {
		t.Fatalf("token reuse err = %v", err)
	}
}

func TestEnrollmentTokenExpires(t *testing.T) {
	h := newHarness(t)
	h.register(userAlice, slotOne)
	token, _, err := h.engine.Reset(h.ctx, userAlice, ceremonyTTL)
	if err != nil {
		t.Fatal(err)
	}
	h.clock.Advance(ceremonyTTL)
	_, err = h.engine.BeginRegistration(h.ctx, engine.RegistrationInput{
		Slot: slotTwo, UserID: userAlice, TTL: ceremonyTTL, EnrollToken: token,
	})
	if !errors.Is(err, engine.ErrAuth) {
		t.Fatalf("err = %v", err)
	}
}

func TestAddAndRemoveCredentials(t *testing.T) {
	h := newHarness(t)
	first, _ := h.register(userAlice, slotOne)
	second, err := h.addDevice(userAlice)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.login(second, slotTwo, ""); err != nil {
		t.Fatal(err)
	}
	if err := h.engine.RemoveCredential(h.ctx, userAlice, first.Credential); err != nil {
		t.Fatal(err)
	}
	if err := h.engine.RemoveCredential(h.ctx, userAlice, second.Credential); !errors.Is(err, store.ErrLastCredential) {
		t.Fatalf("err = %v", err)
	}
	if err := h.engine.RemoveCredential(h.ctx, userAlice, first.Credential); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestCredentialLimit(t *testing.T) {
	h := newHarness(t)
	h.register(userAlice, slotOne)
	for range credentialCap - 1 {
		if _, err := h.addDevice(userAlice); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := h.addDevice(userAlice); !errors.Is(err, store.ErrLimit) {
		t.Fatalf("err = %v", err)
	}
}

func TestSessionLifecycle(t *testing.T) {
	h := newHarness(t)
	device, _ := h.register(userAlice, slotOne)
	session, err := h.login(device, slotTwo, "")
	if err != nil {
		t.Fatal(err)
	}
	same, err := h.engine.Extend(h.ctx, session.Token, time.Minute)
	if err != nil || !same.ExpiresAt.Equal(session.ExpiresAt) {
		t.Fatalf("shorter extension changed expiry: %+v, %v", same, err)
	}
	longer, err := h.engine.Extend(h.ctx, session.Token, 2*sessionTTL)
	if err != nil || !longer.ExpiresAt.After(session.ExpiresAt) {
		t.Fatalf("extension = %+v, %v", longer, err)
	}
	h.clock.Advance(sessionTTL + time.Minute)
	if _, err := h.engine.Introspect(h.ctx, session.Token); err != nil {
		t.Fatalf("extended session expired early: %v", err)
	}
	h.clock.Advance(2 * sessionTTL)
	if _, err := h.engine.Introspect(h.ctx, session.Token); !errors.Is(err, engine.ErrAuth) {
		t.Fatalf("err = %v", err)
	}
	if _, err := h.engine.Extend(h.ctx, session.Token, sessionTTL); !errors.Is(err, engine.ErrAuth) {
		t.Fatalf("expired session was extended: %v", err)
	}
}

func TestLogoutAndLogoutAll(t *testing.T) {
	h := newHarness(t)
	device, _ := h.register(userAlice, slotOne)
	one, err := h.login(device, slotTwo, "")
	if err != nil {
		t.Fatal(err)
	}
	two, err := h.login(device, slotThree, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := h.engine.Logout(h.ctx, one.Token); err != nil {
		t.Fatal(err)
	}
	if _, err := h.engine.Introspect(h.ctx, one.Token); !errors.Is(err, engine.ErrAuth) {
		t.Fatalf("err = %v", err)
	}
	if err := h.engine.LogoutAll(h.ctx, userAlice); err != nil {
		t.Fatal(err)
	}
	if _, err := h.engine.Introspect(h.ctx, two.Token); !errors.Is(err, engine.ErrAuth) {
		t.Fatalf("err = %v", err)
	}
}

func TestDeleteUserRemovesEverything(t *testing.T) {
	h := newHarness(t)
	device, code := h.register(userAlice, slotOne)
	session, err := h.login(device, slotTwo, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := h.engine.DeleteUser(h.ctx, userAlice); err != nil {
		t.Fatal(err)
	}
	if _, err := h.engine.Introspect(h.ctx, session.Token); !errors.Is(err, engine.ErrAuth) {
		t.Fatalf("err = %v", err)
	}
	if _, err := h.engine.Recover(h.ctx, userAlice, code, sessionTTL); !errors.Is(err, engine.ErrAuth) {
		t.Fatalf("err = %v", err)
	}
	if err := h.engine.DeleteUser(h.ctx, userAlice); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
	h.register(userAlice, slotThree)
}

func TestInvalidInputsAreRejected(t *testing.T) {
	h := newHarness(t)
	cases := []engine.RegistrationInput{
		{Slot: "", UserID: userAlice, TTL: ceremonyTTL},
		{Slot: slotOne, UserID: "", TTL: ceremonyTTL},
		{Slot: slotOne, UserID: strings.Repeat("a", 65), TTL: ceremonyTTL},
		{Slot: slotOne, UserID: userAlice, TTL: 0},
	}
	for _, in := range cases {
		if _, err := h.engine.BeginRegistration(h.ctx, in); !errors.Is(err, engine.ErrInvalid) {
			t.Fatalf("%+v: err = %v", in, err)
		}
	}
}
