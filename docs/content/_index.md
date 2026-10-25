---
title: Parvus
layout: hextra-home
---

{{< hextra/hero-badge link="https://github.com/recregt/parvus" >}}
  <span>Free software, MPL-2.0</span>
  {{< icon name="arrow-circle-right" attributes="height=14" >}}
{{< /hextra/hero-badge >}}

<div class="hx:mt-6 hx:mb-6">
{{< hextra/hero-headline >}}
  Passkey sign-in&nbsp;<br class="hx:sm:block hx:hidden" />for any backend
{{< /hextra/hero-headline >}}
</div>

<div class="hx:mb-12">
{{< hextra/hero-subtitle >}}
  One small program between your app and your users' passkeys.&nbsp;<br class="hx:sm:block hx:hidden" />No passwords, no email, no text messages.
{{< /hextra/hero-subtitle >}}
</div>

<div class="hx:mb-6">
{{< hextra/hero-button text="Read the guide" link="guide" >}}
</div>

<div class="hx:mt-6"></div>

{{< hextra/feature-grid >}}
  {{< hextra/feature-card
    title="Passkeys and nothing else"
    subtitle="People sign in with the passkey their phone or laptop already holds. A single recovery code covers the day a device goes missing."
  >}}
  {{< hextra/feature-card
    title="Any language on your side"
    subtitle="Parvus speaks plain HTTP on a private socket. Your backend relays a few small requests, so any framework that can make an HTTP call can use it."
  >}}
  {{< hextra/feature-card
    title="One binary, one file"
    subtitle="A SQLite file by default and Postgres when you run several instances. Configuration is eight environment variables, with no defaults to discover later."
  >}}
  {{< hextra/feature-card
    title="Your rules, stated once"
    subtitle="Every time limit comes from your backend with each call. Parvus keeps no hidden timeouts, rates or thresholds of its own."
  >}}
  {{< hextra/feature-card
    title="Proven the hard way"
    subtitle="Every store passes the same contract, a fuzzer compares them step by step, and every change signs in through a real browser."
  >}}
  {{< hextra/feature-card
    title="Small on purpose"
    subtitle="Fourteen endpoints, frozen as version one. Parvus does sign-in and recovery, and leaves everything else to the app it serves."
  >}}
{{< /hextra/feature-grid >}}
