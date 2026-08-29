/**
 * Learn — where the teaching content lives, instead of inside a wizard.
 *
 * Everything this page shows already existed somewhere, and that was the
 * problem: the colour taxonomy rendered only inside "Start a deck", which is a
 * screen you pass through on the way to something else and never return to.
 * Reference material that can only be reached mid-task is reference material
 * nobody reads twice. So the depth lives here and the create flow keeps a
 * short version with a link across, which is the arrangement the maintainer
 * asked for.
 *
 * Two tabs over two tables, and neither is a second copy of anything.
 *
 * - **The colours** reads `/api/colors` for the 32 and is now an *index*
 *   rather than a reader: every combination has a page of its own at
 *   `/colors/:slug` (`routes/ColorPage.tsx`), and the wheel and the tier
 *   shelves below it are how you get there. It used to expand one combination
 *   in place inside a bordered box that already held nine things, which is
 *   what ran out of room.
 * - **Vocabulary** reads `/api/glossary`, the same table `<Term>` and
 *   `<HelpTip>` read a single sentence out of elsewhere in the app.
 *
 * **`?c=BG` still works and always will.** Those links were shared before the
 * pages existed, so the colours tab resolves the key and sends the reader to
 * the room it names rather than 404ing a bookmark. The resolution needs the
 * served table, which is why it happens after the fetch rather than in the
 * router.
 */

import { useCallback, useMemo, useState } from 'react'
import { Link, Navigate, useNavigate, useSearchParams } from 'react-router-dom'
import {
  type ColorTaxonomy,
  type Combination,
  type Term as TermData,
} from '../lib/api'
import { colorPath, slugForKey, useColorTaxonomy } from '../lib/colors'
import {
  ColorRing, ErrorNote, ManaText, PageMasthead, Spinner,
} from '../components/ui'

/** The card the whole project is named after, so the reference page wears it.
 *  Dominaria Remastered's printing — the artist credited below is that
 *  printing's, which is why the URL and the name are read off the pool
 *  together rather than picked separately. */
const SYLVAN_LIBRARY_ART =
  'https://cards.scryfall.io/art_crop/front/6/a/6ada256f-2e55-4c1f-b4d3-d7b10b498956.jpg'
import { ColorPentagram, TierGlyph } from '../components/pentagram'
import { VideoBackdrop } from '../components/videofx'
import { useGlossary } from '../lib/glossary'
import bookwormMp4 from '../assets/learn/bookworm-loop.mp4'
import bookwormStill from '../assets/learn/bookworm-still.webp'
import bookwormWebm from '../assets/learn/bookworm-loop.webm'

type Tab = 'colors' | 'words'

/* -------------------------------------------------------- the colours tab */

/**
 * One combination as an index entry: what it is called, what colours it is,
 * and the one line that says what the deck is for.
 *
 * A link and not a button, because it goes somewhere. That is the whole
 * difference this branch made to this tab — the entries used to swap a panel
 * underneath them, which is a control that looks like navigation and is not.
 */
function ComboLink({ combo }: { combo: Combination }) {
  return (
    <Link to={colorPath(combo)} className="combo-card">
      <span className="flex items-center gap-2.5">
        <ColorRing colors={combo.colors} size={22} />
        <span className="text-sm font-semibold tracking-tight">{combo.name}</span>
      </span>
      <span className="combo-card-line">{combo.tagline}</span>
      {combo.aliases.length > 0 && (
        <span className="text-[11px]" style={{ color: 'var(--text-muted)' }}>
          also called {combo.aliases.join(', ')}
        </span>
      )}
    </Link>
  )
}

/**
 * One tier and everything in it.
 *
 * All seven are on the page at once rather than behind a selector, which is
 * the same argument the old panel's own member list made and the reason it is
 * kept: this screen is for reading rather than for choosing, and a control
 * that hides six of seven shelves is the wrong shape for a contents page.
 */
function TierShelf({ tier, members }: {
  tier: { key: string; label: string; blurb: string }
  members: Combination[]
}) {
  return (
    <section id={`tier-${tier.key}`} className="scroll-mt-24 space-y-3">
      <div className="flex items-center gap-2.5">
        <TierGlyph tier={tier.key} size={30} />
        <h2 className="text-lg font-semibold tracking-tight">{tier.label}</h2>
      </div>
      <p className="max-w-3xl border-l-2 pl-4 text-sm leading-relaxed"
         style={{ borderColor: 'var(--baseline)', color: 'var(--text-secondary)' }}>
        {tier.blurb}
      </p>
      <div className="grid gap-2.5 sm:grid-cols-2 lg:grid-cols-3">
        {members.map((c) => <ComboLink key={c.key} combo={c} />)}
      </div>
    </section>
  )
}

/**
 * The contents page for all thirty-two.
 *
 * The wheel stays at the top and stays the navigation device — every vertex
 * and every line on it is one of the 32, so pointing at the shape and
 * choosing from a shelf are the same act. What changed is where the act
 * lands: it used to open a panel below and it now opens a page.
 */
function ColorsTab({ taxonomy }: { taxonomy: ColorTaxonomy }) {
  const navigate = useNavigate()
  const shelves = taxonomy.tiers
    .map((tier) => ({
      tier,
      members: taxonomy.combinations.filter((c) => c.tier === tier.key),
    }))
    .filter((s) => s.members.length > 0)

  // Every shelf on by default, keyed by tier: the page opens as it always
  // has, and the filter only ever narrows from there.
  const [hidden, setHidden] = useState<ReadonlySet<string>>(() => new Set())
  const showing = {
    has: (key: string) => !hidden.has(key),
  }
  const shown = shelves.filter(({ tier }) => !hidden.has(tier.key))
  const toggleShelf = (key: string) => setHidden((prev) => {
    const next = new Set(prev)
    if (next.has(key)) next.delete(key)
    else next.add(key)
    return next
  })
  const showEverything = () => setHidden(new Set())

  if (!taxonomy.combinations.length) {
    // The fallback is itself an index, so its absence is not a state this
    // screen can render around: `/api/colors` is checked-in prose served with
    // no card pool and no network, so an empty list means that endpoint
    // answered with nothing. Say so once, here.
    return (
      <p className="text-sm" style={{ color: 'var(--text-muted)' }}>
        The colour guide came back empty. It is checked-in prose and needs
        neither the shelves nor the outside world, so this is the guide itself
        failing rather than missing data.
      </p>
    )
  }

  return (
    <div className="space-y-8">
      <section className="card-surface rounded-xl px-6 py-6">
        <ColorPentagram combinations={taxonomy.combinations}
                        onPick={(c) => navigate(colorPath(c))} />
      </section>

      {/* **A filter, since 2026-08-29.** These were jump links wearing a
          toggle's clothes — the comment here used to argue that a reader who
          came for the clans should not scroll past the guilds, which is true
          and is a better argument for narrowing the page than for scrolling
          it. Aaron's ruling: a toggle presented as a link is awkward, and
          these should be real toggles.

          Every shelf starts on, so the page opens exactly as it always has
          and nobody has to discover a control to read it (commandment 2). */}
      <nav className="flex flex-wrap gap-1.5" aria-label="Which shelves to show">
        {shelves.map(({ tier }) => {
          const on = showing.has(tier.key)
          return (
            <button key={tier.key} type="button" aria-pressed={on}
                    onClick={() => toggleShelf(tier.key)}
                    className={`chip-toggle flex items-center gap-2 rounded-lg py-1.5 pl-2 pr-3 text-sm font-medium${
                      on ? ' is-on' : ''}`}>
              <TierGlyph tier={tier.key} />
              {tier.label}
            </button>
          )
        })}
      </nav>

      {shown.map(({ tier, members }) => (
        <TierShelf key={tier.key} tier={tier} members={members} />
      ))}

      {/* Turning every shelf off is a thing somebody can do by accident on a
          phone, and an empty page with no explanation reads as broken. */}
      {shown.length === 0 && (
        <div className="space-y-2">
          <p className="text-sm" style={{ color: 'var(--text-muted)' }}>
            Every shelf is hidden, so there is nothing to read.
          </p>
          <button type="button" onClick={showEverything}
                  className="btn btn-ghost btn-xs">
            Show them all again
          </button>
        </div>
      )}
    </div>
  )
}

/* ------------------------------------------------------ the vocabulary tab */

function TermEntry({ term, byKey, onJump }: {
  term: TermData
  byKey: Map<string, TermData>
  onJump: (key: string) => void
}) {
  return (
    <div id={`term-${term.key}`} className="scroll-mt-24">
      <dt className="text-base font-semibold tracking-tight">{term.term}</dt>
      <dd className="mt-1 max-w-3xl text-sm leading-relaxed"
          style={{ color: 'var(--text-secondary)' }}>
        {/* `short` first, then `long`. The two are written as a pair rather
            than as alternatives: about a third of the entries -- the whole
            `stat.*` block, plus commander tax and mana base -- open their
            paragraph as sentence *two*, commenting on a thing only `short`
            ever names. Rendering the long form alone left those terms
            undefined on the one page whose job is defining them. The search
            below has always matched `short`, so it was also possible to find
            an entry by text the page then refused to show. */}
        <p style={{ color: 'var(--text-primary)' }}>
          <ManaText>{term.short}</ManaText>
        </p>
        <p className="mt-1.5"><ManaText>{term.long}</ManaText></p>
        {term.see_also.length > 0 && (
          <span className="mt-1.5 flex flex-wrap items-center gap-1.5 text-xs"
                style={{ color: 'var(--text-muted)' }}>
            See also
            {term.see_also.map((key) => (
              <button key={key} onClick={() => onJump(key)}
                      className="chip-place rounded px-1.5 py-0.5">
                {byKey.get(key)?.term ?? key}
              </button>
            ))}
          </span>
        )}
      </dd>
    </div>
  )
}

function WordsTab() {
  const glossary = useGlossary()
  const [query, setQuery] = useState('')

  const byKey = useMemo(
    () => new Map((glossary?.terms ?? []).map((t) => [t.key, t])),
    [glossary],
  )

  // A cross-reference can point into another section, so jumping scrolls
  // rather than filters — filtering to one term would hide the context the
  // reference was sending you to.
  const jump = useCallback((key: string) => {
    setQuery('')
    requestAnimationFrame(() => {
      document.getElementById(`term-${key}`)?.scrollIntoView({
        behavior: 'smooth', block: 'center',
      })
    })
  }, [])

  if (!glossary) {
    return <Spinner label="Reading the glossary…" />
  }

  const needle = query.trim().toLowerCase()
  const matches = (t: TermData) =>
    !needle || t.term.toLowerCase().includes(needle)
    || t.short.toLowerCase().includes(needle)
    || t.long.toLowerCase().includes(needle)

  return (
    <div className="space-y-8">
      <label className="block max-w-sm">
        <span className="text-xs uppercase tracking-wide"
              style={{ color: 'var(--text-muted)' }}>Find a word</span>
        <input value={query} onChange={(e) => setQuery(e.target.value)}
               placeholder="mulligan, ramp, shuffle…"
               className="mt-1 w-full rounded-md px-3 py-2 text-sm"
               style={{ background: 'var(--page)', color: 'var(--text-primary)',
                        border: '1px solid var(--hairline)' }} />
      </label>

      {glossary.sections.map((section) => {
        const terms = glossary.terms.filter(
          (t) => t.section === section.key && matches(t))
        if (!terms.length) return null
        return (
          <section key={section.key} className="space-y-3">
            <div>
              <h2 className="text-xl font-semibold tracking-tight">{section.label}</h2>
              <p className="mt-0.5 max-w-3xl text-sm"
                 style={{ color: 'var(--text-muted)' }}>
                {section.blurb}
              </p>
            </div>
            <dl className="space-y-5">
              {terms.map((t) => (
                <TermEntry key={t.key} term={t} byKey={byKey} onJump={jump} />
              ))}
            </dl>
          </section>
        )
      })}

      {needle && !glossary.terms.some(matches) && (
        <p className="text-sm" style={{ color: 'var(--text-muted)' }}>
          Nothing here matches “{query}”. It may still be a real word.
        </p>
      )}
    </div>
  )
}

/* ------------------------------------------------------------ the reading
   room */

const BOOKWORM_ALT =
  'The Bookworm, painted by Carl Spitzweg: an old scholar stands on top of ' +
  'a library ladder, reading one book with another under his arm and a ' +
  'third clamped between his knees, sunlight falling down the shelves.'

/**
 * The painting at the foot of the page — the first public-domain canvas the
 * animist brought to life as a *committed* asset (bookworm.recipe.yaml,
 * ADR 31: a wikimedia source through the licence gate, then a ken_burns
 * breath that closes on itself). The card the site is named for hangs in
 * the masthead and is Wizards' art, hotlinked under the Fan Content Policy;
 * this one is ours to carry, which is why it can live in the bundle and
 * breathe. Art mode: reduced motion gets the still, the page as it would
 * be in print.
 */
function ReadingRoom() {
  return (
    <figure className="mx-auto max-w-xs space-y-3 pt-6 sm:max-w-sm">
      <div className="reading-room-frame">
        <VideoBackdrop
          webmSrc={bookwormWebm} mp4Src={bookwormMp4} poster={bookwormStill}
          mode="art" className="reading-room-painting"
          fallback={<img src={bookwormStill} alt={BOOKWORM_ALT}
                         className="reading-room-painting" />} />
      </div>
      <figcaption className="text-center text-[11px] leading-relaxed"
                  style={{ color: 'var(--text-muted)' }}>
        <em>Der Bücherwurm</em> (The Bookworm), Carl Spitzweg, c. 1850 —
        public domain. Every library keeps one reader who cannot stop.
      </figcaption>
    </figure>
  )
}

/* --------------------------------------------------------------- the page */

const TABS: { key: Tab; label: string }[] = [
  { key: 'colors', label: 'The colours' },
  { key: 'words', label: 'Vocabulary' },
]

export default function Learn() {
  const [params, setParams] = useSearchParams()
  const { taxonomy, failed } = useColorTaxonomy()

  const tab: Tab = params.get('tab') === 'words' ? 'words' : 'colors'
  // The old colours tab kept its selection here, so links to `?c=BG` are out
  // in the world and have to keep working. A key names a room now, so the
  // answer is the room. Resolving it needs the served table, hence after the
  // fetch rather than in the router — and a key nothing recognises falls
  // through to the index rather than to an error, which is the kinder of the
  // two answers to a mistyped bookmark.
  const asked = tab === 'colors' ? params.get('c') : null
  const crossing = asked && taxonomy ? slugForKey(taxonomy.combinations, asked) : null
  if (crossing) return <Navigate to={`/colors/${crossing}`} replace />

  return (
    <div className="space-y-6">
      <PageMasthead
        art={SYLVAN_LIBRARY_ART}
        alt="Sylvan Library, painted by Yeong-Hao Han: shelves of books grown
             into the trunks of living trees, with green light falling between
             the leaves onto an open volume."
        title="Learn"
        credit={<>
          <em>Sylvan Library</em> by Yeong-Hao Han, Dominaria Remastered — the
          card this place is named for.
        </>}>
        <p>
          Magic has thirty years of vocabulary and this app assumes most of it.
          Here is the part you need: what the colours mean and who fought over
          them, and every word the other screens use without stopping to
          explain.
        </p>
      </PageMasthead>

      <div className="flex flex-wrap gap-1">
        {TABS.map((t) => (
          <button key={t.key}
                  onClick={() => setParams({ tab: t.key }, { replace: true })}
                  aria-pressed={tab === t.key}
                  className={`chip-toggle rounded-md px-3 py-1.5 text-sm font-medium${
                    tab === t.key ? ' is-on' : ''}`}>
            {t.label}
          </button>
        ))}
      </div>

      {failed && (
        <ErrorNote>
          Could not load the colour guide. It is checked-in prose and needs
          neither the shelves nor the outside world, so this is the guide
          itself failing rather than missing data.
        </ErrorNote>
      )}

      {tab === 'words' ? <WordsTab /> : taxonomy ? (
        <ColorsTab taxonomy={taxonomy} />
      ) : !failed && (
        <Spinner label="Reading the colour guide…" />
      )}

      <ReadingRoom />
    </div>
  )
}
