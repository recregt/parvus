#!/bin/sh
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this
# file, You can obtain one at https://mozilla.org/MPL/2.0/.

set -e

if command -v systemd-sysusers >/dev/null 2>&1; then
	echo 'u parvus - "Parvus passkey engine" /var/lib/parvus' | systemd-sysusers -
elif ! id parvus >/dev/null 2>&1; then
	addgroup -S parvus
	adduser -S -D -H -h /var/lib/parvus -s /sbin/nologin -G parvus parvus
fi
