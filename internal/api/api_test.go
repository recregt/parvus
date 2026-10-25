// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/recregt/parvus/internal/api"
	"github.com/recregt/parvus/internal/engine"
	"github.com/recregt/parvus/internal/softauth"
	"github.com/recregt/parvus/internal/store"
)

const (
	rpID       = "example.com"
	origin     = "https://example.com"
	hostSecret = "host-secret"
	ceremony   = 60
	session    = 3600
	userAlice  = "alice"
)

type harness struct {
	t       *testing.T
	handler http.Handler
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	db, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "parvus.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	eng, err := engine.New(db, engine.Config{
		RPID: rpID, RPName: "Example", Origins: []string{origin},
		Pepper: []byte("pepper"), MaxCredentials: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return &harness{t: t, handler: api.New(eng, []byte(hostSecret), log)}
}

func (h *harness) do(method, path string, body io.Reader, secret string) (int, []byte) {
	h.t.Helper()
	request := httptest.NewRequest(method, path, body)
	if secret != "" {
		request.Header.Set("Authorization", "Bearer "+secret)
	}
	recorder := httptest.NewRecorder()
	h.handler.ServeHTTP(recorder, request)
	return recorder.Code, recorder.Body.Bytes()
}

func (h *harness) call(method, path string, body any) (int, []byte) {
	h.t.Helper()
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			h.t.Fatal(err)
		}
		reader = bytes.NewReader(data)
	}
	return h.do(method, path, reader, hostSecret)
}

func (h *harness) expect(status int, method, path string, body any, into any) {
	h.t.Helper()
	got, data := h.call(method, path, body)
	if got != status {
		h.t.Fatalf("%s %s: status %d, want %d: %s", method, path, got, status, data)
	}
	if into == nil {
		return
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(into); err != nil {
		h.t.Fatalf("%s %s: unexpected response shape %s: %v", method, path, data, err)
	}
}

type tracked struct {
	read bool
}

func (b *tracked) Read([]byte) (int, error) {
	b.read = true
	return 0, io.EOF
}

func TestHostSecretIsCheckedBeforeTheBodyIsRead(t *testing.T) {
	h := newHarness(t)
	for _, secret := range []string{"", "wrong"} {
		body := &tracked{}
		status, _ := h.do(http.MethodPost, "/v1/login/begin", body, secret)
		if status != http.StatusUnauthorized || body.read {
			t.Fatalf("secret %q: status %d, body read %v", secret, status, body.read)
		}
	}
	if status, _ := h.do(http.MethodGet, "/no/such/route", nil, ""); status != http.StatusUnauthorized {
		t.Fatalf("unknown route without secret answered %d", status)
	}
	if status, _ := h.do(http.MethodGet, "/healthz", nil, ""); status != http.StatusNoContent {
		t.Fatalf("healthz answered %d", status)
	}
}

func TestRequestsAreStrict(t *testing.T) {
	h := newHarness(t)
	for name, body := range map[string]string{
		"unknown field":  `{"slot":"s","ttl_seconds":60,"surprise":1}`,
		"zero ttl":       `{"slot":"s","ttl_seconds":0}`,
		"missing ttl":    `{"slot":"s"}`,
		"overflowing":    `{"slot":"s","ttl_seconds":18446744074}`,
		"trailing data":  `{"slot":"s","ttl_seconds":60} {}`,
		"not an object":  `[]`,
		"empty document": ``,
	} {
		status, data := h.do(http.MethodPost, "/v1/login/begin", strings.NewReader(body), hostSecret)
		if status != http.StatusBadRequest || !strings.Contains(string(data), `"invalid_request"`) {
			t.Errorf("%s: status %d: %s", name, status, data)
		}
	}
}

type userAndCode struct {
	UserID       string `json:"user_id"`
	RecoveryCode string `json:"recovery_code"`
}

type sessionShape struct {
	UserID    string `json:"user_id"`
	Token     string `json:"token"`
	ExpiresAt string `json:"expires_at"`
}

type recovered struct {
	sessionShape
	RecoveryCode    string `json:"recovery_code"`
	CredentialCount int    `json:"credential_count"`
}

type introspected struct {
	UserID    string `json:"user_id"`
	ExpiresAt string `json:"expires_at"`
}

func TestWireContractOfEveryEndpoint(t *testing.T) {
	h := newHarness(t)
	device := softauth.New(t, rpID, origin)

	options := h.options(http.MethodPost, "/v1/registration/begin",
		map[string]any{"slot": "reg", "user_id": userAlice, "name": "Alice", "ttl_seconds": ceremony})
	var registered userAndCode
	h.expect(http.StatusOK, http.MethodPost, "/v1/registration/finish",
		map[string]any{"slot": "reg", "credential": json.RawMessage(device.Register(options))}, &registered)
	if registered.UserID != userAlice || registered.RecoveryCode == "" {
		t.Fatalf("registration result %+v", registered)
	}

	options = h.options(http.MethodPost, "/v1/login/begin", map[string]any{"slot": "login", "ttl_seconds": ceremony})
	var logged sessionShape
	h.expect(http.StatusOK, http.MethodPost, "/v1/login/finish",
		map[string]any{"slot": "login", "credential": json.RawMessage(device.Assert(options)), "ttl_seconds": session}, &logged)
	if logged.UserID != userAlice || logged.Token == "" {
		t.Fatalf("login result %+v", logged)
	}

	var before, after introspected
	h.expect(http.StatusOK, http.MethodPost, "/v1/introspect", map[string]any{"token": logged.Token}, &before)
	h.expect(http.StatusOK, http.MethodPost, "/v1/introspect",
		map[string]any{"token": logged.Token, "extend_by_seconds": 2 * session}, &after)
	if before.UserID != userAlice || !laterThan(after.ExpiresAt, before.ExpiresAt) {
		t.Fatalf("introspect %+v, extended %+v", before, after)
	}

	second := softauth.New(t, rpID, origin)
	options = h.options(http.MethodPost, "/v1/users/"+userAlice+"/credentials/begin", map[string]any{"ttl_seconds": ceremony})
	h.expect(http.StatusNoContent, http.MethodPost, "/v1/users/"+userAlice+"/credentials/finish",
		map[string]any{"credential": json.RawMessage(second.Register(options))}, nil)
	h.expect(http.StatusNoContent, http.MethodDelete,
		"/v1/users/"+userAlice+"/credentials/"+softauth.B64(device.Credential), nil, nil)
	h.expect(http.StatusConflict, http.MethodDelete,
		"/v1/users/"+userAlice+"/credentials/"+softauth.B64(second.Credential), nil, nil)

	var rotated struct {
		RecoveryCode string `json:"recovery_code"`
	}
	h.expect(http.StatusOK, http.MethodPost, "/v1/users/"+userAlice+"/codes", nil, &rotated)

	var recoveredSession recovered
	h.expect(http.StatusOK, http.MethodPost, "/v1/recover",
		map[string]any{"user_id": userAlice, "code": rotated.RecoveryCode, "ttl_seconds": session}, &recoveredSession)
	if recoveredSession.CredentialCount != 1 || recoveredSession.RecoveryCode == "" || recoveredSession.Token == "" {
		t.Fatalf("recover result %+v", recoveredSession)
	}

	h.expect(http.StatusNoContent, http.MethodPost, "/v1/logout", map[string]any{"token": recoveredSession.Token}, nil)
	h.expect(http.StatusForbidden, http.MethodPost, "/v1/introspect", map[string]any{"token": recoveredSession.Token}, nil)
	h.expect(http.StatusNoContent, http.MethodPost, "/v1/users/"+userAlice+"/logout", nil, nil)

	var reset struct {
		EnrollToken string `json:"enroll_token"`
		ExpiresAt   string `json:"expires_at"`
	}
	h.expect(http.StatusOK, http.MethodPost, "/v1/users/"+userAlice+"/reset", map[string]any{"ttl_seconds": session}, &reset)
	if reset.EnrollToken == "" || reset.ExpiresAt == "" {
		t.Fatalf("reset result %+v", reset)
	}

	h.expect(http.StatusNoContent, http.MethodDelete, "/v1/users/"+userAlice, nil, nil)
	h.expect(http.StatusNotFound, http.MethodDelete, "/v1/users/"+userAlice, nil, nil)
}

func laterThan(a, b string) bool {
	left, err := time.Parse(time.RFC3339, a)
	if err != nil {
		return false
	}
	right, err := time.Parse(time.RFC3339, b)
	return err == nil && left.After(right)
}

func (h *harness) options(method, path string, body any) []byte {
	h.t.Helper()
	status, data := h.call(method, path, body)
	if status != http.StatusOK {
		h.t.Fatalf("%s %s: status %d: %s", method, path, status, data)
	}
	return data
}
