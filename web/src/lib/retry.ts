import { useEffect, useState } from 'react'
import { ApiError } from './api'

/**
 * Seconds still owed to a rate limit's `Retry-After`, ticking down to zero.
 *
 * Login, reset and claim are all rate limited and all answer 429 with the
 * header, and a form that only printed "too many attempts" would leave somebody
 * clicking to find out whether the wait was over. Counting down turns that into
 * a fact on screen.
 *
 * It drifts if the tab sleeps, and that is fine: this is a courtesy, not an
 * enforcement. The budget lives in SQLite on the server, which will simply
 * answer 429 again — the number here has never been what stops anybody.
 *
 * Returns the seconds left and the thing to hand a caught error to; anything
 * that is not a 429 carrying a header is ignored, so a caller can pass every
 * failure through it without checking first.
 */
export function useRetryAfter(): [number, (error: unknown) => void] {
  const [seconds, setSeconds] = useState(0)

  useEffect(() => {
    if (seconds <= 0) return
    const id = setTimeout(() => setSeconds((s) => s - 1), 1000)
    return () => clearTimeout(id)
  }, [seconds])

  return [seconds, (error: unknown) => {
    if (error instanceof ApiError && error.status === 429 && error.retryAfter) {
      setSeconds(error.retryAfter)
    }
  }]
}
