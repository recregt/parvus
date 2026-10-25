// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

import { spawn } from 'node:child_process'
import { mkdtemp } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { createInterface } from 'node:readline'

const recoveryCode = /^[0-9A-HJKMNP-TV-Z]{4}(-[0-9A-HJKMNP-TV-Z]{4}){3}$/

const steps = [
  {
    name: 'registering a new user',
    act: (page) => page.click('register', 'alice'),
    expect: { user_id: 'alice', recovery_code: recoveryCode },
  },
  {
    name: 'signing in without a username',
    act: (page) => page.click('login'),
    expect: { user_id: 'alice' },
  },
  {
    name: 'signing out',
    act: (page) => page.click('logout'),
    expect: { error: 'not_signed_in' },
  },
  {
    name: 'registering the same user again',
    act: (page) => page.click('register', 'alice'),
    expect: { error: 'already_exists' },
  },
  {
    name: 'signing in with a username',
    act: (page) => page.login({ username: 'alice' }),
    expect: { user_id: 'alice' },
  },
]

const chromium = await launchChromium()
try {
  await Promise.race([check(steps), chromium.exited])
} catch (error) {
  console.error(`${error.message}\n${chromium.log.join('\n')}`)
  process.exitCode = 1
} finally {
  chromium.process.kill()
}

async function check(steps) {
  const page = await openPage(await connect(chromium.endpoint), process.env.APP_URL)
  for (const step of steps) report(step, await step.act(page))
}

function report({ name, expect }, actual) {
  if (matches(actual, expect)) {
    console.log(`ok - ${name}`)
  } else {
    console.log(`not ok - ${name}: got ${JSON.stringify(actual)}`)
    process.exitCode = 1
  }
}

async function openPage({ send, nextEvent }, url) {
  const { targetId } = await send('Target.createTarget', { url: 'about:blank' })
  const { sessionId } = await send('Target.attachToTarget', { targetId, flatten: true })
  const inPage = (method, params) => send(method, params, sessionId)

  await inPage('WebAuthn.enable')
  await inPage('WebAuthn.addVirtualAuthenticator', {
    options: {
      protocol: 'ctap2',
      transport: 'internal',
      hasResidentKey: true,
      hasUserVerification: true,
      isUserVerified: true,
      automaticPresenceSimulation: true,
    },
  })
  await inPage('Page.enable')
  const loaded = nextEvent('Page.loadEventFired')
  await inPage('Page.navigate', { url })
  await loaded

  const evaluate = async (expression) => {
    const { result, exceptionDetails } = await inPage('Runtime.evaluate', {
      expression,
      awaitPromise: true,
      returnByValue: true,
    })
    if (exceptionDetails) throw new Error(exceptionDetails.exception?.description ?? exceptionDetails.text)
    return result.value
  }

  return {
    click: (button, username = '') =>
      evaluate(`(${clickAndRead})(${JSON.stringify(button)}, ${JSON.stringify(username)})`),
    login: (account) =>
      evaluate(`import('/parvus.js')
        .then(({ login }) => login(${JSON.stringify(account)}))
        .catch((error) => ({ error: error.message }))`),
  }
}

function clickAndRead(button, username) {
  return new Promise((resolve) => {
    const output = document.getElementById('output')
    new MutationObserver(() => resolve(JSON.parse(output.textContent))).observe(output, {
      childList: true,
      characterData: true,
      subtree: true,
    })
    document.getElementById('username').value = username
    document.getElementById(button).click()
  })
}

async function launchChromium() {
  const profile = await mkdtemp(join(tmpdir(), 'parvus-e2e-'))
  const child = spawn('chromium', [
    '--headless=new',
    '--no-sandbox',
    '--no-first-run',
    '--remote-debugging-port=0',
    `--user-data-dir=${profile}`,
  ])
  const log = []
  const exited = failsWhenExited(child)
  const endpoint = await Promise.race([devToolsEndpoint(child, log), exited])
  return { process: child, endpoint, log, exited }
}

function devToolsEndpoint(child, log) {
  return new Promise((resolve) => {
    createInterface({ input: child.stderr }).on('line', (line) => {
      log.push(line)
      const endpoint = line.match(/DevTools listening on (ws:\S+)/)?.[1]
      if (endpoint) resolve(endpoint)
    })
  })
}

function failsWhenExited(child) {
  const failure = new Promise((_, reject) =>
    child.on('exit', (code, signal) => reject(new Error(`Chromium exited (${code ?? signal})`))),
  )
  failure.catch(() => {})
  return failure
}

async function connect(endpoint) {
  const socket = new WebSocket(endpoint)
  const replies = new Map()
  const events = new Map()
  let lastId = 0

  socket.addEventListener('message', ({ data }) => {
    const message = JSON.parse(data)
    const key = message.id ?? message.method
    const waiters = message.id ? replies : events
    const waiter = waiters.get(key)
    if (!waiter) return
    waiters.delete(key)
    if (message.error) waiter.reject(new Error(message.error.message))
    else waiter.resolve(message.result ?? message.params)
  })

  await new Promise((resolve, reject) => {
    socket.addEventListener('open', resolve, { once: true })
    socket.addEventListener('error', () => reject(new Error(`cannot reach Chromium at ${endpoint}`)), { once: true })
  })

  return {
    send: (method, params = {}, sessionId) =>
      new Promise((resolve, reject) => {
        lastId++
        replies.set(lastId, { resolve, reject })
        socket.send(JSON.stringify({ id: lastId, method, params, sessionId }))
      }),
    nextEvent: (method) => new Promise((resolve, reject) => events.set(method, { resolve, reject })),
  }
}

function matches(actual, expected) {
  if (expected instanceof RegExp) return typeof actual === 'string' && expected.test(actual)
  if (typeof expected !== 'object' || expected === null) return actual === expected
  if (typeof actual !== 'object' || actual === null) return false

  const keys = Object.keys(expected)
  return Object.keys(actual).length === keys.length && keys.every((key) => matches(actual[key], expected[key]))
}
