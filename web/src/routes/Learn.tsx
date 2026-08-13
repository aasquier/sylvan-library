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
 * - **The colours** reads `/api/colors` for the 32 and `/api/colors/{key}` for
 *   one of them with its cards resolved. The split matters: the first works on
 *   a fresh clone with no corpus, the second is where every card fact enters,
 *   and a name that does not resolve is dropped and counted rather than drawn
 *   as an empty card.
 * - **Vocabulary** reads `/api/glossary`, the same table `<Term>` and
 *   `<HelpTip>` read a single sentence out of elsewhere in the app.
 *
 * Both tabs and the selected combination are in the query string, so a link to
 * Golgari is a link to Golgari.
 */

import { useCallback, useEffect, useMemo, useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import {
  api,
  errorMessage,
  type ColorTaxonomy,
  type Combination,
  type CombinationDetail,
  type ReferenceCard,
  type Term as TermData,
} from '../lib/api'
import { COLOR_VAR } from '../lib/mtg'
import { CardHover, ColorRing, ErrorNote, ManaCost, ManaText } from '../components/ui'
import { ColorPentagram, TierGlyph } from '../components/pentagram'
import { useGlossary } from '../lib/glossary'

type Tab = 'colors' | 'words'

/** The era whose story named a tier. Mirrors the create flow's own map. */
const TIER_ERA: Record<string, string> = {
  guild: 'Ravnica',
  shard: 'Alara',
  wedge: 'Tarkir',
}

/* ----------------------------------------------------------------- a card */

/**
 * A real card, rendered from the corpus and captioned with nothing.
 *
 * `note` is the champion's story role and is the only sentence attached to a
 * card anywhere in this page. The signature list passes none at all, which is
 * deliberate — what that list claims is that the card's identity is exactly
 * this combination, and that is checkable rather than editorial. The oracle
 * text below is the card's own, so a role that drifted from the card is
 * visible next to the evidence.
 */
function RefCard({ card, note }: { card: ReferenceCard; note?: string }) {
  return (
    <CardHover card={card} className="block">
      <article className="card-surface flex h-full flex-col overflow-hidden rounded-xl text-left">
        {card.art_crop && (
          <img src={card.art_crop} alt="" loading="lazy"
               className="h-20 w-full object-cover" style={{ objectPosition: 'center 30%' }} />
        )}
        <div className="flex flex-1 flex-col gap-1 px-3 py-2.5">
          <div className="flex flex-wrap items-baseline gap-x-2 gap-y-1">
            <span className="text-sm font-semibold">{card.name}</span>
            <ManaCost cost={card.mana_cost} size={13} />
          </div>
          <p className="text-[11px]" style={{ color: 'var(--text-muted)' }}>
            {card.type_line}
          </p>
          {note && (
            <p className="mt-0.5 text-xs leading-relaxed"
               style={{ color: 'var(--text-primary)' }}>
              {note}
            </p>
          )}
          {/* `whitespace-pre-line`, like the deck page and the rationale
              editor: oracle text is newline-separated and collapsing it runs
              a keyword line into the ability below it. The Gitrog Monster
              read "Deathtouch At the beginning of your upkeep" without it. */}
          {card.oracle_text && (
            <p className="mt-auto whitespace-pre-line pt-1 text-[11px] leading-relaxed"
               style={{ color: 'var(--text-secondary)' }}>
              <ManaText size={11}>{card.oracle_text}</ManaText>
            </p>
          )}
        </div>
      </article>
    </CardHover>
  )
}

/* -------------------------------------------------------- the colours tab */

function CombinationPanel({ combo, taxonomy }: {
  combo: Combination
  taxonomy: ColorTaxonomy
}) {
  const [detail, setDetail] = useState<CombinationDetail | null>(null)
  const [failed, setFailed] = useState(false)

  useEffect(() => {
    let live = true
    setDetail(null)
    setFailed(false)
    api.combination(combo.key)
      .then((d) => { if (live) setDetail(d) })
      .catch(() => { if (live) setFailed(true) })
    return () => { live = false }
  }, [combo.key])

  const era = taxonomy.eras.find((e) => e.name === TIER_ERA[combo.tier])
  const tier = taxonomy.tiers.find((t) => t.key === combo.tier)

  return (
    <article className="card-surface rounded-xl px-6 py-6"
             style={{
               backgroundImage: combo.colors.length
                 ? `linear-gradient(135deg, ${combo.colors
                     .map((c, i) => `color-mix(in srgb, ${COLOR_VAR[c]} 18%, transparent) ${
                       (i / Math.max(combo.colors.length - 1, 1)) * 100}%`)
                     .join(', ')})`
                 : 'none',
             }}>
      <header className="flex flex-wrap items-center gap-4">
        <ColorRing colors={combo.colors} />
        <div className="min-w-0">
          <h2 className="text-2xl font-semibold tracking-tight">{combo.name}</h2>
          <p className="text-xs" style={{ color: 'var(--text-muted)' }}>
            {tier?.label}
            {combo.aliases.length > 0 && ` · also called ${combo.aliases.join(', ')}`}
          </p>
        </div>
        <Link to={`/new?c=${combo.key}`} className="ml-auto rounded-md px-3 py-1.5 text-sm font-medium"
              style={{ background: 'var(--series-1)', color: '#fff' }}>
          Build {combo.name}
        </Link>
      </header>

      <p className="mt-4 text-lg" style={{ color: 'var(--text-primary)' }}>
        {combo.tagline}
      </p>
      <p className="mt-3 max-w-3xl text-sm leading-relaxed"
         style={{ color: 'var(--text-secondary)' }}>
        <ManaText>{combo.history}</ManaText>
      </p>

      {/* The story beat, and the field this page was built for. Only the
          twenty slots that are an actual faction have one; the other twelve
          simply do not render a heading with nothing under it. */}
      {combo.lore && (
        <section className="mt-5 max-w-3xl border-l-2 pl-4"
                 style={{ borderColor: 'var(--baseline)' }}>
          <h3 className="text-xs uppercase tracking-wide"
              style={{ color: 'var(--text-muted)' }}>
            What happened
          </h3>
          <p className="mt-1.5 text-sm leading-relaxed"
             style={{ color: 'var(--text-secondary)' }}>
            {combo.lore}
          </p>
          {era && (
            <p className="mt-2 text-xs leading-relaxed"
               style={{ color: 'var(--text-muted)' }}>
              <strong style={{ color: 'var(--text-secondary)' }}>{era.name}</strong>
              {' '}— {era.setting}. {era.story}
            </p>
          )}
        </section>
      )}

      {failed && (
        <p className="mt-5 text-sm" style={{ color: 'var(--text-muted)' }}>
          Could not load the cards for this combination.
        </p>
      )}

      {/* Names without a corpus, cards with one. The champion list is drawn
          from `colors.py` either way, so a fresh clone still learns who
          Trostani is — it just does not get her card. */}
      {combo.champions.length > 0 && (
        <section className="mt-6">
          <h3 className="text-xs uppercase tracking-wide"
              style={{ color: 'var(--text-muted)' }}>
            Who they are
          </h3>
          {detail?.corpus ? (
            <div className="mt-2 grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
              {detail.champions.map((c) => (
                <RefCard key={c.name} card={c} note={c.role} />
              ))}
            </div>
          ) : (
            <ul className="mt-2 space-y-1.5">
              {combo.champions.map((c) => (
                <li key={c.card} className="text-sm"
                    style={{ color: 'var(--text-secondary)' }}>
                  <strong style={{ color: 'var(--text-primary)' }}>{c.card}</strong>
                  {' '}— {c.role}
                </li>
              ))}
            </ul>
          )}
        </section>
      )}

      <section className="mt-6">
        <h3 className="text-xs uppercase tracking-wide"
            style={{ color: 'var(--text-muted)' }}>
          Exactly these colours
        </h3>
        <p className="mt-1 max-w-3xl text-xs leading-relaxed"
           style={{ color: 'var(--text-muted)' }}>
          Cards whose colour identity is precisely {combo.name} — they can go in
          this deck and in no narrower one.
          {/* Counted over the corpus rather than stored, and it is the
              sharpest sentence available about a four-colour slot: two cards,
              in the entire game. */}
          {detail?.exact_total != null && (
            <> The corpus has <strong style={{ color: 'var(--text-secondary)' }}>
              {detail.exact_total.toLocaleString()}</strong> of them
              {detail.exact_total <= 5 && ' — the whole set'}.
            </>
          )}
        </p>
        {detail?.corpus ? (
          <div className="mt-2 grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
            {detail.signature.map((c) => <RefCard key={c.name} card={c} />)}
          </div>
        ) : (
          <p className="mt-2 text-sm" style={{ color: 'var(--text-secondary)' }}>
            {combo.signature.join(' · ')}
            <span className="block text-xs" style={{ color: 'var(--text-muted)' }}>
              The cards themselves need the corpus — run <code>mtglab data
              refresh</code>.
            </span>
          </p>
        )}
      </section>

      <p className="mt-5 text-xs" style={{ color: 'var(--text-muted)' }}>
        Colour identity here is Scryfall’s, checked against{' '}
        <em>{combo.verified_by}</em>.
        {/* A dropped name is a bug in the reference table, not in the corpus,
            so it says so rather than failing quietly. */}
        {detail && detail.dropped > 0
          && ` ${detail.dropped} named card${detail.dropped === 1 ? '' : 's'} `
             + 'could not be found in the corpus and are not shown.'}
      </p>
    </article>
  )
}

function ColorsTab({ taxonomy, selected, onSelect }: {
  taxonomy: ColorTaxonomy
  selected: string
  onSelect: (key: string) => void
}) {
  const combo = taxonomy.combinations.find((c) => c.key === selected)
    ?? taxonomy.combinations[0]
  const tier = taxonomy.tiers.find((t) => t.key === combo.tier)

  return (
    <div className="space-y-5">
      {/* The wheel is the index, not an illustration. Every vertex and every
          line on it is one of the 32, so pointing at the shape and choosing
          from the list are the same act — which is the argument branch 3 made
          for drawing it in the first place. */}
      <section className="card-surface rounded-xl px-6 py-6">
        <ColorPentagram combinations={taxonomy.combinations}
                        onPick={(c) => onSelect(c.key)} selected={combo.key} />
      </section>

      <div className="space-y-3">
        <div className="flex flex-wrap gap-1.5">
          {taxonomy.tiers.map((t) => {
            const first = taxonomy.combinations.find((c) => c.tier === t.key)
            const on = combo.tier === t.key
            return (
              <button key={t.key} onClick={() => first && onSelect(first.key)}
                      aria-pressed={on}
                      className="flex items-center gap-2 rounded-lg py-1.5 pl-2 pr-3 text-sm font-medium transition"
                      style={{
                        color: on ? 'var(--text-primary)' : 'var(--text-muted)',
                        background: on ? 'var(--gridline)' : 'transparent',
                        border: `1px solid ${on ? 'var(--baseline)' : 'var(--hairline)'}`,
                      }}>
                <TierGlyph tier={t.key} />
                {t.label}
              </button>
            )
          })}
        </div>

        {tier && (
          <p className="max-w-3xl border-l-2 pl-4 text-sm leading-relaxed"
             style={{ borderColor: 'var(--baseline)', color: 'var(--text-secondary)' }}>
            {tier.blurb}
          </p>
        )}

        {/* Every member of the tier at once rather than a carousel. This page
            is for reading rather than for choosing, and an arrow control that
            hides nine of ten guilds is the wrong shape for that. */}
        <div className="flex flex-wrap gap-1.5">
          {taxonomy.combinations.filter((c) => c.tier === combo.tier).map((c) => (
            <button key={c.key} onClick={() => onSelect(c.key)}
                    aria-pressed={c.key === combo.key}
                    className="flex items-center gap-2 rounded-lg px-2.5 py-1.5 text-sm transition"
                    style={{
                      color: c.key === combo.key ? 'var(--text-primary)' : 'var(--text-secondary)',
                      background: c.key === combo.key ? 'var(--gridline)' : 'transparent',
                      border: `1px solid ${c.key === combo.key ? 'var(--baseline)' : 'var(--hairline)'}`,
                    }}>
              <ColorRing colors={c.colors} size={14} />
              {c.name}
            </button>
          ))}
        </div>
      </div>

      <CombinationPanel combo={combo} taxonomy={taxonomy} />
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
                      className="rounded px-1.5 py-0.5"
                      style={{ border: '1px solid var(--hairline)',
                               color: 'var(--text-secondary)' }}>
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
    return <p className="text-sm" style={{ color: 'var(--text-muted)' }}>Loading…</p>
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
               placeholder="mulligan, ramp, seed…"
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

/* --------------------------------------------------------------- the page */

const TABS: { key: Tab; label: string }[] = [
  { key: 'colors', label: 'The colours' },
  { key: 'words', label: 'Vocabulary' },
]

export default function Learn() {
  const [params, setParams] = useSearchParams()
  const [taxonomy, setTaxonomy] = useState<ColorTaxonomy | null>(null)
  const [error, setError] = useState<string | null>(null)

  const tab: Tab = params.get('tab') === 'words' ? 'words' : 'colors'
  const selected = params.get('c') ?? 'WG'

  useEffect(() => {
    api.colors().then(setTaxonomy).catch((e) => setError(errorMessage(e)))
  }, [])

  const select = useCallback((key: string) => {
    setParams({ tab: 'colors', c: key }, { replace: true })
  }, [setParams])

  return (
    <div className="space-y-6">
      <header>
        <h1 className="text-3xl font-semibold tracking-tight">Learn</h1>
        <p className="mt-1 max-w-3xl text-sm" style={{ color: 'var(--text-secondary)' }}>
          Magic has thirty years of vocabulary and this app assumes most of it.
          Here is the part you need: what the colours mean and who fought over
          them, and every word the other screens use without stopping to
          explain.
        </p>
      </header>

      <div className="flex flex-wrap gap-1">
        {TABS.map((t) => (
          <button key={t.key}
                  onClick={() => setParams(
                    t.key === 'words' ? { tab: 'words' } : { tab: 'colors', c: selected },
                    { replace: true })}
                  className="rounded-md px-3 py-1.5 text-sm font-medium transition"
                  style={{
                    color: tab === t.key ? 'var(--text-primary)' : 'var(--text-muted)',
                    background: tab === t.key ? 'var(--gridline)' : 'transparent',
                    border: '1px solid var(--hairline)',
                  }}>
            {t.label}
          </button>
        ))}
      </div>

      {error && <ErrorNote>Could not load the colour guide: {error}</ErrorNote>}

      {tab === 'words' ? <WordsTab /> : taxonomy ? (
        <ColorsTab taxonomy={taxonomy} selected={selected} onSelect={select} />
      ) : !error && (
        <p className="text-sm" style={{ color: 'var(--text-muted)' }}>Loading…</p>
      )}
    </div>
  )
}
