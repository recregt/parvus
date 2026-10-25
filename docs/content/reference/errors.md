---
title: Errors
weight: 3
---

Every failure is a JSON object with a single field, `{"error": "<code>"}`, and one of these eight codes.

| Status | Code | Meaning |
| --- | --- | --- |
| `400` | `invalid_request` | The request is malformed: an unknown field, a missing or out-of-range `ttl_seconds`, a user id longer than 64 bytes, or a body that is not a JSON object. This points at your backend. |
| `401` | `unauthorized` | The `Authorization` header is missing or wrong. Returned for every route, including unknown ones. |
| `403` | `authentication_failed` | A passkey, recovery code, enrollment token or session token did not check out, or a ceremony was unknown or expired. The reason is logged by Parvus and never returned. |
| `404` | `not_found` | The account, passkey or route does not exist, or the account has no passkeys for an operation that needs one. |
| `409` | `already_exists` | The account already has passkeys, or the passkey is already registered. |
| `409` | `last_credential` | Removing this passkey would leave the account with none. |
| `409` | `credential_limit` | The account already holds `PARVUS_MAX_CREDENTIALS` passkeys. |
| `500` | `internal` | Something failed inside Parvus, usually the database. The cause is logged. |
