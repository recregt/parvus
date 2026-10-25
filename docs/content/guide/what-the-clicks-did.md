---
title: What the clicks did
weight: 3
---

Each of the two buttons in the last chapter started a ceremony, which is the WebAuthn name for a challenge and its answer. A ceremony always takes two round trips through your backend: one to begin it and one to finish it. Between the two, the browser and Alice's device do their part, and Parvus is not involved at all.

## Registering

Here is the **Register** click in full.

<svg viewBox="0 0 640 450" role="img" aria-label="Registration in two round trips. The browser asks your backend to begin, your backend asks Parvus, and the creation options travel back unchanged. The device creates a passkey and signs the challenge. The browser sends the credential to your backend, your backend forwards it to Parvus, and the user id and recovery code travel back." style="width:100%;height:auto;margin:1.5rem 0" fill="none" stroke="currentColor" stroke-width="1.5" font-size="13">
  <rect x="25" y="8" width="130" height="34" rx="8"/>
  <rect x="255" y="8" width="130" height="34" rx="8"/>
  <rect x="485" y="8" width="130" height="34" rx="8"/>
  <path d="M90 42v398M320 42v398M550 42v398" stroke-dasharray="3 5" opacity="0.4"/>
  <path d="M90 80h228M310 75l8 5-8 5"/>
  <path d="M320 120h228M540 115l8 5-8 5"/>
  <path d="M550 160h-228M330 155l-8 5 8 5" stroke-dasharray="6 4"/>
  <path d="M320 200h-228M100 195l-8 5 8 5" stroke-dasharray="6 4"/>
  <rect x="12" y="222" width="236" height="40" rx="8" stroke-width="1" opacity="0.6"/>
  <path d="M90 300h228M310 295l8 5-8 5"/>
  <path d="M320 340h228M540 335l8 5-8 5"/>
  <path d="M550 380h-228M330 375l-8 5 8 5" stroke-dasharray="6 4"/>
  <path d="M320 420h-228M100 415l-8 5 8 5" stroke-dasharray="6 4"/>
  <g fill="currentColor" stroke="none" text-anchor="middle">
    <text x="90" y="30" font-size="14">Browser</text>
    <text x="320" y="30" font-size="14">Your backend</text>
    <text x="550" y="30" font-size="14">Parvus</text>
    <text x="205" y="72">POST /auth/registration/begin</text>
    <text x="435" y="112">POST /v1/registration/begin</text>
    <text x="435" y="152">creation options</text>
    <text x="205" y="192">the same options, unchanged</text>
    <text x="130" y="246">the device creates a passkey</text>
    <text x="205" y="292">POST /auth/registration/finish</text>
    <text x="435" y="332">POST /v1/registration/finish</text>
    <text x="435" y="372">user_id, recovery_code</text>
    <text x="205" y="412">user_id, recovery_code</text>
  </g>
</svg>

When the request to begin reaches your backend, it adds the things only your backend knows. The `user_id` says which of your users this is. The `ttl_seconds` says how long the ceremony may stay open. The `slot` names this particular browser.

Parvus answers with creation options: a random challenge, the name of your site, and the signature algorithms it accepts. It also remembers the challenge under that slot until the time runs out.

The browser hands those options to the device. The device makes a new key pair, keeps the private half, and signs the challenge. The browser returns the result, which your backend forwards untouched. Parvus checks that the challenge is the one it issued and that the origin is yours. It also checks that the device confirmed it was really Alice and not just someone holding the phone. Only then does it store the public key, and in the same step it creates Alice's account and hands back a recovery code.

## The slot

The slot is how Parvus knows which challenge belongs to which browser. Your backend picks it, usually a random value in a cookie that lives only as long as a ceremony. Parvus keeps exactly one open ceremony per slot. If Alice presses **Register** twice, the second challenge replaces the first, so a browser can never pile up challenges, and an abandoned ceremony disappears on its own when its time runs out.

A challenge can be finished only once. Replaying a captured answer gets nowhere, because the challenge it signed is already gone.

## Signing in

The **Sign in** click has the same shape: begin, the device, finish. The begin request carries a slot and a time limit but no `user_id`. Parvus answers with a challenge and no list of passkeys, so the browser offers every passkey it holds for this site. Each passkey carries the `user_id` it was made for, which is how Parvus learns who signed in.

If your sign-in form does ask for a username, your backend can send that user's `user_id` with the begin request. Parvus then lists that user's passkeys for the browser. This helps with older security keys that cannot be discovered without a hint.

When the signed challenge comes back, Parvus verifies it against the stored public key. It also checks the device's signature counter, a number that should move forward with every use. Passkeys that sync between devices always report zero, and that is fine. A counter that does report a number but fails to move forward suggests a cloned authenticator, and Parvus refuses that sign-in. If all is well, it returns a session.

```json
{
  "user_id": "alice",
  "token": "Nw3c...",
  "expires_at": "2026-10-26T15:04:05Z"
}
```

The session lasts exactly as long as your backend asked for in `ttl_seconds`. Your backend now holds a token that proves Alice signed in. Everything that follows, every page Alice opens, depends on what your backend does with it. That is the next chapter.
