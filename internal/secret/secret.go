// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package secret

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"encoding/base64"
	"strings"
)

const (
	tokenBytes = 32
	codeBytes  = 10
	codeChars  = 16
	groupChars = 4
)

var alphabet = base32.NewEncoding("0123456789ABCDEFGHJKMNPQRSTVWXYZ").WithPadding(base32.NoPadding)

var confusable = strings.NewReplacer("I", "1", "L", "1", "O", "0")

func Token() (string, error) {
	raw := make([]byte, tokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func Hash(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

func Code() (string, error) {
	raw := make([]byte, codeBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return group(alphabet.EncodeToString(raw)), nil
}

func Normalize(code string) (string, bool) {
	cleaned := strings.ToUpper(strings.NewReplacer("-", "", " ", "").Replace(code))
	cleaned = confusable.Replace(cleaned)
	if len(cleaned) != codeChars {
		return "", false
	}
	if _, err := alphabet.DecodeString(cleaned); err != nil {
		return "", false
	}
	return cleaned, true
}

func MAC(pepper []byte, userID, code string) []byte {
	mac := hmac.New(sha256.New, pepper)
	mac.Write([]byte(userID))
	mac.Write([]byte{0})
	mac.Write([]byte(code))
	return mac.Sum(nil)
}

func Derive(pepper []byte, label, userID string) []byte {
	mac := hmac.New(sha256.New, pepper)
	mac.Write([]byte(label))
	mac.Write([]byte{0})
	mac.Write([]byte(userID))
	return mac.Sum(nil)
}

func group(code string) string {
	var out strings.Builder
	for i := 0; i < len(code); i += groupChars {
		if i > 0 {
			out.WriteByte('-')
		}
		out.WriteString(code[i : i+groupChars])
	}
	return out.String()
}
