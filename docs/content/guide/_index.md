---
title: The guide
weight: 1
next: /guide/the-idea
cascade:
  type: docs
---

This guide is one story, told from start to finish. You are building a small notes app, and Alice is one of its first users. Over nine short chapters you will give Alice a way to sign in, watch what happens on the wire when Alice does, write your own side of it, and then follow Alice through the harder days: a second laptop, a lost phone, and the morning when nothing Alice owns can sign in any more. The story ends with Parvus running in production and a clear picture of what it protects.

Read it in order the first time. Each chapter starts where the previous one stopped, and the reference section is there for later, when you know what you are looking for.

{{< cards >}}
  {{< card link="the-idea" title="1. The idea" subtitle="What sign-in looks like without passwords, and the part Parvus takes off your hands." >}}
  {{< card link="first-sign-in" title="2. A first sign-in" subtitle="Run Parvus and an example app, and register Alice in your browser." >}}
  {{< card link="what-the-clicks-did" title="3. What the clicks did" subtitle="The two round trips behind every registration and sign-in." >}}
  {{< card link="your-side-of-the-wire" title="4. Your side of the wire" subtitle="The four things your backend does, in any language." >}}
  {{< card link="a-second-device" title="5. A second device" subtitle="Alice adds a laptop, and the rule that stops an account from being locked by its own owner." >}}
  {{< card link="the-lost-phone" title="6. The lost phone" subtitle="One recovery code, and what Alice does after getting back in." >}}
  {{< card link="when-nothing-is-left" title="7. When nothing is left" subtitle="Resetting an account and handing it back to its owner." >}}
  {{< card link="going-to-production" title="8. Going to production" subtitle="Secrets, sockets, SQLite or Postgres, and a service that restarts cleanly." >}}
  {{< card link="what-it-protects" title="9. What it protects" subtitle="The threats Parvus answers, and the ones it leaves to you." >}}
{{< /cards >}}
