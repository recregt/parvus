// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package api_test

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/recregt/parvus/internal/api"
	"github.com/recregt/parvus/internal/engine"
	"github.com/recregt/parvus/internal/secret"
	"github.com/recregt/parvus/internal/softauth"
	"github.com/recregt/parvus/internal/store/model"
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
	t       testing.TB
	handler http.Handler
}

func newHarness(t testing.TB) *harness {
	t.Helper()
	eng, err := engine.New(model.New(), engine.Config{
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
	h.expectPasskeys(userAlice, softauth.B64(device.Credential), softauth.B64(second.Credential))

	h.expect(http.StatusNoContent, http.MethodDelete,
		"/v1/users/"+userAlice+"/credentials/"+softauth.B64(device.Credential), nil, nil)
	h.expect(http.StatusConflict, http.MethodDelete,
		"/v1/users/"+userAlice+"/credentials/"+softauth.B64(second.Credential), nil, nil)
	h.expectPasskeys(userAlice, softauth.B64(second.Credential))
	h.expect(http.StatusNotFound, http.MethodGet, "/v1/users/nobody/credentials", nil, nil)

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

func (h *harness) expectPasskeys(user string, want ...string) {
	h.t.Helper()
	var listed struct {
		Credentials []struct {
			ID string `json:"id"`
		} `json:"credentials"`
	}
	h.expect(http.StatusOK, http.MethodGet, "/v1/users/"+user+"/credentials", nil, &listed)
	var got []string
	for _, credential := range listed.Credentials {
		got = append(got, credential.ID)
	}
	if !slices.Equal(got, want) {
		h.t.Fatalf("passkeys of %s are %q, want %q", user, got, want)
	}
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

var errorCodes = map[int][]string{
	http.StatusBadRequest:   {"invalid_request"},
	http.StatusUnauthorized: {"unauthorized"},
	http.StatusForbidden:    {"authentication_failed"},
	http.StatusNotFound:     {"not_found"},
	http.StatusConflict:     {"already_exists", "last_credential", "credential_limit"},
}

func (h *harness) registerAlice() {
	h.t.Helper()
	device := softauth.New(h.t, rpID, origin)
	options := h.options(http.MethodPost, "/v1/registration/begin",
		map[string]any{"slot": "reg", "user_id": userAlice, "ttl_seconds": ceremony})
	h.expect(http.StatusOK, http.MethodPost, "/v1/registration/finish",
		map[string]any{"slot": "reg", "credential": json.RawMessage(device.Register(options))}, nil)
}

func FuzzNoRequestFailsInternallyOrSignsIn(f *testing.F) {
	code, err := secret.Code()
	if err != nil {
		f.Fatal(err)
	}
	for _, seed := range []struct{ method, target, body string }{
		{http.MethodPost, "/v1/registration/begin", `{"slot":"reg","user_id":"bob","ttl_seconds":60}`},
		{http.MethodPost, "/v1/registration/finish", `{"slot":"reg","credential":{}}`},
		{http.MethodPost, "/v1/login/begin", `{"slot":"login","user_id":"alice","ttl_seconds":60}`},
		{http.MethodPost, "/v1/login/finish", `{"slot":"login","credential":{},"ttl_seconds":3600}`},
		{http.MethodPost, "/v1/recover", `{"user_id":"alice","code":"` + code + `","ttl_seconds":3600}`},
		{http.MethodPost, "/v1/introspect", `{"token":"token","extend_by_seconds":60}`},
		{http.MethodPost, "/v1/logout", `{"token":"token"}`},
		{http.MethodPost, "/v1/users/alice/logout", ``},
		{http.MethodPost, "/v1/users/alice/credentials/begin", `{"ttl_seconds":60}`},
		{http.MethodPost, "/v1/users/alice/credentials/finish", `{"credential":{}}`},
		{http.MethodGet, "/v1/users/alice/credentials", ``},
		{http.MethodDelete, "/v1/users/alice/credentials/AAAA", ``},
		{http.MethodPost, "/v1/users/alice/codes", ``},
		{http.MethodPost, "/v1/users/alice/reset", `{"ttl_seconds":3600}`},
		{http.MethodDelete, "/v1/users/alice", ``},
	} {
		f.Add(seed.method, seed.target, []byte(seed.body))
	}
	f.Fuzz(func(t *testing.T, method, target string, body []byte) {
		request, err := http.NewRequestWithContext(t.Context(), method, target, bytes.NewReader(body))
		if err != nil || !strings.HasPrefix(target, "/") {
			return
		}
		request.Header.Set("Authorization", "Bearer "+hostSecret)
		h := newHarness(t)
		h.registerAlice()
		recorder := httptest.NewRecorder()
		h.handler.ServeHTTP(recorder, request)
		status, data := recorder.Code, recorder.Body.Bytes()

		if status >= http.StatusBadRequest {
			var refused struct {
				Error string `json:"error"`
			}
			decoder := json.NewDecoder(bytes.NewReader(data))
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&refused); err != nil || !slices.Contains(errorCodes[status], refused.Error) {
				t.Fatalf("%s %s answered %d %s", method, target, status, data)
			}
		}
		var fields map[string]json.RawMessage
		if json.Unmarshal(data, &fields) == nil && fields["token"] != nil {
			t.Fatalf("%s %s signed someone in: %s", method, target, data)
		}
	})
}
