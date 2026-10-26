#!/usr/bin/env bash
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this
# file, You can obtain one at https://mozilla.org/MPL/2.0/.

set -euo pipefail

export PYTHONDONTWRITEBYTECODE=1
export UV_PROJECT_ENVIRONMENT="${XDG_CACHE_HOME:-$HOME/.cache}/parvus-packages/venv"
dir="$(cd "$(dirname "$0")" && pwd)"
exec uv run --project "$dir" --locked pytest "$dir" "$@"
