---
title: A first sign-in
weight: 2
---

The repository ships a small example app: one page with three buttons, and a host written twice, once in Node and once in Go. Running it takes two terminals, a recent browser, and Node 22 or newer for the Node host. Start with a clone of the repository, because the example app lives there.

```sh
git clone https://github.com/recregt/parvus
cd parvus
```

## Start Parvus

Download Parvus into the clone and start it. The command below fetches the build for Linux on amd64. For macOS or arm64, replace `linux_amd64` with `darwin_arm64`, `darwin_amd64` or `linux_arm64`.

```sh
curl -L https://github.com/recregt/parvus/releases/latest/download/parvus_linux_amd64.tar.gz | tar -xz parvus

PARVUS_SECRET=host-secret PARVUS_PEPPER=change-me \
PARVUS_RP_ID=localhost PARVUS_RP_NAME=Example \
PARVUS_ORIGINS=http://localhost:8080 \
PARVUS_DB=parvus.db PARVUS_LISTEN=127.0.0.1:9000 \
PARVUS_MAX_CREDENTIALS=20 ./parvus
```

Parvus prints one line of JSON saying it is listening, then waits. Each of those variables will come back later in the story, but a few already matter. `PARVUS_SECRET` is the password your backend will present on every call. `PARVUS_RP_ID` and `PARVUS_ORIGINS` tell Parvus which site its passkeys belong to: the browser will show `http://localhost:8080`, so that is the only origin Parvus accepts. `PARVUS_DB` names the SQLite file, which Parvus creates on first start.

## Start the host

In a second terminal, start the Node host.

```sh
cd examples/relay-node
PARVUS_URL=http://127.0.0.1:9000 PARVUS_SECRET=host-secret LISTEN=127.0.0.1:8080 node relay.mjs
```

If you would rather read Go, and have Go installed, the Go host does exactly the same thing.

```sh
cd examples/relay-go
PARVUS_URL=http://127.0.0.1:9000 PARVUS_SECRET=host-secret LISTEN=127.0.0.1:8080 go run .
```

Either way, the host now serves the example page on port 8080 and forwards its sign-in requests to Parvus.

## Register Alice

Open <http://localhost:8080>, type `alice` into the username field and press **Register**. Your browser asks you to create a passkey, using whatever your device offers: a fingerprint, a face, a PIN or a security key. Confirm it, and the page shows Parvus's answer.

```json
{
  "recovery_code": "7KQ2-M9XD-4TPR-0HWB",
  "user_id": "alice"
}
```

Your code will be different. Write it down, because it is Alice's only way back in if the passkey is ever lost, and a later chapter is going to need it.

## Sign in

Press **Sign in**. This time the browser does not ask for a username at all. It shows the passkeys it holds for this site, you pick Alice's, confirm, and the page asks the host who is signed in.

```json
{
  "user_id": "alice"
}
```

Press **Sign out**, and the same question gets a different answer.

```json
{
  "error": "not_signed_in"
}
```

That is the whole experience from Alice's side: one button to create a passkey, one to use it. Nothing was typed except a username, and nothing secret travelled through the page. The next chapter slows those two clicks down and shows what each one actually did.
