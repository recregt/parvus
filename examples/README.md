<!--
This Source Code Form is subject to the terms of the Mozilla Public
License, v. 2.0. If a copy of the MPL was not distributed with this
file, You can obtain one at https://mozilla.org/MPL/2.0/.
-->

# Examples

Parvus never talks to a browser. Your backend, called the host here, stands between the two. These examples show both sides of that arrangement.

| Path | Role |
| --- | --- |
| `browser/parvus.js` | The browser side. `register` and `login`, talking only to the host. |
| `browser/index.html`, `browser/app.js` | A small page that uses it. |
| `relay-node/relay.mjs` | The host side in Node. |
| `relay-go/main.go` | The same host side in Go. |

The two relays have the same structure and the same names, so reading one lets you recognise the other. Each serves the page and the six routes it calls.

## What a host does

1. **Gives every browser a slot.** A random value in an `HttpOnly` cookie. Parvus keeps one pending ceremony per slot.
2. **Adds what only the host knows.** The user id, the time limits (`ttl_seconds`) and the shared secret.
3. **Passes the browser's credential through unchanged.** Parvus does the validation.
4. **Keeps the session token to itself.** Login returns a token. The host stores it in its own `HttpOnly` cookie and the browser never sees it.

The examples use the username as the user id to stay short. A real host keeps its own user records and passes that user's id.

## Run it

You need a recent browser, plus Node 22 or newer for the Node relay or Go for the Go relay. From the repository root, download Parvus and start it. For macOS or arm64, replace `linux_amd64` with `darwin_arm64`, `darwin_amd64` or `linux_arm64`.

```sh
curl -L https://github.com/recregt/parvus/releases/latest/download/parvus_linux_amd64.tar.gz | tar -xz parvus

PARVUS_SECRET=host-secret PARVUS_PEPPER=change-me \
PARVUS_RP_ID=localhost PARVUS_RP_NAME=Example \
PARVUS_ORIGINS=http://localhost:8080 \
PARVUS_DB=parvus.db PARVUS_LISTEN=127.0.0.1:9000 \
PARVUS_MAX_CREDENTIALS=20 ./parvus
```

In a second terminal, start one of the relays.

```sh
cd examples/relay-node
PARVUS_URL=http://127.0.0.1:9000 PARVUS_SECRET=host-secret LISTEN=127.0.0.1:8080 node relay.mjs
```

```sh
cd examples/relay-go
PARVUS_URL=http://127.0.0.1:9000 PARVUS_SECRET=host-secret LISTEN=127.0.0.1:8080 go run .
```

Open <http://localhost:8080>, register a username, then sign in.

The origin the browser shows, `http://localhost:8080`, must match `PARVUS_ORIGINS`, and the relying party id must match the host name, here `localhost`.

`nix flake check` runs this page in headless Chromium with a virtual authenticator, against both relays (`e2e/browser/`).
