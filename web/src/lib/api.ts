/** Typed client for the mtglab API. Mirrors the server's /api route families. */

export interface Health {
  pool: boolean
  oracle_cards: number
  printings: number
  bulk_files?: string[]
  decks?: number
  message?: string
}

/**
 * A deck's full address (ADR 22): whose it is, and which one.
 *
 * Slugs are unique **per owner** rather than globally, so a slug on its own no
 * longer identifies a deck and every deck call takes one of these.
 *
 * An object rather than two positional strings, and that is the whole reason it
 * is a type at all: `deck(owner, slug)` and `deck(slug, owner)` are both two
 * strings, so transposing them is a runtime 404 against somebody else's
 * library. Named fields make it a compile error instead.
 */
export interface DeckRef {
  owner: string
  slug: string
}

/** The path prefix for one deck, with an optional tail like `/validate`.
 *
 * Both segments are encoded. Neither can currently need it — a slug is
 * `[a-z0-9-]` and `users.USERNAME_RE` allows only letters, digits and `.` `_`
 * `-` — but this function is handed whatever was in the address bar, and the
 * cost of being right about that is one call.
 */
function deckPath({ owner, slug }: DeckRef, tail = ''): string {
  return `/api/decks/${encodeURIComponent(owner)}/${encodeURIComponent(slug)}${tail}`
}

/** Where this deck lives in the app's own router. Mirrors `deckPath`. */
export function deckUrl({ owner, slug }: DeckRef): string {
  return `/decks/${encodeURIComponent(owner)}/${encodeURIComponent(slug)}`
}

export interface DeckSummary {
  slug: string
  /** The account this deck belongs to, and half of its address (ADR 22). With
   *  auth off it is the literal `local`: one person, holding the file the app
   *  reads. */
  owner: string
  /** Whether anybody signed in may read this deck, or only its owner.
   *
   *  Never a claim about somebody *else's* deck being private — a private deck
   *  of another owner is not in any response this client sees at all. So this
   *  is only ever interesting on your own decks, where it is the answer to
   *  "is this one on display". */
  shared: boolean
  /** Who sleeves this deck up (second 2026-08-15 punch list, item 10): a
   *  household tag — "Mark's wife", "the kids" — not an account. The owner
   *  says whose library it lives in; this says which human plays it. Empty
   *  means untagged. */
  pilot: string
  name: string
  /** "built" = the cards are sleeved up; "theoretical" = a list under consideration. */
  status: string
  /**
   * "curated" = every card justifies its slot; "draft" = imported and not yet
   * reasoned about. Orthogonal to `status` — all four combinations are real.
   */
  stage: string
  /** Whether this viewer may change this deck. See `DeckDetail.writable` —
   *  same field, carried on the shelf so the grid can decide whether to offer
   *  a delete control. A courtesy; the server refuses independently. */
  writable: boolean
  /** Cards with no `why` yet. A draft's to-do list, as a number. */
  needs_rationale: number
  commander: string[]
  companion: string | null
  bracket: number | null
  /** What this deck is about, declared in `deck.yaml` from the vocabulary
   *  `GET /api/themes` serves. Several per deck; empty means undeclared,
   *  which is not the same as "about nothing". */
  themes: string[]
  /** ADR 37's *reading* of `themes`, never a separate declaration: the
   *  worst-Forge-piloted class word among those declared, or null when none
   *  is. Read-only on the wire — writing `themes` is what changes it. */
  archetype: string | null
  total_cards: number
  land_count: number
  strategy: string
  art_crop: string | null
  color_identity: string[]
  // Gate counts, so the shelf can flag a deck that does not validate. null
  // means the pool was unavailable and the gate never ran -- which is not
  // the same as passing, and must not render as a pass.
  errors: number | null
  warnings: number | null
}

/**
 * A deck as the library shelf receives it: a summary, plus where it sits.
 *
 * `showcase` is on this and not on `DeckSummary` because it is a fact about the
 * *library view* rather than about a deck — `GET /api/decks/{owner}/{slug}` has
 * no such field and a type claiming one there would be fiction.
 *
 * It says this owner is the curated six's: the maintainer, or `local` on a
 * laptop. ADR 22 says the showcase is always visible, so it sits in the default
 * shelf beside your own decks while everybody else's go behind the browse tab.
 * The client cannot work this out for itself — `writable` identifies *your*
 * decks and nothing else identifies the maintainer's.
 */
export interface DeckTile extends DeckSummary {
  showcase: boolean
}

export interface Card {
  name: string
  category: string
  why: string
  qty: number
  known: boolean
  /** Which printing's art this deck picked for the slot, or "" for the
   *  pool's default. The card-level `commander_art`. */
  art?: string
  mana_cost?: string | null
  cmc?: number
  type_line?: string
  oracle_text?: string
  color_identity?: string[]
  image?: string | null
  art_crop?: string | null
  edhrec_rank?: number | null
  reserved?: boolean
  price_usd?: number | null
  // Printed stats, so a claim about "a 6/6" is checkable against the card.
  power?: string | null
  toughness?: string | null
  loyalty?: string | null
  game_changer?: boolean
  // Only on `commander_card`, which is the only place the hero panel reads
  // them from — `service._card_json` sends these under `full=True` rather
  // than on all 99 rows. Flavour text belongs to a printing, so plenty of
  // cards legitimately have none.
  flavor_text?: string | null
  artist?: string | null
  /** Also commander-only: the key the motion tier (ADR 32) is cached on.
   *  Null when the pool is absent or the name did not resolve. */
  oracle_id?: string | null
  /** Set only when the deck picked a specific printing for its commander, so
   *  the header can name which painting it is showing. Its absence means the
   *  pool's default printing. */
  printing?: { set_name: string | null; set_code: string } | null
}

/**
 * One card the finder is offering, for a name somebody is still typing.
 *
 * Deliberately not a `Card`: a `Card` is a card *in a deck*, and carries the
 * three fields that only exist once it is one — `category`, `why`, `qty`. A
 * card that has not been added yet has none of them, and a shape that
 * pretended otherwise would be one optional field away from a surface writing
 * a rationale, which is the one thing no surface may do (rule 4, ADR 8).
 */
export interface CardOffer {
  name: string
  mana_cost: string | null
  type_line: string
  oracle_text: string
  color_identity: string[]
  /** The whole card, hot-linked (ADR 6). Never proxied, never re-hosted, and
   *  never filtered, cropped or desaturated (ADR 32, commandment 9). */
  image: string | null
  /** Owed a credit line wherever the painting renders. */
  artist: string | null
  legal_commander: boolean
  /** The one category a card pool fact can fill — the importer's own rule,
   *  right about the double-faced cards a type line is wrong about. Every
   *  other category is an opinion and stays the user's. */
  is_land: boolean
  /** How alike the name is to what was typed. */
  score: number
  /** Which tier offered it: the name is `exact`ly what was typed, `holds`
   *  it somewhere, holds every one of its `words`, or is `near` it — the last
   *  being the only one that is a guess, and the only one the interface says
   *  so about. */
  via: 'exact' | 'holds' | 'words' | 'near'
}

export interface DeckDetail extends DeckSummary {
  notes: Record<string, string>
  commander_card: Card | null
  cards: Card[]
  swap_board: Card[]
  /** Entombed cards (ADR 27): out of the 99 but not gone, each keeping the
   *  category and `why` it left with. Newest first. */
  graveyard: Card[]
  pool_available: boolean
  /** The Scryfall printing id whose art this deck shows, or '' for the
   *  default. A deck property rather than a viewer preference: it lives in
   *  `deck.yaml` and travels with the deck. */
  commander_art: string
  /** Whether this viewer may change this deck.
   *
   *  Read from the server rather than derived from `auth.is_admin`, and the
   *  difference matters: today those two are the same answer, because there
   *  is one library and it is the maintainer's. When decks have owners they
   *  stop being the same answer, and a component gating on `is_admin` would
   *  be quietly wrong rather than loudly broken.
   *
   *  **Hiding a control is a courtesy, not a defence.** Every write route
   *  refuses independently with a 403; this only stops the app offering
   *  somebody a button that cannot work. */
  writable: boolean
}

/** One printing of the commander, as the art picker offers it. */
export interface Printing {
  id: string
  set_code: string
  set_name: string | null
  collector_number: string | null
  rarity: string | null
  released_at: string | null
  promo: boolean
  image: string | null
  art_crop: string | null
  price_usd: number | null
  selected: boolean
}

export interface PrintingList {
  slug: string
  commander: string
  selected: string
  printings: Printing[]
}

export interface Issue {
  code: string
  message: string
  card: string | null
}

export interface ValidationReport {
  ok: boolean
  errors: Issue[]
  warnings: Issue[]
}

export interface CurveBucket {
  mv: number
  count: number
  names: string[]
}

export interface CategoryRow {
  category: string
  count: number
  target_low: number | null
  target_high: number | null
  status: 'ok' | 'low' | 'high' | null
}

export interface ColorNeed {
  color: string
  pips: number
  sources: number
  cards: number
  sources_per_pip: number
}

export interface DeckStats {
  slug: string
  name: string
  bracket: number | null
  total_cards: number
  land_count: number
  curve: { average_mv: number; nonland_cards: number; buckets: CurveBucket[] }
  categories: CategoryRow[]
  colors: ColorNeed[]
  types: Record<string, number>
  /** Which Game Changers the deck runs against its bracket's allowance.
   *  `allowed: null` is an unlimited bracket; `verdict: "unknown"` means
   *  nobody could look (no pool, or no declared bracket) — not zero. */
  game_changers: {
    cards: string[]
    count: number
    allowed: number | null
    bracket: number | null
    verdict: 'ok' | 'over' | 'unknown'
  }
  /** Hypergeometric draw odds (punch list 2026-08-15 item 6) — arithmetic on
   *  the deck file's own counts, no pool and no simulation. Castability
   *  stays Tier 1's job; the caveat rendered beside these says so. */
  opening: {
    deck_size: number
    hand_size: number
    lands: {
      count: number
      distribution: { lands: number; chance: number }[]
      keepable: number
    }
    categories: {
      category: string
      count: number
      in_opening_hand: number
      by_turn_four: number
    }[]
    singleton: { turn: number; cards_seen: number; chance: number }[]
  }
}

/** One turn of the Wheel of Fortune — a fate, and (usually) a card in the
 *  deck's colours that answers to it. `answered_by: "dice"` is the wire's
 *  token for chance rather than judgment — seeded rolls over the pool, never
 *  a model. Clients key on the token; nothing ever renders it. */
export interface WheelSpin {
  pool_available: boolean
  symbol: 'cup' | 'heart' | 'sword' | 'skull' | null
  label?: string
  meaning?: string
  /** The cup's second landing: which way the coin fell. */
  coin?: 'heads' | 'tails'
  /** The heart's second landing: whole, or broken. */
  heart_face?: 'whole' | 'broken'
  /** The sword's second landing: which way the winning blade presents. */
  sword_face?: 'edge' | 'hilt'
  /** The skull's second landing: the grave takes, or gives back. */
  skull_face?: 'buried' | 'risen'
  seed?: number
  answered_by?: string
  caveat?: string
  message?: string
  reason?: string
  card: {
    name: string
    mana_cost: string | null
    type_line: string
    oracle_text: string
    color_identity: string[]
    image: string | null
    art_crop: string | null
  } | null
}

/** One card as it lies on the table in a dealt opening hand. `turn` is the
 *  earliest turn it could be cast off the lands in that same hand, or null
 *  when those lands never pay for it — and always null for a land, which is
 *  played rather than cast. */
export interface DealtCard {
  name: string
  mana_cost: string | null
  type_line: string
  image: string | null
  mana_value: number
  is_land: boolean
  turn: number | null
}

/** The counting beside a dealt hand. Arithmetic and nothing else: there is
 *  deliberately no verdict field, no score and no keep-or-mulligan call
 *  (ADR 14, and the argument is in `go/internal/sim/opening`). */
export interface HandReading {
  lands: number
  spells: number
  /** The commander's colours, split by whether a land in the hand makes them. */
  colors_covered: string[]
  colors_missing: string[]
  /** Earliest turn any spell here could be cast, or null for none of them. */
  first_spell_turn: number | null
  castable_by_horizon: number
  /** The turn `castable_by_horizon` counts through — served, never assumed. */
  horizon: number
}

/** Seven cards off a real shuffle of this deck, and the counting beside
 *  them. There is no seed on this wire in either direction: the shuffle is
 *  the server's and a practice hand is not a fortune anybody replays. */
export interface OpeningHand {
  pool_available: boolean
  cards: DealtCard[]
  reading?: HandReading
  /** How many cards were actually shuffled — the 99, never the commander. */
  deck_size?: number
  declared_size?: number
  /** Names the pool did not know, which is why `deck_size` can be short. */
  unresolved_count?: number
  commander?: DealtCard | null
  answered_by?: string
  caveat?: string
  message?: string
}

/** One token this deck can put onto the battlefield, and the cards that make
 *  it. The picture and its credit are absent together or present together —
 *  no library has a printing of every token, and a painting credited to
 *  nobody is worse than no painting. */
export interface TokenPlate {
  name: string
  /** Scryfall's own classification: "Token Creature — Cat Warrior". */
  type_line: string
  image: string | null
  art_crop: string | null
  artist: string | null
  set_code: string | null
  set_name: string | null
  /** This deck's own cards, spelled as the deck spells them, sorted. */
  made_by: string[]
}

/** What a deck makes.
 *
 *  `read` is the honest third state, and it is not the same as an empty
 *  `tokens`: false means the library has not been read for this yet, true with
 *  an empty list means it was read and this deck makes nothing. Showing one
 *  sentence for both would tell somebody something false about their deck. */
export interface DeckTokens {
  pool_available: boolean
  read: boolean
  tokens: TokenPlate[]
  message?: string
}

export interface TurnRow {
  turn: number
  lands: number
  mana: number
  unused: number
  spells: number
  commander_down: number
  /** P(no land to play this turn) — the drop that could not be made. */
  missed_drop: number
}

/** When one card comes online, over many games (second 2026-08-15 punch
 *  list, item 11). Served sorted worst-first: never-cast, then latest
 *  medians. */
export interface CardTimingRow {
  name: string
  mv: number
  cast_rate: number
  median_turn: number | null
  by_t8: number
}

/** What every Tier 1 result carries about its own provenance.
 *
 * `seed` is on the payload because a simulation is only reproducible if you
 * know which sample you are looking at, and `cached` because a figure served
 * from a previous run is honest only when it says so. Both are rendered.
 */
export interface SimProvenance {
  seed: number
  cached: boolean
  /** When the numbers were computed. Null when they were computed just now. */
  computed_at: string | null
}

export interface ManaResult extends SimProvenance {
  deck_check?: DeckCheck
  slug: string
  deck_name: string
  games: number
  turns: number
  mulligan_rate: number
  avg_mulligans: number
  median_commander_turn: number | null
  never_cast_commander: number
  color_screw_rate: number
  by_turn: TurnRow[]
  median_first_spell_turn: number | null
  stalled_turns: number
  card_timings: CardTimingRow[]
  caveat: string
}

export interface LandRow {
  lands: number
  commander_by_t5: number
  spells_through_t8: number
  wasted_through_t8: number
  mulligan_rate: number
}

export interface LandResult extends SimProvenance {
  deck_check?: DeckCheck
  slug: string
  deck_name: string
  games: number
  rows: LandRow[]
  deployment_spread: number
  argmax_lands: number
  flat: boolean
  caveat: string
}

/** The gate's verdict on the deck a simulation just described.
 *
 * Carried on every Tier 1 and Tier 1.5 result, because a number about an
 * illegal deck is only honest if it says so. The simulator deliberately does
 * **not** refuse an invalid deck — two of the library's decks fail the gate on
 * a banned card by choice, and a deck mid-import fails it by construction, so
 * refusing would take the diagnosis away exactly when somebody wants it. The
 * one refused state is a deck with no cards in it, which has no answer.
 *
 * `affects_numbers` is the field that decides what to say. A missing rationale
 * blocks a curated deck and changes nothing about mana; a banned card is
 * sitting in the 99 being shuffled, and an unresolved card was dropped, which
 * shrinks the very population every probability is computed over. The server
 * decides which is which — see `movesTheNumbers` in its sim-run routes.
 */
export interface DeckCheck {
  ok: boolean
  error_count: number
  warning_count: number
  errors: { code: string; message: string; card: string | null }[]
  affects_numbers: boolean
  unresolved: string[]
  unresolved_count: number
  commander_unresolved: boolean
  declared_size: number
  simulated_size: number
}

/** One turn of the mana curve. */
export interface CurveTurnRow {
  turn: number
  from_lands: number
  from_ramp: number
  expected_mana: number
  /** P(a land available every turn up to and including this one). */
  land_drop_odds: number
  /** P(at least `turn` mana on `turn`). The headline. */
  odds: number
}

/** What to add, and which kind.
 *
 * `recommend` is the server's verdict and must not be re-derived here. The
 * rule behind it — lands up to the curve, ramp past it — is arithmetic, and a
 * second copy in TypeScript would be a second chance to get its *direction*
 * wrong, which is the one error nobody spots from a screenshot.
 */
export interface CurveAdvice {
  target_turn: number
  target_mana: number
  odds: number
  odds_per_land: number
  odds_per_ramp: number
  recommend: 'lands' | 'ramp' | 'either' | 'none'
  slots: number | null
  ramp_is_generic: boolean
  /** True when the mana target exceeds the turn — where a land is worth
   *  nothing at all and only ramp can help. */
  beyond_the_curve: boolean
  /** Lands that would make every land drop through the target turn. Almost
   *  always absurd, and carried for exactly that reason. */
  lands_for_every_drop: number | null
}

export interface ManaCurve {
  deck_size: number
  lands: number
  accelerants: number
  target_turn: number
  target_mana: number
  target: number
  turns: CurveTurnRow[]
  advice: CurveAdvice
  caveat: string
}

/** Tier 1.5, the closed form. One rung of a colour's ladder.
 *
 * A colour is reported as a ladder of these rather than as one verdict: a
 * deck short for its greediest triple-symbol card is not a deck short of that
 * colour, and collapsing the two tells a beginner their mana base is broken
 * when one card is greedy.
 */
export interface PipTierRow {
  pips: number
  turn: number
  need: number
  have: number
  met: boolean
  shortfall: number
  odds_now: number
  /** Capped server-side; `card_count` is the true total. */
  cards: string[]
  card_count: number
}

export interface ColorRequirementRow {
  color: string
  have: number
  have_lands: number
  met: boolean
  shortfall: number
  tiers: PipTierRow[]
}

/** One card's castability across turns 1..`horizon` — the heatmap's row.
 *
 * `on_curve` is null past the horizon rather than zero, because zero would be
 * a claim the shelf never made. `lag` is null when the card never becomes
 * reliable; the server has already sorted the list worst-first, so a client
 * rendering the head is showing the rows worth reading.
 */
export interface CardOddsRow {
  name: string
  mv: number
  on_curve: number | null
  reliable_turn: number | null
  lag: number | null
  by_turn: number[]
}

export interface LandEstimate {
  lands_now: number
  recommended: number
  delta: number
  average_mana_value: number
  cheap_accelerants: number
  caveats: string[]
}

/** The closed form for one deck. Note it carries no `SimProvenance`: it is
 *  computed in the request every time, so there is no seed to report and
 *  nothing to be cached-and-stale about. */
export interface ShelfResult {
  deck_check?: DeckCheck
  mana_curve?: ManaCurve
  slug: string
  deck_name: string
  deck_size: number
  lands: number
  target: number
  on_the_play: boolean
  horizon: number
  colors: ColorRequirementRow[]
  lands_estimate: LandEstimate
  cards: CardOddsRow[]
  approximated: string[]
  caveat: string
}

export interface PolicyRow {
  min_lands: number
  max_lands: number
  min_pieces: number
  describe: string
  spells_through_t8: number
  mulligan_rate: number
  avg_mulligans: number
  median_commander_turn: number | null
  color_screw_rate: number
  stalled_turns: number
}

/** The mulligan policy search.
 *
 * `flat` is the server's verdict and the client must not recompute it: it is
 * measured against the default rule rather than against the grid's range, and
 * a second implementation of that rule here would be a second chance to get
 * it wrong.
 */
export interface PolicyResult extends SimProvenance {
  deck_check?: DeckCheck
  slug: string
  deck_name: string
  games: number
  turns: number
  rows: PolicyRow[]
  best: PolicyRow
  baseline: PolicyRow
  gentlest: PolicyRow
  spread: number
  gain: number
  flat: boolean
  caveat: string
}

/** Whether Tier 3 is installed where the server runs (ADR 35). `why` is
 *  maintainer-facing prose and is never rendered — the client says it in its
 *  own words or, better, says nothing (the mode is honestly absent). */
export interface ForgeStatus {
  available: boolean
  why: string | null
}

export interface ForgeDeckRow {
  slug: string
  name: string
  address: string
  wins: number
}

export interface ForgeGameRow {
  game: number
  winner: string | null
  seconds: number
  turns: number | null
  draw: boolean
  timed_out: boolean
}

/** One beat of a game, as the Coliseum receives it.
 *
 * The server shapes these from Forge's own narration, and the shaping does one
 * thing worth knowing about: **a beat names a deck, never a seat.** `who` is
 * the slug of whoever acted and `against` the slug on the other end, both null
 * when the line named no player — which is most of them, because Forge usually
 * names the card instead.
 *
 * `kind` is a plain string rather than a union of the twelve the parser
 * raises. That is deliberate: a Forge release that learns a thirteenth beat
 * must not be a type error in a browser, and the room renders an unknown kind
 * as its own plain words rather than refusing to render the game. */
export interface ForgeBeat {
  kind: string
  turn?: number
  who: string | null
  card?: string
  /** The board id of `card`, so a beat can point at the exact permanent in
   *  the picture. Absent on a match played without the scribe, which has no
   *  ids — read a missing id as "not said", never as a card.
   *
   *  A name cannot answer "which one": two Egg Tokens are one string between
   *  them, and so are two copies of a commander. */
  id?: number
  /** Where an `'ability'` beat's source was standing — `'Command'` for an
   *  eminence trigger, `'Battlefield'` for most things. Absent on every other
   *  kind. */
  zone?: string
  /** Whether an `'ability'` beat was raised by the game rather than activated
   *  by a player: *triggers* against *activates*. */
  trigger?: boolean
  /** How an `'enters'` permanent reached the battlefield — `'cast'`, or
   *  `'put'` there by something else. Magic's own two words, and absent on
   *  every other kind.
   *
   *  **Missing is a third state and it is not `'put'`.** It means nobody
   *  said: every match already in the ledger was narrated before the scribe
   *  learned to ask, and a worker running an older image still cannot answer.
   *  A room that read absence as "put onto the battlefield" would tell a
   *  newcomer that every creature in every older match appeared out of
   *  nowhere — the exact opposite of the distinction this draws. */
  entered?: string
  target?: string
  against: string | null
  amount?: number
  life?: number
  note?: string
}

/** One game's narration.
 *
 * Only ever the **most recent** game's: the job's partial is re-fetched on
 * every poll, so it holds the newest thing it has to hand over rather than a
 * growing transcript. The room accumulates. */
export interface ForgeBeats {
  game: number
  beats: ForgeBeat[]
  /** The game outran the ceiling on what crosses. The beats kept are the
   *  first ones — a game's opening is what makes the rest of it legible. */
  truncated: boolean
  /** The battlefield, or null.
   *
   * Null for a match played by a worker without the scribe — Forge's own game
   * log has no category for a token or a counter, so a board built from it
   * would be silently wrong on exactly the decks that most want one (ADR 42).
   * A room handed no board draws the account alone, which is what every room
   * did before there was one. */
  board: ForgeBoard | null
}

/** One side of the table. The **only** place a seat number becomes a deck.
 *
 * Everything else in a board refers to a seat by its number, because a board
 * is a place: two sides, laid out, needing a stable index. Threading a slug
 * through three hundred changes would be the same fact three hundred times. */
export interface ForgeBoardSeat {
  seat: number
  slug: string | null
  /** What Forge called the player, which is the deck's own title — the
   *  fallback for a seat whose slug the shelf has not answered yet. */
  name: string
  life: number
  /** This seat's commanders by board id, **in the deck's own order** — one
   *  for most decks and two for a pairing.
   *
   *  Sent because the browser cannot work it out: a commander, a partner and
   *  a companion all arrive as a card in the `command` zone on step zero, and
   *  only `deck.yaml` knows which is which. Absent for a board shaped before
   *  a deck was known. */
  commanders?: number[]
  /** What this seat's command zone **is** — `'commander'` for one, or
   *  `'partners'` for a pairing — and absent for a board shaped before any
   *  deck was known.
   *
   *  The zone has only three legal shapes, and a list of ids names none of
   *  them: two ids and one commander that happened to get cloned read alike.
   *  Decided on the server off `deck.yaml`'s own declaration, which the gate
   *  has already refused if the pairing is not a legal one. The companion is
   *  beside this rather than in it, because it can come with either shape. */
  shape?: string
  /** This seat's companion by board id, or absent.
   *
   *  A companion really does sit in the command zone — Forge moves it there
   *  at setup — but it is not a commander and owes no tax. */
  companion?: number
}

/** One card in one game, named and painted once.
 *
 * The dictionary is what makes a board small: every change below refers to a
 * card by Forge's per-game instance id, and this is the only place the name
 * and the painting are spelled out. */
export interface ForgeBoardCard {
  id: number
  name: string
  token?: boolean
  types?: string
  seat?: number
  /** The whole card face, which is what a permanent looks like on a table. */
  image?: string
  /** The painting alone, for a card shown large. */
  art?: string
  /** Carried for tokens, whose printing is chosen rather than looked up. */
  artist?: string
  /** Every name this card answers to, for the cards with more than one.
   *  Absent for everything else, which is nearly every card.
   *
   *  Forge renames a card when its other half is cast, and this board learns a
   *  name once; without this the beat says *Stomp* and nothing in the match is
   *  called Stomp. See `halfNamed` in `lib/board.ts`. */
  faces?: string[]
  /** Each face's own type line, index-aligned with `faces`. A card's two halves
   *  are two different kinds of spell — Locthwain Scorn is a Sorcery printed on
   *  an Enchantment — and the plate has to name the half that was cast. */
  face_types?: string[]
  /** Each face's own **painting**, index-aligned with `faces`, and absent for
   *  the cards whose faces share one — an Adventure, a split card, a flip
   *  card, where `image` is the picture both names are printed on.
   *
   *  Present is the room's permission to change the picture: a modal
   *  double-faced card played as its land back is a painting of a land, and
   *  the front's sorcery is the wrong card to hold up for it. See `pictureOf`
   *  and `faceInPlay` in `lib/board.ts`. */
  face_images?: string[]
  /** Scryfall's own word for how the card is printed — `adventure`, `split`,
   *  `flip` — and only ever sent alongside `faces`. The one thing that reads
   *  it is the room deciding *where on the picture* the half being cast is,
   *  which is a question a card with `face_images` does not have: its halves
   *  are not on one picture. */
  layout?: string
  /** Whether the card makes mana, from Scryfall's `produced_mana`. A card
   *  fact, sent because a board keeps mana rocks back with the lands and
   *  reading rules text in a browser is how that judgement would rot. */
  mana?: boolean
  /** Which mana this printing taps for, from Scryfall's `produced_mana` and
   *  gated on the same test `mana` is. **What the card does, never what this
   *  game's pool received** — nothing on that pipe can say the second thing. */
  makes?: string[]
  /** Every keyword Scryfall lists for the card, unfiltered — the board draws
   *  the ones it has a sign for and ignores the rest, so adding a sign is a
   *  change to one file in the browser. */
  keywords?: string[]
  /** The board id of the card whose ability made this one a **copy**, and
   *  absent for every card that is not one — a token minted fresh carries
   *  nothing here and a populated one carries this.
   *
   *  **Not what it was copied *from*.** A Centaur Token populated by Growing
   *  Ranks names Growing Ranks, because that is the card whose ability made
   *  it; the permanent it duplicated never crosses this wire. */
  copied_by?: number
}

/** One card's change at one step. Everything is optional because almost
 *  everything is usually unchanged — a land tapping is one field. */
export interface ForgeBoardChange {
  id: number
  zone?: string
  seat?: number
  tapped?: boolean
  power?: number
  toughness?: number
  types?: string
  /** The card's whole counter set, whenever any of it moved.
   *
   *  **An empty array is a real answer and `undefined` is not the same
   *  thing.** Absent means nothing about counters changed this step; empty
   *  means the card has none, which is what a creature that has just died or
   *  had its last counter removed really is. */
  counters?: { kind: string; n: number }[]
  /** Every counter event this card raised at this step — which kind moved,
   *  and from what to what.
   *
   *  A delta rather than a running list, because the board is folded from the
   *  first step on every render and a full history re-sent on each of a
   *  hundred and thirty steps would be paid for a hundred and thirty times.
   *  `foldBoard` accumulates it into `BoardCard.counterHistory`. */
  counter_moves?: { kind: string; was: number; now: number }[]
  /** What this creature is doing in the combat happening now —
   *  `'attacking'`, `'blocking'`, or `''` for one standing out of it.
   *
   *  The empty string is the value that says a creature has *stopped*, so
   *  absent and empty are different facts here too. */
  combat?: string
  /** The seat this creature is attacking, or 0 for one that is not. */
  attacking?: number
  /** The **board id** of the attacker this creature is blocking, or 0 for one
   *  that is not blocking.
   *
   *  The id rather than the name, because two Egg Tokens are one name and
   *  pairing a blocker to "the attacker called Egg Token" pairs it to
   *  whichever is drawn first. */
  blocking?: number
  /** How many times this card has left the command zone, which is what
   *  commander tax is counted from.
   *
   *  Counted on the server. The browser used to count the same zone
   *  transitions itself, which put a reading of the game in the one file
   *  that decides none. */
  casts?: number
  /** The card this one is now attached to — an Aura on what it enchants, an
   *  Equipment on what it is equipping.
   *
   *  **Zero is a real value here and `undefined` is not the same thing.**
   *  Absent means this step did not touch the attachment, which is true of
   *  almost every card almost every step; zero means attached to nothing now,
   *  which a sword coming off a bear really is. */
  attached_to?: number
  /** Every keyword this card **instance** has right now, granted ones
   *  included — not the keywords its printing carries, which are
   *  `ForgeBoardCard.keywords` and are the same for every copy of a card.
   *
   *  **An empty array is a real answer**, as it is for `counters`: it means a
   *  creature has lost the last keyword something was giving it. */
  live?: string[]
  /** The subset of `live` that this card's **printing does not carry** — the
   *  keywords something else gave it. A Beast standing beside Kaheera has
   *  vigilance here and nothing in its own text that explains it.
   *
   *  **It says that a keyword was granted and never by what.** Forge erases
   *  attribution completely, so the card to blame is not available at any
   *  price — copy that renders this must not imply an agent. */
  granted?: string[]
  /** **How** this permanent left, when Forge said so: `'sacrificed'`, and
   *  nothing else.
   *
   *  Sacrifice is the only word this pipe has. Forge announces a destruction
   *  without saying which card it was, and a combat death as nothing at all,
   *  so the board says what it knows and stays quiet about the rest. */
  fate?: string
}

/** The board's movement between one beat and the next.
 *
 * **Steps are parallel to `beats`, one for one**, which is the whole pacing
 * design: the room drains the beats at reading speed, and the picture moves
 * exactly when the sentence is spoken, from one clock. */
export interface ForgeBoardStep {
  /** Forge's own turn number, which counts each player's turn separately —
   *  `playerTurns` in `lib/theater.ts` converts at the last moment. */
  turn?: number
  seat?: number
  life?: { seat: number; life: number }[]
  /** Every seat whose **own** counters moved at this step — poison, and the
   *  energy and experience that arrive through the same event.
   *
   *  Beside `life` because both are facts about a player rather than a card,
   *  and shaped like a card's `counters` for the same reason it is: the whole
   *  set crosses, so a reader replaces what it is holding instead of adding
   *  deltas and drifting the first time one is dropped. **An empty array is a
   *  real answer** — a player who has just had every counter taken off. */
  counters?: { seat: number; counters: { kind: string; n: number }[] }[]
  /** Every seat whose **commander damage** moved at this step, kept apart by
   *  the commander that dealt it.
   *
   *  **The third clock a player can die on, and the only one that is per
   *  source.** Twenty-one combat damage from a single commander ends a game
   *  whatever the life total says (rule 903.10a) — so twenty from each of two
   *  commanders is a player still standing, and a reader that summed them
   *  would call a game nineteen points early. Take the largest, never the
   *  total.
   *
   *  `id` is a board id and resolves through `cards`; `damage` is a running
   *  total from Forge's own tracker, so nothing here adds anything up.
   *  **Absent is not nought** — a match played by a worker built before the
   *  scribe learned to ask carries none of this, and drawing a `0` would claim
   *  the question had been answered. */
  generals?: { seat: number; from: { id: number; damage: number }[] }[]
  changes?: ForgeBoardChange[]
  /** Every value a mana pool took during this step, in order — `'GGW'` is two
   *  green and one white, and `''` is a pool that has drained.
   *
   *  **A sequence rather than a state, and the only one on this wire.** A pool
   *  fills and empties several times between two beats, so a step holding only
   *  the value it ends on holds an empty pool nearly every time — measured on a
   *  real match at nine empty out of ten. A seat therefore appears more than
   *  once here, and the last entry for it is the resting value. */
  floating?: { seat: number; pool: string }[]
  /** Every ability that went on the stack at this step, in order.
   *
   *  **Transient**: using an ability is a moment rather than a state, so this
   *  rides the step and there is nothing to clear afterwards. `zone` is where
   *  the source was — `'Command'` for an eminence trigger, whose card never
   *  moves and so has no other signal anywhere in this stream.
   *
   *  `targets` is what the ability was aimed at, **by board id** — the half
   *  that turns eminence from a shrug into a picture, because zone alone says
   *  a commander in the command zone did *something* and this says which cat
   *  got bigger. A list rather than an id: nothing measured has ever carried
   *  two, and narrowing the wire to the first one would lose the second
   *  silently the day something does. **Absent is the common case and not a
   *  gap** — three abilities in four are aimed at nothing at all, because
   *  Forge's own pump effects pick their creature by definition rather than
   *  by targeting it. */
  abilities?: {
    id: number; seat?: number; zone?: string; trigger?: boolean
    targets?: number[]
  }[]
}

/** The battlefield, as the room receives it. */
export interface ForgeBoard {
  seats: ForgeBoardSeat[]
  cards: ForgeBoardCard[]
  steps: ForgeBoardStep[]
}

export interface ForgeResult {
  decks: ForgeDeckRow[]
  games: number
  played: number
  draws: number
  timed_out: number
  median_seconds: number | null
  max_seconds: number | null
  startup_seconds: number
  wall_seconds: number
  clock: number
  seed: number
  rows: ForgeGameRow[]
  caveat: string
  /** **Every** game's narration and board, in the order they were played.
   *
   * The job's `partial` carries one game — the newest — and the server clears
   * it the moment the job finishes. So a short match could finish inside a
   * single poll and leave the room with nothing to draw, and a long one left
   * you stuck on whichever game happened to be current. A match is worth
   * watching back, which is when somebody has the time for it. Empty for a
   * match nobody asked to narrate. */
  beats: ForgeBeats[]
}

export interface SuggestionCandidate {
  name: string
  mana_cost: string | null
  cmc: number
  type_line: string
  oracle_text: string
  color_identity: string[]
  image: string | null
  art_crop: string | null
  edhrec_rank: number | null
  /** Weighted similarity to the card being replaced, not a quality judgement. */
  score: number
  /** What actually scored, so the number can be argued with. */
  reasons: string[]
}

export interface SuggestionTarget {
  card: string
  code: string
  why: string
  candidates: SuggestionCandidate[]
}

export interface Suggestions {
  slug: string
  pool_available: boolean
  targets: SuggestionTarget[]
}

/** One thing that was done to a deck (ADR 28).
 *
 * `summary` is rendered on the server and shown verbatim: the CLI prints the
 * same string, and a second renderer here would be a second thing to keep in
 * step. `action` is the stable verb beside it, which is what a client may
 * safely switch on.
 *
 * There is deliberately no rationale field. The log records that a `why`
 * changed and never what it says — rule 4's text lives in `deck.yaml`.
 */
export interface DeckLogEntry {
  id: number
  /** ISO-8601 UTC. */
  created_at: string
  /** A username, or `null` for whoever is at the machine — the CLI, and the
   *  app with auth off. An unnamed actor, not an unknown one. */
  actor: string | null
  action: string
  summary: string
}

export interface DeckLog {
  slug: string
  entries: DeckLogEntry[]
}

/** One generated deliverable, as the library holds it. */
export interface DeckArtifact {
  name: string
  /** Bytes, not characters. */
  size: number
  /** ISO-8601 UTC — the store's own timestamp, so it can be compared to
   *  anything else about the deck. */
  built_at: string
}

/** What a deck has been built into, and whether that is still true.
 *
 * `baseline` is the field this whole surface exists for. Three states rather
 * than a `stale` boolean because the third one is honest and a boolean would
 * have to lie about it: a deck built before ADR 30's snapshot mechanism has
 * artifacts and no baseline, so nothing can say whether they match. Every
 * artifact on the volume was in exactly that position on 2026-08-21.
 *
 * Never recomputed here. The server compares the stored snapshot against the
 * deck and this renders the answer — the readout rule the stance dial and the
 * labels editor already follow, for the same reason: a second copy of the
 * comparison in TypeScript would disagree with the served one silently.
 */
export interface DeckArtifacts {
  artifacts: DeckArtifact[]
  baseline: 'current' | 'different' | 'unknown'
  /** False for a draft: ADR 13 keeps the artifacts shut until promotion. */
  buildable: boolean
  stage: string
  /** Present only on a build's own response. */
  issues?: ValidationReport
  forced?: boolean
}

/** What Claude made of one photographed card (ADR 34).
 *
 * `transcribed` is what it actually read off the card — carried beside the
 * reading so the page can show both. A wrong match next to the words it came
 * from is a mistake somebody can catch; a wrong match alone is one they
 * cannot. `reading` is the same shape the browser's own reader produces,
 * because it went through the same `identify` door.
 */
export interface ScanResult {
  reading: Reading | null
  transcribed: { title?: string; corner?: string }
  refused: boolean
  model: string
}

/** One card as the camera's review list renders it. Narrower than a search
 *  result on purpose: this list can hold forty entries with pictures on a
 *  phone, and oracle text is what a card's own page is for. */
export interface IdentifiedCard {
  name: string
  mana_cost: string | null
  type_line: string
  color_identity: string[]
  image: string | null
  art_crop: string | null
}

/** A candidate the title tier offers. `score` is a string distance and is
 *  carried so a list can be ordered and shaded — **never so anything can
 *  threshold on it.** The scores of right and wrong answers overlap; the
 *  server's card reader carries the measurement. */
export interface IdentifiedCandidate extends IdentifiedCard {
  score: number
}

/** What the pool made of one sighting.
 *
 * `resolved` is non-null only when a set code and collector number found a
 * printing — a lookup with no judgement in it. A title produces
 * `candidates` and nothing else, however certain it looks. */
export interface Reading {
  via: 'printing' | 'title' | 'nothing'
  resolved: IdentifiedCard | null
  candidates: IdentifiedCandidate[]
}

export interface IdentifyResult {
  readings: Reading[]
  /** Counted apart, and never summed: two of the three are work nobody has
   *  done yet. */
  resolved: number
  offered: number
  unread: number
  dropped: number
  message?: string
}

/** A name that was misspelled, and the card the pool read it as.
 *
 *  Applied, not offered: the deck contains the real card by the time this is
 *  reported (`deckimport.Respell`). It is here so nothing is silent. */
export interface Correction {
  written: string
  read: string
  score: number
}

/** A shortlist for a name that could NOT be read confidently — the close-run
 *  field and the genuine non-word. Offered, never applied. */
export interface DidYouMean {
  /** Exactly as it was pasted. */
  written: string
  candidates: { name: string; score: number }[]
}

export interface ImportResult {
  slug: string
  /** Whose library it landed in — always the caller's. Sent back rather than
   *  assumed, because the deck's URL needs it and guessing it here is guessing
   *  which tier the server chose. */
  owner: string
  name: string
  stage: string
  status: string
  /** False for a dry run, which runs the identical path and writes nothing. */
  created: boolean
  commander: string[]
  companion: string | null
  total_cards: number
  land_count: number
  swap_board: string[]
  needs_rationale: number
  /** How many cards arrived with a reason already written, from the quoted
   *  column. Beside `needs_rationale` rather than instead of it: somebody who
   *  wrote sixty of them should be told so, and a count of what is still owed
   *  on its own reads as though nothing arrived. */
  rationales: number
  /** Misspellings the pool read as the card they are nearest to. The deck
   *  holds the real card; this is the saying-so. */
  read: Correction[]
  /** Names that could not be read as anything either. Kept in the deck
   *  verbatim, and reported by the gate as `unknown-card`. */
  unknown: string[]
  /** A shortlist beside each name that survived the reading unresolved.
   *  Offered rather than applied, because by construction no single card was
   *  clearly enough what these meant. */
  did_you_mean: DidYouMean[]
  /** Unresolved names that got no shortlist because the list had more of
   *  them than the server will run scans for. Reported rather than hidden:
   *  a silent cap reads as "there was nothing to suggest". */
  did_you_mean_skipped: number
  unreadable: { line: number; text: string }[]
  skipped: { line: number; text: string }[]
  notes: string[]
  /** The deck.yaml as written, so a preview can show the real thing. */
  yaml: string
  ok: boolean
  errors: Issue[]
  warnings: Issue[]
}

/**
 * What every deck edit hands back: the gate, re-run on the result.
 *
 * An edit that did not report the gate would leave a deck changed and
 * unchecked, so this shape is shared by all of them. `needs_rationale` rides
 * along because an edit is the likeliest moment for it to move — filling in
 * the last blank `why` is what makes a draft promotable.
 */
export interface EditResult {
  slug: string
  stage: string
  total_cards: number
  needs_rationale: number
  ok: boolean
  errors: Issue[]
  warnings: Issue[]
}

export interface SwapResult extends EditResult {
  swapped_out: string
  swapped_in: string
  why: string
}

/** The intake sheet: what an imported deck is being asked to have done to it
 *  (ADR 41). Every field is off unless it is sent true. */
export interface IntakeSheet {
  /** Draft the `why` on cards that have none. The one action gated on the
   *  stance's write axis, and the one that marks what it wrote. */
  rationales?: boolean
  /** File cards still sitting on the importer's `utility` default. */
  categories?: boolean
  /** Write the deck's strategy and themes. */
  description?: boolean
  /** Fill the cached commander dossier. Writes nothing to the deck. */
  dossier?: boolean
  /** Run the slot sweep over the 99. Writes nothing to the deck. */
  argue?: boolean
  stance?: string
}

/** One action's outcome: what it changed, out of what it looked at, and a
 *  sentence when the number alone would read as a failure. */
export interface IntakeStep {
  changed: number
  considered: number
  note?: string
}

export interface IntakeResult {
  slug: string
  /** False when the stance made no call at all, with `reason` saying so. */
  asked: boolean
  reason?: string
  steps: Partial<Record<
    'rationales' | 'categories' | 'description' | 'dossier' | 'argue',
    IntakeStep
  >>
}

export interface Job {
  id: string
  kind: string
  status: 'queued' | 'running' | 'done' | 'error'
  done: number
  total: number
  percent: number
  label: string
  result: unknown
  /** What is true *so far*, while the job is still running — and `null` the
   *  moment it is not.
   *
   *  The server clears it on completion deliberately: `result` is
   *  the whole answer, so a partial left lying beside it is a stale second
   *  copy of the same match. Read it for a live view and read `result` for
   *  the record; never merge the two.
   *
   *  `unknown` rather than a union, for the same reason `result` is: the
   *  shape belongs to the job's `kind`, and the screen that submitted the job
   *  is the only thing that knows which. The Forge's is
   *  `{ rows: ForgeGameRow[]; beats: ForgeBeats | null }` — see `theaterRows`
   *  and `theaterBeats`. */
  partial: unknown
  error: string | null
  created_at: string
}

export interface UpcomingSet {
  code: string
  name: string
  released_at: string
  card_count: number
  icon: string | null
  set_type: string
}

/** `GET /api/auth/me`. Who the caller is, and what this instance requires.
 *
 * `auth_required` and `authenticated` are separate because the app has to tell
 * "logged out of an instance that wants a login" from "this instance has no
 * login" — collapsing them makes the local app render a sign-in form it has no
 * server for.
 *
 * `is_admin` is the one to read for gating UI, not `user?.is_admin`. With auth
 * off the caller is the local single user: an admin, and not authenticated as
 * anybody, so `user` is null and the nested flag is unreachable. Gating on the
 * top-level field is what makes the admin page work on a laptop.
 */
export interface AuthState {
  auth_required: boolean
  authenticated: boolean
  is_admin: boolean
  user: { id: number | null; username: string | null; is_admin: boolean } | null
}

/** One account, as `/api/admin/users` reports it.
 *
 * `email` is here and nowhere else in this client. ADR 17 decided an admin sees
 * addresses; the constraint that replaced "the CLI only" is that an address may
 * be serialised into a response an admin authenticated for, and this is that
 * response.
 */
export interface Account {
  id: number
  username: string
  email: string | null
  is_admin: boolean
  disabled: boolean
  created_at: string
  /** "active" | "invited" | "no password" | "disabled" — four real states. */
  state: string
  sessions: number
  /** Which Claude answers this account. Always a key the server still knows:
   *  a row holding a tier this build has forgotten reports the one it will
   *  actually be answered by, never the stale string. */
  model_tier: string
}

/** One model tier, as the Admin page offers it. No model id — commandment 10,
 *  and the reason the roster is served rather than written in TypeScript. */
export interface ModelTier {
  key: string
  label: string
  blurb: string
}

export interface AccountList {
  users: Account[]
  /** Admins who can actually sign in. The last one cannot be demoted. */
  admins: number
  /** The tiers this instance knows, in the order it wants them shown. Served
   *  rather than hard-coded here so the page can only offer what the server
   *  will accept — a second list in TypeScript would drift the day one is
   *  added, and the drift would present as a control that 422s. */
  tiers: ModelTier[]
}

/** `GET /api/admin/stats/system` — the process and the box, self-reported.
 *
 * `process.kind` is the honesty flag: `current` comes from the kernel's own
 * per-process accounting (the deployed case), `peak` from the high-water
 * mark that is all the dev Mac can report — a peak shown as a level would
 * read as a leak. */
export interface AdminSystem {
  /** What migration version this box's `app.db` reached, and what the code
   *  running here was written against. Equal on every healthy boot — the
   *  pair is the point, since ADR 23 applies migrations on a deploy with
   *  nobody watching and the ladder is forward-only. `applied` is null only
   *  when there is no database yet, which is a fresh laptop. */
  schema: { applied: number | null; expected: number }
  process: { bytes: number; kind: 'current' | 'peak' }
  memory: { total_bytes: number | null; available_bytes: number | null }
  load: number[]
  cpus: number | null
  disk: { path: string; total_bytes: number; used_bytes: number
          free_bytes: number }
}

/** `GET /api/admin/stats/storage`. Null means "nothing there yet" — a fresh
 *  instance has no pool, and that is different from a present, empty one. */
export interface AdminStorage {
  app_db_bytes: number | null
  pool_bytes: number | null
  scryfall_bulk_bytes: number | null
  cache_bytes: number | null
  /** The three shelves, plus whatever they do not account for. `other_bytes`
   *  is the honest half: a fixed list of tenants cannot show the one nobody
   *  added to it, and the reading engine was 38% of this cache while the tile
   *  named only two of them. */
  cache: {
    symbols_bytes: number | null
    cardmotion_bytes: number | null
    ocr_bytes: number | null
    other_bytes: number | null
  }
  decks: { count: number; bytes: number | null; trashed: number }
}

/** `GET /api/admin/upkeep` — the two things about this instance that go out
 *  of date on their own.
 *
 *  The library half is a state as much as a reading: `refreshing` and
 *  `job_id` are what let a page reloaded mid-gathering attach to the run
 *  already going instead of offering to start a second one, which is the
 *  thing ADR 6 asks be made hard.
 *
 *  The arena half is a reading and never a lever — `playing_with` is the
 *  Forge the last recorded game here was actually played with, which is a
 *  stronger fact than anything a build could claim about itself. There is no
 *  button beside it on purpose; `go/internal/api/upkeep.go` argues why. */
export interface AdminUpkeep {
  library: {
    /** False while a gathering holds the shelves, and on an instance that has
     *  never gathered one. The counts read zero in both cases. */
    present: boolean
    cards: number
    printings: number
    /** The day the shelves were last written, `2026-08-24`, or null. */
    gathered: string | null
    refreshing: boolean
    job_id: string | null
  }
  arena: {
    here: boolean
    playing_with: string | null
  }
}

/** One mode's ledger roll-up (the server's Claude ledger): counters, a mode
 *  name, and nothing that could name a person, a deck or a question. */
export interface ClaudeUsageRow {
  mode: string
  /** The model that answered. Every row carries both axes; the one that was
   *  *not* grouped on reads `(various)`, so a per-mode row spanning models
   *  says so rather than naming an arbitrary winner.
   *
   *  Kept in the payload but **not rendered** — it is a model id, which is
   *  technology (commandment 10). Show `model_label`. */
  model: string
  /** How to name that Claude on a screen: "Sonnet", "Opus", "Several" for an
   *  aggregated row, "Another Claude" for a model this build does not know.
   *  Computed server-side, so there is no id-to-name table in TypeScript to
   *  drift from the server's own. */
  model_label: string
  conversations: number
  requests: number
  input_tokens: number
  output_tokens: number
  cache_read_tokens: number
  first_at: string
  last_at: string
}

/** What a window came to in money, and what could not be counted.
 *
 *  `unpriced` is not a footnote: a conversation whose model carries no rate
 *  contributes nothing to `usd`, so rendering the figure without it shows a
 *  number that is wrong downward and reads as reassuring. */
export interface ClaudeSpend {
  usd: number
  unpriced: number
  unpriced_models: string[]
  complete: boolean
  /** When a person last read the pricing page. Shown beside the figure — the
   *  honest substitute for a freshness guarantee it cannot make. */
  checked: string
}

/** One window of the ledger, on both axes. They sum to the same totals by
 *  construction — each is its own `GROUP BY`, not a pivot of the other. */
export interface ClaudeWindow {
  by_mode: ClaudeUsageRow[]
  by_model: ClaudeUsageRow[]
  /** Estimated from the per-model rollup only. Pricing the per-mode one would
   *  mean guessing a rate for rows that span models. */
  estimated_usd: ClaudeSpend
}

/** `GET /api/admin/stats/claude`. The caveat rides with the numbers because
 *  they are a floor on the bill, and any surface that shows one without the
 *  other is quoting a cached number as fresh, one abstraction over. */
export interface AdminClaude {
  windows: { week: ClaudeWindow; month: ClaudeWindow; all: ClaudeWindow }
  caveat: string
  prices: { checked: string; source: string; note: string }
}

/** `GET /api/admin/stats/activity`. Counts all the way down. */
export interface AdminActivity {
  accounts: Record<string, number>
  sessions: { total: number; seen_day: number; seen_week: number }
  deck_edits_by_day: { day: string; edits: number }[]
  sim_cache_rows: number
  jobs: Record<string, number>
}

/** `GET /api/admin/stats/fly` — what the platform sees, when it is asked.
 *
 * The only admin view that can be switched off: `configured` is false on an
 * instance with no `FLY_METRICS_TOKEN` (every laptop), and the widget hides
 * rather than showing a broken panel. `ok: false` is the other state worth
 * rendering — configured but unreachable, which is a clouded glass, not an
 * absent one. A value may be null: an empty series is not a zero. */
export interface AdminFly {
  configured: boolean
  ok: boolean
  error?: string
  app?: string
  org?: string
  values: {
    memory_bytes?: number | null
    memory_total_bytes?: number | null
    edge_2xx?: number | null
    edge_4xx?: number | null
    edge_5xx?: number | null
  }
}

/** One day of the visitor ledger: a total and whichever status classes the
 *  day actually saw (`2xx`…`5xx` arrive as optional keys). */
export interface TrafficDay {
  day: string
  total: number
  '2xx'?: number
  '3xx'?: number
  '4xx'?: number
  '5xx'?: number
}

/** `GET /api/admin/stats/traffic` — the visitor ledger (schema v9). Route
 *  templates and status classes only; the recording side never held an
 *  address, an agent, a name or a concrete path, so this cannot show one. */
export interface AdminTraffic {
  days: TrafficDay[]
  top_routes: { route: string; count: number }[]
  note: string
}

/** `POST /api/auth/login`. The session is the cookie; this is who it belongs to. */
export interface LoginResult {
  user: { id: number; username: string; is_admin: boolean }
}

/** `POST /api/auth/claim`. Note what is *not* here: a session.
 *
 * Redeeming an emailed link sets a password and does not log anybody in — a
 * cookie here would turn a link that arrived by mail into a session-minting
 * endpoint, and what it would save is one trip through a login form that has to
 * work anyway. The username comes back so that form can be filled in.
 */
export interface ClaimResult {
  detail: string
  username: string
}

/** `POST /api/auth/claim/preview` — what kind of link this is, before spending it.
 *
 * A POST rather than a GET, and that is not a style choice: the token lives in
 * the URL *fragment* precisely so it reaches no server's access log, and reading
 * it back over a query string would undo that at the first hop. It goes in a body
 * for the same reason the claim itself does.
 *
 * It spends nothing. The claim screen calls it on mount so it can offer a
 * username field to an invite and not to a reset — the server refuses a rename
 * on a reset either way, so this only keeps the form from offering what the
 * server would decline.
 */
export interface ClaimPreview {
  purpose: 'invite' | 'reset'
  username: string
}

export class ApiError extends Error {
  // Declared explicitly rather than as a constructor parameter property, which
  // tsconfig's erasableSyntaxOnly disallows.
  status: number
  /** Seconds from `Retry-After`, on the 429s login, reset and claim can give. */
  retryAfter: number | null

  constructor(message: string, status: number, retryAfter: number | null = null) {
    super(message)
    this.status = status
    this.retryAfter = retryAfter
  }
}

/* ------------------------------------------------------- the lost session */

/**
 * A 401 from anywhere means the session ended under us — expired, revoked from
 * the Admin page, or signed out in another tab.
 *
 * Handled here rather than in each route, and that is the same argument
 * the door makes for auth middleware over a per-route check: eleven
 * routes each catching their own 401 is eleven chances to forget, and the one
 * added in a year is the one that renders a blank page with an unexplained
 * error. One place means every screen gets it, including the ones not written
 * yet.
 *
 * The listener is deliberately *not* a redirect. This module knows nothing
 * about the router, and the shell already re-asks `/api/auth/me` — which is
 * public, so it answers when nothing else does — rather than assuming what a
 * 401 meant.
 */
type SessionListener = () => void

const sessionListeners = new Set<SessionListener>()

/** Be told when a request was refused for want of a session. Returns the
 *  unsubscribe, so a `useEffect` can return it directly. */
export function onSessionLost(listener: SessionListener): () => void {
  sessionListeners.add(listener)
  return () => {
    sessionListeners.delete(listener)
  }
}

/**
 * The two paths whose 401 is an answer rather than an expiry.
 *
 * `login` answers 401 for a wrong password, which belongs in the form as the
 * sentence the server wrote — announcing a lost session there would clear the
 * screen somebody is mid-typing on.
 *
 * `me` is public and cannot 401 today. It is listed anyway because the listener
 * re-asks it: if that ever changed, an unlisted `me` would be a loop.
 */
const ANSWERS_WITH_401 = new Set(['/api/auth/login', '/api/auth/me'])

function retryAfterOf(resp: Response): number | null {
  // `?.` because a hand-built response stub in a test has no headers, and a
  // missing `Retry-After` and an absent header bag mean the same thing here.
  const raw = resp.headers?.get('Retry-After')
  if (!raw) return null
  const seconds = Number(raw)
  return Number.isFinite(seconds) && seconds >= 0 ? seconds : null
}

/** Turn a failed response into the error to throw, announcing a lost session. */
async function refuse(resp: Response, path: string): Promise<never> {
  let detail = `${resp.status} ${resp.statusText}`
  try {
    const parsed = await resp.json()
    // A refusal is a plain sentence the user should read as written — "a card
    // in a curated deck needs a `why`". FastAPI's own validation errors arrive
    // as an array, which has no such sentence, so those get stringified.
    if (typeof parsed?.detail === 'string') detail = parsed.detail
    else if (parsed?.detail) detail = JSON.stringify(parsed.detail)
  } catch {
    /* non-JSON error body; the status line is all we have */
  }
  // `?? path` because `split` on a non-empty string always yields a non-empty
  // first element -- but a `!` here would be asserting that about whatever
  // `path` becomes later, and the honest fallback is the unsplit path, which
  // is what the set is keyed on when there is no query string anyway.
  if (resp.status === 401 && !ANSWERS_WITH_401.has(path.split('?')[0] ?? path)) {
    for (const listener of sessionListeners) listener()
  }
  throw new ApiError(detail, resp.status, retryAfterOf(resp))
}

/** The class names the Forge worker's door puts in front of a failure.
 *
 * `cmd/mtglab/shim.go`'s `failureText` renders every refusal as
 * `<ClassName>: <sentence>` — `TimeoutExpired`, `RuntimeError` and four
 * others, spellings inherited from the wire the Go rewrite had to keep
 * answering (ADR 38). The class name is the half a maintainer wants: reading a
 * failed match, "was Forge missing, did it time out, or were the results
 * untrustworthy" is the first question. **It is also the half a user must
 * never see** — commandment 10 lets exactly one technology be named on screen
 * and it is Claude, not a Python exception class from a retired backend.
 *
 * So the prefix crosses the wire and is dropped at the last moment, which is
 * the right seam: the recorded string stays recorded, the job row keeps its
 * diagnosis, and the sentence a person reads is a sentence.
 *
 * **A closed list, not a pattern.** `/^\w+: /` would also eat "Deck not
 * found: gyome" and every other honest message with a colon in it. These six
 * are all of them — every one is emitted from `failureText` and the shim's
 * one 400, and nothing else in the Go tree produces the shape.
 */
const FAILURE_CLASSES = [
  'ForgeNotInstalled', 'CoverageFailed', 'ResultsUntrustworthy',
  'TimeoutExpired', 'RuntimeError', 'ValueError',
]

/** The message to show for a caught value.
 *
 * Every `catch` in the app wanted this and each wrote `catch (e: any)` with
 * `String(e.message ?? e)` inline. `any` on a catch binding switches off
 * `useUnknownInCatchVariables` for that clause, so `e.message` was unchecked
 * property access on six paths -- and `throw "a string"` anywhere upstream
 * would have produced the literal text `undefined` on screen.
 *
 * Everything this app throws is an `Error` (`ApiError` extends it, and the
 * job poller rejects with one), so the narrowing is exact rather than
 * defensive; the `String` fallback is only there for what a third party might
 * throw.
 *
 * It is also where a machine's name is taken off the front of a failure — see
 * [FAILURE_CLASSES]. Here rather than at each of the twenty `catch` sites,
 * because this is already the one place every one of them goes through, and a
 * rule applied in twenty places is a rule forgotten in the twenty-first.
 */
export function errorMessage(e: unknown): string {
  const message = e instanceof Error ? e.message : String(e)
  for (const name of FAILURE_CLASSES) {
    if (message.startsWith(`${name}: `)) {
      // The sentence, with its first letter left exactly as the server wrote
      // it: these read "Forge reported problems that…" and "Command '…' timed
      // out", which are already sentences. Capitalising would be this file
      // rewriting somebody else's prose on a guess.
      return message.slice(name.length + 2)
    }
  }
  return message
}

/**
 * GETs that are already in the air, by path.
 *
 * **Not a cache — a queue of one.** An entry lives only while its request is
 * unresolved and is deleted the moment it settles, so nothing here is ever
 * read after the network has answered and no screen can be handed a stale
 * body. What it removes is the *duplicate* ask: two components that mount
 * together and want the same thing make one request instead of two.
 *
 * `/api/health` is the one that made this worth writing — the shell asks for
 * it to show the pool's state and the Library asks for it again in the same
 * tick, so every visit to the front page opened two identical connections to
 * a one-machine server. It is the client's half of what `jobs.submit(key=…)`
 * already does on the server: asking twice at once is one question.
 *
 * Failures are shared too, deliberately. Both callers asked at the same
 * instant; answering one with an error and the other with a retry would make
 * the screen disagree with itself about whether the server is up.
 */
const inFlight = new Map<string, Promise<unknown>>()

async function get<T>(path: string): Promise<T> {
  const waiting = inFlight.get(path)
  if (waiting) return waiting as Promise<T>

  const request = (async () => {
    const resp = await fetch(path)
    if (!resp.ok) await refuse(resp, path)
    return resp.json() as Promise<T>
  })()
  inFlight.set(path, request)
  try {
    return await request
  } finally {
    inFlight.delete(path)
  }
}

async function send<T>(method: string, path: string, body?: unknown): Promise<T> {
  const resp = await fetch(path, {
    method,
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body ?? {}),
  })
  if (!resp.ok) await refuse(resp, path)
  return resp.json() as Promise<T>
}

async function post<T>(path: string, body: unknown): Promise<T> {
  return send<T>('POST', path, body)
}

/**
 * A guild speaking for itself: one line of printed flavour text, the person
 * who says it, and the card it is read off.
 *
 * `words` is the sentence inside the card's quotation marks and nothing else —
 * the marks are the flavour-text convention rather than part of the line, so
 * the renderer draws them. `card` is an attribution and never a card to
 * render: `/api/colors` resolves no cards at all, and the citation is meant to
 * read whole on a clone with no pool.
 */
export interface Creed {
  words: string
  speaker: string
  card: string
  printing: string
}

/**
 * A faction's heraldry as Wizards actually painted it: the mana rock its own
 * block printed to carry its device, with the artist and printing that
 * painting belongs to.
 *
 * **Twenty of the 32 have one and that is the finished design**, not a gap —
 * Ravnica's ten Signets, Alara's five Obelisks, Tarkir's five Banners, which
 * is every multicolour faction and no leftovers. The other twelve wear their
 * own mana symbols at size instead, and need no card to do it.
 *
 * `art` is a hotlink to one exact printing, and `artist` and `printing` are
 * the credit that printing is owed — a surface that draws the picture must
 * name both in the same room (commandment 19). They travel together and were
 * read out of the pool in one pass, so they cannot come apart.
 *
 * `flavor` is that printing's own flavour text, and it is a caption for the
 * device rather than a second motto: a `creed` is what a faction *says*, this
 * is what its emblem *means*.
 */
export interface Sigil {
  card: string
  artist: string
  printing: string
  art: string
  flavor: string
}

/** One of the 32 colour combinations. Mirrors `mtglab.colors.Combination`. */
export interface Combination {
  /** Canonical WUBRG-ordered key, or "C" for colourless. */
  key: string
  name: string
  /** colorless | mono | guild | shard | wedge | quad | five */
  tier: string
  colors: string[]
  size: number
  tagline: string
  history: string
  /**
   * The other naming convention. Scryfall's Commander 2016 names are primary
   * and EDHREC's Nephilim are the aliases — they describe identical colour
   * sets, and someone arriving from EDHREC needs to find the slot they came
   * for.
   */
  aliases: string[]
  /** A real card whose Scryfall colour identity proves this row. */
  verified_by: string
  /**
   * The guild's own words. `null` for the twenty-two slots that are not one
   * of the ten guilds — the shards and clans have creed-shaped flavour of
   * their own, but nobody has read it off the cards yet, and an empty string
   * would be indistinguishable from someone leaving the field blank.
   */
  creed: Creed | null
  /**
   * The faction's device, painted. `null` for the twelve slots that are not a
   * faction — see [Sigil], where the twenty and the twelve are argued.
   */
  sigil: Sigil | null
  /**
   * What happened to this faction. Empty for the twelve slots that are not
   * one — Mono-Red is not from anywhere, and does not get a story pretending
   * it is.
   */
  lore: string
  /**
   * The faces of the faction: a card name and who they are in the story.
   *
   * Names only here, because `/api/colors` reaches no card pool. The cards
   * themselves come from `combination()` below, which does — that split is
   * what keeps this payload working on a fresh clone.
   */
  champions: { card: string; role: string }[]
  /** Names of cards whose colour identity is exactly this combination. */
  signature: string[]
}

/**
 * One combination with its cards resolved against the pool.
 *
 * `dropped` counts names that did not resolve, which are left out rather than
 * rendered from the name alone — the same instrument ADR 19 uses for the
 * dossier's competitors. `exact_total` is how many cards in the whole pool have
 * exactly this identity, and it is the sharpest thing on the page: two, for
 * Artifice.
 */
export interface CombinationDetail extends Omit<Combination,
  'champions' | 'signature'> {
  pool: boolean
  champions: (ReferenceCard & { role: string })[]
  signature: ReferenceCard[]
  dropped: number
  exact_total: number | null
}

/**
 * A card as reference data renders it.
 *
 * Deliberately not `Card`, which carries `category`, `why` and `qty` — a
 * champion is not in anybody's deck, and giving it a blank rationale would be
 * the one shape rule 4 exists to prevent.
 */
export interface ReferenceCard {
  name: string
  mana_cost?: string | null
  type_line?: string | null
  oracle_text?: string | null
  color_identity: string[]
  image?: string | null
  art_crop?: string | null
}

/** One glossary entry. Mirrors `mtglab.glossary.Term`. */
export interface Term {
  key: string
  term: string
  /** One sentence — a tooltip, and the Learn page's lead line. */
  short: string
  /** A paragraph, elaborating `short` rather than repeating it. */
  long: string
  section: string
  see_also: string[]
}

export interface Glossary {
  sections: { key: string; label: string; blurb: string }[]
  terms: Term[]
}

/**
 * The labelling vocabulary, from `GET /api/themes`. Mirrors the server's
 * theme and archetype tables.
 *
 * Served rather than copied into this file: TypeScript cannot check a string
 * against the server's list, so a copy would drift silently and start offering
 * labels the server refuses.
 *
 * `archetypes` is the subset of `themes` that are *class* words. It is not a
 * second thing to declare — ADR 37 derives a deck's archetype by reading the
 * class words out of its declared themes, worst-Forge-piloted winning — but
 * the editor needs to know which chips carry that consequence in order to say
 * so as they are ticked.
 */
export interface ThemeVocabulary {
  themes: string[]
  archetypes: string[]
}

/** One fact from the shelves. Mirrors `mtglab.lore.Fact`, with the named
 *  cards already resolved through the pool (or dropped and counted). */
export interface LoreFact {
  key: string
  volume: string
  fact: string
  more: string
  cards: ReferenceCard[]
  learn: { tab: string; key: string } | null
}

export interface LoreShelves {
  volumes: { key: string; label: string; blurb: string }[]
  facts: LoreFact[]
  pool: boolean
  dropped: number
}

export interface ColorTaxonomy {
  colors: { code: string; name: string; wants: string; fears: string }[]
  // `blurb` is what the tier *is* and every tier has one; `eras` below is the
  // block of Magic that supplied the names, and only three tiers have that.
  tiers: { key: string; label: string; blurb: string }[]
  eras: { name: string; setting: string; named: string; story: string }[]
  combinations: Combination[]
}

/**
 * What the pool knows about a deck's commander, beyond the card itself.
 *
 * Every number here was counted by `service.commander_dossier` over the
 * pool. None of it is written into the UI, which is rule 1 applied to the
 * one panel most tempting to fill with remembered trivia.
 */
export interface CommanderDossier {
  slug: string
  card: Card | null
  supertypes: string[]
  /** How crowded each of the commander's creature types is. */
  subtypes: { name: string; total: number; legendary: number }[]
  /** Other cards whose name contains the character's — offered as related,
   *  not asserted as the same character. */
  other_cards: {
    name: string; type_line?: string | null; mana_cost?: string | null
    image?: string | null; art_crop?: string | null
  }[]
  printings: { count: number; first_released: string | null; first_set: string | null } | null
}

export interface ChallengeProgress {
  pool: boolean
  filled: number
  total: number
  slots: {
    key: string
    name: string
    tier: string
    decks: { slug: string; name: string }[]
  }[]
}

export interface DeleteResult {
  slug: string
  name: string
  deleted: boolean
  /** Whether the deck can be raised again — separately true and separately
   *  visible from `deleted`, which is the good half of the argument the old
   *  `moved_to` field made. That field carried a filesystem path, and the
   *  library page printed it. */
  recoverable: boolean
  /** The handle `returnEntombed` takes. Opaque: it names an entry in this
   *  player's crypt and nothing else, and **nothing renders it**. Empty when
   *  the crypt could not be read back in that instant, which is why
   *  `recoverable` is its own field rather than `crypt_id !== ''` computed at
   *  every call site. */
  crypt_id: string
  total_cards: number
  stage: string
  status: string
}

/** One deck in the crypt: entombed, not erased.
 *
 * `entombed_at` is null rather than absent when nothing recorded the burial.
 * A missing time rendered as a date would be a lie told in the one place a
 * player looks to check their deck is still there, so the surface says it
 * does not know instead.
 */
export interface EntombedDeck {
  id: string
  slug: string
  name: string
  total_cards: number
  commander: string[]
  entombed_at: string | null
}

export interface CreateResult {
  slug: string
  /** As `ImportResult.owner`: the caller's, and needed to link to the deck. */
  owner: string
  name: string
  stage: string
  status: string
  created: boolean
  commander: string[]
  companion: string | null
  color_identity: string[]
  combination: { key: string; name: string; tier: string }
  total_cards: number
}

/* ------------------------------------------------------------------ claude */

/** One axis of the stance dial, as `mtglab.claude.stance.describe` renders it. */
export interface StanceAxis {
  axis: string
  question: string
  level: string
  means: string
  levels: string[]
}

export interface StanceView {
  /** The preset's name when the axes happen to equal one, else null. */
  preset: string | null
  allows_calls: boolean
  may_write: boolean
  axes: StanceAxis[]
}

/**
 * A Claude surface: a prompt, a tool set, and what it may write.
 *
 * `writes` is empty for every mode and is served anyway — a client that has to
 * assume the capability set will eventually assume wrong.
 */
export interface ClaudeMode {
  name: string
  purpose: string
  tools: string[]
  writes: string[]
}

export interface ClaudeStatus {
  /** The SDK rides with the `claude` extra; a base install has neither. */
  installed: boolean
  /** Whether a credential is present. Never what it is. */
  configured: boolean
  model: string
  stance: StanceView
  ceiling: StanceView
  default: StanceView
  presets: { name: string; blurb: string; stance: StanceView; available: boolean }[]
  never: string
  modes: ClaudeMode[]
}

export interface InterviewQuestion {
  question: string
  /** role | alternative | redundancy | cost | cut | legality */
  angle: string
  /** The pool or gate fact the question rests on. Never an opinion. */
  fact: string
}

/**
 * What the rationale interview came back with.
 *
 * Note what is not in this shape: there is no field containing a rationale,
 * because there is no such field in the schema the model answers against.
 * `answered_by` is always present so a UI can never render an opinion as
 * though it were the gate's reproducible output.
 */
export interface InterviewReport {
  answered_by: string
  mode: string
  model: string
  slug: string
  card: string
  /** False when the stance was `off` — no call was made, which is not the
   *  same as a call that found nothing to say. */
  asked: boolean
  reason: string
  stance: StanceView
  questions: InterviewQuestion[]
  /** Answers that were not questions, dropped before reaching here. */
  questions_dropped: number
  tool_calls: { tool: string; arguments: Record<string, unknown> }[]
  usage: { input_tokens: number; output_tokens: number }
  never: string
}

export interface SlotCharge {
  /** One argument *against* this card's slot. There is no field here for an
   *  argument in favour, and that is ADR 25 rather than an omission. */
  claim: string
  /** redundancy | cost | speed | conditionality | count | ceiling | legality */
  ground: string
  /** The pool, gate or brief fact the charge rests on. Never an opinion, and a
   *  charge that arrived without one was dropped before reaching here. */
  fact: string
  /** decisive | serious | minor */
  strength: string
}

/** A card the model named as an alternative, *after* the server checked it. */
export interface SlotAlternative {
  name: string
  mana_cost?: string | null
  type_line?: string | null
  oracle_text?: string | null
  color_identity?: string[]
  cmc?: number | null
  image?: string | null
  art_crop?: string | null
}

/**
 * What the slot argument came back with (ADR 25).
 *
 * Note what is not in this shape, and note that it is a different absence from
 * the interview's. There is no rationale field for the same reason there is
 * none there — but there is also **no field holding the case for the card**,
 * which is this mode's whole design. A balanced version of this endpoint would
 * return a finished `why` grounded in the user's own deck, and a UI guard
 * against rendering it would not be a guard, because the CLI renders the same
 * payload and the endpoint is public.
 *
 * `alternatives_dropped` is per reason rather than a total: "you invented that
 * card" and "that card is off-colour" say different things, and one number
 * says neither.
 */
export interface SlotArgumentReport {
  answered_by: string
  mode: string
  model: string
  slug: string
  card: string
  /** False when the stance was `off` — no call was made, which is not the
   *  same as a call that found nothing to say. */
  asked: boolean
  reason: string
  stance: StanceView
  charges: SlotCharge[]
  /** Charges that cited nothing, dropped before reaching here. A number that
   *  climbs is a mode asserting rather than arguing. */
  charges_dropped: number
  alternatives: SlotAlternative[]
  alternatives_dropped: {
    not_in_pool: string[]
    banned: string[]
    off_colour: string[]
    /** Suggestions the deck already runs — a swap to one would be a no-op,
     *  so the server drops them before they are shown (punch list 2026-08-15). */
    already_in_deck: string[]
    no_pool: string[]
  }
  tool_calls: { tool: string; arguments: Record<string, unknown> }[]
  usage: { input_tokens: number; output_tokens: number }
  never: string
}

/** The argue sweep's answer — a `Job.result` for `kind: claude.argue.deck`. */
export interface DeckReviewResult {
  slug: string
  /** False when the stance was `off`: one report saying no calls were made,
   *  never N copies of it. */
  asked: boolean
  reason: string
  total: number
  /** One per argued card, in selection order. A card whose call failed is in
   *  `errors` instead — partial results are the point of paying for a sweep. */
  reports: SlotArgumentReport[]
  errors: Record<string, string>
}

/**
 * The commander dossier (ADR 19) — the half of the header the pool cannot
 * count.
 *
 * The shape is the ADR made into fields, and the reason it is not four strings
 * is provenance: every passage carries the ids of the pages it rests on, and
 * `sources` carries the pages themselves. A renderer that dropped
 * `source_ids` would produce exactly the unattributed prose the design
 * rejected, so the UI treats a passage and its citations as one thing.
 *
 * `sources` has already been checked server-side against the pages the search
 * actually returned; anything the model cited but never read is gone before it
 * reaches here, and `sources_dropped` counts what went.
 */
export interface DossierSection {
  prose: string
  source_ids: string[]
}

export interface DossierCompetitor extends DossierSection {
  name: string
  mana_cost?: string | null
  type_line?: string | null
  color_identity?: string[]
  image?: string | null
  art_crop?: string | null
  legal_commander?: boolean
  /** The pool's own text for the competitor, so the real card sits next to
   *  the sentence comparing it. */
  oracle_text?: string | null
}

export interface DossierBody {
  who: DossierSection
  archetype: DossierSection & { name: string }
  /** Commanders somebody would build instead — every one a pool row. */
  competitors: DossierCompetitor[]
  /** The story's allies: cited prose, same argument as `rivals`. Known
   *  associates from Magic's history, never cards that pair well. */
  allies: DossierSection
  /** The story's rivals: cited prose, not a card list. A plot line is not a
   *  pool row, which is why this has `who`'s shape and not `competitors`'. */
  rivals: DossierSection
  standing: DossierSection
  sources: { id: string; title: string; url: string }[]
  /** Cited pages the search never returned. A number that climbs is a prompt
   *  inventing citations, which is why it is rendered rather than logged. */
  sources_dropped: number
  /** Named competitors the pool does not have. */
  competitors_dropped: number
  /** How many pages were read to produce this. */
  searched: number
}

export interface DossierReport {
  answered_by: string
  slug: string
  commander: string
  /** Empty when there is none — never a missing key, so a caller never has to
   *  tell "absent" from "not yet fetched". */
  dossier: DossierBody | Record<string, never>
  cached: boolean
  generated_at: string | null
  /** False when no call was made: the stance was off, or a stored dossier was
   *  served. Only on the POST response. */
  asked?: boolean
  reason?: string
  model?: string
  stance?: StanceView
  usage?: { input_tokens: number; output_tokens: number }
  never?: string
}

export function hasDossier(
  report: DossierReport | null,
): report is DossierReport & { dossier: DossierBody } {
  return !!report && 'who' in report.dossier
}

/* ------------------------------------------------------- research (ADR 26) */

/**
 * One thing the pages said, and the pages it came from.
 *
 * `source_ids` is never empty on a finding that reached here: the server drops
 * a finding whose citations did not survive checking, and counts it. That is
 * one step past what the dossier does to a section, and the reason is that a
 * dossier passage may rest on the pool facts it was handed while this mode has
 * no brief at all — an uncited claim here is resting on the model's recall.
 */
export interface ResearchFinding {
  claim: string
  source_ids: string[]
}

/**
 * A card the answer named, and the one field that decides how to render it.
 *
 * **`in_pool` is not a detail.** A card the pool has carries the pool's own
 * text and its facts are rule 1 facts. A card the pool lacks — anything
 * spoiled since the last `data refresh` — carries a name and nothing else, and
 * everything the prose says about it is a claim resting on a cited page. ADR 26
 * keeps those apart deliberately, and a renderer that merged them would produce
 * exactly the blended paragraph ADR 19 rejected, one source further out.
 */
export interface ResearchCard {
  name: string
  in_pool: boolean
  mana_cost?: string | null
  type_line?: string | null
  oracle_text?: string | null
  color_identity?: string[]
  image?: string | null
  art_crop?: string | null
  legal_commander?: boolean
}

export interface ResearchBody {
  answer: string
  findings: ResearchFinding[]
  cards: ResearchCard[]
  /** settled | contested | thin. Rendered rather than hidden: a mode with no
   *  way to say "people disagree" writes consensus that is not there. */
  confidence: string
  sources: { id: string; title: string; url: string }[]
  /** Cited pages the search never returned. A number that climbs is a prompt
   *  inventing citations, which is why it is shown rather than logged. */
  sources_dropped: number
  /** Findings left citing nothing once the pages were checked. */
  findings_dropped: number
  /** Named cards the pool has never seen. **Not an error** — for a spoiler
   *  question the right value is above zero. */
  cards_unresolved: number
  searched: number
}

export interface ResearchReport {
  answered_by: string
  mode: string
  model: string
  question: string
  asked: boolean
  reason: string
  stance?: StanceView
  /** Empty when there is none, so a caller never has to tell "absent" from
   *  "not yet fetched". */
  research: ResearchBody | Record<string, never>
  generated_at: string
  usage?: { input_tokens: number; output_tokens: number }
  never?: string
}

export function hasResearch(
  report: ResearchReport | null,
): report is ResearchReport & { research: ResearchBody } {
  return !!report && 'answer' in report.research
}

/* ------------------------------------------------ the theme interview (ADR 20) */

/**
 * One turn of the conversation, as it crosses the wire.
 *
 * Deliberately **not** an Anthropic message block. The transcript is held by
 * this client and resent on every turn, so this type is the whole of what a
 * browser may put into a request — plain text with a role. The server refuses
 * anything else, and the reason is that an endpoint accepting real message
 * blocks would be a free proxy for somebody else's key.
 */
export interface ThemeTurn {
  role: 'user' | 'assistant'
  text: string
}

/**
 * What the interview believes about you, and the words you used to say it.
 *
 * `quote` is not decoration and it is not for display: the server checks it
 * against your own turns and throws the slot away if it is not there. That is
 * what stops the interview deciding who you are and then reporting it back as
 * something you said.
 */
export interface ThemeSlot {
  /** taste | temperament | posture | anchor */
  kind: string
  value: string
  quote: string
}

export interface ThemeFact {
  text: string
  /** A page title, or the literal `taxonomy` for the checked-in colour data. */
  source: string
  url: string
}

export interface ThemeReport {
  answered_by: string
  mode: string
  model: string
  asked: boolean
  reason: string
  stance: StanceView
  /** Which voice answered. Echoed back rather than assumed, so a client that
   *  sent nothing can still see it got the plain one. */
  persona: string
  question: string
  fact: ThemeFact | null
  /** A fact dropped because the person had already been told it — the check
   *  behind the prompt's "never give the same fact twice". */
  facts_dropped?: number
  slots: ThemeSlot[]
  /** Readings whose quote was not in the transcript. A number that climbs is a
   *  model inventing preferences, which is why it is rendered, not logged. */
  slots_dropped: number
  grounded: number
  floor: number
  /** Counted server-side from the grounded slots. There is no field the model
   *  can set to change this, which is the point of it. */
  may_propose: boolean
  exchanges: number
  max_exchanges: number
  usage: { input_tokens: number; output_tokens: number }
  never?: string
}

export interface ThemeCommander {
  name: string
  prose: string
  source_ids: string[]
  mana_cost?: string | null
  type_line?: string | null
  oracle_text?: string | null
  color_identity?: string[]
  image?: string | null
  art_crop?: string | null
}

/**
 * One suggested colour combination.
 *
 * `reading` and `grounding` are two fields because one of them can be wrong.
 * The reading is the leap from what you said to these colours — an
 * interpretation, offered as one. The grounding is what is factually true
 * about the colours and rests on `source_ids` or on the checked-in taxonomy.
 * A renderer that merged them would produce exactly the blended paragraph
 * ADR 19 rejected, so the UI keeps them visibly apart.
 */
export interface ThemeCombination {
  key: string
  name: string
  colors: string[]
  tier: string
  tagline: string
  reading: string
  grounding: string
  source_ids: string[]
  commanders: ThemeCommander[]
}

export interface ThemeProposal {
  answered_by: string
  mode: string
  model: string
  asked: boolean
  reason: string
  stance: StanceView
  combinations: ThemeCombination[]
  sources: { id: string; title: string; url: string }[]
  sources_dropped: number
  commanders_dropped: number
  /** Whole suggestions lost because every legend named for them turned out to
   *  have a subset identity. Counted because losing half a proposal silently
   *  is how a thin answer looks like a deliberate one. */
  combinations_dropped: number
  searched: number
  slots: ThemeSlot[]
  usage: { input_tokens: number; output_tokens: number }
  never?: string
}

/* ------------------------------------ personas and the tarot deal (ADR 21) */

/**
 * A voice the theme interview can adopt.
 *
 * `voice` is deliberately absent: the server keeps the prompt itself, because
 * a client that received one would eventually send one back and "the persona
 * is one of a fixed set" is worth keeping structural.
 *
 * The roster is fetched rather than written here, and that is the whole payoff
 * of ADR 21's shape — adding a reader is a `Persona` and a prompt server-side,
 * and this door grows a panel for it with nothing rebuilt.
 */
export interface Persona {
  key: string
  label: string
  blurb: string
  /** Whether this reader is dealt a spread before the conversation starts.
   *  Only the fortune teller is, today. */
  deals: boolean
}

export interface PersonaRoster {
  personas: Persona[]
  default: string
}

/**
 * One dealt card. `slot` is the load-bearing field and it is not decorative:
 * it is `taste` | `temperament` | `posture`, which are ADR 20's first three
 * slot kinds, so a card is dealt *for* a slot and the readiness instrument is
 * untouched. `position` is what the reader calls that place out loud.
 *
 * There is no `meaning`, here or on the server. The server shuffles; the
 * reader reads.
 */
export interface TarotDrawn {
  key: string
  name: string
  /** What goes under the picture. Same as `name` for all but the
   *  double-faced echoes, where `name` is the pool's whole-card name
   *  ("Murderous Rider // Swift End") and this is the front face alone. */
  face_name: string
  /** major | minor */
  arcana: string
  suit: string | null
  number: number
  /** `/tarot/<key>.webp` — package data, served beside the API — or, for a
   *  Magic crossover, a hotlinked Scryfall art crop. */
  image: string
  /** A Magic crossover's artist, owed a credit line wherever the art
   *  renders. Null for the 78. */
  artist: string | null
  /** Which original the Magic card answers — "The Fool" under Flubs.
   *  Null for the 78, and the render key for everything crossover-shaped. */
  after: string | null
  /** Why the Magic card holds its slot: the resonance with its original,
   *  stated as checkable facts. Null for the 78. */
  note: string | null
  reversed: boolean
  slot: string
  position: string
}

export interface TarotReading {
  /** Carried by the client and re-sent every turn, so a reload deals the same
   *  three cards. The same stateless trick the transcript uses. */
  seed: number
  cards: TarotDrawn[]
}

/** One slide of an arena's rotation.
 *
 *  Five kinds, and the two text fields are filled to match: `roman`,
 *  `gladiator` and `coliseum` carry `rome`; `magic` carries `magic`; `paired`
 *  carries both. The Go side refuses an unknown kind at boot, so a slide that
 *  arrives here is one this renderer has a branch for. */
export interface ColiseumFact {
  kind: 'roman' | 'gladiator' | 'coliseum' | 'magic' | 'paired'
  rome?: string
  magic?: string
  card?: string
}

/** A champion of one arena: a card the pool resolved, and why they fight. */
export interface ColiseumChampion {
  role: string
  name: string
  mana_cost: string | null
  type_line: string
  oracle_text: string
  color_identity: string[]
  image: string | null
  art_crop: string | null
}

/** One of the six houses a Tier 3 match is watched in.
 *
 *  `backdrop` is null when the pool has not been seeded — the prose still
 *  answers whole and the arena renders on its palette alone, which is a
 *  legible state rather than an error. */
/** The painting an arena is shown as — a **named printing**, not whatever the
 *  pool holds. The pool answers with a card's newest printing, which for these
 *  six meant Ninja Turtles art on the Grand Coliseum and Marvel art on Valor's
 *  Reach. Chosen, credited, and hotlinked. */
export interface ArenaArt {
  url: string
  artist: string
  printing: string
}

export interface ColiseumArena {
  key: string
  name: string
  plane: string
  art: ArenaArt
  motion: 'sand' | 'banners' | 'stone' | 'wind' | 'oil' | 'water'
  palette: { ink: string; glow: string }
  backdrop: ColiseumChampion | null
  champions: ColiseumChampion[]
  facts: ColiseumFact[]
}

/** The painting one of the board's own zones is dressed in.
 *
 *  Not an arena and not a card in play: the graveyard, exile and the command
 *  zone are furniture, and they were three-letter labels on a 26px tile. The
 *  printing is pinned in the checked-in prose, because the pool answers a bare
 *  name with the *newest* printing and the newest is increasingly a crossover
 *  — Path to Exile's default art is a Marvel Secret Lair of the Thing.
 *
 *  `ghost` is not a zone but the mark a graveyard raises when a card reaches
 *  it, carried here because it is the same kind of fact: a chosen painting,
 *  pinned, credited and hotlinked. */
export interface ColiseumZone {
  key: 'command' | 'graveyard' | 'exile' | 'ghost'
  card: string
  art: ArenaArt
  /** The argument for this painting over another, so a later session inherits
   *  the reasoning rather than the conclusion. Not rendered. */
  why: string
}

export interface Coliseum {
  arenas: ColiseumArena[]
  /** Checked-in prose, so this answers even with no pool at all. */
  zones: ColiseumZone[]
  /** Whether a card pool answered at all. */
  pool: boolean
  /** Names the pool could not resolve, dropped rather than guessed. */
  dropped: number
}

/** One contestant's tally, with the interval that says how much to believe it.
 *
 *  `rate` is **null below the floor** and that is not an error case to paper
 *  over — it is the answer. A surface that falls back to `wins / played` when
 *  `rate` is null has reinstated exactly the two-game win rate the server
 *  withheld on purpose. Render the counts instead; they are always right. */
export interface ColiseumRecord {
  played: number
  wins: number
  losses: number
  draws: number
  /** Bouts the clock ended. Counted for nobody, and reported apart so the
   *  difference between this and `played` is visible rather than missing. */
  timed_out: number
  rate: number | null
  /** The interval, and `lower` is the order every board is sorted in. */
  lower: number
  upper: number
  /** Past the point where the board stops calling a record provisional. */
  settled: boolean
}

export interface ColiseumDeckRecord {
  owner_id: number | null
  slug: string
  commander: string[]
  archetype: string
  themes: string[]
  matches: number
  record: ColiseumRecord
}

export interface ColiseumClassRecord {
  archetype: string
  decks: number
  record: ColiseumRecord
}

export interface ColiseumDeckRef {
  owner_id: number | null
  slug: string
  commander: string[]
}

export interface ColiseumMeeting {
  a: ColiseumDeckRef
  b: ColiseumDeckRef
  played: number
  a_wins: number
  b_wins: number
  draws: number
  matches: number
  /** Read from `a`'s side, so `record.rate` is a's share of the pair. */
  record: ColiseumRecord
}

export interface ColiseumStandings {
  matches: number
  /** Every bout run, clock-outs included. The records will sum to fewer. */
  games: number
  timed_out: number
  since: string
  until: string
  decks: ColiseumDeckRecord[]
  archetypes: ColiseumClassRecord[]
  meetings: ColiseumMeeting[]
  /** The fewest bouts that may be shown as a rate, and the point a record
   *  stops being provisional. Both travel from the server so the copy says
   *  the real number rather than hard-coding it in a second language. */
  floor: number
  proven: number
}

export const api = {
  health: () => get<Health>('/api/health'),
  // The coliseum's six arenas, their champions and the facts they rotate
  // while a Forge match warms up. Checked-in prose with every card name
  // resolved through the pool; answers before any match is asked for,
  // because the arena has to be on screen while the worker is still waking.
  coliseum: () => get<Coliseum>('/api/coliseum'),
  // What the room remembers: every recorded bout read back as boards. Scoped
  // to the caller — a match they were not in, and the house did not host, is
  // simply not there.
  coliseumStandings: () => get<ColiseumStandings>('/api/coliseum/standings'),
  // Every deck this caller may see, across every owner (ADR 22) — their own
  // first, then the showcase, then everybody else's shared decks. The only
  // place a client learns the owner segment it needs to build any other deck
  // URL at all, which is why nothing here takes a bare slug any more.
  decks: () => get<DeckTile[]>('/api/decks'),
  deck: (ref: DeckRef) => get<DeckDetail>(deckPath(ref)),
  validate: (ref: DeckRef) => get<ValidationReport>(deckPath(ref, '/validate')),
  stats: (ref: DeckRef) => get<DeckStats>(deckPath(ref, '/stats')),
  /** One turn of the Wheel of Fortune (punch list item 9). A POST because
   *  each spin is a fresh draw, but read-only with respect to the deck. */
  wheelSpin: (ref: DeckRef, seed?: number) =>
    post<WheelSpin>(deckPath(ref, '/wheel'),
                    seed === undefined ? {} : { seed }),
  /** Shuffle this deck and turn over seven. A POST for the Wheel's reason —
   *  every press is a fresh deal, not a resource a browser may repeat — and
   *  read-only with respect to the deck, so a reader may deal from a shared
   *  one. **It takes no seed and reports none**: the shuffle is the
   *  server's, and a practice hand has nothing to replay. */
  dealOpeningHand: (ref: DeckRef) =>
    post<OpeningHand>(deckPath(ref, '/opening-hand'), {}),
  suggestions: (ref: DeckRef) =>
    get<Suggestions>(deckPath(ref, '/suggestions')),
  /** Everything this deck makes on the battlefield that was never in it. A
   *  GET where the Wheel and the deal are POSTs: those are fresh draws, this
   *  is a derived fact about the deck that answers the same way every time. */
  deckTokens: (ref: DeckRef) => get<DeckTokens>(deckPath(ref, '/tokens')),
  // What has been done to this deck, newest first (ADR 28). As reachable as
  // the deck and no more — the server resolves it through the same source, so
  // a deck you cannot read has a history you cannot read either, answered by
  // the same 404.
  deckLog: (ref: DeckRef, limit?: number) =>
    get<DeckLog>(deckPath(ref, `/log${limit ? `?limit=${limit}` : ''}`)),
  // The five deliverables (rule 3). Reading follows the deck — they are the
  // shareable surface — and building is a deck write like any other, so a
  // reader gets the list and a 403 on the POST.
  deckArtifacts: (ref: DeckRef) =>
    get<DeckArtifacts>(deckPath(ref, '/artifacts')),
  deckArtifact: (ref: DeckRef, name: string) =>
    get<{ name: string; text: string }>(
      deckPath(ref, `/artifacts/${encodeURIComponent(name)}`)),
  /** Regenerate them. A plain POST rather than a job: measured at 70-83ms on
   *  the instance, which is under what a submit and a poll would cost.
   *  `force` overrides the gate's errors only — a draft is refused outright
   *  and no flag here reaches it. */
  buildArtifacts: (ref: DeckRef, force = false) =>
    post<DeckArtifacts>(deckPath(ref, '/artifacts'), force ? { force } : {}),
  upcomingSets: () => get<{ sets: UpcomingSet[]; as_of: string }>('/api/sets/upcoming'),
  searchCards: (params: Record<string, string | number>) => {
    const qs = new URLSearchParams()
    for (const [k, v] of Object.entries(params)) {
      if (v !== '' && v !== undefined && v !== null) qs.set(k, String(v))
    }
    return get<{ cards: Card[]; total: number; message?: string }>(
      `/api/cards/search?${qs}`,
    )
  },
  /** The typeahead behind "add a card": a short ranked shortlist for a few
   *  letters somebody is still typing, including the misspelled ones that
   *  `searchCards` answers with nothing at all. `limit` is capped server-side.
   *
   *  Unlike `searchCards` this does **not** filter to legal cards: a banned
   *  card is offered and marked, because a card hidden from the list is
   *  indistinguishable from a card that does not exist. */
  suggestCards: (q: string, limit = 8) =>
    get<{ cards: CardOffer[]; message?: string }>(
      `/api/cards/suggest?q=${encodeURIComponent(q)}&limit=${limit}`),
  swapCard: (ref: DeckRef, body: { out: string; into: string; why: string }) =>
    post<SwapResult>(deckPath(ref, '/swap'), body),
  addCard: (
    ref: DeckRef,
    body: { name: string; category: string; why?: string; qty?: number; to?: string },
  ) => post<EditResult>(deckPath(ref, '/cards'), body),
  // The delete from the 99 is an entombment (ADR 27): the server moves the
  // card to the deck's graveyard with its `why` intact, where `returnCard`
  // and `exileCard` reach it. The route is the old DELETE — what changed is
  // where the card goes, not how it is asked for.
  entombCard: (ref: DeckRef, name: string) =>
    send<EditResult>('DELETE', deckPath(ref, `/cards/${encodeURIComponent(name)}`)),
  // Several at once, in one write and one gate verdict. All or nothing on the
  // server, so a partial sweep can never be mistaken for a chosen deck state.
  entombCards: (ref: DeckRef, names: string[]) =>
    post<EditResult>(deckPath(ref, '/entomb'), { names }),
  returnCard: (ref: DeckRef, name: string) =>
    post<EditResult>(deckPath(ref, `/graveyard/${encodeURIComponent(name)}/return`), {}),
  // The only genuinely permanent delete left, and it only reaches a card that
  // was already entombed — two deliberate steps by construction.
  exileCard: (ref: DeckRef, name: string) =>
    send<EditResult>('DELETE', deckPath(ref, `/graveyard/${encodeURIComponent(name)}`)),
  // One field at a time, matching the operation underneath. `why` goes through
  // here: it is the rationale editor's write path, and the value is whatever
  // the user typed — nothing composes, tidies or infers one.
  setCardField: (ref: DeckRef, name: string, field: string, value: string | number) =>
    send<EditResult>('PATCH', deckPath(ref, `/cards/${encodeURIComponent(name)}`),
      { field, value }),
  setNote: (ref: DeckRef, key: string, value: string) =>
    send<EditResult>('PUT', deckPath(ref, `/notes/${encodeURIComponent(key)}`),
      { value }),
  // The deck's own scalars. `stage: curated` is promotion, and the server
  // refuses it while any card is blank rather than writing a deck the gate
  // would immediately reject. `commander_art` goes through here too, and is
  // refused unless the id is a printing of *this* deck's commander.
  // `string[]` is here for `themes`, the one settable field that is a list
  // rather than a scalar. The server validates every entry against its own
  // vocabulary and refuses the whole edit on an unknown one, so a stale client
  // fails loudly rather than writing a label nothing will ever match.
  setDeckField: (ref: DeckRef, field: string,
                 value: string | number | string[]) =>
    send<EditResult>('PATCH', deckPath(ref), { field, value }),
  // Put a deck on display to other accounts, or take it off (ADR 22).
  //
  // Its own route rather than a `field` on `setDeckField`, because the two
  // deck tiers hold this fact in different places — `deck.yaml` for the
  // curated six, a column for everybody else — and the server's source is what
  // knows which. Answers with the whole deck, because `shared` is the one deck
  // field with no `EditResult` to report: it changes who can see the deck and
  // nothing the gate has an opinion about.
  setShared: (ref: DeckRef, shared: boolean) =>
    send<DeckDetail>('PUT', deckPath(ref, '/shared'), { shared }),
  importDeck: (body: {
    slug: string
    text: string
    name?: string
    commander?: string[]
    companion?: string
    bracket?: number | null
    status?: string
    dry_run?: boolean
  }) => post<ImportResult>('/api/decks/import', body),
  // What a camera thought it saw, read against the pool. **No image is sent**
  // — a sighting is a set code, a collector number and a title, and the
  // photograph never leaves the browser. A POST because forty of them with
  // free-text titles do not belong in an access log.
  identifyCards: (sightings: { corner?: string; title?: string }[]) =>
    post<IdentifyResult>('/api/cards/identify', { sightings }),
  // **The one call that sends a photograph** (ADR 34). Never automatic: it
  // happens because somebody pressed a button on one card, having been told
  // what pressing it does. Returns a job — the duration is unmeasured, which
  // is the reason it is a job rather than an argument that it needs to be.
  scanCard: (image: string, mediaType = 'image/jpeg') =>
    post<Job>('/api/claude/scan', { image, media_type: mediaType }),
  // The 32 combinations and their history. No card pool, no decks, no network —
  // so the first screen of the create flow renders on a fresh clone.
  // Separate from `deck()` on purpose: it runs several extra pool queries
  // for a panel that is decorative, so the 99 must never wait on it.
  commander: (ref: DeckRef) => get<CommanderDossier>(deckPath(ref, '/commander')),
  colors: () => get<ColorTaxonomy>('/api/colors'),
  challengeProgress: () => get<ChallengeProgress>('/api/colors/progress'),
  // One combination with its champions and signature cards resolved. Separate
  // from `colors()` because this one needs the pool and that one must not.
  combination: (key: string) =>
    get<CombinationDetail>(`/api/colors/${encodeURIComponent(key)}`),
  // The vocabulary. Memoised below rather than here, because a tooltip may ask
  // for it from anywhere and asking twice on one screen is the normal case.
  glossary: () => get<Glossary>('/api/glossary'),
  /** The labelling vocabulary, for the deck page's editor. Checked-in data:
   *  no pool, no network, and the same for every viewer, so it is fetched
   *  once and held. */
  themes: () => get<ThemeVocabulary>('/api/themes'),
  lore: () => get<LoreShelves>('/api/lore'),
  // The only call here that can lose work — and even this one does not, which
  // is the point of the two calls at the foot of this object. `confirm` must
  // be a word somebody typed — `bury`, or the slug itself — which a mis-aimed
  // click cannot satisfy. The deck goes to the crypt rather than being
  // unlinked, and the answer carries the handle that raises it again.
  deleteDeck: (ref: DeckRef, confirm: string) =>
    send<DeleteResult>('DELETE',
      deckPath(ref, `?confirm=${encodeURIComponent(confirm)}`)),
  // Start a deck from a commander and nothing else. There is no colour field:
  // identity is derived from the commander, and a second source for it would
  // be a second thing to be wrong.
  createDeck: (body: {
    slug: string
    commander: string[]
    name?: string
    companion?: string
    bracket?: number | null
    status?: string
  }) => post<CreateResult>('/api/decks', body),
  // Installed, configured and wanted are three separate answers, and this is
  // the only place that knows all three. Reaches no network.
  // `owner` rides alongside `slug` here because this route resolves a deck by
  // name, exactly as the sim routes do — the slug is a parameter rather than a
  // path segment, so nothing about the URL says whose deck it is (ADR 22).
  claudeStatus: (params: {
    slug?: string; owner?: string; stance?: string
    /** Which mode is asking. Only needed where the default is not the deck's
     *  — the theme interview runs before a deck exists. */
    surface?: string
  } = {}) => {
    const qs = new URLSearchParams()
    for (const [k, v] of Object.entries(params)) if (v) qs.set(k, v)
    return get<ClaudeStatus>(`/api/claude?${qs}`)
  },
  // The rationale interview. A POST because it costs money and calls out, not
  // because it writes anything — it cannot, and there is no field in the
  // response for a rationale even if it wanted to hand one over.
  interview: (ref: DeckRef, body: { card: string; stance?: string; focus?: string }) =>
    post<InterviewReport>(deckPath(ref, '/interview'), body),
  // The slot argument (ADR 25). Same shape as the interview and the same
  // reason for the verb, with one difference worth stating at the call site:
  // this one is **one-directional**. It returns the case against a card and
  // there is no field in the response for the case for it, so a caller cannot
  // render "the balanced view" however it lays the payload out.
  argue: (ref: DeckRef, body: { card: string; stance?: string; focus?: string }) =>
    post<SlotArgumentReport>(deckPath(ref, '/argue'), body),
  // The sweep. Returns a **job**, not a report: one Claude call per selected
  // card means minutes the moment the selection is more than a handful, so
  // follow it with `followJob` and read `job.result` as a `DeckReviewResult`.
  // The server dedupes an identical selection in flight, so a double-click
  // joins the run rather than paying twice.
  argueDeck: (ref: DeckRef, body: { cards: string[]; stance?: string }) =>
    post<Job>(deckPath(ref, '/argue/deck'), body),
  // The intake sheet (ADR 41). Returns a **job**, because five actions over a
  // 99-card deck is minutes, and one job rather than five because the actions
  // run in an order and a person who ticked four boxes wants one thing to
  // watch. Follow it with `followJob` and read `job.result` as an
  // `IntakeResult`.
  //
  // `rationales` is the only action the server can refuse outright: drafting a
  // `why` needs a stance whose write axis is above `none`, so the control for
  // it is not offered below that and a 422 here means the page was stale.
  intake: (ref: DeckRef, body: IntakeSheet) =>
    post<Job>(deckPath(ref, '/intake'), body),
  // The commander dossier, in two halves that are deliberately different verbs.
  // The GET is free and reads a stored row, so the deck page can ask on every
  // load; the POST spends money and reaches the network. One function with a
  // flag is how the free one ends up in a polling loop that is not free.
  dossier: (ref: DeckRef) => get<DossierReport>(deckPath(ref, '/dossier')),
  // Returns a **job**, not a dossier. Measured at 236 seconds on the deployed
  // instance — longer than the theme proposal below, which has been a job
  // since #60 — and what a four-minute POST looks like on a phone is a spinner
  // and then `Load failed`, a transport error carrying no status code to show.
  // Follow it with `followJob` and read `job.result` as a `DossierReport`.
  // A stored dossier comes back already `done`, so a hit still costs nothing.
  writeDossier: (ref: DeckRef, body: { stance?: string; refresh?: boolean } = {}) =>
    post<Job>(deckPath(ref, '/dossier'), body),
  // Every non-digital printing of the commander, newest first. Its own call
  // because most visits never open the picker and Goreclaw has twelve.
  printings: (ref: DeckRef, card?: string) =>
    get<PrintingList>(deckPath(ref, '/printings')
      + (card ? `?card=${encodeURIComponent(card)}` : '')),
  // Research (ADR 26). **Note there is no deck in the path, in the body, or
  // anywhere in this signature** — that absence is the feature, not an
  // omission. The mode cannot reach a library, so it cannot critique a deck,
  // cannot read a rationale, and cannot quietly become the deck conversation
  // ADR 15 leaves unbuilt. Adding an owner or a slug here is the change ADR 26
  // exists to make somebody argue for.
  //
  // Returns a **job**. It searches more than the dossier, which broke deployed
  // at 236 seconds behind a spinner and then `Load failed` — a transport
  // error, so no status code ever reached the client. Follow it with
  // `followJob` and read `job.result` as a `ResearchReport`; pass the job
  // straight back in as `initial`, because a stance of `off` arrives already
  // `done` and the cheap case should still cost exactly one request.
  //
  // Nothing is cached — the subject is the part of Magic that moves — but two
  // identical questions in flight are one job, so a double click costs one
  // search rather than two.
  research: (body: { question: string; stance?: string }) =>
    post<Job>('/api/claude/research', body),
  // The theme interview (ADR 20). Note there is no slug in either path: this
  // runs before a deck exists and never sees one, which is what makes "it
  // builds, it does not critique" structural rather than a request. The whole
  // conversation goes up every turn because this client is where it lives.
  //
  // `persona` and `seed` are client-held for exactly the reason the transcript
  // is: the server stores no conversation, so everything that makes this one
  // the same conversation as last turn has to travel with it. `seed` re-deals
  // the identical spread rather than carrying the cards, which is why three
  // pictures cost one integer.
  //
  // Returns a **job**, like the proposal below and for a weaker-looking but
  // identical reason. A turn is 4.3–37.7s measured, with one at 133.8s, and
  // the transport ceiling is known only as "at or below 236s" — that is where
  // the dossier broke, and nobody has narrowed it. Follow it with `followJob`
  // and read `job.result` as a `ThemeReport`.
  //
  // Pass the job straight back in as `initial`: a turn that reaches nobody
  // (stance `off`, or a finished conversation) arrives already `done`, and
  // `followJob` resolves it without a single poll. The cheap case still costs
  // exactly one request.
  themeAsk: (body: {
    transcript: ThemeTurn[]
    slots: ThemeSlot[]
    stance?: string
    persona?: string
    seed?: number
    /** The texts of every fun fact already shown, client-held and resent the
     *  way the transcript is — the server quotes them back to the model and
     *  drops a repeat, which is what makes "never give the same fact twice"
     *  a rule rather than a hope. */
    facts?: string[]
  }) => post<Job>('/api/claude/theme', body),
  // Returns a **job**, not a proposal — this one was measured at 226 seconds
  // and no hosted proxy holds a POST open that long. Follow it with
  // `followJob` and read `job.result` as a `ThemeProposal`, exactly as the
  // simulator does. Still answers 409 synchronously below the floor: the
  // button is disabled for the same reason, but a floor that lived only here
  // would not be one, and a 409 wrapped in a job error would not be one either.
  themePropose: (body: {
    transcript: ThemeTurn[]
    slots: ThemeSlot[]
    budget?: number
    avoid?: string
    stance?: string
    persona?: string
    seed?: number
  }) => post<Job>('/api/claude/theme/proposal', body),
  // The reader roster (ADR 21). Free, deterministic, and reaching nothing —
  // the same class of thing as `/api/colors`. It answers with no key set and
  // no card pool, which is what lets the door render its whole first screen
  // before anybody has committed to spending a penny.
  personas: () => get<PersonaRoster>('/api/claude/personas'),
  // Deal three cards. No model, no card pool, no network, no cost: a shuffle has
  // a right answer, so the server does it (ADR 14). Pass the seed back to re-deal
  // the same spread — which is what a reload does.
  tarotReading: (seed?: number) =>
    get<TarotReading>(
      seed === undefined ? '/api/tarot/reading' : `/api/tarot/reading?seed=${seed}`),
  // Public, so it is the one call that works before anything else does. The
  // shell reads it to decide whether to ask for a login at all, and the nav to
  // decide whether to offer the admin page.
  me: () => get<AuthState>('/api/auth/me'),
  // The session is the cookie the server sets, which is `HttpOnly` — so this
  // client never holds it, cannot read it, and has nothing to store. What comes
  // back is who it belongs to; the shell re-asks `me` for the rest.
  login: (body: { username: string; password: string }) =>
    post<LoginResult>('/api/auth/login', body),
  logout: () => post<{ authenticated: boolean }>('/api/auth/logout', {}),
  // Answers the same fixed 202 for every address, and the UI must not add a
  // confirmation the server carefully declined to give (ADR 16). Show
  // `detail` as written; it is the whole of what anybody is allowed to learn.
  requestReset: (email: string) =>
    post<{ detail: string }>('/api/auth/reset', { email }),
  // Redeem an emailed link. The token comes out of `location.hash` — never the
  // query string, which would put a live credential in every access log along
  // the way; the server's invite builder makes the same argument. This POST is
  // the only request that carries it, in a JSON body rather than a URL.
  claim: (body: { token: string; password: string; username?: string }) =>
    post<ClaimResult>('/api/auth/claim', body),
  claimPreview: (body: { token: string }) =>
    post<ClaimPreview>('/api/auth/claim/preview', body),
  // Everything under here is refused to a non-admin by the middleware, before
  // routing (ADR 17). Hiding the nav entry is a courtesy to the person using
  // the app, never the protection — a 403 is what actually stops anybody.
  accounts: () => get<AccountList>('/api/admin/users'),
  adminSystem: () => get<AdminSystem>('/api/admin/stats/system'),
  adminStorage: () => get<AdminStorage>('/api/admin/stats/storage'),
  adminClaude: () => get<AdminClaude>('/api/admin/stats/claude'),
  adminActivity: () => get<AdminActivity>('/api/admin/stats/activity'),
  adminTraffic: () => get<AdminTraffic>('/api/admin/stats/traffic'),
  adminFly: () => get<AdminFly>('/api/admin/stats/fly'),
  adminUpkeep: () => get<AdminUpkeep>('/api/admin/upkeep'),
  // ADR 6's long-specified admin refresh, cashed in. It answers 202 with the
  // job to poll, or 409 with the id of the gathering already under way — one
  // at a time, because the shelves can only be rewritten by one thing and
  // because needless bulk traffic is exactly what we were asked not to make.
  refreshLibrary: () => post<Job>('/api/admin/library/refresh', {}),
  inviteAccount: (body: { email: string; username?: string; is_admin?: boolean }) =>
    post<Account>('/api/admin/users', body),
  // One route for both levers, and both refusals come from the server: the
  // last admin who can sign in cannot be demoted or disabled, and the answer
  // is a 409 rather than a silent no-op.
  updateAccount: (username: string,
                  body: { is_admin?: boolean; disabled?: boolean
                          model_tier?: string | null }) =>
    send<Account>('PATCH', `/api/admin/users/${encodeURIComponent(username)}`, body),
  // The only thing an admin may do about a forgotten password. ADR 16 is
  // unconditional that nobody chooses a password for anybody else.
  sendReset: (username: string) =>
    post<{ detail: string }>(
      `/api/admin/users/${encodeURIComponent(username)}/reset`, {}),
  revokeSessions: (username: string) =>
    send<{ username: string; revoked: number }>(
      'DELETE', `/api/admin/users/${encodeURIComponent(username)}/sessions`),
  // The irreversible one, and the only call in this client that asks the caller
  // to type something back. `confirm` must equal the username: the server
  // refuses with 422 otherwise, so a mis-aimed request deletes nothing.
  deleteAccount: (username: string, confirm: string) =>
    send<{ username: string; revoked: number; jobs_dropped: number }>(
      'DELETE', `/api/admin/users/${encodeURIComponent(username)}`, { confirm }),
  // Both take their deck in the **payload** rather than the path, so `owner`
  // goes in the payload too (ADR 22) — a bare slug would reach a deck by name
  // with nobody asked whose it is. Absent, the server reads it as the caller's
  // own library, which is what an old bookmark's `?deck=` amounts to.
  simMana: (payload: Record<string, unknown>) => post<Job>('/api/sim/mana', payload),
  simLands: (payload: Record<string, unknown>) => post<Job>('/api/sim/lands', payload),
  /** The closed form. Answers in the request rather than handing back a job —
   *  it is 40ms of arithmetic, and the one simulation route that is not a
   *  job. The server's shelf-run route carries the measurement behind that. */
  simShelf: (payload: Record<string, unknown>) =>
    post<ShelfResult>('/api/sim/shelf', payload),
  simPolicy: (payload: Record<string, unknown>) =>
    post<Job>('/api/sim/policy', payload),
  forgeStatus: () => get<ForgeStatus>('/api/forge'),
  simForge: (payload: Record<string, unknown>) => post<Job>('/api/sim/forge', payload),
  job: (id: string) => get<Job>(`/api/jobs/${id}`),
  // The deck's description, drafted (lane G, 2026-08-29). The import intake
  // runs this same mode and *writes* what it answers; this route does not,
  // which is the only difference and the whole point of it: on the deck page
  // the field may already hold a paragraph its owner wrote. The draft comes
  // back to the editor's own box, and it reaches the deck file — if it reaches
  // it at all — through `setDeckField`, the same call the person's typing uses.
  //
  // A plain route rather than a job: one call about the whole deck, in the
  // interview's seconds class rather than the intake's minutes.
  describeDeck: (ref: DeckRef, body: { stance?: string } = {}) =>
    post<DeckDescriptionDraft>(deckPath(ref, '/describe'), body),
  /** The crypt: what this player has entombed, newest first.
   *
   * No owner in the path, unlike every other deck call in this client. Your
   * crypt is yours — the server resolves it from who is asking — so there is
   * no URL anybody can type that names somebody else's. */
  entombed: () => get<{ entombed: EntombedDeck[] }>('/api/decks/entombed'),
  /** Raise one deck out of the crypt, under its own name.
   *
   * 422 when a living deck already holds that name: a deck always comes back
   * as itself, so the server asks rather than renaming. The detail is the
   * sentence to show. */
  returnEntombed: (id: string) =>
    post<{ slug: string; name: string; restored: boolean }>(
      `/api/decks/entombed/${encodeURIComponent(id)}/return`, {}),
}

/**
 * A drafted deck description, before anything has written it down.
 *
 * **Nothing here is marked as Claude's in the deck file, and that is a
 * decision.** `why_by: claude` (ADR 41) exists because a rationale is a claim
 * about somebody's thinking and a drafted one is a claim nobody made yet; the
 * mark is dropped the first time a person edits the sentence. This paragraph
 * lands in the owner's own textarea, where they read it and may rewrite half of
 * it before pressing save — it has passed that moment before it is ever
 * written. The honesty is paid where it is owed instead: on screen, while the
 * draft is still a draft, labelled as Claude's and not the gate's.
 */
export interface DeckDescriptionDraft {
  /** ADR 14's third boundary as a field: which system answered. Never a model
   *  id — commandment 10, and `lib/claudecopy.ts` is that rule in code. */
  answered_by: string
  mode: string
  slug: string
  /** False when the stance was `off` — no call was made, which is not the same
   *  as a call that had nothing to say. Render the `reason`, not an error. */
  asked: boolean
  reason: string
  stance: StanceView
  /** The paragraph. Empty when nothing usable came back, in which case
   *  `reason` says so. */
  strategy: string
  /** The deck's index terms. Always a list — `[]` and never null, so a
   *  component may map it without a guard that would read as a fact. */
  themes: string[]
  /** What the draft rests on: counts, the commander's ability, the cards that
   *  make the theme. Shown beside the paragraph, because a draft whose facts
   *  are visible is one somebody can disagree with. */
  fact: string
  /** The promise the payload carries about itself, said by the server so a
   *  second client cannot render this as anything other than a draft. */
  never: string
}

/** How many polls in a row may fail before a followed job is given up on.
 *
 * Not zero, which is what this was. A poll is a bare GET and the thing it is
 * watching runs for minutes — the theme proposal is measured at 226 seconds —
 * so over a four-minute run on a phone in somebody's living room, one dropped
 * request is ordinary. It used to end the run: the first throw rejected the
 * promise, the surface showed an error, and the *server carried on and
 * finished the work* nobody was listening for any more.
 *
 * Six at two seconds is around ten seconds of silence tolerated, which is long
 * enough to cross a wifi handover and short enough that a server that really
 * has gone still reports quickly.
 */
const POLL_FAILURES_TOLERATED = 6

/** Poll a job to completion. Resolves on done, rejects on error.
 *
 * `initial` is the job as the submitting POST returned it. Since results are
 * cached server-side, that response can already be `done` — the work was
 * finished before the request arrived — and polling for a job that has nothing
 * left to do is a wasted round trip and a frame of "Running…" for something
 * that is not running.
 *
 * A failing *poll* is not a failing job (`POLL_FAILURES_TOLERATED`). Two kinds
 * are told apart: a 404 is the server saying it has never heard of this job —
 * definitive, since jobs live in memory and die with the process — and
 * anything else (a dropped connection, a 502 from the proxy, a moment of no
 * network) is provisional and worth asking again about.
 */
export function followJob(
  id: string,
  onTick: (job: Job) => void,
  intervalMs = 400,
  initial?: Job,
): { promise: Promise<Job>; cancel: () => void } {
  let cancelled = false
  const cancel = () => {
    cancelled = true
  }
  if (initial && initial.status === 'done') {
    onTick(initial)
    return { promise: Promise.resolve(initial), cancel }
  }
  const promise = new Promise<Job>((resolve, reject) => {
    // Consecutive, not cumulative: one blip an hour into a long watch says
    // nothing about the next one, so a successful poll clears the count.
    let missed = 0
    const tick = async () => {
      if (cancelled) return
      try {
        const job = await api.job(id)
        missed = 0
        onTick(job)
        if (job.status === 'done') return resolve(job)
        if (job.status === 'error') return reject(new Error(job.error ?? 'job failed'))
        setTimeout(tick, intervalMs)
      } catch (err) {
        const gone = err instanceof ApiError && err.status === 404
        if (gone || ++missed > POLL_FAILURES_TOLERATED) return reject(err)
        setTimeout(tick, intervalMs)
      }
    }
    tick()
  })
  return { promise, cancel }
}
