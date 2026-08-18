/** The job-polling state machine, and what the client does with a refusal.
 *
 * `followJob` is the only genuine state machine in the frontend -- queued ->
 * running -> done/error, with a cancel path -- and it was entirely untested.
 * Everything the simulator screen shows depends on it terminating correctly:
 * a poll that never stops burns a request every 400ms forever, and a poll that
 * resolves early shows a result that does not exist yet.
 *
 * Timers are faked, so a test that would take seconds of real polling takes
 * none, and the interval is asserted rather than waited out.
 *
 * The second half of the file is the 401 interceptor, which is the frontend's
 * version of the argument `api/auth.py` makes for a middleware: one place, so
 * no screen can forget. Its two carve-outs are the interesting part -- a 401
 * from `login` is an answer about a password, not a session that ended.
 */

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ApiError, api, followJob, onSessionLost, type Job } from './api'

function job(overrides: Partial<Job> = {}): Job {
  return {
    id: 'j1',
    kind: 'sim.mana',
    status: 'queued',
    done: 0,
    total: 100,
    percent: 0,
    label: 'gyome-food: mana, 20,000 games',
    result: null,
    error: null,
    created_at: '2026-08-10T00:00:00Z',
    ...overrides,
  }
}

/** Queue up the responses `/api/jobs/{id}` will give, in order. */
function respondWith(...jobs: Job[]) {
  const fetchMock = vi.fn()
  for (const j of jobs) {
    fetchMock.mockResolvedValueOnce({
      ok: true,
      status: 200,
      json: async () => j,
    })
  }
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
}

/** Let pending promises settle, then fire the next poll's timer. */
async function nextPoll() {
  await vi.advanceTimersByTimeAsync(400)
}

beforeEach(() => {
  vi.useFakeTimers()
})

afterEach(() => {
  vi.useRealTimers()
  vi.unstubAllGlobals()
})

describe('followJob', () => {
  it('resolves with the finished job once the status reaches done', async () => {
    respondWith(job({ status: 'done', percent: 100, result: { games: 300 } }))

    const { promise } = followJob('j1', () => {})
    await expect(promise).resolves.toMatchObject({ status: 'done' })
  })

  it('resolves without a request when the submitted job is already done', async () => {
    // A cache hit: the server memoises Tier 1 results, so the POST that starts
    // a simulation can come back finished. Polling for it would be a wasted
    // round trip and a frame of "Running…" for something that is not running.
    const fetchMock = respondWith(job({ status: 'done', percent: 100 }))
    const finished = job({
      status: 'done', percent: 100, done: 1, total: 1,
      result: { games: 300, cached: true, computed_at: '2026-08-12T09:00:00Z' },
    })

    const seen: Job[] = []
    const { promise } = followJob('j1', (j) => seen.push(j), undefined, finished)

    await expect(promise).resolves.toBe(finished)
    expect(fetchMock).not.toHaveBeenCalled()
    // The screen still gets its one update, so the result renders.
    expect(seen).toEqual([finished])
  })

  it('still polls when the submitted job has work left to do', async () => {
    const fetchMock = respondWith(job({ status: 'done', percent: 100 }))
    const { promise } = followJob('j1', () => {}, undefined, job({ status: 'queued' }))

    await expect(promise).resolves.toMatchObject({ status: 'done' })
    expect(fetchMock).toHaveBeenCalledTimes(1)
  })

  it('polls through queued and running before resolving', async () => {
    const fetchMock = respondWith(
      job({ status: 'queued' }),
      job({ status: 'running', percent: 40 }),
      job({ status: 'done', percent: 100 }),
    )

    const seen: string[] = []
    const { promise } = followJob('j1', (j) => seen.push(`${j.status}:${j.percent}`))

    await nextPoll()
    await nextPoll()
    const finished = await promise

    expect(seen).toEqual(['queued:0', 'running:40', 'done:100'])
    expect(finished.percent).toBe(100)
    expect(fetchMock).toHaveBeenCalledTimes(3)
    expect(fetchMock).toHaveBeenCalledWith('/api/jobs/j1')
  })

  it('reports progress on every tick, not only at the end', async () => {
    respondWith(
      job({ status: 'running', percent: 25 }),
      job({ status: 'running', percent: 75 }),
      job({ status: 'done', percent: 100 }),
    )

    const onTick = vi.fn()
    const { promise } = followJob('j1', onTick)
    await nextPoll()
    await nextPoll()
    await promise

    expect(onTick).toHaveBeenCalledTimes(3)
  })

  it('rejects with the job error when the job fails', async () => {
    respondWith(job({
      status: 'error',
      error: 'simulation needs the card pool -- run `mtglab data refresh`',
    }))

    const { promise } = followJob('j1', () => {})
    await expect(promise).rejects.toThrow(/needs the card pool/)
  })

  it('rejects with a fallback message when a failed job carries no error text', async () => {
    respondWith(job({ status: 'error', error: null }))

    const { promise } = followJob('j1', () => {})
    await expect(promise).rejects.toThrow('job failed')
  })

  /* A failing poll is not a failing job. The theme proposal runs for around
     226 seconds, so a run spans a hundred-odd bare GETs on whatever network
     the person happens to be on — and one dropped request used to end it,
     while the server carried on and finished work nobody was listening for.
     These three pin the shape of the tolerance rather than its exact size. */
  it('rides out a dropped poll and keeps watching', async () => {
    const fetchMock = vi.fn()
      .mockRejectedValueOnce(new TypeError('network down'))
      .mockResolvedValue({ ok: true, status: 200,
                           json: async () => job({ status: 'done', percent: 100 }) })
    vi.stubGlobal('fetch', fetchMock)

    const { promise } = followJob('j1', () => {})
    await nextPoll()

    await expect(promise).resolves.toMatchObject({ status: 'done' })
    expect(fetchMock).toHaveBeenCalledTimes(2)
  })

  it('gives up once the failures stop being a blip', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new TypeError('network down')))

    const { promise } = followJob('j1', () => {})
    const settled = expect(promise).rejects.toThrow('network down')
    // Well past the tolerance, so this asserts it ends rather than that it
    // ends on any particular attempt.
    await vi.advanceTimersByTimeAsync(400 * 20)
    await settled
  })

  it('surfaces the API error detail for a job that no longer exists', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: false,
      status: 404,
      statusText: 'Not Found',
      json: async () => ({ detail: "no job 'j1'" }),
    })
    vi.stubGlobal('fetch', fetchMock)

    const { promise } = followJob('j1', () => {})
    await expect(promise).rejects.toThrow("no job 'j1'")
    // A 404 is the server saying it has never heard of this job — jobs live in
    // memory and die with the process — so there is nothing to wait out.
    expect(fetchMock).toHaveBeenCalledTimes(1)
  })

  it('stops polling once cancelled', async () => {
    const fetchMock = respondWith(
      job({ status: 'running', percent: 10 }),
      job({ status: 'running', percent: 20 }),
    )

    const onTick = vi.fn()
    const { cancel } = followJob('j1', onTick)
    await vi.advanceTimersByTimeAsync(0)   // let the first poll land
    expect(fetchMock).toHaveBeenCalledTimes(1)

    cancel()
    await nextPoll()
    await nextPoll()

    // The scheduled tick fired, saw the cancel flag, and returned without
    // fetching. This is what stops an unmounted Simulator screen from polling
    // a job forever.
    expect(fetchMock).toHaveBeenCalledTimes(1)
    expect(onTick).toHaveBeenCalledTimes(1)
  })

  it('waits the full interval between polls', async () => {
    const fetchMock = respondWith(
      job({ status: 'running' }),
      job({ status: 'done' }),
    )

    followJob('j1', () => {})
    await vi.advanceTimersByTimeAsync(0)
    expect(fetchMock).toHaveBeenCalledTimes(1)

    await vi.advanceTimersByTimeAsync(399)
    expect(fetchMock).toHaveBeenCalledTimes(1)

    await vi.advanceTimersByTimeAsync(1)
    expect(fetchMock).toHaveBeenCalledTimes(2)
  })
})

/** Refuse every request with this status and body. */
function refuseWith(status: number, detail: string, headers: Record<string, string> = {}) {
  const fetchMock = vi.fn().mockResolvedValue({
    ok: false,
    status,
    statusText: 'Refused',
    headers: { get: (name: string) => headers[name] ?? null },
    json: async () => ({ detail }),
  })
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
}

describe('a 401', () => {
  const unsubscribes: (() => void)[] = []

  /** Register a listener that is torn down after the test. The subscriber set
   *  is module state, so a leaked listener would fire in every later test. */
  function listen() {
    const heard = vi.fn()
    unsubscribes.push(onSessionLost(heard))
    return heard
  }

  afterEach(() => {
    for (const off of unsubscribes.splice(0)) off()
  })

  it('announces a lost session, from wherever the request was made', async () => {
    refuseWith(401, 'authentication required')
    const heard = listen()

    await expect(api.decks()).rejects.toThrow('authentication required')

    expect(heard).toHaveBeenCalledTimes(1)
  })

  it('announces it for a write as well as a read', async () => {
    refuseWith(401, 'authentication required')
    const heard = listen()

    await expect(api.setNote({ owner: 'aasquier', slug: 'gyome-food' },
                             'mulligan', 'keep two lands'))
      .rejects.toThrow('authentication required')

    expect(heard).toHaveBeenCalledTimes(1)
  })

  it('reaches every listener, not only the first', async () => {
    refuseWith(401, 'authentication required')
    const first = listen()
    const second = listen()

    await expect(api.decks()).rejects.toThrow()

    expect(first).toHaveBeenCalledTimes(1)
    expect(second).toHaveBeenCalledTimes(1)
  })

  it('says nothing once the listener has unsubscribed', async () => {
    refuseWith(401, 'authentication required')
    const heard = vi.fn()
    onSessionLost(heard)()

    await expect(api.decks()).rejects.toThrow()

    expect(heard).not.toHaveBeenCalled()
  })

  it('is not announced for a failed login', async () => {
    // The 401 that means "wrong password" belongs in the form as the sentence
    // the server wrote. Announcing a lost session here would re-render the
    // screen somebody is mid-typing on, over an event that did not happen.
    refuseWith(401, 'invalid username or password')
    const heard = listen()

    await expect(api.login({ username: 'root', password: 'nope' }))
      .rejects.toThrow('invalid username or password')

    expect(heard).not.toHaveBeenCalled()
  })

  it('is not announced for `me`, which is what the announcement re-asks', async () => {
    // `me` is public and cannot 401 today. If that ever changed, announcing it
    // here would be a loop: the listener's whole job is to call this endpoint.
    refuseWith(401, 'authentication required')
    const heard = listen()

    await expect(api.me()).rejects.toThrow()

    expect(heard).not.toHaveBeenCalled()
  })

  it('is not announced for a 403, which is a different answer', async () => {
    // An admin route refused to a non-admin (ADR 17). The session is fine; it
    // is the person who is not an admin, and bouncing them to a login form
    // would suggest signing in again would help.
    refuseWith(403, 'admin only')
    const heard = listen()

    await expect(api.accounts()).rejects.toThrow('admin only')

    expect(heard).not.toHaveBeenCalled()
  })
})

describe('a 429', () => {
  it('carries Retry-After onto the error, in seconds', async () => {
    refuseWith(429, 'too many attempts -- wait and try again', { 'Retry-After': '45' })

    await expect(api.login({ username: 'root', password: 'nope' }))
      .rejects.toMatchObject({ status: 429, retryAfter: 45 })
  })

  it('leaves it null when the header is absent', async () => {
    refuseWith(429, 'too many attempts -- wait and try again')

    const caught = await api.requestReset('someone@example.com').catch((e) => e)
    expect(caught).toBeInstanceOf(ApiError)
    expect(caught.retryAfter).toBeNull()
  })

  it('leaves it null for a header that is not a number of seconds', async () => {
    // `Retry-After` also has an HTTP-date spelling. The API does not send one,
    // but a proxy in front of it might, and a countdown from `NaN` is worse
    // than no countdown.
    refuseWith(429, 'too many requests', { 'Retry-After': 'Wed, 12 Aug 2026 09:00:00 GMT' })

    const caught = await api.requestReset('someone@example.com').catch((e) => e)
    expect(caught.retryAfter).toBeNull()
  })
})

/**
 * The owner segment (ADR 22).
 *
 * A slug alone stopped identifying a deck the moment slugs became unique per
 * owner rather than globally, so every deck call takes an address. What is
 * pinned here is the *shape of the URL*, because that is the one thing no
 * component test can see: a screen mocking `api` will happily pass its checks
 * while the real client asks for `/api/decks/goreclaw` — the route that no
 * longer exists, and the failure this whole branch's browser half is for.
 */
describe('deck URLs', () => {
  /** Answer any request with an empty object, and hand back the mock so the
   *  path it was called with can be read off. */
  function capture() {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true, status: 200, json: async () => ({}),
    })
    vi.stubGlobal('fetch', fetchMock)
    return fetchMock
  }

  const REF = { owner: 'mitch', slug: 'goreclaw' }

  it('puts the owner ahead of the slug', async () => {
    const fetchMock = capture()
    await api.deck(REF)
    expect(fetchMock.mock.calls[0]?.[0]).toBe('/api/decks/mitch/goreclaw')
  })

  it('keeps the owner ahead of the slug on every sub-resource', async () => {
    const fetchMock = capture()
    await api.validate(REF)
    await api.stats(REF)
    await api.suggestions(REF)
    await api.printings(REF)
    await api.commander(REF)
    await api.dossier(REF)
    expect(fetchMock.mock.calls.map((c) => c[0])).toEqual([
      '/api/decks/mitch/goreclaw/validate',
      '/api/decks/mitch/goreclaw/stats',
      '/api/decks/mitch/goreclaw/suggestions',
      '/api/decks/mitch/goreclaw/printings',
      '/api/decks/mitch/goreclaw/commander',
      '/api/decks/mitch/goreclaw/dossier',
    ])
  })

  it('addresses the writes the same way', async () => {
    const fetchMock = capture()
    await api.setCardField(REF, 'Llanowar Elves', 'why', 'a turn-one dork')
    await api.setNote(REF, 'mulligan', 'keep two lands')
    await api.setDeckField(REF, 'stage', 'curated')
    await api.setShared(REF, true)
    expect(fetchMock.mock.calls.map((c) => c[0])).toEqual([
      '/api/decks/mitch/goreclaw/cards/Llanowar%20Elves',
      '/api/decks/mitch/goreclaw/notes/mulligan',
      '/api/decks/mitch/goreclaw',
      '/api/decks/mitch/goreclaw/shared',
    ])
  })

  it('keeps the confirmation on the delete, after the owner', async () => {
    const fetchMock = capture()
    await api.deleteDeck(REF, 'bury')
    expect(fetchMock.mock.calls[0]?.[0])
      .toBe('/api/decks/mitch/goreclaw?confirm=bury')
  })

  it('encodes both segments rather than trusting what was in the URL bar', async () => {
    // Neither can need it today — a slug is `[a-z0-9-]` and a username is
    // letters, digits, `.`, `_` and `-`. This function is handed whatever the
    // address bar held, so the guarantee is worth having anyway.
    const fetchMock = capture()
    await api.deck({ owner: 'a b', slug: 'c/d' })
    expect(fetchMock.mock.calls[0]?.[0]).toBe('/api/decks/a%20b/c%2Fd')
  })
})

/**
 * The admin stats URLs. Same argument as the deck URLs above: `Admin.tsx`
 * mocks `api`, so its tests pass whatever these functions actually fetch —
 * and a stats call that drifted off `/api/admin/` would stop being refused
 * to non-admins before routing (ADR 17), which no component test can see.
 */
describe('admin stats URLs', () => {
  function capture() {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true, status: 200, json: async () => ({}),
    })
    vi.stubGlobal('fetch', fetchMock)
    return fetchMock
  }

  it('keeps all four views under the admin prefix', async () => {
    const fetchMock = capture()
    await api.adminSystem()
    await api.adminStorage()
    await api.adminClaude()
    await api.adminActivity()
    expect(fetchMock.mock.calls.map((c) => c[0])).toEqual([
      '/api/admin/stats/system',
      '/api/admin/stats/storage',
      '/api/admin/stats/claude',
      '/api/admin/stats/activity',
    ])
  })
})
