// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package api

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/recregt/parvus/internal/engine"
	"github.com/recregt/parvus/internal/store"
)

func TestEveryFailureMapsToOneFixedCode(t *testing.T) {
	for _, tc := range []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"invalid", engine.ErrInvalid, http.StatusBadRequest, "invalid_request"},
		{"authentication", engine.ErrAuth, http.StatusForbidden, "authentication_failed"},
		{"not found", store.ErrNotFound, http.StatusNotFound, "not_found"},
		{"exists", store.ErrExists, http.StatusConflict, "already_exists"},
		{"last credential", store.ErrLastCredential, http.StatusConflict, "last_credential"},
		{"limit", store.ErrLimit, http.StatusConflict, "credential_limit"},
		{"unknown", errors.New("disk on fire"), http.StatusInternalServerError, "internal"},
		{
			"authentication wrapping a store error stays authentication",
			fmt.Errorf("%w: %w", engine.ErrAuth, store.ErrNotFound),
			http.StatusForbidden, "authentication_failed",
		},
	} {
		status, code := classify(tc.err)
		if status != tc.status || code != tc.code {
			t.Errorf("%s: got %d %s, want %d %s", tc.name, status, code, tc.status, tc.code)
		}
	}
}
