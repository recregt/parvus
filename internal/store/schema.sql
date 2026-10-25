-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at https://mozilla.org/MPL/2.0/.

CREATE TABLE users (
    id TEXT NOT NULL PRIMARY KEY CHECK (length(id) > 0),
    cred_count INTEGER NOT NULL DEFAULT 0 CHECK (cred_count >= 0),
    recovery_mac BLOB,
    enroll_hash BLOB,
    enroll_expires INTEGER
) STRICT;

CREATE TABLE credentials (
    id BLOB NOT NULL PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    data BLOB NOT NULL
) STRICT;

CREATE INDEX credentials_user ON credentials (user_id);

CREATE TABLE sessions (
    hash BLOB NOT NULL PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    expires INTEGER NOT NULL
) STRICT;

CREATE INDEX sessions_user ON sessions (user_id);
CREATE INDEX sessions_expires ON sessions (expires);

CREATE TABLE challenges (
    slot TEXT NOT NULL PRIMARY KEY,
    data BLOB NOT NULL,
    expires INTEGER NOT NULL
) STRICT;

CREATE INDEX challenges_expires ON challenges (expires);

PRAGMA user_version = 1;
