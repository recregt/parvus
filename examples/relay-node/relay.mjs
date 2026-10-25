// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

import { randomUUID } from 'node:crypto'
import { readFile } from 'node:fs/promises'
import { createServer } from 'node:http'

const parvus = connect(process.env.PARVUS_URL, process.env.PARVUS_SECRET)
const address = new URL(`http://${process.env.LISTEN}`)

const ceremonySeconds = 120
const sessionSeconds = 3600

const routes = {
  'POST /auth/registration/begin': registrationBegin,
  'POST /auth/registration/finish': registrationFinish,
  'POST /auth/login/begin': loginBegin,
  'POST /auth/login/finish': loginFinish,
  'GET /auth/me': me,
  'POST /auth/logout': logout,
}

const pages = {
  '/': 'index.html',
  '/app.js': 'app.js',
  '/parvus.js': 'parvus.js',
}

createServer(serve).listen(address.port, address.hostname)

function registrationBegin({ body, slot }) {
  return parvus('/v1/registration/begin', {
    slot,
    user_id: body.username,
    name: body.username,
    ttl_seconds: ceremonySeconds,
  })
}

function registrationFinish({ body, slot }) {
  return parvus('/v1/registration/finish', {
    slot,
    credential: body.credential,
  })
}

function loginBegin({ body, slot }) {
  return parvus('/v1/login/begin', {
    slot,
    user_id: body.username,
    ttl_seconds: ceremonySeconds,
  })
}

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

async function me({ session }) {
  const result = await parvus('/v1/introspect', { token: session })
  if (result.status !== 200) return { status: 401, body: { error: 'not_signed_in' } }

  return { status: 200, body: { user_id: result.body.user_id } }
}

async function logout({ session }) {
  await parvus('/v1/logout', { token: session })
  return { status: 204, cookies: [cookie('session', '', 0)] }
}

function connect(url, secret) {
  return async (path, payload) => {
    try {
      const response = await fetch(url + path, {
        method: 'POST',
        headers: { authorization: `Bearer ${secret}`, 'content-type': 'application/json' },
        body: JSON.stringify(payload),
      })
      const text = await response.text()
      return { status: response.status, body: text ? JSON.parse(text) : undefined }
    } catch {
      return { status: 502, body: { error: 'bad_gateway' } }
    }
  }
}

async function serve(request, response) {
  const page = pages[request.url]
  if (request.method === 'GET' && page) return sendPage(response, page)

  const route = routes[`${request.method} ${request.url}`]
  if (!route) return send(response, { status: 404, body: { error: 'not_found' } })

  const exchange = await read(request)
  const result = await route(exchange)
  const slotCookie = cookie('slot', exchange.slot, ceremonySeconds)
  send(response, { ...result, cookies: [slotCookie, ...(result.cookies ?? [])] })
}

async function read(request) {
  const cookies = readCookies(request)
  return {
    body: await readJson(request),
    slot: cookies.slot ?? randomUUID(),
    session: cookies.session ?? '',
  }
}

async function readJson(request) {
  const chunks = []
  for await (const chunk of request) chunks.push(chunk)
  try {
    return JSON.parse(Buffer.concat(chunks))
  } catch {
    return {}
  }
}

function readCookies(request) {
  const pairs = (request.headers.cookie ?? '').split('; ').filter(Boolean)
  return Object.fromEntries(
    pairs.map((pair) => {
      const [name, ...value] = pair.split('=')
      return [name, value.join('=')]
    }),
  )
}

function cookie(name, value, maxAgeSeconds) {
  return `${name}=${value}; Path=/; HttpOnly; Secure; SameSite=Strict; Max-Age=${maxAgeSeconds}`
}

function send(response, { status, body, cookies = [] }) {
  response.writeHead(status, { 'content-type': 'application/json', 'set-cookie': cookies })
  response.end(body === undefined ? undefined : JSON.stringify(body))
}

async function sendPage(response, name) {
  const type = name.endsWith('.html') ? 'text/html' : 'text/javascript'
  response.writeHead(200, { 'content-type': type })
  response.end(await readFile(new URL(`../browser/${name}`, import.meta.url)))
}
