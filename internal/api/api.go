// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package api

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/recregt/parvus/internal/engine"
	"github.com/recregt/parvus/internal/store"
)

const (
	bearerPrefix  = "Bearer "
	healthPath    = "/healthz"
	maxTTLSeconds = math.MaxInt64 / int64(time.Second)
)

type API struct {
	engine     *engine.Engine
	secretHash [sha256.Size]byte
	log        *slog.Logger
	mux        *http.ServeMux
}

type handler func(http.ResponseWriter, *http.Request) error

func New(e *engine.Engine, hostSecret []byte, log *slog.Logger) http.Handler {
	a := &API{engine: e, secretHash: sha256.Sum256(hostSecret), log: log, mux: http.NewServeMux()}
	a.mux.HandleFunc("GET "+healthPath, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	a.handle("POST /v1/registration/begin", a.registrationBegin)
	a.handle("POST /v1/registration/finish", a.registrationFinish)
	a.handle("POST /v1/login/begin", a.loginBegin)
	a.handle("POST /v1/login/finish", a.loginFinish)
	a.handle("POST /v1/recover", a.recoverAccount)
	a.handle("POST /v1/introspect", a.introspect)
	a.handle("POST /v1/logout", a.logout)
	a.handle("POST /v1/users/{id}/logout", a.logoutAll)
	a.handle("POST /v1/users/{id}/credentials/begin", a.credentialBegin)
	a.handle("POST /v1/users/{id}/credentials/finish", a.credentialFinish)
	a.handle("DELETE /v1/users/{id}/credentials/{cid}", a.credentialDelete)
	a.handle("POST /v1/users/{id}/codes", a.rotateCode)
	a.handle("POST /v1/users/{id}/reset", a.reset)
	a.handle("DELETE /v1/users/{id}", a.deleteUser)
	a.handle("/", func(http.ResponseWriter, *http.Request) error { return store.ErrNotFound })
	return a
}

func (a *API) handle(pattern string, fn handler) {
	a.mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
		if err := fn(w, r); err != nil {
			a.fail(w, err)
		}
	})
}

func (a *API) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
	if r.URL.Path != healthPath && !a.authorized(r) {
		writeError(recorder, http.StatusUnauthorized, "unauthorized")
	} else {
		a.mux.ServeHTTP(recorder, r)
	}
	a.log.Info("request", "method", r.Method, "path", r.URL.Path, "status", recorder.status)
}

func (a *API) authorized(r *http.Request) bool {
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, bearerPrefix) {
		return false
	}
	presented := sha256.Sum256([]byte(strings.TrimPrefix(header, bearerPrefix)))
	return subtle.ConstantTimeCompare(presented[:], a.secretHash[:]) == 1
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(status int) {
	s.status = status
	s.ResponseWriter.WriteHeader(status)
}

func classify(err error) (int, string) {
	switch {
	case errors.Is(err, engine.ErrInvalid):
		return http.StatusBadRequest, "invalid_request"
	case errors.Is(err, engine.ErrAuth):
		return http.StatusForbidden, "authentication_failed"
	case errors.Is(err, store.ErrNotFound):
		return http.StatusNotFound, "not_found"
	case errors.Is(err, store.ErrExists):
		return http.StatusConflict, "already_exists"
	case errors.Is(err, store.ErrLastCredential):
		return http.StatusConflict, "last_credential"
	case errors.Is(err, store.ErrLimit):
		return http.StatusConflict, "credential_limit"
	default:
		return http.StatusInternalServerError, "internal"
	}
}

func (a *API) fail(w http.ResponseWriter, err error) {
	status, code := classify(err)
	if status == http.StatusInternalServerError {
		a.log.Error("internal error", "error", err)
	} else {
		a.log.Info("request refused", "code", code, "cause", err)
	}
	writeError(w, status, code)
}

func writeError(w http.ResponseWriter, status int, code string) {
	writeJSON(w, status, map[string]string{"error": code})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	data, err := json.Marshal(body)
	if err != nil {
		status, data = http.StatusInternalServerError, []byte(`{"error":"internal"}`)
	}
	writeRaw(w, status, data)
}

func writeRaw(w http.ResponseWriter, status int, data []byte) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(data) //nolint:gosec // the body is JSON for a machine client, never rendered as HTML
}

func invalid(cause error) error {
	return fmt.Errorf("%w: %w", engine.ErrInvalid, cause)
}

func decode(r *http.Request, into any) error {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(into); err != nil {
		return invalid(err)
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return invalid(errors.New("unexpected data after the JSON body"))
	}
	return nil
}

func seconds(n int64) (time.Duration, error) {
	if n < 1 || n > maxTTLSeconds {
		return 0, invalid(errors.New("ttl is out of range"))
	}
	return time.Duration(n) * time.Second, nil
}

func stamp(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}

type sessionBody struct {
	UserID    string `json:"user_id"`
	Token     string `json:"token"`
	ExpiresAt string `json:"expires_at"`
}

func sessionOf(s engine.Session) sessionBody {
	return sessionBody{UserID: s.UserID, Token: s.Token, ExpiresAt: stamp(s.ExpiresAt)}
}

func (a *API) registrationBegin(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Slot        string `json:"slot"`
		UserID      string `json:"user_id"`
		Name        string `json:"name"`
		EnrollToken string `json:"enroll_token"`
		TTLSeconds  int64  `json:"ttl_seconds"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	ttl, err := seconds(in.TTLSeconds)
	if err != nil {
		return err
	}
	options, err := a.engine.BeginRegistration(r.Context(), engine.RegistrationInput{
		Slot: in.Slot, UserID: in.UserID, Name: in.Name, EnrollToken: in.EnrollToken, TTL: ttl,
	})
	if err != nil {
		return err
	}
	writeRaw(w, http.StatusOK, options)
	return nil
}

func (a *API) registrationFinish(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Slot       string          `json:"slot"`
		Credential json.RawMessage `json:"credential"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	result, err := a.engine.FinishRegistration(r.Context(), in.Slot, in.Credential)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"user_id":       result.UserID,
		"recovery_code": result.RecoveryCode,
	})
	return nil
}

func (a *API) loginBegin(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Slot       string `json:"slot"`
		UserID     string `json:"user_id"`
		TTLSeconds int64  `json:"ttl_seconds"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	ttl, err := seconds(in.TTLSeconds)
	if err != nil {
		return err
	}
	options, err := a.engine.BeginLogin(r.Context(), in.Slot, in.UserID, ttl)
	if err != nil {
		return err
	}
	writeRaw(w, http.StatusOK, options)
	return nil
}

func (a *API) loginFinish(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Slot       string          `json:"slot"`
		Credential json.RawMessage `json:"credential"`
		TTLSeconds int64           `json:"ttl_seconds"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	ttl, err := seconds(in.TTLSeconds)
	if err != nil {
		return err
	}
	session, err := a.engine.FinishLogin(r.Context(), in.Slot, in.Credential, ttl)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, sessionOf(session))
	return nil
}

func (a *API) recoverAccount(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		UserID     string `json:"user_id"`
		Code       string `json:"code"`
		TTLSeconds int64  `json:"ttl_seconds"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	ttl, err := seconds(in.TTLSeconds)
	if err != nil {
		return err
	}
	result, err := a.engine.Recover(r.Context(), in.UserID, in.Code, ttl)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, struct {
		sessionBody
		RecoveryCode    string `json:"recovery_code"`
		CredentialCount int    `json:"credential_count"`
	}{sessionOf(result.Session), result.RecoveryCode, result.CredentialCount})
	return nil
}

func (a *API) introspect(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Token           string `json:"token"`
		ExtendBySeconds int64  `json:"extend_by_seconds"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	session, err := a.sessionFor(r, in.Token, in.ExtendBySeconds)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"user_id":    session.UserID,
		"expires_at": stamp(session.ExpiresAt),
	})
	return nil
}

func (a *API) sessionFor(r *http.Request, token string, extendBy int64) (engine.Session, error) {
	if extendBy == 0 {
		return a.engine.Introspect(r.Context(), token)
	}
	by, err := seconds(extendBy)
	if err != nil {
		return engine.Session{}, err
	}
	return a.engine.Extend(r.Context(), token, by)
}

func (a *API) logout(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Token string `json:"token"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	if err := a.engine.Logout(r.Context(), in.Token); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (a *API) logoutAll(w http.ResponseWriter, r *http.Request) error {
	if err := a.engine.LogoutAll(r.Context(), r.PathValue("id")); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (a *API) credentialBegin(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		TTLSeconds int64 `json:"ttl_seconds"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	ttl, err := seconds(in.TTLSeconds)
	if err != nil {
		return err
	}
	options, err := a.engine.BeginAddCredential(r.Context(), r.PathValue("id"), ttl)
	if err != nil {
		return err
	}
	writeRaw(w, http.StatusOK, options)
	return nil
}

func (a *API) credentialFinish(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Credential json.RawMessage `json:"credential"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	if err := a.engine.FinishAddCredential(r.Context(), r.PathValue("id"), in.Credential); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (a *API) credentialDelete(w http.ResponseWriter, r *http.Request) error {
	id, err := base64.RawURLEncoding.DecodeString(r.PathValue("cid"))
	if err != nil {
		return invalid(err)
	}
	if err := a.engine.RemoveCredential(r.Context(), r.PathValue("id"), id); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (a *API) rotateCode(w http.ResponseWriter, r *http.Request) error {
	code, err := a.engine.RotateRecovery(r.Context(), r.PathValue("id"))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]string{"recovery_code": code})
	return nil
}

func (a *API) reset(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		TTLSeconds int64 `json:"ttl_seconds"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	ttl, err := seconds(in.TTLSeconds)
	if err != nil {
		return err
	}
	token, until, err := a.engine.Reset(r.Context(), r.PathValue("id"), ttl)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"enroll_token": token,
		"expires_at":   stamp(until),
	})
	return nil
}

func (a *API) deleteUser(w http.ResponseWriter, r *http.Request) error {
	if err := a.engine.DeleteUser(r.Context(), r.PathValue("id")); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}
