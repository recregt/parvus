---
title: Your side of the wire
weight: 4
---

The example host is about a hundred and fifty lines, and most of that is plumbing that your web framework already does for you. Strip the plumbing away and four duties remain. Whatever language your backend is written in, these four are all it owes Parvus.

## 1. Give every browser a slot

Before a ceremony begins, make sure the browser has a slot: a random value in an `HttpOnly` cookie, kept for as long as a ceremony may take. Send the same slot to Parvus with both the begin and the finish request. The browser never needs to read it, and Parvus never needs to know it is a cookie.

## 2. Add what only you know

The browser sends a username or nothing at all. Your backend adds three things before the request reaches Parvus.

The first is the **user id**. Parvus stores whatever id you give it, up to 64 bytes, and gives it back after every sign-in. Use the id from your own users table, not the username. People change usernames, and an id that means nothing outside your database tells an outsider nothing about who has an account. The example app uses the username directly only to stay short.

The second is the **time limits**. Every call that opens something takes `ttl_seconds`. For a ceremony, that is how long a person may take to reach for a phone and touch the sensor. For a session, it is how long a sign-in should last, which is a product decision only you can make. Parvus has no defaults for either, so each limit lives in your code where you can see it.

The third is the **secret**. Every request to Parvus carries `Authorization: Bearer` followed by the value of `PARVUS_SECRET`. Parvus checks it before it reads a single byte of the body.

## 3. Pass the credential through unchanged

When the browser finishes a ceremony, it sends a credential: a block of JSON produced by the device. Forward it to Parvus exactly as you received it. Parvus checks every part of it, and a backend that tries to tidy it up can only break a signature. On the browser side, the [snippet](https://github.com/recregt/parvus/blob/main/examples/browser/parvus.js) in the examples does the whole job in a few lines.

```js
export async function login(account) {
  const options = await begin('login', account)
  const credential = await navigator.credentials.get(requestOptions(options))
  return finish('login', credential)
}
```

## 4. Keep the session token to yourself

When a sign-in finishes, Parvus gives your backend a token. Put it in your own `HttpOnly`, `Secure`, `SameSite=Strict` cookie and give the browser nothing but the user id. Script running in the page can then never read the token, even if something goes wrong elsewhere on the page. In the Node host, that is the whole of the sign-in route.

```js
async function loginFinish({ body, slot }) {
  const result = await parvus('/v1/login/finish', {
    slot,
    credential: body.credential,
    ttl_seconds: sessionSeconds,
  })
  if (result.status !== 200) return result

  const { user_id, token } = result.body
  return {
    status: 200,
    body: { user_id },
    cookies: [cookie('session', token, sessionSeconds)],
  }
}
```

## Every request after that

From now on, each request Alice makes arrives with that cookie. To find out who it belongs to, your backend sends the token to `/v1/introspect` and gets back the user id and the moment the session ends. If the answer is `authentication_failed`, treat the request as signed out: the token was wrong, expired or ended.

Sessions in Parvus are absolute by default. They end at the time your backend asked for, however busy Alice is. If you would rather keep active people signed in, add `extend_by_seconds` to the same introspect call. Parvus moves the end time forward and never backwards, so a short extension can never cut a long session short. Whether sessions slide, and by how much, stays your decision.

Signing out is one call to `/v1/logout` with the token. Signing out everywhere, on every device at once, is `/v1/users/{id}/logout`.

## When something goes wrong

Every failure comes back as a small JSON object with one of eight fixed codes, listed in the [reference]({{< relref "/reference/errors" >}}). Two matter most to the people using your app. `authentication_failed` means the passkey, code or token did not check out, and the honest message is "that did not work, try again". `already_exists` at registration means the user id is taken. The others point at your backend or your configuration, not at the person in front of the screen.

Alice can now sign in, but only on the phone that made the passkey. The next chapter brings in a laptop.
