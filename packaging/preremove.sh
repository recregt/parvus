#!/bin/sh
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this
# file, You can obtain one at https://mozilla.org/MPL/2.0/.

set -e

if [ -d /run/systemd/system ]; then
	case "$1" in
		remove | 0) systemctl disable --now parvus.service ;;
	esac
elif command -v rc-service >/dev/null 2>&1; then
	rc-service --ifstarted parvus stop
	rc-update del parvus default 2>/dev/null || true
fi
