// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

import { login, register } from './parvus.js'

const username = document.getElementById('username')
const output = document.getElementById('output')

onClick('register', () => register({ username: username.value }))
onClick('login', () => login().then(currentUser))
onClick('logout', () => logout().then(currentUser))

function onClick(id, action) {
  document.getElementById(id).addEventListener('click', () =>
    action().then(show, (error) => show({ error: error.message })),
  )
}

function show(value) {
  output.textContent = JSON.stringify(value, null, 2)
}

function currentUser() {
  return fetch('/auth/me').then((response) => response.json())
}

function logout() {
  return fetch('/auth/logout', { method: 'POST' })
}
