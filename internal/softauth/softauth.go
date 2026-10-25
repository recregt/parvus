// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package softauth

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"testing"
)

const (
	flagUserPresent  = 0x01
	flagUserVerified = 0x04
	flagAttested     = 0x40
	coordinateBytes  = 32
	credentialBytes  = 32
)

type Authenticator struct {
	t          testing.TB
	rpID       string
	origin     string
	key        *ecdsa.PrivateKey
	Credential []byte
	Counter    uint32
	userHandle []byte
}

func New(t testing.TB, rpID, origin string) *Authenticator {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	credential := make([]byte, credentialBytes)
	if _, err := rand.Read(credential); err != nil {
		t.Fatal(err)
	}
	return &Authenticator{t: t, rpID: rpID, origin: origin, key: key, Credential: credential}
}

func B64(data []byte) string {
	return base64.RawURLEncoding.EncodeToString(data)
}

func cborHead(major byte, n int) []byte {
	switch {
	case n < 24:
		return []byte{major<<5 | byte(n)}
	case n < 256:
		return []byte{major<<5 | 24, byte(n)}
	default:
		out := []byte{major<<5 | 25, 0, 0}
		binary.BigEndian.PutUint16(out[1:], uint16(n))
		return out
	}
}

func cborText(s string) []byte {
	return append(cborHead(3, len(s)), s...)
}

func cborBytes(b []byte) []byte {
	return append(cborHead(2, len(b)), b...)
}

func cborInt(n int) []byte {
	if n >= 0 {
		return cborHead(0, n)
	}
	return cborHead(1, -1-n)
}

func concat(parts ...[]byte) []byte {
	var out []byte
	for _, part := range parts {
		out = append(out, part...)
	}
	return out
}

func (a *Authenticator) coseKey() []byte {
	point, err := a.key.PublicKey.Bytes()
	if err != nil {
		a.t.Fatal(err)
	}
	x := point[1 : 1+coordinateBytes]
	y := point[1+coordinateBytes:]
	return concat(
		cborHead(5, 5),
		cborInt(1), cborInt(2),
		cborInt(3), cborInt(-7),
		cborInt(-1), cborInt(1),
		cborInt(-2), cborBytes(x),
		cborInt(-3), cborBytes(y),
	)
}

func (a *Authenticator) challenge(options []byte) string {
	a.t.Helper()
	var parsed struct {
		PublicKey struct {
			Challenge string `json:"challenge"`
			User      struct {
				ID string `json:"id"`
			} `json:"user"`
		} `json:"publicKey"`
	}
	if err := json.Unmarshal(options, &parsed); err != nil {
		a.t.Fatal(err)
	}
	if parsed.PublicKey.User.ID != "" {
		handle, err := base64.RawURLEncoding.DecodeString(parsed.PublicKey.User.ID)
		if err != nil {
			a.t.Fatal(err)
		}
		a.userHandle = handle
	}
	return parsed.PublicKey.Challenge
}

func (a *Authenticator) clientData(kind, challenge string) []byte {
	out, err := json.Marshal(map[string]any{
		"type":      kind,
		"challenge": challenge,
		"origin":    a.origin,
	})
	if err != nil {
		a.t.Fatal(err)
	}
	return out
}

func (a *Authenticator) rpIDHash() []byte {
	sum := sha256.Sum256([]byte(a.rpID))
	return sum[:]
}

func (a *Authenticator) Register(options []byte) []byte {
	a.t.Helper()
	clientData := a.clientData("webauthn.create", a.challenge(options))
	length := make([]byte, 2)
	binary.BigEndian.PutUint16(length, uint16(len(a.Credential)))
	counter := make([]byte, 4)
	binary.BigEndian.PutUint32(counter, a.Counter)
	authData := concat(
		a.rpIDHash(),
		[]byte{flagUserPresent | flagUserVerified | flagAttested},
		counter,
		make([]byte, 16),
		length, a.Credential, a.coseKey(),
	)
	attestation := concat(
		cborHead(5, 3),
		cborText("fmt"), cborText("none"),
		cborText("attStmt"), cborHead(5, 0),
		cborText("authData"), cborBytes(authData),
	)
	return a.response(map[string]any{
		"clientDataJSON":    B64(clientData),
		"attestationObject": B64(attestation),
		"transports":        []string{"internal", "hybrid"},
	})
}

func (a *Authenticator) Assert(options []byte) []byte {
	a.t.Helper()
	clientData := a.clientData("webauthn.get", a.challenge(options))
	a.Counter++
	counter := make([]byte, 4)
	binary.BigEndian.PutUint32(counter, a.Counter)
	authData := concat(a.rpIDHash(), []byte{flagUserPresent | flagUserVerified}, counter)
	clientHash := sha256.Sum256(clientData)
	digest := sha256.Sum256(concat(authData, clientHash[:]))
	signature, err := ecdsa.SignASN1(rand.Reader, a.key, digest[:])
	if err != nil {
		a.t.Fatal(err)
	}
	return a.response(map[string]any{
		"clientDataJSON":    B64(clientData),
		"authenticatorData": B64(authData),
		"signature":         B64(signature),
		"userHandle":        B64(a.userHandle),
	})
}

func (a *Authenticator) response(inner map[string]any) []byte {
	out, err := json.Marshal(map[string]any{
		"id":       B64(a.Credential),
		"rawId":    B64(a.Credential),
		"type":     "public-key",
		"response": inner,
	})
	if err != nil {
		a.t.Fatal(err)
	}
	return out
}
