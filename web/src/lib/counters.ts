/**
 * What a counter on a card means, and how it got there.
 *
 * **A chip that says `+3` is a number with nothing behind it.** The board has
 * drawn counters as coloured chips since #311 and every one of them has been a
 * figure with no sentence — green for adding, red for taking away, and no way
 * at all to learn what was added or why there are three of them. That is the
 * same fault the keyword marks had (`lib/keywords.ts`, and `KEYWORD_MEANS`
 * there is this file's model in every respect), found for the fourth time and
 * fixed the same way: `FieldHint` gives a mark a panel, and this decides what
 * goes in it.
 *
 * ## The phrasing rules, which are `KEYWORD_MEANS`'s
 *
 * What the counter *does* at a table, never the rule number; the consequence a
 * player will meet rather than the comprehensive rules' wording; and short
 * enough to be read in a panel over a moving board.
 *
 * ## The one rule that is this file's own: say nothing the card already says
 *
 * Most counters in Magic are *fuel for the card that made them* — charge, oil,
 * verse, quest, blood, page — and their whole meaning is printed on that card:
 * *"Remove three charge counters: ..."*. There is no general fact about a
 * charge counter to tell somebody, and inventing one would be this board
 * having an opinion about a card it is only counting for. So the fallback is
 * not a shrug, it is the true answer — **the card's own text is where that
 * rule lives** — and the table below holds only the counters whose rule is
 * *not* printed anywhere a player can see it: the ones that come from the
 * rulebook (`+1/+1`, `-1/-1`, loyalty), and the keyword-shaped ones a card
 * mentions once by name and never explains (stun, shield, lore, defence).
 *
 * That is why this table is short and is meant to stay short. A counter kind
 * added here has to be one where a player looking at the card in front of them
 * would still not know.
 */

import { type CounterMoment } from './board'

/**
 * Which way a counter cuts.
 *
 * **The sign is on the kind, not on the number.** `n` is how many counters of
 * that kind are on the card and it is a count — it is never negative. A single
 * -1/-1 arrives as `{kind: '-1/-1', n: 1}`, and reading the sign off `n` drew
 * it as a cheerful green `+1`, which is the exact opposite of the news.
 *
 * Three answers rather than two, because most counters are neither: charge,
 * loyalty, quest and stun counters are not good or bad, they are just counters,
 * and colouring them green would be the board having an opinion it has no
 * basis for.
 *
 * Lived in `components/board.tsx` until the chips learned to talk; it is here
 * now because the panel and the colour have to agree about the same word, and
 * two files deciding that separately is two files that will one day disagree.
 */
export function counterSign(kind: string): 'up' | 'down' | 'flat' {
  if (kind.startsWith('-')) return 'down'
  if (kind.startsWith('+')) return 'up'
  return 'flat'
}

/** A counter written the way Magic writes the pumping ones: two signed figures
 *  over a slash. Matched rather than listed, because the family is open —
 *  `+1/+1` and `-1/-1` are almost all of it, and `+1/+0`, `-0/-1` and `+2/+2`
 *  all exist on real cards. The two halves are captured so the sentence can be
 *  built from the card's actual figures instead of assuming the common one. */
const SIZED = /^([+-])(\d+)\/([+-])(\d+)$/

/**
 * What each named counter means, in one plain line.
 *
 * Keyed lowercase and looked up lowercased, for `drawableKeywords`' reason:
 * these strings come from Forge, which builds a counter's display name from an
 * enum constant, and which case reaches a browser is not something a board
 * should be sensitive to.
 *
 * **Deliberately not exhaustive.** See the file comment — a counter whose rule
 * is printed on the card that made it is answered better by the card.
 */
export const COUNTER_MEANS: Record<string, string> = {
  loyalty: 'a planeswalker\'s life. Its abilities put loyalty on and take it '
    + 'off, and at nought it goes to the graveyard.',
  stun: 'the next time this would untap, one of these comes off instead and it '
    + 'stays turned.',
  shield: 'if this would be destroyed or dealt damage, remove one of these '
    + 'instead and nothing happens to it.',
  lore: 'a Saga\'s chapter. One arrives each turn, and the Saga is sacrificed '
    + 'after the last chapter has happened.',
  defense: 'a battle\'s armour. Damage takes them off, and the battle flips '
    + 'over when the last one goes.',
  level: 'how far this creature has levelled up. Its own text says what it '
    + 'becomes at each tier.',
  time: 'one comes off at the start of each of its controller\'s turns, and '
    + 'something happens to the card when the last one does.',
}

/** One counter chip's explanation, in the three parts `FieldHint` sets it in.
 *  The same shape as `KeywordSaid`, on purpose — one panel, one grammar. */
export interface CounterSaid {
  /** The counter's own name, which is the panel's heading and the chip's
   *  accessible name. Carries the count, because *three* +1/+1 counters is
   *  what the chip is showing and a heading that said "+1/+1" beside a chip
   *  reading "+3" would be two different answers to one question. */
  name: string
  /** What this kind of counter does, in one plain line. */
  says: string
  /** How this card came by them — "two on turn 4, one more on turn 6" — or
   *  absent for a card whose counters arrived before the account starts. */
  note?: string
}

/** Small numbers as a person says them. Past ten a figure reads faster than a
 *  word does, which is the same threshold prose style guides land on. */
const SPOKEN = ['nought', 'one', 'two', 'three', 'four', 'five', 'six', 'seven',
  'eight', 'nine', 'ten']

function spoken(n: number): string {
  return n >= 0 && n < SPOKEN.length ? SPOKEN[n] as string : String(n)
}

/** How many moments of the account a panel will carry. Three is what fits
 *  under two lines of sentence without the panel becoming the thing you have
 *  to read instead of the board; the earlier ones are elided rather than
 *  dropped silently, so a trimmed account still says it was trimmed. */
const MOMENTS_SHOWN = 3

/**
 * How a card came by the counters it is carrying, in words.
 *
 * **This is Aaron's ask from 2026-08-26 — *"keep a history of why a creature
 * has all of the counters it does"* — arriving somewhere a person can read
 * it.** `foldBoard` has accumulated `BoardCard.counterHistory` since then and
 * nothing has ever rendered a line of it: the account existed, was tested, and
 * was invisible. A chip showing `+3` is exactly the place it was for — by the
 * time a creature has three, the arithmetic that made three has scrolled past.
 *
 * Deltas rather than totals, because that is how the turn is remembered: *two
 * on turn four, one more on turn six*, not *0→2, 2→3*. The first moment is
 * phrased as an arrival when it starts from nothing and as an addition when it
 * does not, which is the honest reading of a trimmed history — the counters
 * were already there and this is what happened next.
 *
 * Empty when there is nothing to say. A card that walked onto the battlefield
 * wearing counters has no moments at all, and a note reading "no account" is
 * worse than no note: it draws attention to a hole instead of to the card.
 */
export function counterStory(history: CounterMoment[], kind: string):
string | undefined {
  const mine = history.filter((m) => m.kind === kind && m.now !== m.was)
  if (mine.length === 0) return undefined
  const trimmed = mine.length > MOMENTS_SHOWN
  const shown = trimmed ? mine.slice(-MOMENTS_SHOWN) : mine
  const said = shown.map((m, i) => {
    const step = m.now - m.was
    const turn = `turn ${m.turn}`
    if (step < 0) return `${spoken(-step)} off on ${turn}`
    // "one more" for everything after the first, and for a first moment that
    // starts from a number — a history this board trimmed, or a card that
    // arrived already carrying some.
    const more = i > 0 || m.was > 0 || trimmed
    return more ? `${spoken(step)} more on ${turn}` : `${spoken(step)} on ${turn}`
  })
  return `${trimmed ? '… ' : ''}${said.join(', ')}.`
}

/**
 * One counter chip, said in full: what it is, what it does, and how it got
 * here.
 *
 * **The heading carries the count and the meaning does not**, which is the
 * split `keywordSaid` makes for the same reason. *Three +1/+1 counters* is a
 * fact about this creature; *each one makes it a point bigger* is a fact about
 * the counter, and a sentence that mixed them would have to be rewritten every
 * time the number moved.
 */
export function counterSaid(kind: string, n: number,
  history: CounterMoment[] = []): CounterSaid {
  const story = counterStory(history, kind)
  return {
    name: `${n} ${kind} counter${n === 1 ? '' : 's'}`,
    says: counterMeans(kind),
    ...(story ? { note: story } : {}),
  }
}

/**
 * What one kind of counter does, in a sentence.
 *
 * Three answers in order, and the order is the argument. A `+1/+1`-shaped name
 * is built from its own figures, so the board is never wrong about a `+2/+2`
 * by having assumed a `+1/+1`. A name in [COUNTER_MEANS] gets the rulebook's
 * fact. Everything else gets **the true answer rather than a guess**: the card
 * that made these counters is where their rule is written, and this board is
 * only counting them.
 */
export function counterMeans(kind: string): string {
  const sized = SIZED.exec(kind)
  if (sized) {
    const [, ps, pn, ts, tn] = sized
    const power = Number(pn), tough = Number(tn)
    const up = ps === '+'
    // Both halves move the same way on every counter Magic prints, so the
    // sentence is written for that and the odd one out falls through to the
    // plainest true reading rather than to a wrong tidy one.
    if (ps !== ts) {
      return `each one changes this creature's power by ${ps}${power} and its `
        + `toughness by ${ts}${tough}, for as long as it stays on the field.`
    }
    const both = power === tough
    const size = both
      ? `${spoken(power)} bigger in both figures`
      : `${spoken(power)} bigger in power and ${spoken(tough)} in toughness`
    if (up) {
      return `each one makes this creature ${size}, and it stays that way for `
        + `as long as the creature does.`
    }
    const smaller = both
      ? `${spoken(power)} smaller in both figures`
      : `${spoken(power)} smaller in power and ${spoken(tough)} in toughness`
    return `each one makes this creature ${smaller}. A creature whose `
      + `toughness reaches nought dies.`
  }
  const known = COUNTER_MEANS[kind.toLowerCase()]
  if (known) return known
  return 'what these do is written on the card that put them here — this board '
    + 'only counts them.'
}
