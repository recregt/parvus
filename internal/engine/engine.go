// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package engine

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/protocol/webauthncose"
	"github.com/go-webauthn/webauthn/webauthn"

	"github.com/recregt/parvus/internal/secret"
	"github.com/recregt/parvus/internal/store"
)

const maxUserIDBytes = 64

var (
	ErrInvalid = errors.New("invalid request")
	ErrAuth    = errors.New("authentication failed")
)

type Config struct {
	RPID           string
	RPName         string
	Origins        []string
	Pepper         []byte
	MaxCredentials int
	Now            func() time.Time
}

type Engine struct {
	store          store.Store
	wa             *webauthn.WebAuthn
	pepper         []byte
	maxCredentials int
	now            func() time.Time
}

type account struct {
	id          string
	name        string
	credentials []webauthn.Credential
}

func (a account) WebAuthnID() []byte                         { return []byte(a.id) }
func (a account) WebAuthnName() string                       { return a.name }
func (a account) WebAuthnDisplayName() string                { return a.name }
func (a account) WebAuthnCredentials() []webauthn.Credential { return a.credentials }

type envelope struct {
	Kind       string               `json:"kind"`
	UserID     string               `json:"user_id,omitempty"`
	Name       string               `json:"name,omitempty"`
	EnrollHash []byte               `json:"enroll_hash,omitempty"`
	Session    webauthn.SessionData `json:"session"`
}

const (
	kindRegister = "reg:"
	kindAdd      = "add:"
	kindLogin    = "login:"
)

type RegistrationInput struct {
	Slot        string
	UserID      string
	Name        string
	TTL         time.Duration
	EnrollToken string
}

type RegistrationResult struct {
	UserID       string
	RecoveryCode string
}

type Session struct {
	UserID    string
	Token     string
	ExpiresAt time.Time
}

type RecoverResult struct {
	Session
	RecoveryCode    string
	CredentialCount int
}

func New(s store.Store, cfg Config) (*Engine, error) {
	if len(cfg.Pepper) == 0 || cfg.MaxCredentials < 1 {
		return nil, fmt.Errorf("%w: pepper and credential limit are required", ErrInvalid)
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	wa, err := webauthn.New(&webauthn.Config{
		RPID:                  cfg.RPID,
		RPDisplayName:         cfg.RPName,
		RPOrigins:             cfg.Origins,
		AttestationPreference: protocol.PreferNoAttestation,
		AuthenticatorSelection: protocol.AuthenticatorSelection{
			ResidentKey:      protocol.ResidentKeyRequirementPreferred,
			UserVerification: protocol.VerificationRequired,
		},
	})
	if err != nil {
		return nil, err
	}
	return &Engine{store: s, wa: wa, pepper: cfg.Pepper, maxCredentials: cfg.MaxCredentials, now: now}, nil
}

func validUserID(id string) error {
	if len(id) < 1 || len(id) > maxUserIDBytes {
		return fmt.Errorf("%w: user id must be 1 to %d bytes", ErrInvalid, maxUserIDBytes)
	}
	return nil
}

func validTTL(ttl time.Duration) error {
	if ttl <= 0 {
		return fmt.Errorf("%w: ttl must be positive", ErrInvalid)
	}
	return nil
}

func authFailure(cause error) error {
	return fmt.Errorf("%w: %w", ErrAuth, cause)
}

func (e *Engine) unix() int64 {
	return e.now().Unix()
}

func (e *Engine) stage(ctx context.Context, slot string, env envelope, expires time.Time) error {
	data, err := json.Marshal(env)
	if err != nil {
		return err
	}
	return e.store.PutChallenge(ctx, slot, data, expires.Unix(), e.unix())
}

func (e *Engine) take(ctx context.Context, slot string) (envelope, error) {
	var env envelope
	data, err := e.store.TakeChallenge(ctx, slot, e.unix())
	if err != nil {
		return env, authFailure(err)
	}
	if err := json.Unmarshal(data, &env); err != nil {
		return env, authFailure(err)
	}
	return env, nil
}

func (e *Engine) load(ctx context.Context, userID, name string) (account, error) {
	stored, err := e.store.Credentials(ctx, userID)
	if err != nil {
		return account{}, err
	}
	acct := account{id: userID, name: name}
	for _, item := range stored {
		var credential webauthn.Credential
		if err := json.Unmarshal(item.Data, &credential); err != nil {
			return account{}, err
		}
		acct.credentials = append(acct.credentials, credential)
	}
	return acct, nil
}

func (e *Engine) newCode(userID string) (code string, mac []byte, err error) {
	code, err = secret.Code()
	if err != nil {
		return "", nil, err
	}
	normalized, _ := secret.Normalize(code)
	return code, secret.MAC(e.pepper, userID, normalized), nil
}

func allowedAlgorithms() []protocol.CredentialParameter {
	algorithms := []webauthncose.COSEAlgorithmIdentifier{
		webauthncose.AlgES256,
		webauthncose.AlgEdDSA,
		webauthncose.AlgRS256,
	}
	out := make([]protocol.CredentialParameter, 0, len(algorithms))
	for _, algorithm := range algorithms {
		out = append(out, protocol.CredentialParameter{Type: protocol.PublicKeyCredentialType, Algorithm: algorithm})
	}
	return out
}

func exclusions(credentials []webauthn.Credential) []protocol.CredentialDescriptor {
	out := make([]protocol.CredentialDescriptor, 0, len(credentials))
	for _, credential := range credentials {
		out = append(out, credential.Descriptor())
	}
	return out
}

func (e *Engine) BeginRegistration(ctx context.Context, in RegistrationInput) ([]byte, error) {
	if in.Slot == "" {
		return nil, fmt.Errorf("%w: slot is required", ErrInvalid)
	}
	if err := validUserID(in.UserID); err != nil {
		return nil, err
	}
	if err := validTTL(in.TTL); err != nil {
		return nil, err
	}
	env := envelope{Kind: kindRegister, UserID: in.UserID, Name: in.Name}
	if env.Name == "" {
		env.Name = in.UserID
	}
	user, err := e.store.User(ctx, in.UserID, e.unix())
	switch {
	case errors.Is(err, store.ErrNotFound):
	case err != nil:
		return nil, err
	case user.CredCount > 0:
		return nil, store.ErrExists
	default:
		hash := secret.Hash(in.EnrollToken)
		if user.EnrollHash == nil || subtle.ConstantTimeCompare(hash, user.EnrollHash) != 1 {
			return nil, authFailure(errors.New("enrollment token rejected"))
		}
		env.EnrollHash = hash
	}
	return e.begin(ctx, kindRegister+in.Slot, env, account{id: env.UserID, name: env.Name}, in.TTL)
}

func (e *Engine) begin(ctx context.Context, slot string, env envelope, acct account, ttl time.Duration) ([]byte, error) {
	creation, session, err := e.wa.BeginRegistration(acct,
		webauthn.WithExclusions(exclusions(acct.credentials)),
		webauthn.WithCredentialParameters(allowedAlgorithms()))
	if err != nil {
		return nil, err
	}
	expires := e.now().Add(ttl)
	session.Expires = expires
	creation.Response.Timeout = int(ttl.Milliseconds())
	env.Session = *session
	if err := e.stage(ctx, slot, env, expires); err != nil {
		return nil, err
	}
	return json.Marshal(creation)
}

func (e *Engine) FinishRegistration(ctx context.Context, slot string, body []byte) (RegistrationResult, error) {
	env, err := e.take(ctx, kindRegister+slot)
	if err != nil {
		return RegistrationResult{}, err
	}
	credential, err := e.createCredential(account{id: env.UserID, name: env.Name}, env, body)
	if err != nil {
		return RegistrationResult{}, err
	}
	code, mac, err := e.newCode(env.UserID)
	if err != nil {
		return RegistrationResult{}, err
	}
	if env.EnrollHash != nil {
		err = e.store.Enroll(ctx, *credential, env.EnrollHash, mac, e.unix())
		if errors.Is(err, store.ErrNotFound) {
			err = authFailure(err)
		}
	} else {
		err = e.store.CreateUser(ctx, *credential, mac, e.unix())
	}
	if err != nil {
		return RegistrationResult{}, err
	}
	return RegistrationResult{UserID: env.UserID, RecoveryCode: code}, nil
}

func (e *Engine) createCredential(acct account, env envelope, body []byte) (*store.Credential, error) {
	parsed, err := protocol.ParseCredentialCreationResponseBytes(body)
	if err != nil {
		return nil, authFailure(err)
	}
	credential, err := e.wa.CreateCredential(acct, env.Session, parsed)
	if err != nil {
		return nil, authFailure(err)
	}
	data, err := json.Marshal(credential)
	if err != nil {
		return nil, err
	}
	return &store.Credential{ID: credential.ID, UserID: env.UserID, Data: data}, nil
}

func (e *Engine) BeginAddCredential(ctx context.Context, userID string, ttl time.Duration) ([]byte, error) {
	if err := validUserID(userID); err != nil {
		return nil, err
	}
	if err := validTTL(ttl); err != nil {
		return nil, err
	}
	user, err := e.store.User(ctx, userID, e.unix())
	if err != nil {
		return nil, err
	}
	if user.CredCount == 0 {
		return nil, store.ErrNotFound
	}
	acct, err := e.load(ctx, userID, userID)
	if err != nil {
		return nil, err
	}
	return e.begin(ctx, kindAdd+userID, envelope{Kind: kindAdd, UserID: userID, Name: userID}, acct, ttl)
}

func (e *Engine) FinishAddCredential(ctx context.Context, userID string, body []byte) error {
	env, err := e.take(ctx, kindAdd+userID)
	if err != nil {
		return err
	}
	credential, err := e.createCredential(account{id: userID, name: env.Name}, env, body)
	if err != nil {
		return err
	}
	return e.store.AddCredential(ctx, *credential, e.maxCredentials, e.unix())
}

func (e *Engine) RemoveCredential(ctx context.Context, userID string, credentialID []byte) error {
	return e.store.RemoveCredential(ctx, userID, credentialID, e.unix())
}

func (e *Engine) BeginLogin(ctx context.Context, slot, userID string, ttl time.Duration) ([]byte, error) {
	if slot == "" {
		return nil, fmt.Errorf("%w: slot is required", ErrInvalid)
	}
	if err := validTTL(ttl); err != nil {
		return nil, err
	}
	env := envelope{Kind: kindLogin, UserID: userID}
	var assertion *protocol.CredentialAssertion
	var session *webauthn.SessionData
	var err error
	if userID == "" {
		assertion, session, err = e.wa.BeginDiscoverableLogin(webauthn.WithUserVerification(protocol.VerificationRequired))
	} else {
		if err = validUserID(userID); err != nil {
			return nil, err
		}
		acct, loadErr := e.load(ctx, userID, userID)
		if loadErr != nil {
			return nil, loadErr
		}
		if len(acct.credentials) == 0 {
			acct.credentials = []webauthn.Credential{{ID: secret.Derive(e.pepper, "credential", userID)}}
		}
		for i := range acct.credentials {
			acct.credentials[i].Transport = nil
		}
		assertion, session, err = e.wa.BeginLogin(acct, webauthn.WithUserVerification(protocol.VerificationRequired))
	}
	if err != nil {
		return nil, err
	}
	expires := e.now().Add(ttl)
	session.Expires = expires
	assertion.Response.Timeout = int(ttl.Milliseconds())
	env.Session = *session
	if err := e.stage(ctx, kindLogin+slot, env, expires); err != nil {
		return nil, err
	}
	return json.Marshal(assertion)
}

func (e *Engine) FinishLogin(ctx context.Context, slot string, body []byte, sessionTTL time.Duration) (Session, error) {
	if err := validTTL(sessionTTL); err != nil {
		return Session{}, err
	}
	env, err := e.take(ctx, kindLogin+slot)
	if err != nil {
		return Session{}, err
	}
	parsed, err := protocol.ParseCredentialRequestResponseBytes(body)
	if err != nil {
		return Session{}, authFailure(err)
	}
	userID := env.UserID
	var credential *webauthn.Credential
	if userID != "" {
		acct, loadErr := e.load(ctx, userID, userID)
		if loadErr != nil {
			return Session{}, authFailure(loadErr)
		}
		credential, err = e.wa.ValidateLogin(acct, env.Session, parsed)
	} else {
		var user webauthn.User
		user, credential, err = e.wa.ValidatePasskeyLogin(e.discover(ctx), env.Session, parsed)
		if err == nil {
			userID = string(user.WebAuthnID())
		}
	}
	if err != nil {
		return Session{}, authFailure(err)
	}
	if credential.Authenticator.CloneWarning {
		return Session{}, authFailure(errors.New("sign counter did not advance"))
	}
	data, err := json.Marshal(credential)
	if err != nil {
		return Session{}, err
	}
	if err := e.store.UpdateCredential(ctx, credential.ID, data, e.unix()); err != nil {
		return Session{}, err
	}
	return e.issue(ctx, userID, sessionTTL)
}

func (e *Engine) discover(ctx context.Context) webauthn.DiscoverableUserHandler {
	return func(_, userHandle []byte) (webauthn.User, error) {
		acct, err := e.load(ctx, string(userHandle), string(userHandle))
		if err != nil {
			return nil, err
		}
		if len(acct.credentials) == 0 {
			return nil, store.ErrNotFound
		}
		return acct, nil
	}
}

func (e *Engine) issue(ctx context.Context, userID string, ttl time.Duration) (Session, error) {
	token, err := secret.Token()
	if err != nil {
		return Session{}, err
	}
	expires := e.now().Add(ttl)
	if err := e.store.CreateSession(ctx, secret.Hash(token), userID, expires.Unix(), e.unix()); err != nil {
		return Session{}, err
	}
	return Session{UserID: userID, Token: token, ExpiresAt: time.Unix(expires.Unix(), 0)}, nil
}

func (e *Engine) Introspect(ctx context.Context, token string) (Session, error) {
	userID, expires, err := e.store.Session(ctx, secret.Hash(token), e.unix())
	if err != nil {
		return Session{}, authFailure(err)
	}
	return Session{UserID: userID, ExpiresAt: time.Unix(expires, 0)}, nil
}

func (e *Engine) Extend(ctx context.Context, token string, by time.Duration) (Session, error) {
	if err := validTTL(by); err != nil {
		return Session{}, err
	}
	current, err := e.Introspect(ctx, token)
	if err != nil {
		return Session{}, err
	}
	extended := e.now().Add(by).Unix()
	if extended <= current.ExpiresAt.Unix() {
		return current, nil
	}
	if err := e.store.ExtendSession(ctx, secret.Hash(token), extended, e.unix()); err != nil {
		return Session{}, authFailure(err)
	}
	current.ExpiresAt = time.Unix(extended, 0)
	return current, nil
}

func (e *Engine) Logout(ctx context.Context, token string) error {
	return e.store.DeleteSession(ctx, secret.Hash(token), e.unix())
}

func (e *Engine) LogoutAll(ctx context.Context, userID string) error {
	return e.store.DeleteSessions(ctx, userID, e.unix())
}

func (e *Engine) Recover(ctx context.Context, userID, code string, ttl time.Duration) (RecoverResult, error) {
	if err := validTTL(ttl); err != nil {
		return RecoverResult{}, err
	}
	normalized, ok := secret.Normalize(code)
	oldMAC := secret.MAC(e.pepper, userID, normalized)
	if !ok || validUserID(userID) != nil {
		return RecoverResult{}, authFailure(errors.New("recovery code rejected"))
	}
	fresh, newMAC, err := e.newCode(userID)
	if err != nil {
		return RecoverResult{}, err
	}
	if err := e.store.ReplaceRecovery(ctx, userID, oldMAC, newMAC, e.unix()); err != nil {
		return RecoverResult{}, authFailure(err)
	}
	session, err := e.issue(ctx, userID, ttl)
	if err != nil {
		return RecoverResult{}, err
	}
	user, err := e.store.User(ctx, userID, e.unix())
	if err != nil {
		return RecoverResult{}, err
	}
	return RecoverResult{Session: session, RecoveryCode: fresh, CredentialCount: user.CredCount}, nil
}

func (e *Engine) RotateRecovery(ctx context.Context, userID string) (string, error) {
	code, mac, err := e.newCode(userID)
	if err != nil {
		return "", err
	}
	if err := e.store.RotateRecovery(ctx, userID, mac, e.unix()); err != nil {
		return "", err
	}
	return code, nil
}

func (e *Engine) Reset(ctx context.Context, userID string, ttl time.Duration) (string, time.Time, error) {
	if err := validUserID(userID); err != nil {
		return "", time.Time{}, err
	}
	if err := validTTL(ttl); err != nil {
		return "", time.Time{}, err
	}
	token, err := secret.Token()
	if err != nil {
		return "", time.Time{}, err
	}
	until := time.Unix(e.now().Add(ttl).Unix(), 0)
	if err := e.store.Reset(ctx, userID, secret.Hash(token), until.Unix(), e.unix()); err != nil {
		return "", time.Time{}, err
	}
	return token, until, nil
}

func (e *Engine) DeleteUser(ctx context.Context, userID string) error {
	return e.store.DeleteUser(ctx, userID, e.unix())
}
