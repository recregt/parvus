---
title: Configuration
weight: 2
---

Parvus reads eight environment variables. All of them are required, none of them has a default, and Parvus refuses to start until every one is set. It reports every missing or invalid variable at once.

| Variable | Meaning |
| --- | --- |
| `PARVUS_SECRET` | The secret your backend presents as `Authorization: Bearer`. |
| `PARVUS_PEPPER` | The key for recovery codes. Keep it outside the database and back it up separately. |
| `PARVUS_RP_ID` | Your site's domain, which passkeys are bound to. |
| `PARVUS_RP_NAME` | Your site's name, shown by devices when they create a passkey. |
| `PARVUS_ORIGINS` | The origins your users' browsers show, separated by commas, for example `https://notes.example.com`. |
| `PARVUS_DB` | A SQLite file path, or a `postgres://` or `postgresql://` address. |
| `PARVUS_LISTEN` | `host:port` for TCP, or `unix:/path` for a unix socket that Parvus's own user and group can open. |
| `PARVUS_MAX_CREDENTIALS` | The most passkeys one account may hold, a whole number of at least one. |

## Reading a setting from a file

Every variable can be given as a file instead. Set `PARVUS_PEPPER_FILE=/path/to/file` in place of `PARVUS_PEPPER`, and Parvus reads the value from the file, ignoring trailing newlines. Setting both `X` and `X_FILE` for the same variable is an error.

## Values that are not settings

Some limits come with each call instead of from configuration, so that your backend decides them where it uses them: the length of every ceremony, session and enrollment token, given as `ttl_seconds`.
