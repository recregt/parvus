#!/bin/sh
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this
# file, You can obtain one at https://mozilla.org/MPL/2.0/.

set -e

write_secret() {
	path=$1
	owner=$2
	mode=$3
	if [ ! -e "$path" ]; then
		(umask 077 && head -c 32 /dev/urandom | base64 > "$path")
	fi
	chown "$owner" "$path"
	chmod "$mode" "$path"
}

write_secret /etc/parvus/secret root:parvus 0640

if command -v systemd-sysusers >/dev/null 2>&1; then
	write_secret /etc/parvus/pepper root:root 0600
else
	write_secret /etc/parvus/pepper parvus:parvus 0400
fi

if [ -d /run/systemd/system ]; then
	systemctl daemon-reload
	systemctl try-restart parvus.service
elif command -v rc-service >/dev/null 2>&1; then
	rc-service --ifstarted parvus restart
fi
