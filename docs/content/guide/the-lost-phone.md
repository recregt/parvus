---
title: The lost phone
weight: 6
---

Say Alice never got around to adding the laptop. The phone is gone, and with it the only passkey. This is the moment the recovery code from chapter two was written down for.

## Using the code

Recovery happens on a page your backend serves, usually a form with two fields: the username and the code. Your backend looks up the user id for that username and asks Parvus.

```
POST /v1/recover

{
  "user_id": "...",
  "code": "7KQ2-M9XD-4TPR-0HWB",
  "ttl_seconds": 3600
}
```

The code is forgiving to type. Upper or lower case, with or without the dashes and spaces, an `O` where a zero belongs or an `I` or `L` where a one belongs: Parvus reads them all the same way. People type these codes from paper, often in a hurry, and a typo of that kind should not cost anyone an account.

If the code is right, Parvus signs Alice in and answers with three things.

```json
{
  "user_id": "alice",
  "token": "Qm4x...",
  "expires_at": "2026-10-26T16:04:05Z",
  "recovery_code": "R2VN-8CXK-T1Q4-MZ7D",
  "credential_count": 1
}
```

The first is a session, which your backend handles just like one from a passkey sign-in. The second is a **new** recovery code. The old one is gone the moment it is used, and there is only ever one valid code per account, so show this one to Alice once and ask for it to be written down again. The third, `credential_count`, says how many passkeys the account still has. Here it is one: the passkey on the lost phone.

If the code is wrong, or the user does not exist, the answer is the same `authentication_failed` either way. The response does not reveal whether an account exists.

## Afterwards

Alice is in, but still has no working passkey, and the old one is out there on a phone that someone may find. Your recovery page can lead through three steps:

1. **Add a new passkey** on the device in hand, as in chapter five.
2. **Remove the lost one.** The list of the account's passkeys shows which one it is, by the name your backend stored next to its id.
3. **Sign out everywhere**, with `/v1/users/{id}/logout`, and sign back in with the new passkey. This ends every session, including any that were open on the lost phone.

At any calm moment, Alice can also ask for a fresh code without using the old one. Your backend calls `/v1/users/{id}/codes`, and the previous code stops working at once.

## Why there are no attempt limits

You may notice that Parvus never locks a code after a few wrong guesses. That is deliberate. A recovery code carries eighty bits of randomness, so guessing one online would take longer than anyone has. A lockout, on the other hand, would hand a stranger an easy way to burn Alice's only way back in, simply by typing nonsense into the form. If your public recovery form needs protection from floods, that belongs in your backend, which sees the network traffic. Parvus does not.

Codes are stored only as keyed hashes, so the database alone does not reveal them. The key is `PARVUS_PEPPER`, which never touches the database.

Recovery covers a lost device. It cannot cover a lost device and a lost piece of paper at the same time. For that day, you are the last line, and the next chapter is about what you do then.
