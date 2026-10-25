// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package model_test

import (
	"testing"

	"github.com/recregt/parvus/internal/store"
	"github.com/recregt/parvus/internal/store/contract"
	"github.com/recregt/parvus/internal/store/model"
)

func TestContract(t *testing.T) {
	contract.Run(t, func(*testing.T) store.Store { return model.New() })
}
