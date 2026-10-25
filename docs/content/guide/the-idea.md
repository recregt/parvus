---
title: The idea
weight: 1
---

Your notes app needs accounts, and accounts usually arrive with a long tail. Passwords need hashing, then a reset flow, and the reset flow needs email. Email needs a provider, a sending domain, bounce handling and a support inbox for the messages that never arrive. Somewhere along the way someone suggests text messages as a second factor, and the bill grows again.

Passkeys cut that tail off at the root. Instead of a secret that Alice remembers and types, Alice's phone or laptop holds a private key that never leaves the device. When Alice signs in, the browser asks the device to sign a fresh challenge, and Alice confirms with a fingerprint, a face or the device's own PIN. Nothing Alice types can be phished, because the browser refuses to use a passkey on any site other than the one it was made for.

What remains is the careful part in the middle. Someone has to create those challenges and remember them for a moment, verify the signatures that come back, store each passkey's public key, notice when a device looks cloned, keep sessions, and decide what happens when Alice loses the only device that holds the key. That middle part is what Parvus does, and it is all that Parvus does.

## Three parties

There are three programs in the story, and only two of them ever talk to each other at a time.

<svg viewBox="0 0 640 96" role="img" aria-label="Alice's browser talks to your backend through your routes, and your backend talks to Parvus over a private socket." style="width:100%;height:auto;margin:1.5rem 0" fill="none" stroke="currentColor" stroke-width="1.5" font-size="15">
  <rect x="10" y="30" width="150" height="50" rx="10"/>
  <rect x="245" y="30" width="150" height="50" rx="10"/>
  <rect x="480" y="30" width="150" height="50" rx="10"/>
  <path d="M166 55h73M173 50l-7 5 7 5M232 50l7 5-7 5"/>
  <path d="M401 55h73M408 50l-7 5 7 5M467 50l7 5-7 5"/>
  <g fill="currentColor" stroke="none" text-anchor="middle">
    <text x="85" y="60">Alice's browser</text>
    <text x="320" y="60">Your backend</text>
    <text x="555" y="60">Parvus</text>
    <text x="202" y="20" font-size="13" opacity="0.7">your routes</text>
    <text x="437" y="20" font-size="13" opacity="0.7">private socket</text>
  </g>
</svg>

Alice's browser talks to your backend, exactly as it does today. Your backend, which the rest of this guide calls the host, talks to Parvus over a private socket that nothing else can reach. The browser never talks to Parvus, and Parvus never sees a cookie, a page or an email address.

This split decides who is responsible for what. Parvus checks passkeys, issues sessions and keeps recovery codes. Your backend decides who may sign up, which user is which, what a session lets someone do and how long it should last. Parvus does not send messages, keep profiles or hold opinions about your users. It answers questions about passkeys, and it answers them the same way every time.

## One binary

Parvus is a single program with no runtime of its own to install. It keeps its data in one SQLite file, or in Postgres once you run more than one copy. You configure it with eight environment variables, none of which have defaults, so nothing about its behavior is hidden in a value you never chose.

The fastest way to understand all of this is to watch it happen, so the next chapter runs Parvus next to an example app and signs Alice in.
