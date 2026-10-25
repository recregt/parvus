// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package secret_test

import (
	"bytes"
	"regexp"
	"testing"

	"github.com/recregt/parvus/internal/secret"
)

var codeShape = regexp.MustCompile(`^[0-9A-HJKMNP-TV-Z]{4}(-[0-9A-HJKMNP-TV-Z]{4}){3}$`)

func TestCodeShapeAndUniqueness(t *testing.T) {
	seen := map[string]bool{}
	for range 64 {
		code, err := secret.Code()
		if err != nil {
			t.Fatal(err)
		}
		if !codeShape.MatchString(code) {
			t.Fatalf("code %q has unexpected shape", code)
		}
		if seen[code] {
			t.Fatalf("code %q repeated", code)
		}
		seen[code] = true
	}
}

func TestNormalizeAcceptsHumanVariants(t *testing.T) {
	want, ok := secret.Normalize("0123-4567-89AB-CDEF")
	if !ok {
		t.Fatal("canonical form rejected")
	}
	for _, variant := range []string{
		"0123456789abcdef",
		" 0123 4567 89ab cdef ",
		"O123-4567-89AB-CDEF",
		"0123-4567-89AB-CDEF",
	} {
		got, ok := secret.Normalize(variant)
		if !ok || got != want {
			t.Fatalf("%q normalized to %q, %v", variant, got, ok)
		}
	}
	got, ok := secret.Normalize("I123-4567-89AB-CDEF")
	if want1, _ := secret.Normalize("1123-4567-89AB-CDEF"); !ok || got != want1 {
		t.Fatalf("I did not fold to 1: %q", got)
	}
}

func TestNormalizeRejectsMalformed(t *testing.T) {
	for _, bad := range []string{"", "short", "0123-4567-89AB-CDE", "0123-4567-89AB-CDEFG", "0123-4567-89AB-CDEU", "0123-4567-89AB-CDE!"} {
		if _, ok := secret.Normalize(bad); ok {
			t.Fatalf("%q accepted", bad)
		}
	}
}

func TestMACBindsPepperUserAndCode(t *testing.T) {
	base := secret.MAC([]byte("pepper"), "alice", "CODE")
	if !bytes.Equal(base, secret.MAC([]byte("pepper"), "alice", "CODE")) {
		t.Fatal("MAC is not deterministic")
	}
	for name, other := range map[string][]byte{
		"pepper": secret.MAC([]byte("other"), "alice", "CODE"),
		"user":   secret.MAC([]byte("pepper"), "bob", "CODE"),
		"code":   secret.MAC([]byte("pepper"), "alice", "CODF"),
		"split":  secret.MAC([]byte("pepper"), "alic", "eCODE"),
	} {
		if bytes.Equal(base, other) {
			t.Fatalf("MAC ignores %s", name)
		}
	}
}

func TestTokenHashing(t *testing.T) {
	first, err := secret.Token()
	if err != nil {
		t.Fatal(err)
	}
	second, err := secret.Token()
	if err != nil {
		t.Fatal(err)
	}
	if first == second || len(first) != 43 {
		t.Fatalf("tokens %q %q", first, second)
	}
	if bytes.Equal(secret.Hash(first), secret.Hash(second)) || len(secret.Hash(first)) != 32 {
		t.Fatal("hash does not separate tokens")
	}
}

func TestDeriveSeparatesLabels(t *testing.T) {
	if bytes.Equal(secret.Derive([]byte("p"), "a", "u"), secret.Derive([]byte("p"), "b", "u")) {
		t.Fatal("labels not separated")
	}
}
