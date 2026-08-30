/**
 * Settings — the two things you decide *per deck*, in one room.
 *
 * The gear in the header keeps the preferences that are about **you** — the
 * theme, the ambience, the table's sound, how much Claude does. Those are four
 * switches and they live happily in a popup. These two are about **your
 * decks**, one row each, and a popup is the wrong container for a list that
 * grows: it is 320px wide, and it closes on Escape and on any mousedown
 * outside itself, so a shelf of toggles firing writes inside it would dismiss
 * itself halfway down the list. So the gear gained a door instead, and this is
 * the room behind it.
 *
 * **Two flags, two master switches, one row per deck.**
 *
 * - **Who can see your decks** is the flag that already existed, reached
 *   through the same one verb the deck page's own control uses. A shared deck
 *   is readable by anybody signed in here and by nobody else — there are no
 *   unlisted links and nothing outside the door (ADR 22), which is why the
 *   copy never says "public".
 * - **Coliseum at Night** is new, and **it does nothing yet**. That is not a
 *   caveat to bury: a switch that reads as though it starts something tonight
 *   would be the interface lying, and this room is the one place a person
 *   decides it. So every sentence about it is in the future tense, and the
 *   panel says the torches are unlit rather than implying a fire.
 *
 * **The master switch is three-state, and it has to be.** A shelf where some
 * decks are shared and some are not is the ordinary case — it is what every
 * library looks like before anybody visits this page — and a two-state control
 * would have to describe it as either "on" or "off", both of which are false.
 * `aria-pressed="mixed"` is the ARIA value for exactly this, so the switch says
 * "some" out loud rather than only drawing a half-filled box a sighted person
 * could read.
 *
 * **Only decks you can write appear here**, which is the same set the server
 * sweeps: `writable` on the shelf is the client's half of it, and the master
 * routes resolve whose decks they are from who is asking rather than from
 * anything this page sends. So the two cannot disagree about scope.
 *
 * **A showcase deck cannot be entered for the night games**, and the row says
 * so rather than offering a switch that fails. The flag is kept in one place
 * and the showcase's decks are not in it — Aaron's ruling, and the honest
 * consequence of it. The server refuses with a sentence too; this is so nobody
 * has to press anything to find out.
 */

import { useCallback, useEffect, useMemo, useState } from 'react'
import { Link } from 'react-router-dom'

import { type DeckTile, api, deckUrl, errorMessage } from '../lib/api'
import { ErrorNote, PageMasthead, Spinner } from '../components/ui'
import { FieldHint } from '../components/hint'

/** Grand Coliseum's original printing. The arena the night games will be
 *  played in, which is half of what this room decides — and a land, so it is
 *  a place rather than a person, which is what a settings room is. */
const COLISEUM_ART =
  'https://cards.scryfall.io/art_crop/front/c/2/c2dc8061-a855-4a81-9eb7-350b355a9b3f.jpg'

/** Which of the two flags a control is about. The string is the wire key, so
 *  a row and its master switch cannot drift apart. */
type Flag = 'shared' | 'coliseum_at_night'

/** What a master switch shows: every deck, none of them, or some. Mixed is the
 *  ordinary state and is named rather than treated as a leftover. */
type Tri = 'all' | 'none' | 'some'

function tally(decks: DeckTile[], flag: Flag): Tri {
  const on = decks.filter((d) => d[flag]).length
  if (on === 0) return 'none'
  if (on === decks.length) return 'all'
  return 'some'
}

/** The ARIA value for a three-state toggle. `"mixed"` is a real `aria-pressed`
 *  value and is the only way this control can be honest to a screen reader
 *  about a half-shared shelf. */
function pressed(state: Tri): 'true' | 'false' | 'mixed' {
  return state === 'all' ? 'true' : state === 'none' ? 'false' : 'mixed'
}

/** English for a count of decks, because "1 decks" is the kind of small wrong
 *  thing that makes a careful page feel careless. */
function decksWord(n: number): string {
  return n === 1 ? '1 deck' : `${n} decks`
}

/** The question mark a `FieldHint` hangs its panel on. Drawn rather than
 *  typed so it sits on the baseline at any size and carries the room's ink. */
function HintMark() {
  return (
    <span aria-hidden
          className="inline-flex h-[15px] w-[15px] items-center justify-center
                     rounded-full text-[10px] font-semibold leading-none"
          style={{ border: '1px solid var(--hairline)', color: 'var(--text-muted)' }}>
      ?
    </span>
  )
}

/* ------------------------------------------------------------ the switches */

/**
 * One deck's answer to one flag.
 *
 * A `.chip-toggle` and never a link (commandment 20): it changes what is on
 * this page rather than taking anybody anywhere, so it is a button, it carries
 * `aria-pressed`, and the stylesheet's hover, focus and press all reach it
 * because nothing here sets an inline `color` for a `:hover` to lose to.
 *
 * The label says the **state**, not the action, which is right for a toggle
 * and is the opposite of the deck page's share button — that one is a lone
 * control whose label has to carry the whole sentence, and this one is a chip
 * in a column of chips under a heading that already said what the column is.
 */
function DeckFlagChip({ label, on, disabled, busy, deckName, columnName, onToggle }: {
  label: string
  on: boolean
  disabled?: boolean
  busy: boolean
  deckName: string
  columnName: string
  onToggle: () => void
}) {
  return (
    <button
      type="button"
      aria-pressed={on}
      disabled={disabled || busy}
      onClick={onToggle}
      // The deck's name is in the accessible name because a screen reader
      // meeting the fourth "Shared" of the page has no other way to know which
      // deck it belongs to. The eye has the row for that; the ear has this.
      aria-label={`${columnName}: ${deckName}`}
      className={`chip-toggle rounded-full px-3 py-1 text-xs font-medium${on ? ' is-on' : ''}`}
    >
      {busy ? '…' : label}
    </button>
  )
}

/**
 * The master switch for one column: every deck at once.
 *
 * Pressing it makes the shelf uniform — all on unless they are already all on,
 * in which case all off. From "some", a press always turns everything **on**,
 * which is the behaviour that matches what the control looks like: a half-lit
 * switch reads as unfinished, and finishing it means filling it.
 */
function MasterSwitch({ state, tally: count, busy, disabled, columnName, verb, onPress }: {
  state: Tri
  /** How many decks wear this flag now, and out of how many. Shown rather
   *  than hidden for a screen reader: "2 of 5" is the fact somebody arriving
   *  at a half-lit switch actually wants, and there is no version of that
   *  which the eye needs less than the ear. */
  tally: { on: number; of: number }
  busy: boolean
  disabled?: boolean
  columnName: string
  /** What a press will do, in words, under the switch. */
  verb: string
  onPress: () => void
}) {
  const label = state === 'all' ? 'All of them'
    : state === 'none' ? 'None of them' : 'Some of them'
  return (
    <div className="flex flex-col gap-1.5">
      <button
        type="button"
        aria-pressed={pressed(state)}
        disabled={disabled || busy}
        onClick={onPress}
        aria-label={`${columnName}: every deck`}
        className={`chip-toggle self-start rounded-full px-3.5 py-1.5 text-xs font-semibold${
          state === 'all' ? ' is-on' : ''}${state === 'some' ? ' is-part' : ''}`}
      >
        {busy ? 'Working…' : label}
      </button>
      <span className="text-[11px]" style={{ color: 'var(--text-muted)' }}>
        {busy
          ? 'Turning the whole shelf over…'
          : <>{count.on} of {count.of} · {verb}</>}
      </span>
    </div>
  )
}

/* ---------------------------------------------------------------- the room */

export default function Settings() {
  const [decks, setDecks] = useState<DeckTile[] | null>(null)
  const [error, setError] = useState<string | null>(null)
  /** Which single control is mid-write. One at a time on purpose: two writes
   *  against one shelf would race to re-read it, and the loser would paint a
   *  state that is one press out of date. */
  const [busy, setBusy] = useState<string | null>(null)

  /**
   * Re-read the shelf, and **return** what went wrong rather than posting it.
   *
   * The returning is the whole point and it was a bug before it was a
   * decision: this runs again after every write, including a refused one, so a
   * version that set the error itself cleared the refusal it had just been
   * told about one line earlier. The page reported a successful-looking
   * nothing. Whoever owns the error decides what it says; this only reports.
   */
  const load = useCallback(async (): Promise<string | null> => {
    try {
      const all = await api.decks()
      // Yours, and only yours. The shelf carries everybody's shared decks and
      // the showcase too, and neither is this page's business — you cannot
      // change a flag on a deck you do not own, so offering the switch would
      // be offering a refusal.
      setDecks(all.filter((d) => d.writable))
      return null
    } catch (e) {
      setDecks([])
      return errorMessage(e)
    }
  }, [])

  // The first read. `live` is the house's usual guard and earns its keep here
  // for the ordinary reason: this fetch outlives a quick visit, and a page
  // that has already been navigated away from must not set state.
  useEffect(() => {
    let live = true
    void (async () => {
      const failure = await load()
      if (live) setError(failure)
    })()
    return () => { live = false }
  }, [load])

  const mine = useMemo(() => decks ?? [], [decks])
  const sharedState = tally(mine, 'shared')
  // The night column counts only the decks that can actually hold the flag,
  // because a master switch that reported "none of them" over decks it cannot
  // reach would be describing a refusal as a choice.
  const nightEligible = useMemo(() => mine.filter((d) => !d.showcase), [mine])
  const nightState = tally(nightEligible, 'coliseum_at_night')

  async function write(key: string, job: () => Promise<unknown>) {
    setBusy(key)
    setError(null)
    let refusal: string | null = null
    try {
      await job()
    } catch (e) {
      refusal = errorMessage(e)
    }
    // **Re-read whether or not it worked.** A master switch can be refused
    // partway through the shelf, which leaves it in a state this page has
    // never seen — and showing the shelf we *hoped* for beside the error
    // saying it did not happen is the more confident of the two wrong
    // answers. So the flags on screen always come from the server.
    const unread = await load()
    // The write's own refusal wins: it is the news the person was waiting
    // for. A failure to re-read is reported only when nothing worse happened.
    setError(refusal ?? unread)
    setBusy(null)
  }

  const toggleDeck = (deck: DeckTile, flag: Flag) => {
    const ref = { owner: deck.owner, slug: deck.slug }
    const next = !deck[flag]
    void write(`${flag}:${deck.slug}`, () => (
      flag === 'shared'
        ? api.setShared(ref, next)
        : api.setColiseumAtNight(ref, next)
    ))
  }

  const toggleAll = (flag: Flag, state: Tri) => {
    const next = state !== 'all'
    void write(`${flag}:*`, () => (
      flag === 'shared'
        ? api.setEveryDeckShared(next)
        : api.setEveryDeckColiseumAtNight(next)
    ))
  }

  return (
    <div className="space-y-6">
      <PageMasthead
        art={COLISEUM_ART}
        alt="A vast tiered arena under an open sky, its stands full"
        title="Settings"
        credit={<>
          Grand Coliseum — Carl Critchlow, <em>Onslaught</em>. The arena the
          night games will be played in, standing empty until they are.
        </>}>
        <p>
          Two decisions, kept per deck: who is allowed to read a deck, and
          whether it goes down to the arena after dark. Everything else the app
          remembers about you lives behind the gear in the header.
        </p>
      </PageMasthead>

      {error && <ErrorNote>{error}</ErrorNote>}

      {decks === null && <Spinner label="Fetching your shelf…" />}

      {decks !== null && mine.length === 0 && !error && (
        <div className="card-surface rounded-xl px-6 py-10 text-center">
          <p className="text-sm" style={{ color: 'var(--text-secondary)' }}>
            You have no decks of your own yet — so there is nothing here to
            decide. Build one and this room fills up.
          </p>
          <Link to="/new" className="btn btn-quiet mt-4 inline-flex">
            Start a deck
          </Link>
        </div>
      )}

      {decks !== null && mine.length > 0 && (
        <>
          {/* The two master switches, each in its own panel with the sentence
              that explains the column it governs. Side by side where there is
              room, stacked on a phone. */}
          <div className="grid gap-4 sm:grid-cols-2">
            <section className="card-surface rounded-xl p-5">
              <h2 className="flex items-center gap-2 text-sm font-semibold tracking-tight">
                Who can see your decks
                <FieldHint
                  name="Sharing a deck"
                  says="A shared deck can be read by anyone signed in here — the list, the reasons behind every card, the primers. Nobody outside this table ever sees it, and nobody but you can change it."
                  note="You can put a deck back to private at any time.">
                  <HintMark />
                </FieldHint>
              </h2>
              <p className="mt-1.5 text-xs" style={{ color: 'var(--text-secondary)' }}>
                A private deck is yours alone. A shared one is readable by the
                other players at this table — and by no one beyond it.
              </p>
              <div className="mt-3">
                <MasterSwitch
                  state={sharedState}
                  tally={{ on: mine.filter((d) => d.shared).length, of: mine.length }}
                  busy={busy === 'shared:*'}
                  columnName="Share"
                  verb={sharedState === 'all'
                    ? `Press to make all ${decksWord(mine.length)} private again.`
                    : `Press to share all ${decksWord(mine.length)}.`}
                  onPress={() => toggleAll('shared', sharedState)} />
              </div>
            </section>

            <section className="settings-night card-surface rounded-xl p-5">
              <h2 className="flex items-center gap-2 text-sm font-semibold tracking-tight">
                Coliseum at Night
                <FieldHint
                  name="Coliseum at Night"
                  says="A standing answer, given early. When the arena starts running games after dark, the decks entered here are the ones that will play."
                  note="Nothing happens tonight — the torches are not lit yet.">
                  <HintMark />
                </FieldHint>
              </h2>
              <p className="mt-1.5 text-xs" style={{ color: 'var(--text-secondary)' }}>
                The torches are not lit yet. When the arena opens after dark,
                the decks entered here are the ones that will be called down —
                so you can choose now and be on the card for the first night.
              </p>
              <div className="mt-3">
                <MasterSwitch
                  state={nightState}
                  tally={{
                    on: nightEligible.filter((d) => d.coliseum_at_night).length,
                    of: nightEligible.length,
                  }}
                  busy={busy === 'coliseum_at_night:*'}
                  disabled={nightEligible.length === 0}
                  columnName="Enter for the night games"
                  verb={nightEligible.length === 0
                    ? 'None of your decks can be entered yet.'
                    : nightState === 'all'
                      ? `Press to withdraw all ${decksWord(nightEligible.length)}.`
                      : `Press to enter all ${decksWord(nightEligible.length)}.`}
                  onPress={() => toggleAll('coliseum_at_night', nightState)} />
              </div>
            </section>
          </div>

          {/* One row per deck, both flags on it. A list rather than a table:
              two chips and a name wrap onto a phone without a horizontal
              scroller, and a table of three columns would need one. */}
          <section className="card-surface rounded-xl p-5">
            <div className="flex items-baseline justify-between gap-4">
              <h2 className="text-sm font-semibold tracking-tight">
                Your {decksWord(mine.length)}
              </h2>
              <span className="text-xs" style={{ color: 'var(--text-muted)' }}>
                Each one decides for itself
              </span>
            </div>
            <ul className="mt-3 divide-y" style={{ borderColor: 'var(--hairline)' }}>
              {mine.map((deck) => (
                <li key={`${deck.owner}/${deck.slug}`}
                    className="flex flex-wrap items-center gap-x-4 gap-y-2 py-3">
                  <span className="min-w-0 flex-1">
                    {/* A real destination, so a real link — commandment 20's
                        other half. The two controls beside it change this
                        page and are buttons for exactly that reason. */}
                    <Link to={deckUrl({ owner: deck.owner, slug: deck.slug })}
                          className="btn btn-ghost btn-ghost-accent btn-xs font-medium">
                      {deck.name}
                    </Link>
                    <span className="block text-[11px]"
                          style={{ color: 'var(--text-muted)' }}>
                      {deck.commander.length > 0
                        ? deck.commander.join(' & ')
                        : 'No commander named yet'}
                    </span>
                  </span>
                  <DeckFlagChip
                    columnName="Share"
                    deckName={deck.name}
                    label={deck.shared ? 'Shared' : 'Private'}
                    on={deck.shared}
                    busy={busy === `shared:${deck.slug}`}
                    onToggle={() => toggleDeck(deck, 'shared')} />
                  {deck.showcase ? (
                    // Not a disabled switch with a tooltip: a control that
                    // cannot be operated should not look like one waiting to
                    // be. The sentence takes its place.
                    <span className="text-[11px] italic"
                          style={{ color: 'var(--text-muted)' }}>
                      Not yet open to showcase decks
                    </span>
                  ) : (
                    <DeckFlagChip
                      columnName="Enter for the night games"
                      deckName={deck.name}
                      label={deck.coliseum_at_night ? 'In for the night' : 'Not entered'}
                      on={deck.coliseum_at_night}
                      busy={busy === `coliseum_at_night:${deck.slug}`}
                      onToggle={() => toggleDeck(deck, 'coliseum_at_night')} />
                  )}
                </li>
              ))}
            </ul>
          </section>
        </>
      )}
    </div>
  )
}
