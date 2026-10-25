// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

export async function register(account) {
  const options = await begin('registration', account)
  const credential = await navigator.credentials.create(creationOptions(options))
  return finish('registration', credential)
}

export async function login(account) {
  const options = await begin('login', account)
  const credential = await navigator.credentials.get(requestOptions(options))
  return finish('login', credential)
}

function begin(ceremony, account = {}) {
  return post(`/auth/${ceremony}/begin`, account)
}

function finish(ceremony, credential) {
  return post(`/auth/${ceremony}/finish`, { credential })
}

function creationOptions({ publicKey }) {
  return { publicKey: PublicKeyCredential.parseCreationOptionsFromJSON(publicKey) }
}

function requestOptions({ publicKey }) {
  return { publicKey: PublicKeyCredential.parseRequestOptionsFromJSON(publicKey) }
}

async function post(path, body) {
  const response = await fetch(path, {
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify(body),
  })
  const reply = await response.json()
  if (!response.ok) throw new Error(reply.error)
  return reply
}
