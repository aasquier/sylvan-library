/** Typed client for the mtglab API. Mirrors src/mtglab/api/service.py. */

export interface Health {
  corpus: boolean
  oracle_cards: number
  printings: number
  bulk_files?: string[]
  decks?: number
  message?: string
}

export interface DeckSummary {
  slug: string
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
  total_cards: number
  land_count: number
  strategy: string
  art_crop: string | null
  color_identity: string[]
  // Gate counts, so the shelf can flag a deck that does not validate. null
  // means the corpus was unavailable and the gate never ran -- which is not
  // the same as passing, and must not render as a pass.
  errors: number | null
  warnings: number | null
}

export interface Card {
  name: string
  category: string
  why: string
  qty: number
  known: boolean
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
  /** Set only when the deck picked a specific printing for its commander, so
   *  the header can name which painting it is showing. Its absence means the
   *  corpus's default printing. */
  printing?: { set_name: string | null; set_code: string } | null
}

export interface DeckDetail extends DeckSummary {
  notes: Record<string, string>
  commander_card: Card | null
  cards: Card[]
  swap_board: Card[]
  corpus_available: boolean
  /** The Scryfall printing id whose art this deck shows, or '' for the
   *  default. A deck property rather than a viewer preference: it lives in
   *  `deck.yaml` and travels with the deck through git. */
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
}

export interface TurnRow {
  turn: number
  lands: number
  mana: number
  unused: number
  spells: number
  commander_down: number
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
  slug: string
  deck_name: string
  games: number
  rows: LandRow[]
  deployment_spread: number
  argmax_lands: number
  flat: boolean
  caveat: string
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
  corpus_available: boolean
  targets: SuggestionTarget[]
}

export interface ImportResult {
  slug: string
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
  /** Names the corpus does not know. Kept in the deck verbatim, never guessed. */
  unknown: string[]
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

export interface Job {
  id: string
  kind: string
  status: 'queued' | 'running' | 'done' | 'error'
  done: number
  total: number
  percent: number
  label: string
  result: unknown
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
}

export interface AccountList {
  users: Account[]
  /** Admins who can actually sign in. The last one cannot be demoted. */
  admins: number
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
 * the Accounts page, or signed out in another tab.
 *
 * Handled here rather than in each route, and that is the same argument
 * `api/auth.py` makes for a middleware over a per-route dependency: eleven
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
  if (resp.status === 401 && !ANSWERS_WITH_401.has(path.split('?')[0])) {
    for (const listener of sessionListeners) listener()
  }
  throw new ApiError(detail, resp.status, retryAfterOf(resp))
}

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
 */
export function errorMessage(e: unknown): string {
  return e instanceof Error ? e.message : String(e)
}

async function get<T>(path: string): Promise<T> {
  const resp = await fetch(path)
  if (!resp.ok) await refuse(resp, path)
  return resp.json() as Promise<T>
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
   * What happened to this faction. Empty for the twelve slots that are not
   * one — Mono-Red is not from anywhere, and does not get a story pretending
   * it is.
   */
  lore: string
  /**
   * The faces of the faction: a card name and who they are in the story.
   *
   * Names only here, because `/api/colors` reaches no corpus. The cards
   * themselves come from `combination()` below, which does — that split is
   * what keeps this payload working on a fresh clone.
   */
  champions: { card: string; role: string }[]
  /** Names of cards whose colour identity is exactly this combination. */
  signature: string[]
}

/**
 * One combination with its cards resolved against the corpus.
 *
 * `dropped` counts names that did not resolve, which are left out rather than
 * rendered from the name alone — the same instrument ADR 19 uses for the
 * dossier's rivals. `exact_total` is how many cards in the whole corpus have
 * exactly this identity, and it is the sharpest thing on the page: two, for
 * Artifice.
 */
export interface CombinationDetail extends Omit<Combination,
  'champions' | 'signature'> {
  corpus: boolean
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

export interface ColorTaxonomy {
  colors: { code: string; name: string; wants: string; fears: string }[]
  // `blurb` is what the tier *is* and every tier has one; `eras` below is the
  // block of Magic that supplied the names, and only three tiers have that.
  tiers: { key: string; label: string; blurb: string }[]
  eras: { name: string; setting: string; named: string; story: string }[]
  combinations: Combination[]
}

/**
 * What the corpus knows about a deck's commander, beyond the card itself.
 *
 * Every number here was counted by `service.commander_dossier` over the
 * corpus. None of it is written into the UI, which is rule 1 applied to the
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
  corpus: boolean
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
  /** Where the deck went. Not a boolean, because "deleted" and "recoverable"
   *  have to be separately true and separately visible. */
  moved_to: string
  total_cards: number
  stage: string
  status: string
}

export interface CreateResult {
  slug: string
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
  /** The corpus or gate fact the question rests on. Never an opinion. */
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

/**
 * The commander dossier (ADR 19) — the half of the header the corpus cannot
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

export interface DossierRival extends DossierSection {
  name: string
  mana_cost?: string | null
  type_line?: string | null
  color_identity?: string[]
  image?: string | null
  art_crop?: string | null
  legal_commander?: boolean
  /** The corpus's own text for the rival, so the real card sits next to the
   *  sentence comparing it. */
  oracle_text?: string | null
}

export interface DossierBody {
  who: DossierSection
  archetype: DossierSection & { name: string }
  rivals: DossierRival[]
  standing: DossierSection
  sources: { id: string; title: string; url: string }[]
  /** Cited pages the search never returned. A number that climbs is a prompt
   *  inventing citations, which is why it is rendered rather than logged. */
  sources_dropped: number
  /** Named rivals the corpus does not have. */
  rivals_dropped: number
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
 * There is no `meaning`, here or on the server. Python shuffles; the reader
 * reads.
 */
export interface TarotDrawn {
  key: string
  name: string
  /** major | minor */
  arcana: string
  suit: string | null
  number: number
  /** `/tarot/<key>.webp` — package data, served beside the API. */
  image: string
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

export const api = {
  health: () => get<Health>('/api/health'),
  decks: () => get<DeckSummary[]>('/api/decks'),
  deck: (slug: string) => get<DeckDetail>(`/api/decks/${slug}`),
  validate: (slug: string) => get<ValidationReport>(`/api/decks/${slug}/validate`),
  stats: (slug: string) => get<DeckStats>(`/api/decks/${slug}/stats`),
  suggestions: (slug: string) =>
    get<Suggestions>(`/api/decks/${slug}/suggestions`),
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
  swapCard: (slug: string, body: { out: string; into: string; why: string }) =>
    post<SwapResult>(`/api/decks/${slug}/swap`, body),
  addCard: (
    slug: string,
    body: { name: string; category: string; why?: string; qty?: number; to?: string },
  ) => post<EditResult>(`/api/decks/${slug}/cards`, body),
  removeCard: (slug: string, name: string) =>
    send<EditResult>('DELETE', `/api/decks/${slug}/cards/${encodeURIComponent(name)}`),
  // One field at a time, matching the operation underneath. `why` goes through
  // here: it is the rationale editor's write path, and the value is whatever
  // the user typed — nothing composes, tidies or infers one.
  setCardField: (slug: string, name: string, field: string, value: string | number) =>
    send<EditResult>('PATCH', `/api/decks/${slug}/cards/${encodeURIComponent(name)}`,
      { field, value }),
  setNote: (slug: string, key: string, value: string) =>
    send<EditResult>('PUT', `/api/decks/${slug}/notes/${encodeURIComponent(key)}`,
      { value }),
  // The deck's own scalars. `stage: curated` is promotion, and the server
  // refuses it while any card is blank rather than writing a deck the gate
  // would immediately reject. `commander_art` goes through here too, and is
  // refused unless the id is a printing of *this* deck's commander.
  setDeckField: (slug: string, field: string, value: string | number) =>
    send<EditResult>('PATCH', `/api/decks/${slug}`, { field, value }),
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
  // The 32 combinations and their history. No corpus, no decks, no network —
  // so the first screen of the create flow renders on a fresh clone.
  // Separate from `deck()` on purpose: it runs several extra corpus queries
  // for a panel that is decorative, so the 99 must never wait on it.
  commander: (slug: string) => get<CommanderDossier>(`/api/decks/${slug}/commander`),
  colors: () => get<ColorTaxonomy>('/api/colors'),
  challengeProgress: () => get<ChallengeProgress>('/api/colors/progress'),
  // One combination with its champions and signature cards resolved. Separate
  // from `colors()` because this one needs the corpus and that one must not.
  combination: (key: string) =>
    get<CombinationDetail>(`/api/colors/${encodeURIComponent(key)}`),
  // The vocabulary. Memoised below rather than here, because a tooltip may ask
  // for it from anywhere and asking twice on one screen is the normal case.
  glossary: () => get<Glossary>('/api/glossary'),
  // The only call here that can lose work. `confirm` must be a word somebody
  // typed — `bury`, or the slug itself — which a mis-aimed click cannot
  // satisfy. The deck moves to `.trash/` rather than being unlinked, and the
  // response says where.
  deleteDeck: (slug: string, confirm: string) =>
    send<DeleteResult>('DELETE',
      `/api/decks/${slug}?confirm=${encodeURIComponent(confirm)}`),
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
  claudeStatus: (params: { slug?: string; stance?: string } = {}) => {
    const qs = new URLSearchParams()
    for (const [k, v] of Object.entries(params)) if (v) qs.set(k, v)
    return get<ClaudeStatus>(`/api/claude?${qs}`)
  },
  // The rationale interview. A POST because it costs money and calls out, not
  // because it writes anything — it cannot, and there is no field in the
  // response for a rationale even if it wanted to hand one over.
  interview: (slug: string, body: { card: string; stance?: string; focus?: string }) =>
    post<InterviewReport>(`/api/decks/${slug}/interview`, body),
  // The commander dossier, in two halves that are deliberately different verbs.
  // The GET is free and reads a stored row, so the deck page can ask on every
  // load; the POST spends money and reaches the network. One function with a
  // flag is how the free one ends up in a polling loop that is not free.
  dossier: (slug: string) => get<DossierReport>(`/api/decks/${slug}/dossier`),
  // Returns a **job**, not a dossier. Measured at 236 seconds on the deployed
  // instance — longer than the theme proposal below, which has been a job
  // since #60 — and what a four-minute POST looks like on a phone is a spinner
  // and then `Load failed`, a transport error carrying no status code to show.
  // Follow it with `followJob` and read `job.result` as a `DossierReport`.
  // A stored dossier comes back already `done`, so a hit still costs nothing.
  writeDossier: (slug: string, body: { stance?: string; refresh?: boolean } = {}) =>
    post<Job>(`/api/decks/${slug}/dossier`, body),
  // Every non-digital printing of the commander, newest first. Its own call
  // because most visits never open the picker and Goreclaw has twelve.
  printings: (slug: string) => get<PrintingList>(`/api/decks/${slug}/printings`),
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
  // no corpus, which is what lets the door render its whole first screen
  // before anybody has committed to spending a penny.
  personas: () => get<PersonaRoster>('/api/claude/personas'),
  // Deal three cards. No model, no corpus, no network, no cost: a shuffle has
  // a right answer, so Python does it (ADR 14). Pass the seed back to re-deal
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
  // the way; see `auth/invites.py`. This POST is the only request that carries
  // it, in a JSON body rather than a URL.
  claim: (body: { token: string; password: string; username?: string }) =>
    post<ClaimResult>('/api/auth/claim', body),
  claimPreview: (body: { token: string }) =>
    post<ClaimPreview>('/api/auth/claim/preview', body),
  // Everything under here is refused to a non-admin by the middleware, before
  // routing (ADR 17). Hiding the nav entry is a courtesy to the person using
  // the app, never the protection — a 403 is what actually stops anybody.
  accounts: () => get<AccountList>('/api/admin/users'),
  inviteAccount: (body: { email: string; username?: string; is_admin?: boolean }) =>
    post<Account>('/api/admin/users', body),
  // One route for both levers, and both refusals come from the server: the
  // last admin who can sign in cannot be demoted or disabled, and the answer
  // is a 409 rather than a silent no-op.
  updateAccount: (username: string, body: { is_admin?: boolean; disabled?: boolean }) =>
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
  simMana: (payload: Record<string, unknown>) => post<Job>('/api/sim/mana', payload),
  simLands: (payload: Record<string, unknown>) => post<Job>('/api/sim/lands', payload),
  job: (id: string) => get<Job>(`/api/jobs/${id}`),
}

/** Poll a job to completion. Resolves on done, rejects on error.
 *
 * `initial` is the job as the submitting POST returned it. Since results are
 * cached server-side, that response can already be `done` — the work was
 * finished before the request arrived — and polling for a job that has nothing
 * left to do is a wasted round trip and a frame of "Running…" for something
 * that is not running.
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
    const tick = async () => {
      if (cancelled) return
      try {
        const job = await api.job(id)
        onTick(job)
        if (job.status === 'done') return resolve(job)
        if (job.status === 'error') return reject(new Error(job.error ?? 'job failed'))
        setTimeout(tick, intervalMs)
      } catch (err) {
        reject(err)
      }
    }
    tick()
  })
  return { promise, cancel }
}
