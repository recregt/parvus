// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

export async function register(base, details) {
  const { publicKey } = await post(`${base}/registration/begin`, details)
  const credential = await navigator.credentials.create({
    publicKey: PublicKeyCredential.parseCreationOptionsFromJSON(publicKey),
  })
  return post(`${base}/registration/finish`, { credential: credential.toJSON() })
}

export async function login(base, details) {
  const { publicKey } = await post(`${base}/login/begin`, details)
  const credential = await navigator.credentials.get({
    publicKey: PublicKeyCredential.parseRequestOptionsFromJSON(publicKey),
  })
  return post(`${base}/login/finish`, { credential: credential.toJSON() })
}

async function post(url, body = {}) {
  const response = await fetch(url, {
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify(body),
  })
  const payload = response.status === 204 ? null : await response.json()
  if (!response.ok) throw new Error(payload.error)
  return payload
}
