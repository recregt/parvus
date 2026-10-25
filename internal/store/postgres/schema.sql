-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at https://mozilla.org/MPL/2.0/.

CREATE TABLE users (
    id TEXT NOT NULL PRIMARY KEY CHECK (length(id) > 0),
    cred_count INTEGER NOT NULL DEFAULT 0 CHECK (cred_count >= 0),
    recovery_mac BYTEA,
    enroll_hash BYTEA,
    enroll_expires BIGINT
);

CREATE TABLE credentials (
    id BYTEA NOT NULL PRIMARY KEY,
    seq BIGINT GENERATED ALWAYS AS IDENTITY,
    user_id TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    data BYTEA NOT NULL
);

CREATE INDEX credentials_user ON credentials (user_id, seq);

CREATE TABLE sessions (
    hash BYTEA NOT NULL PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    expires BIGINT NOT NULL
);

CREATE INDEX sessions_user ON sessions (user_id);
CREATE INDEX sessions_expires ON sessions (expires);

CREATE TABLE challenges (
    slot TEXT NOT NULL PRIMARY KEY,
    data BYTEA NOT NULL,
    expires BIGINT NOT NULL
);

CREATE INDEX challenges_expires ON challenges (expires);

CREATE TABLE parvus_schema (
    version INTEGER NOT NULL
);

INSERT INTO parvus_schema (version) VALUES (1);
