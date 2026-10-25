// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

import { randomUUID } from 'node:crypto'
import { readFile } from 'node:fs/promises'
import { createServer } from 'node:http'

const parvusUrl = process.env.PARVUS_URL
const parvusSecret = process.env.PARVUS_SECRET
const listenOn = new URL(`http://${process.env.LISTEN}`)

const ceremonyTtlSeconds = 120
const sessionTtlSeconds = 3600

const badGateway = { status: 502, body: { error: 'bad_gateway' } }

const routes = {
  'POST /auth/registration/begin': registrationBegin,
  'POST /auth/registration/finish': registrationFinish,
  'POST /auth/login/begin': loginBegin,
  'POST /auth/login/finish': loginFinish,
  'GET /auth/me': me,
  'POST /auth/logout': logout,
}

const browserFiles = {
  '/': ['index.html', 'text/html'],
  '/app.js': ['app.js', 'text/javascript'],
  '/parvus.js': ['parvus.js', 'text/javascript'],
}

function registrationBegin({ body, slot }) {
  return callParvus('/v1/registration/begin', {
    slot,
    user_id: body.username,
    name: body.username,
    ttl_seconds: ceremonyTtlSeconds,
  })
}

function registrationFinish({ body, slot }) {
  return callParvus('/v1/registration/finish', { slot, credential: body.credential })
}

function loginBegin({ slot }) {
  return callParvus('/v1/login/begin', { slot, ttl_seconds: ceremonyTtlSeconds })
}

async function loginFinish({ body, slot }) {
  const result = await callParvus('/v1/login/finish', {
    slot,
    credential: body.credential,
    ttl_seconds: sessionTtlSeconds,
  })
  if (result.status !== 200) return result

  const { user_id, token } = result.body
  return {
    status: 200,
    body: { user_id },
    cookies: [cookie('session', token, sessionTtlSeconds)],
  }
}

async function me({ sessionToken }) {
  const result = await callParvus('/v1/introspect', { token: sessionToken })
  if (result.status !== 200) return { status: 401, body: { error: 'not_signed_in' } }

  return { status: 200, body: { user_id: result.body.user_id } }
}

async function logout({ sessionToken }) {
  await callParvus('/v1/logout', { token: sessionToken })
  return { status: 204, cookies: [cookie('session', '', 0)] }
}

async function callParvus(path, payload) {
  try {
    const response = await fetch(parvusUrl + path, {
      method: 'POST',
      headers: {
        authorization: `Bearer ${parvusSecret}`,
        'content-type': 'application/json',
      },
      body: JSON.stringify(payload),
    })
    const text = await response.text()
    return { status: response.status, body: text === '' ? undefined : JSON.parse(text) }
  } catch {
    return badGateway
  }
}

async function serve(request, response) {
  const file = browserFiles[request.url]
  if (request.method === 'GET' && file) return sendFile(response, ...file)

  const handler = routes[`${request.method} ${request.url}`]
  if (!handler) return send(response, { status: 404, body: { error: 'not_found' } })

  const exchange = await describe(request)
  const result = await handler(exchange)
  send(response, {
    ...result,
    cookies: [cookie('slot', exchange.slot, ceremonyTtlSeconds), ...(result.cookies ?? [])],
  })
}

async function describe(request) {
  const cookies = readCookies(request)
  return {
    body: await readJson(request),
    slot: cookies.slot ?? randomUUID(),
    sessionToken: cookies.session ?? '',
  }
}

async function readJson(request) {
  const chunks = []
  for await (const chunk of request) chunks.push(chunk)
  try {
    return JSON.parse(Buffer.concat(chunks).toString())
  } catch {
    return {}
  }
}

function readCookies(request) {
  const cookies = {}
  for (const pair of (request.headers.cookie ?? '').split('; ').filter(Boolean)) {
    const separator = pair.indexOf('=')
    cookies[pair.slice(0, separator)] = pair.slice(separator + 1)
  }
  return cookies
}

function cookie(name, value, maxAgeSeconds) {
  return `${name}=${value}; Path=/; HttpOnly; Secure; SameSite=Strict; Max-Age=${maxAgeSeconds}`
}

function send(response, { status, body, cookies = [] }) {
  response.writeHead(status, { 'content-type': 'application/json', 'set-cookie': cookies })
  response.end(body === undefined ? undefined : JSON.stringify(body))
}

async function sendFile(response, name, type) {
  response.writeHead(200, { 'content-type': type })
  response.end(await readFile(new URL(`../browser/${name}`, import.meta.url)))
}

createServer(serve).listen({ host: listenOn.hostname, port: listenOn.port })
