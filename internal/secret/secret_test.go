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

func TestIssuedCodeKeepsTheDocumentedFormat(t *testing.T) {
	code, err := secret.Code()
	if err != nil {
		t.Fatal(err)
	}
	if !codeShape.MatchString(code) {
		t.Fatalf("code %q is not 16 Crockford characters in groups of four", code)
	}
	if _, ok := secret.Normalize(code); !ok {
		t.Fatalf("issued code %q is rejected by Normalize", code)
	}
}

func TestNormalizeAcceptsWhatPeopleTypeFromPaper(t *testing.T) {
	want, ok := secret.Normalize("0123-4567-89AB-CDEF")
	if !ok {
		t.Fatal("canonical form rejected")
	}
	for _, variant := range []string{
		"0123456789abcdef",
		" 0123 4567 89ab cdef ",
		"O123-4567-89AB-CDEF",
	} {
		got, ok := secret.Normalize(variant)
		if !ok || got != want {
			t.Fatalf("%q normalized to %q, %v", variant, got, ok)
		}
	}
	folded, _ := secret.Normalize("1123-4567-89AB-CDEF")
	if got, ok := secret.Normalize("L123-4567-89AB-CDEF"); !ok || got != folded {
		t.Fatalf("L did not fold to 1: %q, %v", got, ok)
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
