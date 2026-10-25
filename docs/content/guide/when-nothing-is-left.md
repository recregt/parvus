---
title: When nothing is left
weight: 7
---

The phone is gone and so is the paper with the code. Alice writes to your support address and asks for the account back. Parvus cannot judge whether the person writing is really Alice, and it does not try. That check is yours: a video call, an ID document, a question only Alice could answer, whatever your app's stakes justify. Once you are satisfied, Parvus does the rest.

## Resetting the account

From your admin tooling, reset the account.

```
POST /v1/users/{id}/reset

{
  "ttl_seconds": 86400
}
```

A reset removes everything that could still sign in as Alice: every passkey, the recovery code and every open session. It keeps the account itself, so the user id stays the same, and everything in your own database that points at it stays valid. In return, Parvus hands you an enrollment token.

```json
{
  "enroll_token": "pB0f...",
  "expires_at": "2026-10-27T15:04:05Z"
}
```

## Handing the account back

Send the token to Alice by whatever channel your check just verified. Alice opens your registration page, and your backend includes the token in the begin request.

```
POST /v1/registration/begin

{
  "slot": "...",
  "user_id": "...",
  "enroll_token": "pB0f...",
  "ttl_seconds": 120
}
```

Without the token, or with a wrong one, the request fails with `authentication_failed`. With it, Alice creates a new passkey exactly as on the first day, and gets a new recovery code at the end.

The token works once. It also stops working when its time runs out, and resetting the account again replaces it with a new one. While a reset account waits for its owner, nobody can sign in to it at all, not even with an old passkey that turns up later.

## Saying goodbye

Some accounts end for good. When you delete a person from your own records, tell Parvus too.

```
DELETE /v1/users/{id}
```

That removes the account, its passkeys, its code and its sessions in one step. Use a reset when the person stays and only the keys change. Use a delete when the person leaves.

That covers the whole life of an account, from the first passkey to the last goodbye. What remains is running Parvus somewhere real, which is the next chapter.
