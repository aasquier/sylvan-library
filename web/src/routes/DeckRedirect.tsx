import { useEffect, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { api, deckUrl } from '../lib/api'
import { Spinner } from '../components/ui'

/**
 * A bookmark from before decks had owners, sent to where the deck lives now.
 *
 * `/decks/<slug>` was every deck's address for the whole life of this app and
 * ADR 22 replaced it with `/decks/<owner>/<slug>`. The bundle is committed and
 * served with the server, so no *client* is stale — but a link somebody sent
 * to a friend is, a browser's history is, and the instance has been driven for
 * days. Answering those with the catch-all's "Nothing here" would be the app
 * losing decks that are still on the shelf.
 *
 * Resolving it needs the library, because a slug alone genuinely does not
 * identify a deck any more: slugs are unique per owner. So this asks
 * `/api/decks` — which spans every owner the caller may read — and takes the
 * **first** match. That is not arbitrary. `Library.visible()` returns the
 * caller's own decks first, then the showcase, then everybody else's, so
 * first-match is precisely the precedence a person would want: your own
 * `goreclaw` before the maintainer's, and the maintainer's before a stranger's.
 *
 * `replace` rather than a push, so Back goes where the reader came from instead
 * of bouncing off this page again.
 */
export default function DeckRedirect() {
  const { slug = '' } = useParams()
  const navigate = useNavigate()
  const [missing, setMissing] = useState(false)

  useEffect(() => {
    let live = true
    setMissing(false)
    api.decks()
      .then((decks) => {
        if (!live) return
        const found = decks.find((d) => d.slug === slug)
        if (found) navigate(deckUrl(found), { replace: true })
        else setMissing(true)
      })
      // A library that cannot be listed cannot be searched. The honest answer
      // is the same one an unknown slug gets: this link does not lead anywhere
      // we can reach.
      .catch(() => { if (live) setMissing(true) })
    return () => { live = false }
  }, [slug, navigate])

  if (!missing) return <Spinner label="Finding that deck…" />
  return (
    <div className="card-surface rounded-xl px-6 py-10 text-center">
      <p className="text-sm" style={{ color: 'var(--text-secondary)' }}>
        No deck called <code>{slug}</code> on any shelf you can see. Deck
        addresses include the owner now — try the library.
      </p>
    </div>
  )
}
