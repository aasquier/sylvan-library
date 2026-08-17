/**
 * The net under the routed page.
 *
 * Every screen except the landing page arrives as a lazy chunk (web/README:
 * "Routes are lazy"), and a chunk fetch can fail for reasons the app cannot
 * see coming — the few seconds of downtime every deploy costs (ADR 23, one
 * machine), a phone losing its connection between taps, a cache revalidating
 * against a server that just restarted. Without a boundary that rejection
 * unmounts the React root: the page goes black **and stays black**, because
 * the nav died with everything else. That is not a hypothetical — it is what
 * Aaron's phone showed on 2026-08-16, and what `boundary.test.tsx` pins.
 *
 * Two answers, in order:
 *
 * 1. **Reload once, silently.** A fresh document load heals almost every
 *    cause (the deploy finished, the connection came back), so the first
 *    failure in a session is not worth a message. `sessionStorage` guards
 *    the once — a deterministic crash reloads into itself and must not loop.
 * 2. **Then say so, in the room's own voice.** The card offers the way back
 *    as a real `<a>` — a full document load, owing nothing to the tree that
 *    just fell over — and never names chunks, modules, or networks
 *    (Commandment 10).
 *
 * It renders inside App.tsx's `key={location.pathname}` wrapper, so simply
 * navigating remounts it clean — an error on one page never follows the
 * reader to the next.
 */

import { Component, type ReactNode } from 'react'

const RELOADED_KEY = 'sylvan-route-reloaded'

/** Read/write the reload guard defensively: Safari in private browsing
 *  throws on `sessionStorage.setItem`, and a boundary that throws while
 *  handling a throw is worse than no boundary at all. */
function alreadyReloaded(): boolean {
  try {
    return sessionStorage.getItem(RELOADED_KEY) === '1'
  } catch {
    return true
  }
}

function markReloaded(): boolean {
  try {
    sessionStorage.setItem(RELOADED_KEY, '1')
    return true
  } catch {
    return false
  }
}

export class RouteErrorBoundary extends Component<
  { children: ReactNode },
  { failed: boolean; card: boolean }
> {
  state = { failed: false, card: false }

  static getDerivedStateFromError(): { failed: boolean } {
    return { failed: true }
  }

  componentDidCatch(): void {
    // First failure this session: a fresh load is the likeliest cure and
    // costs less attention than any message. The guard is written before
    // the reload so a crash that survives one cannot ask for another —
    // and if the guard cannot be written (private browsing), the card is
    // the answer, because an unguarded reload could spin forever.
    if (!alreadyReloaded() && markReloaded()) {
      window.location.reload()
      return
    }
    this.setState({ card: true })
  }

  render(): ReactNode {
    if (!this.state.failed) return this.props.children
    // Between the catch and the verdict (or while the reload lands), keep
    // the frame quiet rather than flashing the card first.
    if (this.state.card) {
      return (
        <div className="card-surface rounded-xl px-6 py-10 text-center">
          <p className="text-base font-medium" style={{ color: 'var(--text-primary)' }}>
            This page slipped off the shelf.
          </p>
          <p className="mt-2 text-sm" style={{ color: 'var(--text-secondary)' }}>
            The library could not fetch it just now — a moment's lapse on the
            path between here and there, most likely, and nothing you did.
          </p>
          {/* A document load on purpose: it re-enters through the front door
              rather than trusting the tree this card is standing in. */}
          <a href="/" className="mt-5 inline-block rounded-md px-4 py-2 text-sm font-medium"
             style={{ color: 'var(--text-primary)', border: '1px solid var(--hairline)' }}>
            Return to the library
          </a>
        </div>
      )
    }
    return null
  }
}
