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
}

export interface DeckDetail extends DeckSummary {
  notes: Record<string, string>
  commander_card: Card | null
  cards: Card[]
  swap_board: Card[]
  corpus_available: boolean
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

export interface ManaResult {
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

export interface LandResult {
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

export class ApiError extends Error {
  // Declared explicitly rather than as a constructor parameter property, which
  // tsconfig's erasableSyntaxOnly disallows.
  status: number

  constructor(message: string, status: number) {
    super(message)
    this.status = status
  }
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
  if (!resp.ok) {
    let detail = `${resp.status} ${resp.statusText}`
    try {
      const body = await resp.json()
      if (body?.detail) detail = body.detail
    } catch {
      /* non-JSON error body; the status line is all we have */
    }
    throw new ApiError(detail, resp.status)
  }
  return resp.json() as Promise<T>
}

async function send<T>(method: string, path: string, body?: unknown): Promise<T> {
  const resp = await fetch(path, {
    method,
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body ?? {}),
  })
  if (!resp.ok) {
    let detail = `${resp.status} ${resp.statusText}`
    try {
      const parsed = await resp.json()
      // A refusal is a plain sentence the user should read as written — "a
      // card in a curated deck needs a `why`". FastAPI's own validation errors
      // arrive as an array, which has no such sentence, so those get stringified.
      if (typeof parsed?.detail === 'string') detail = parsed.detail
      else if (parsed?.detail) detail = JSON.stringify(parsed.detail)
    } catch {
      /* keep the status line */
    }
    throw new ApiError(detail, resp.status)
  }
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
}

export interface ColorTaxonomy {
  colors: { code: string; name: string; wants: string; fears: string }[]
  tiers: { key: string; label: string }[]
  eras: { name: string; setting: string; named: string; story: string }[]
  combinations: Combination[]
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
  // would immediately reject.
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
  colors: () => get<ColorTaxonomy>('/api/colors'),
  challengeProgress: () => get<ChallengeProgress>('/api/colors/progress'),
  // The only call here that can lose work. `confirm` must equal the slug —
  // a value only somebody looking at the right deck can produce, which a
  // mis-aimed click cannot satisfy. The deck moves to `.trash/` rather than
  // being unlinked, and the response says where.
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
  // Public, so it is the one call that works before anything else does. The
  // nav reads it to decide whether to offer the admin page at all.
  me: () => get<AuthState>('/api/auth/me'),
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
  simMana: (payload: Record<string, unknown>) => post<Job>('/api/sim/mana', payload),
  simLands: (payload: Record<string, unknown>) => post<Job>('/api/sim/lands', payload),
  job: (id: string) => get<Job>(`/api/jobs/${id}`),
}

/** Poll a job to completion. Resolves on done, rejects on error. */
export function followJob(
  id: string,
  onTick: (job: Job) => void,
  intervalMs = 400,
): { promise: Promise<Job>; cancel: () => void } {
  let cancelled = false
  const cancel = () => {
    cancelled = true
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
