---
title: A second device
weight: 5
---

Alice buys a laptop. Depending on how Alice's phone and laptop are set up, the story can go two ways.

If both devices share a passkey manager, such as the same platform account or the same password manager, the passkey may already be on the laptop. The browser offers it on the sign-in screen and there is nothing to add.

If not, the browser can still sign in with the phone. It shows a code on the laptop's screen, Alice scans it with the phone, confirms on the phone, and the laptop is signed in. That works every time, but it is a detour, so once Alice is in it is worth giving the laptop a passkey of its own.

## Adding a passkey

Adding a passkey looks almost exactly like registering, with one difference. Alice is already signed in, so your backend knows who this is and says so in the path.

```
POST /v1/users/{id}/credentials/begin

{
  "ttl_seconds": 120
}
```

```
POST /v1/users/{id}/credentials/finish

{
  "credential": { ... }
}
```

There is no slot this time. A signed-in user has only one place to add a passkey, so Parvus keys the open ceremony by the user id. The browser side is the same `register` call as before. A tidy way to set this up is to let your registration route look at the session: no session means a new account, a session means a new passkey for that account.

The creation options that Parvus returns list the passkeys Alice already has. The browser uses that list to refuse a second passkey on a device that already holds one, so Alice cannot end up with duplicates by pressing the button twice.

Each account can hold up to `PARVUS_MAX_CREDENTIALS` passkeys. That number is yours to choose when you configure Parvus. Once an account reaches it, adding another answers `credential_limit` until Alice removes one.

## Keeping track of passkeys

Parvus stores passkeys but does not name them. If you want to show Alice a list such as "phone" and "laptop", note each passkey's id when it is created. The credential your backend forwards at the end of the ceremony carries it in its `id` field. Store that id with a name Alice chooses, and keep forwarding the credential itself unchanged.

## Removing a passkey

Removing one takes a single call with that id.

```
DELETE /v1/users/{id}/credentials/{credential-id}
```

There is one rule here, and Parvus enforces it even when two requests arrive at the same moment: an account never loses its last passkey this way. Removing the only remaining passkey answers `last_credential`. However confused a page or a person gets, no account can be locked by its own owner. The only road to an account with no passkeys at all goes through you, and chapter seven walks down it.

Alice now has two devices, and losing one is no longer a crisis. Losing the only one still would be, and one rainy evening that is exactly what happens.
