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
 *    failure in a while is not worth a message. `sessionStorage` guards the
 *    once — a deterministic crash reloads into itself and must not loop.
 * 2. **Then say so, in the room's own voice.** The card offers the way back
 *    as a real `<a>` — a full document load, owing nothing to the tree that
 *    just fell over — and never names chunks, modules, or networks
 *    (Commandment 10).
 *
 * **The guard expires, and that is the whole of the 2026-08-19 fix.** It was
 * a one-way flag: written on the first failure of a session and never
 * cleared by anything, anywhere. iOS Safari keeps a tab's `sessionStorage`
 * alive for weeks, so a single transient failure — one deploy restart caught
 * mid-tap — spent the reload permanently on that device. Every later hiccup
 * then went straight to the card with no retry, and the only way out was
 * closing the tab. That is what Aaron's phone showed on 2026-08-19, at the
 * end of a day that shipped seven deploys. So the guard now records **when**
 * it reloaded rather than merely **that** it did, and only a reload inside
 * the last `EPISODE_MS` suppresses the next one.
 *
 * Why a window, and not simply clearing the stamp once a route renders: this
 * boundary sits *above* the route's `Suspense`, so it commits — and would
 * clear — while the chunk is still in flight, which is exactly the instant
 * before a failing chunk rejects. A deterministic failure would clear the
 * guard and then trip it, clear it and trip it, reloading forever. A window
 * cannot be fooled that way: reload-then-fail is far shorter than the
 * window, so the second failure always meets a fresh stamp and gets the card.
 *
 * It renders inside App.tsx's `key={location.pathname}` wrapper, so simply
 * navigating remounts it clean — an error on one page never follows the
 * reader to the next.
 */

import { Component, type ReactNode } from 'react'

const RELOADED_KEY = 'sylvan-route-reloaded'

/** How long one silent reload speaks for. Long enough that a reload which
 *  fails again always lands inside it — so a deterministic crash gets the
 *  card rather than another reload, even on a connection slow enough that
 *  the retry takes half a minute to give up — and short enough that a
 *  genuinely new failure later still earns its own heal. */
const EPISODE_MS = 60_000

/** Read/write the reload guard defensively: Safari in private browsing
 *  throws on `sessionStorage.setItem`, and a boundary that throws while
 *  handling a throw is worse than no boundary at all. */
function reloadedRecently(): boolean {
  try {
    const raw = sessionStorage.getItem(RELOADED_KEY)
    if (raw === null) return false
    const at = Number(raw)
    // Anything unreadable reads as "just now": declining to reload is always
    // the safe side of this call. The `1` written by the build that shipped
    // the one-way flag parses as one millisecond past the epoch, so it reads
    // as ancient — which buys every tab still carrying one the heal it has
    // been denied ever since.
    if (!Number.isFinite(at)) return true
    return Date.now() - at < EPISODE_MS
  } catch {
    return true
  }
}

function markReloaded(): boolean {
  try {
    sessionStorage.setItem(RELOADED_KEY, String(Date.now()))
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
    // Nothing reloaded in the last episode: a fresh load is the likeliest
    // cure and costs less attention than any message. The stamp is written
    // before the reload so a crash that survives one cannot ask for another
    // — and if the stamp cannot be written (private browsing), the card is
    // the answer, because an unguarded reload could spin forever.
    if (!reloadedRecently() && markReloaded()) {
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
