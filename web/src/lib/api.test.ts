/** The job-polling state machine.
 *
 * `followJob` is the only genuine state machine in the frontend -- queued ->
 * running -> done/error, with a cancel path -- and it was entirely untested.
 * Everything the simulator screen shows depends on it terminating correctly:
 * a poll that never stops burns a request every 400ms forever, and a poll that
 * resolves early shows a result that does not exist yet.
 *
 * Timers are faked, so a test that would take seconds of real polling takes
 * none, and the interval is asserted rather than waited out.
 */

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { followJob, type Job } from './api'

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
      error: 'simulation needs the card corpus -- run `mtglab data refresh`',
    }))

    const { promise } = followJob('j1', () => {})
    await expect(promise).rejects.toThrow(/needs the card corpus/)
  })

  it('rejects with a fallback message when a failed job carries no error text', async () => {
    respondWith(job({ status: 'error', error: null }))

    const { promise } = followJob('j1', () => {})
    await expect(promise).rejects.toThrow('job failed')
  })

  it('rejects when the request itself fails, rather than polling forever', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new TypeError('network down')))

    const { promise } = followJob('j1', () => {})
    await expect(promise).rejects.toThrow('network down')
  })

  it('surfaces the API error detail for a job that no longer exists', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: false,
      status: 404,
      statusText: 'Not Found',
      json: async () => ({ detail: "no job 'j1'" }),
    }))

    const { promise } = followJob('j1', () => {})
    await expect(promise).rejects.toThrow("no job 'j1'")
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
