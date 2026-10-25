---
title: API
weight: 1
---

## Conventions

Every route except `/healthz` requires the header `Authorization: Bearer <PARVUS_SECRET>`. Without it, or with a wrong value, the answer is `401 unauthorized`, whatever the route.

Request bodies are JSON objects. A field Parvus does not know, trailing data after the object, or a body that is not an object at all is refused with `400 invalid_request`.

Times are given as `ttl_seconds`, a whole number of seconds from now, at least one. Times returned by Parvus are `expires_at`, in RFC 3339 format and UTC.

A user `id` is any string of 1 to 64 bytes chosen by your backend. A credential id in a path is the passkey's id in unpadded base64url, the same value the browser reports in the credential's `id` field.

Failures return `{"error": "<code>"}` with one of the codes in [Errors]({{< relref "errors" >}}).

## Registration

### `POST /v1/registration/begin`

Starts creating an account, or re-enrolls a reset account.

| Field | | |
| --- | --- | --- |
| `slot` | required | Your name for this browser. |
| `user_id` | required | The account's id. |
| `name` | optional | Shown by the device when it creates the passkey. Defaults to `user_id`. |
| `enroll_token` | for reset accounts | The token from `/v1/users/{id}/reset`. |
| `ttl_seconds` | required | How long the ceremony may stay open. |

Returns `200` with the creation options, to be handed to the browser unchanged. Fails with `already_exists` if the account already has passkeys, and with `authentication_failed` if the account was reset and the token is missing, wrong or expired.

### `POST /v1/registration/finish`

| Field | | |
| --- | --- | --- |
| `slot` | required | The same slot as in begin. |
| `credential` | required | The browser's credential JSON, unchanged. |

Returns `200` with `{"user_id", "recovery_code"}`. Fails with `authentication_failed` if the ceremony is unknown, expired or does not verify, or if an enrollment token expired during the ceremony. Fails with `already_exists` if the account or the passkey was registered by another request in the meantime.

## Sign-in

### `POST /v1/login/begin`

| Field | | |
| --- | --- | --- |
| `slot` | required | Your name for this browser. |
| `user_id` | optional | Lists that user's passkeys for the browser. Leave it out to let the browser offer every passkey it holds for your site. |
| `ttl_seconds` | required | How long the ceremony may stay open. |

Returns `200` with the request options, to be handed to the browser unchanged.

### `POST /v1/login/finish`

| Field | | |
| --- | --- | --- |
| `slot` | required | The same slot as in begin. |
| `credential` | required | The browser's credential JSON, unchanged. |
| `ttl_seconds` | required | How long the session lasts. |

Returns `200` with `{"user_id", "token", "expires_at"}`. Fails with `authentication_failed` if the ceremony is unknown, expired or does not verify.

## Recovery

### `POST /v1/recover`

| Field | | |
| --- | --- | --- |
| `user_id` | required | The account's id. |
| `code` | required | The recovery code, in any case, with or without dashes and spaces. |
| `ttl_seconds` | required | How long the session lasts. |

Returns `200` with `{"user_id", "token", "expires_at", "recovery_code", "credential_count"}`. The returned `recovery_code` replaces the one just used. Fails with `authentication_failed` if the code is wrong or the account does not exist.

### `POST /v1/users/{id}/codes`

No body. Returns `200` with `{"recovery_code"}` and invalidates the previous code. Fails with `not_found` if the account does not exist or has no passkeys.

## Sessions

### `POST /v1/introspect`

| Field | | |
| --- | --- | --- |
| `token` | required | The session token. |
| `extend_by_seconds` | optional | Moves the end of the session to this many seconds from now, if that is later than its current end. |

Returns `200` with `{"user_id", "expires_at"}`. Fails with `authentication_failed` if the token is unknown or the session has ended.

### `POST /v1/logout`

| Field | | |
| --- | --- | --- |
| `token` | required | The session token. |

Returns `204`, whether or not the session existed.

### `POST /v1/users/{id}/logout`

No body. Ends every session of the account. Returns `204`.

## Passkeys

### `POST /v1/users/{id}/credentials/begin`

| Field | | |
| --- | --- | --- |
| `ttl_seconds` | required | How long the ceremony may stay open. |

Starts adding a passkey to an account that already has one. Returns `200` with the creation options. Fails with `not_found` if the account does not exist or has no passkeys.

### `POST /v1/users/{id}/credentials/finish`

| Field | | |
| --- | --- | --- |
| `credential` | required | The browser's credential JSON, unchanged. |

Returns `204`. Fails with `authentication_failed` if the ceremony is unknown, expired or does not verify, with `credential_limit` if the account already holds `PARVUS_MAX_CREDENTIALS` passkeys, with `already_exists` if the passkey is already registered, and with `not_found` if the account was reset or deleted during the ceremony.

### `DELETE /v1/users/{id}/credentials/{credential-id}`

Returns `204`. Fails with `not_found` if the account has no such passkey, and with `last_credential` if it is the account's only one.

## Accounts

### `POST /v1/users/{id}/reset`

| Field | | |
| --- | --- | --- |
| `ttl_seconds` | required | How long the enrollment token stays valid. |

Removes every passkey, the recovery code and every session, and keeps the account. Returns `200` with `{"enroll_token", "expires_at"}`. A later reset replaces the token. Fails with `not_found` if the account does not exist.

### `DELETE /v1/users/{id}`

Removes the account with everything it holds. Returns `204`. Fails with `not_found` if the account does not exist.

## Health

### `GET /healthz`

Needs no secret. Returns `204` while Parvus is running.
