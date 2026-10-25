// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

import { login, register } from './parvus.js'

const username = document.querySelector('#username')
const result = document.querySelector('#result')

document.querySelector('#register').addEventListener('click', () =>
  show(() => register('/auth', { username: username.value })),
)

document.querySelector('#login').addEventListener('click', () =>
  show(async () => {
    await login('/auth')
    return whoAmI()
  }),
)

document.querySelector('#logout').addEventListener('click', () =>
  show(async () => {
    await fetch('/auth/logout', { method: 'POST' })
    return whoAmI()
  }),
)

async function show(action) {
  try {
    result.textContent = JSON.stringify(await action(), null, 2)
  } catch (error) {
    result.textContent = JSON.stringify({ error: error.message }, null, 2)
  }
}

function whoAmI() {
  return fetch('/auth/me').then((response) => response.json())
}
