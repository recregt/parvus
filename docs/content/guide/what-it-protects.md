---
title: What it protects
weight: 9
---

Over eight chapters, Alice signed up, signed in, added a laptop, lost a phone, lost everything, and got the account back each time. This last chapter looks at the same story from the other side: the people who would like to be Alice, and what stands in their way.

## Phishing

A fake copy of your site gets nowhere. The browser ties every passkey to the domain it was made for and refuses to use it anywhere else. Parvus then checks the origin inside the signed answer against `PARVUS_ORIGINS`. Without a password to type, there is nothing for a convincing page to collect.

## A stolen phone

A thief holding Alice's unlocked phone still cannot sign in. Parvus requires every passkey use to be confirmed by the device itself, with a fingerprint, a face or the device's own PIN, and refuses any signature without that confirmation. Once Alice reports the loss, the steps in chapter six remove the phone's passkey and end every session it had.

## A copied passkey

Some security keys count how often they have been used, and each signature carries that count. If a key's signature arrives with a count that did not move forward, someone may have copied it, and Parvus refuses the sign-in. Passkeys that sync between devices do not count and always report zero, which Parvus accepts.

Treat this as a tripwire, not a wall. Two sign-ins with the same key that arrive at the same moment are both checked against the count stored before either of them, so both can pass. A copy used at exactly the same time as the original can slip through once, and the next sign-in that falls behind the count is refused.

## A captured request

Every challenge is random, belongs to one slot and can be answered only once. A recorded answer replayed later finds its challenge already used. An answer that arrives after its time limit finds the challenge expired.

## A guessed recovery code

A recovery code holds eighty bits of randomness. At a million guesses a second, trying them all would take tens of billions of years. That is why Parvus needs no attempt counter, and why it refuses to keep one: a counter would let a stranger lock Alice out on purpose.

## A copy of the database

Suppose someone walks away with the database. Session tokens and enrollment tokens are stored only as hashes, so the copy holds nothing that signs anyone in. Passkeys are public keys and useless without the devices that hold the private halves. Recovery codes are stored as hashes keyed with `PARVUS_PEPPER`, which lives outside the database, so the copy cannot even be used to test guesses. Only a thief who also has the pepper can test guesses, and then still has to search eighty bits per account.

## Finding out who has an account

Parvus never tells a caller whether an account exists. A wrong recovery code and an unknown user get the same answer. A sign-in that names an unknown user still receives a passkey list, made up for that name so that it looks the same every time it is asked. The made-up list cannot be fully identical to a real one, though, since it always lists one passkey of the same length. The real protection is the advice from chapter four: use opaque user ids, and let your backend decide what its own forms reveal.

## What Parvus leaves to you

Some things Parvus cannot protect, because they happen where it cannot see.

**Your backend.** Anything that holds `PARVUS_SECRET` can do what your backend does, including resetting any account. Guard that secret and the machine it lives on as carefully as your database.

**Your pages.** A script injected into your site can act as Alice for as long as Alice's page is open. The `HttpOnly` session cookie keeps it from carrying the token away, but it cannot stop the script from acting inside the page. That is the job of the usual web defenses, such as a content security policy.

**Your support desk.** A reset hands an account to whoever convinced you they were its owner. Parvus makes the reset itself safe. Whether it goes to the right person is decided by your check.

**Floods and slow clients.** Parvus has no rate limits and no time limits on connections, because it expects to be reached only by your backend. Put those defenses in front of your public routes, where you can see who is asking.

## Looking further ahead

The hashes and keyed hashes in Parvus hold up even against the quantum computers people expect in the future. The signatures inside passkeys do not: the algorithms in use today would fall to a large enough quantum computer, and browsers and authenticators do not yet support replacements. Parvus stores each passkey in a form that leaves room for new algorithms, so it can follow when the ecosystem moves.

That is the whole story. Your notes app has accounts without passwords, and Alice has a way back in after every kind of loss. You know which guarantees come from Parvus and which ones are yours. From here, the [reference]({{< relref "/reference" >}}) has every route, setting and error code in one place.
