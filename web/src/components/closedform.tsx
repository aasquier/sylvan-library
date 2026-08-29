import { useState } from 'react'
import type {
  CardOddsRow, ColorRequirementRow, DeckCheck, ManaCurve, PolicyResult,
  ShelfResult, ValidationReport,
} from '../lib/api'
import { Badge, ManaCost, StatTile } from './ui'
import { HelpTip, Term } from './term'
import { heatInk, heatPercent, heatWash, lagTone } from '../lib/heat'

/**
 * Tier 1.5 on screen: the closed form, rendered *beside* the simulation.
 *
 * Every block here obeys one rule, and it is commandment 2 rather than a
 * styling preference: **say what the number means before saying what it is.**
 * A shelf of bare percentages is a screen that makes a newcomer feel stupid,
 * and this is the one screen in the app whose whole purpose is teaching a
 * thing that is genuinely hard — why a triple-symbol card is a different
 * animal from a single-symbol one, and why "add more lands" is not the
 * answer to every mana problem.
 *
 * Pure presentation. Every verdict — met, flat, which rung is short — is
 * decided on the server and read off the payload, never recomputed here. A second
 * implementation of `flat` in TypeScript would be a second chance to get it
 * wrong, which is the same argument `stance.tsx` makes about `clamp`.
 */

/** One colour's ladder: a rung per symbol count the deck actually asks for. */
function ColorLadder({ req }: { req: ColorRequirementRow }) {
  return (
    <div className="rounded-lg p-3" style={{ background: 'var(--gridline)' }}>
      <div className="flex items-baseline gap-2">
        <ManaCost cost={`{${req.color}}`} size={18} />
        <span className="text-sm font-semibold">
          {req.have} sources
        </span>
        <span className="text-xs" style={{ color: 'var(--text-secondary)' }}>
          {req.have_lands} lands, {req.have - req.have_lands} other
        </span>
      </div>
      <div className="mt-2 space-y-1.5">
        {req.tiers.map((tier) => (
          <div key={tier.pips} className="text-xs">
            <div className="flex flex-wrap items-center gap-2">
              {/* One template literal rather than three interleaved
                  expressions: JSX would emit each as its own text node, and a
                  label split across nodes cannot be found by the text a
                  reader sees -- by a test or by a screen reader. */}
              <span className="font-medium" style={{ minWidth: '5.5rem' }}>
                {`${tier.pips} symbol${tier.pips > 1 ? 's' : ''} on T${tier.turn}`}
              </span>
              <span style={{ color: 'var(--text-secondary)' }}>
                wants {tier.need}
              </span>
              <Badge tone={tier.met ? 'good' : 'critical'}>
                {tier.met ? 'covered' : `short ${tier.shortfall}`}
              </Badge>
              <span style={{ color: 'var(--text-secondary)' }}>
                you make it {(tier.odds_now * 100).toFixed(0)}% of the time
              </span>
            </div>
            {/* Counted against what was actually shown, not against three:
                the server caps the list at six but a rung may name fewer, and
                subtracting a constant reports the wrong remainder whenever it
                does. */}
            <div className="mt-0.5 italic" style={{ color: 'var(--text-muted)' }}>
              {(() => {
                const shown = tier.cards.slice(0, 3)
                const rest = tier.card_count - shown.length
                return shown.join(', ') + (rest > 0 ? ` and ${rest} more` : '')
              })()}
            </div>
          </div>
        ))}
      </div>
    </div>
  )
}

/**
 * The castability heatmap: a card per row, a turn per column.
 *
 * Deliberately not a chart. Ninety rows of ten values is a table, and a table
 * is the one shape that lets somebody find their own card and read across it —
 * which is what people actually do here. The colour is a wash behind real
 * numbers rather than instead of them, so the grid is still readable to
 * anybody who cannot separate the hues.
 */
function Heatmap({ shelf, rows }: { shelf: ShelfResult; rows: CardOddsRow[] }) {
  const turns = Array.from({ length: shelf.horizon }, (_, i) => i + 1)
  return (
    <div className="overflow-x-auto">
      <table className="w-full min-w-[34rem] border-collapse text-xs">
        <thead>
          <tr style={{ color: 'var(--text-muted)' }}>
            <th className="sticky left-0 z-10 px-2 py-1 text-left font-medium"
                style={{ background: 'var(--surface-1)' }}>
              Card
            </th>
            <th className="px-1 py-1 text-right font-medium">Cost</th>
            {turns.map((t) => (
              <th key={t} className="px-1 py-1 text-center font-medium">T{t}</th>
            ))}
            <th className="px-2 py-1 text-right font-medium">
              Lag<HelpTip name="stat.card_lag" />
            </th>
          </tr>
        </thead>
        <tbody>
          {rows.map((row) => (
            <tr key={row.name}>
              <td className="sticky left-0 z-10 max-w-[13rem] truncate px-2 py-1"
                  style={{ background: 'var(--surface-1)' }} title={row.name}>
                {row.name}
              </td>
              <td className="px-1 py-1 text-right"
                  style={{ color: 'var(--text-secondary)' }}>
                {row.mv}
              </td>
              {turns.map((t) => {
                const odds = row.by_turn[t - 1] ?? 0
                // The turn a card costs is ringed, so the eye lands on the
                // question being asked -- "is it there when I need it" --
                // rather than on the rightmost column, which is always best.
                const onCurve = t === row.mv
                return (
                  <td key={t} className="px-1 py-1 text-center tabular-nums"
                      style={{
                        background: heatWash(odds),
                        color: heatInk(odds),
                        outline: onCurve ? '1px solid var(--text-primary)' : undefined,
                        outlineOffset: '-1px',
                      }}>
                    {heatPercent(odds)}
                  </td>
                )
              })}
              <td className="px-2 py-1 text-right font-medium"
                  style={{ color: lagTone(row.lag) }}>
                {row.lag === null
                  ? (row.mv > shelf.horizon ? '—' : 'never')
                  : `+${row.lag}`}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

export function ClosedForm({ shelf }: { shelf: ShelfResult }) {
  const [showAll, setShowAll] = useState(false)
  const est = shelf.lands_estimate
  const short = shelf.colors.filter((c) => !c.met)
  const allTiers = shelf.colors.flatMap((c) => c.tiers)
  const rungs = allTiers.length
  const unmetRungs = allTiers.filter((t) => !t.met).length
  // Cards past the horizon are excluded from the default view rather than
  // sorted last: the shelf was never asked about turn 12, and a list led by
  // twelve-drops answers a question nobody had.
  const inHorizon = shelf.cards.filter((c) => c.mv <= shelf.horizon)
  const rows = showAll ? shelf.cards : inHorizon.slice(0, 12)

  return (
    <div className="space-y-5">
      <p className="text-sm" style={{ color: 'var(--text-secondary)' }}>
        <Term name="tier-1.5">The closed form</Term> works the odds out
        directly instead of shuffling — exactly, and instantly, but about a
        simpler game than the one Tier 1 plays. It assumes you keep your
        opening seven and hit every land drop, and it cannot see ramp at all.
        Run the mana simulation beside it: where the two disagree is a reading
        of what actually governs your deck.
      </p>

      <div className="grid gap-3 sm:grid-cols-3">
        <StatTile
          label="Deck"
          value={`${shelf.lands} / ${shelf.deck_size}`}
          hint="lands in the ninety-nine" />
        <StatTile
          label="Suggested lands"
          value={`${est.recommended}`}
          tone={Math.abs(est.delta) >= 3 ? 'warning' : 'good'}
          hint={est.delta === 0 ? 'exactly what you run'
            : `${est.delta > 0 ? '+' : ''}${est.delta} on what you run`}
          help={<HelpTip name="stat.regression_lands" />} />
        {/* Counted in rungs, not colours. A colour-level tile reads "0 / 1"
            for a mono-green deck that covers every single- and double-symbol
            card it owns and misses only one triple — which is precisely the
            collapsed verdict the ladder below exists to avoid, put back at
            the top of the screen in bigger type. */}
        <StatTile
          label="Demands met"
          value={`${rungs - unmetRungs} / ${rungs}`}
          tone={unmetRungs === 0 ? 'good' : 'warning'}
          hint={unmetRungs === 0 ? 'every symbol count covered'
            : `short on ${short.map((c) => c.color).join(', ')} at its greediest`}
          help={<HelpTip name="stat.sources_needed" />} />
      </div>

      <section className="space-y-2">
        <h3 className="text-sm font-semibold">
          Coloured sources<HelpTip name="stat.sources_needed" />
        </h3>
        <p className="text-xs" style={{ color: 'var(--text-secondary)' }}>
          One rung per <Term name="pip">symbol</Term> count your deck actually
          asks for, judged on the cheapest card making that demand. Being short
          for your greediest card is not the same as being short of the colour
          — a deck can cover every single-symbol card it owns and still miss on
          one triple.
        </p>
        <div className="grid gap-2 sm:grid-cols-2">
          {shelf.colors.map((req) => <ColorLadder key={req.color} req={req} />)}
        </div>
        {shelf.colors.length === 0 && (
          <p className="text-xs" style={{ color: 'var(--text-muted)' }}>
            This deck asks for no coloured mana at all.
          </p>
        )}
      </section>

      <section className="space-y-2">
        <h3 className="text-sm font-semibold">
          Suggested land count<HelpTip name="stat.regression_lands" />
        </h3>
        <p className="text-xs" style={{ color: 'var(--text-secondary)' }}>
          You run {est.lands_now}; the formula says {est.recommended}, from an
          average mana value of {est.average_mana_value} and{' '}
          {est.cheap_accelerants} cheap accelerant
          {est.cheap_accelerants === 1 ? '' : 's'}. It is a line fitted to
          sixty-card tournament decks and rescaled, so read it as a starting
          point — and read the land sweep beside it, which simulates this deck
          and prices <Term name="flood">flooding</Term>.
        </p>
        <ul className="ml-4 list-disc text-xs" style={{ color: 'var(--text-muted)' }}>
          {est.caveats.map((c) => <li key={c}>{c}</li>)}
        </ul>
      </section>

      <section className="space-y-2">
        <h3 className="text-sm font-semibold">
          When the mana is there<HelpTip name="stat.card_lag" />
        </h3>
        <p className="text-xs" style={{ color: 'var(--text-secondary)' }}>
          The chance you could pay for each card on each turn,{' '}
          <strong>assuming it is in your hand</strong> — this asks about the
          mana base, not about drawing. The ringed cell is the turn the card
          costs. Sorted by how far each one runs behind its own cost, so the
          rows worth acting on are at the top.
        </p>
        <Heatmap shelf={shelf} rows={rows} />
        {inHorizon.length > 12 && (
          <button type="button" className="btn btn-quiet btn-sm"
                  aria-expanded={showAll}
                  onClick={() => setShowAll((v) => !v)}>
            {showAll ? 'Show the worst twelve'
              : `Show all ${shelf.cards.length} cards`}
          </button>
        )}
        {shelf.approximated.length > 0 && (
          <p className="text-xs" style={{ color: 'var(--text-muted)' }}>
            {shelf.approximated.length} card
            {shelf.approximated.length === 1 ? '' : 's'} demand two or more
            colours at once, where this method approximates and reads slightly
            low: {shelf.approximated.slice(0, 4).join(', ')}
            {shelf.approximated.length > 4 && ', and others'}.
          </p>
        )}
      </section>
    </div>
  )
}

/** The mulligan policy search: which keep rule deploys most, or that none does. */
export function PolicyReport({ policy }: { policy: PolicyResult }) {
  const { best, baseline, gentlest } = policy
  const gentlerThanDefault = gentlest.mulligan_rate < baseline.mulligan_rate - 0.05

  return (
    <div className="space-y-5">
      <p className="text-sm" style={{ color: 'var(--text-secondary)' }}>
        Your <Term name="mulligan">mulligan</Term> rule is a real lever, so it
        is searched rather than assumed: {policy.rows.length} rules, each
        played out over {policy.games.toLocaleString()} games. They are judged
        on <Term name="stat.spells_through_t8">spells deployed through turn
        8</Term> — mulligan rate alone would recommend keeping every hand, and
        hand quality alone would recommend mulliganing forever.
      </p>

      {/* The verdict, in words, and the server decided it. */}
      {policy.flat ? (
        <div className="rounded-lg p-4" style={{ background: 'var(--gridline)' }}>
          <div className="text-sm font-semibold">
            Your default rule is already right.
          </div>
          <p className="mt-1 text-xs" style={{ color: 'var(--text-secondary)' }}>
            The best rule in the grid beats it by {policy.gain.toFixed(2)}{' '}
            spells through turn 8, which is inside the noise. The grid spans{' '}
            {policy.spread.toFixed(2)} spells overall, but most of that range
            is rules nobody would play — so flatness is measured against your
            default, not against the grid.
          </p>
          {gentlerThanDefault && (
            <p className="mt-2 text-xs">
              <strong>Still worth knowing:</strong> “{gentlest.describe}”
              deploys the same ({gentlest.spells_through_t8.toFixed(2)}) while
              mulliganing {(gentlest.mulligan_rate * 100).toFixed(0)}% of hands
              instead of {(baseline.mulligan_rate * 100).toFixed(0)}%. Same
              result, fewer hands thrown away.
            </p>
          )}
        </div>
      ) : (
        <div className="rounded-lg p-4"
             style={{ background: 'color-mix(in srgb, var(--status-good) 12%, transparent)' }}>
          <div className="text-sm font-semibold">{best.describe}</div>
          <p className="mt-1 text-xs" style={{ color: 'var(--text-secondary)' }}>
            {best.spells_through_t8.toFixed(2)} spells through turn 8,{' '}
            {policy.gain > 0 ? '+' : ''}{policy.gain.toFixed(2)} against your
            default’s {baseline.spells_through_t8.toFixed(2)} — mulliganing{' '}
            {(best.mulligan_rate * 100).toFixed(0)}% of hands against{' '}
            {(baseline.mulligan_rate * 100).toFixed(0)}%.
          </p>
        </div>
      )}

      <div className="grid gap-3 sm:grid-cols-3">
        <StatTile label="Policy gain" value={`${policy.gain > 0 ? '+' : ''}${policy.gain.toFixed(2)}`}
                  tone={policy.flat ? undefined : 'good'}
                  hint="spells, best against default"
                  help={<HelpTip name="stat.policy_gain" />} />
        <StatTile label="Best rule mulligans"
                  value={`${(best.mulligan_rate * 100).toFixed(0)}%`}
                  hint={`default throws ${(baseline.mulligan_rate * 100).toFixed(0)}%`}
                  help={<HelpTip name="stat.mulligan_rate" />} />
        <StatTile label="Grid spread" value={policy.spread.toFixed(2)}
                  hint="best minus worst, all rules"
                  help={<HelpTip name="stat.deployment_spread" />} />
      </div>

      <div className="overflow-x-auto">
        <table className="w-full min-w-[30rem] border-collapse text-xs">
          <thead>
            <tr style={{ color: 'var(--text-muted)' }}>
              <th className="px-2 py-1 text-left font-medium">Keep rule</th>
              <th className="px-2 py-1 text-right font-medium">Spells T8</th>
              <th className="px-2 py-1 text-right font-medium">Mulligans</th>
              <th className="px-2 py-1 text-right font-medium">Commander</th>
            </tr>
          </thead>
          <tbody>
            {policy.rows.slice(0, 10).map((row) => {
              const isBest = row.describe === best.describe
              const isDefault = row.describe === baseline.describe
              return (
                <tr key={row.describe}
                    style={{ fontWeight: isBest || isDefault ? 600 : undefined }}>
                  <td className="px-2 py-1">
                    {row.describe}
                    {isBest && <> <Badge tone="good">best</Badge></>}
                    {isDefault && <> <Badge>default</Badge></>}
                  </td>
                  <td className="px-2 py-1 text-right tabular-nums">
                    {row.spells_through_t8.toFixed(2)}
                  </td>
                  <td className="px-2 py-1 text-right tabular-nums">
                    {(row.mulligan_rate * 100).toFixed(0)}%
                  </td>
                  <td className="px-2 py-1 text-right tabular-nums">
                    {row.median_commander_turn === null
                      ? '—' : `T${row.median_commander_turn}`}
                  </td>
                </tr>
              )
            })}
          </tbody>
        </table>
      </div>
    </div>
  )
}

/**
 * The gate's verdict, shown above the numbers it applies to.
 *
 * The simulator does not refuse an invalid deck, so this is what keeps that
 * honest. Two registers, and the server decides which one applies rather than
 * this file guessing from the error codes: a deck that is illegal in a way
 * that leaves the figures describing *the deck as written*, and one that is
 * illegal in a way that makes the figures describe **a different deck** — a
 * card the pool never resolved was dropped, so every probability below is
 * over a smaller library than the one on the page.
 *
 * Renders nothing at all for a clean deck. A banner that always says
 * something is a banner people stop reading.
 */
/**
 * What a run is *about to* leave out, said before anybody pays for it.
 *
 * `DeckVerdict` below is the same fact after the fact, and it is the better
 * of the two: it reports `simulated_size` of `declared_size` because the
 * compiler counted them, rather than deriving a number the compiler alone
 * can know. What it cannot do is arrive in time. A Tier 1 run is seconds and
 * a Forge match is minutes, and in both cases the first thing anybody learns
 * about the cards that fell out of their deck is a panel underneath figures
 * they have already read (Aaron, 2026-08-24: "Simulator should not allow
 * errored decks, or should omit the error cards at the least"). Aaron's
 * ruling was to omit loudly and never refuse — refusing takes the diagnosis
 * away exactly when it is wanted, which is `compileChecked`'s whole argument
 * and commandment 2 with the sign flipped.
 *
 * So this is the loud half, and it is **deliberately not numeric**. It names
 * the cards the pool does not know and says the figures will be about what
 * is left; it does not say how many are left, because that number belongs to
 * `compile.Report` and a second derivation of it here is exactly the drift
 * `theaterRows` and `_row` were built to make impossible one layer down. The
 * count arrives with the results, from the code that counted it.
 */
export function DeckCaution({ report, name }: {
  report: ValidationReport | undefined
  name: string
}) {
  if (!report || report.ok) return null
  // The pool not knowing a name is a different kind of trouble from the deck
  // being illegal: one shrinks what gets simulated, the other does not.
  const missing = report.errors.filter((e) => e.code === 'unknown-card')
  const others = report.errors.length - missing.length
  const tone = missing.length > 0 ? 'var(--status-critical)' : 'var(--status-warning)'
  return (
    <div className="rounded-lg p-3 text-xs"
         style={{
           background: `color-mix(in srgb, ${tone} 10%, transparent)`,
           borderLeft: `2px solid ${tone}`,
         }}>
      {missing.length > 0 && (
        <p style={{ color: 'var(--text-secondary)' }}>
          <span className="font-semibold" style={{ color: 'var(--text-primary)' }}>
            {missing.length} card{missing.length === 1 ? '' : 's'} in {name} {missing.length === 1 ? 'is' : 'are'} not
            in the card pool
          </span>{' '}
          — {missing.slice(0, 4).map((e) => e.card).filter(Boolean).join(', ')}
          {missing.length > 4 && `, and ${missing.length - 4} more`}. A run
          leaves {missing.length === 1 ? 'it' : 'them'} out and the figures
          describe what is left; every result says exactly how many cards that
          was. Check the spelling on the deck page, or refresh the pool.
        </p>
      )}
      {others > 0 && (
        <p className={missing.length > 0 ? 'mt-1.5' : ''}
           style={{ color: 'var(--text-secondary)' }}>
          <span className="font-semibold" style={{ color: 'var(--text-primary)' }}>
            {name} does not pass the gate
          </span>{' '}
          ({others} {others === 1 ? 'error' : 'errors'} beyond the spelling).
          It is still simulated: the figures describe the deck exactly as
          written, which is what you want while you are fixing it.
        </p>
      )}
    </div>
  )
}

export function DeckVerdict({ check }: { check: DeckCheck | undefined }) {
  if (!check || (check.ok && check.unresolved_count === 0)) return null

  const dropped = check.unresolved_count > 0 || check.commander_unresolved
  const tone = check.affects_numbers ? 'var(--status-critical)' : 'var(--status-warning)'

  return (
    <div className="rounded-lg p-3 text-xs"
         style={{
           background: `color-mix(in srgb, ${tone} 10%, transparent)`,
           borderLeft: `2px solid ${tone}`,
         }}>
      <div className="font-semibold" style={{ color: 'var(--text-primary)' }}>
        {dropped
          ? `${check.simulated_size} of ${check.declared_size} cards were simulated`
          : 'This deck does not pass the gate'}
      </div>
      <p className="mt-1" style={{ color: 'var(--text-secondary)' }}>
        {dropped ? (
          <>
            {check.unresolved_count > 0 && <>
              {check.unresolved_count} card
              {check.unresolved_count === 1 ? ' is' : 's are'} not in the card
              pool and {check.unresolved_count === 1 ? 'was' : 'were'} left
              out ({check.unresolved.join(', ')}
              {check.unresolved_count > check.unresolved.length && ', and others'}).{' '}
            </>}
            {check.commander_unresolved && <>The commander did not resolve either.{' '}</>}
            Every figure below is worked out over the {check.simulated_size} cards
            that did resolve, so read them as being about that deck rather than
            about yours.
          </>
        ) : check.affects_numbers ? (
          <>
            The figures below describe the deck exactly as written — which is
            not a deck you can legally play. Fixing the gate’s complaint will
            move them.
          </>
        ) : (
          <>
            The gate’s complaints are about the deck’s paperwork rather than its
            mana, so the figures below are unaffected by them.
          </>
        )}
      </p>
      {check.errors.length > 0 && (
        <ul className="mt-1.5 ml-4 list-disc" style={{ color: 'var(--text-muted)' }}>
          {check.errors.map((e) => (
            <li key={`${e.code}-${e.card ?? ''}`}>
              {e.card ? `${e.card}: ${e.message}` : e.message}
            </li>
          ))}
          {check.error_count > check.errors.length && (
            <li>and {check.error_count - check.errors.length} more</li>
          )}
        </ul>
      )}
    </div>
  )
}

/**
 * The mana curve: do you have the mana you want, when you want it.
 *
 * Two controls upstream of this — a turn and an amount — and the second one is
 * what makes the advice mean anything. Asked for "four mana on turn four" the
 * answer is always lands, because a land is one mana a turn and you may play
 * one a turn; nothing beats that at exactly the curve. Ask for *more* than the
 * turn and a land is worth nothing at all, because you cannot play a fifth one
 * on turn four — and ramp becomes the only answer.
 *
 * That rule is the thing this panel is really teaching, so it is stated in
 * words rather than left to be inferred from two percentages.
 */
export function ManaCurvePanel({ curve }: { curve: ManaCurve }) {
  const a = curve.advice
  const peak = Math.max(...curve.turns.map((t) => t.expected_mana), 1)

  const verdict =
    a.recommend === 'none'
      ? `You already make ${a.target_mana} mana on turn ${a.target_turn} often enough.`
      : a.recommend === 'ramp'
        ? 'Add ramp, not lands.'
        : a.recommend === 'lands'
          ? 'Add lands.'
          : 'Lands or ramp — they buy the same thing here.'

  return (
    <section className="space-y-3">
      <h3 className="text-sm font-semibold">
        The mana curve<HelpTip name="stat.mana_curve" />
      </h3>

      <div className="rounded-lg p-4"
           style={{
             background: a.recommend === 'none'
               ? 'color-mix(in srgb, var(--status-good) 12%, transparent)'
               : 'var(--gridline)',
           }}>
        <div className="text-sm font-semibold">{verdict}</div>
        <p className="mt-1 text-xs" style={{ color: 'var(--text-secondary)' }}>
          You make {a.target_mana} mana on turn {a.target_turn} in{' '}
          <strong>{(a.odds * 100).toFixed(0)}%</strong> of games. One more land
          takes that to {(a.odds_per_land * 100).toFixed(0)}%; one more
          accelerant, to {(a.odds_per_ramp * 100).toFixed(0)}%.
          {a.slots !== null && a.recommend !== 'none' && <>
            {' '}Reaching {(curve.target * 100).toFixed(0)}% would take about{' '}
            <strong>{a.slots}</strong>{' '}
            {a.recommend === 'ramp' ? 'more accelerants' : 'more lands'}.
          </>}
          {a.slots === null && a.recommend !== 'none' && <>
            {' '}Nothing you could reasonably add gets this deck to{' '}
            {(curve.target * 100).toFixed(0)}% — that is a target to move, not
            a deck to fix.
          </>}
        </p>
        {a.beyond_the_curve && (
          <p className="mt-2 text-xs" style={{ color: 'var(--text-secondary)' }}>
            <strong>You asked for more mana than the turn number.</strong> A
            land cannot help with that — you may only play one a turn, so a
            fifth land does nothing on turn four. This is precisely what ramp
            is for.
          </p>
        )}
        {a.ramp_is_generic && (
          <p className="mt-2 text-xs" style={{ color: 'var(--text-muted)' }}>
            This deck runs no acceleration, so the comparison uses a plain
            two-mana rock as a stand-in.
          </p>
        )}
      </div>

      {/* Lands and ramp stacked, so where the mana comes from is the shape of
          the bar rather than a second number to cross-reference. */}
      <div className="overflow-x-auto">
        <table className="w-full min-w-[28rem] border-collapse text-xs">
          <thead>
            <tr style={{ color: 'var(--text-muted)' }}>
              <th className="px-2 py-1 text-left font-medium">Turn</th>
              <th className="px-2 py-1 text-left font-medium">Where the mana comes from</th>
              <th className="px-2 py-1 text-right font-medium">Expected</th>
              <th className="px-2 py-1 text-right font-medium">On curve</th>
              <th className="px-2 py-1 text-right font-medium">All drops</th>
            </tr>
          </thead>
          <tbody>
            {curve.turns.map((t) => (
              <tr key={t.turn}
                  style={{ fontWeight: t.turn === a.target_turn ? 600 : undefined }}>
                <td className="px-2 py-1">T{t.turn}</td>
                <td className="px-2 py-1">
                  <div className="flex h-3 overflow-hidden rounded-sm"
                       style={{ background: 'var(--gridline)' }}
                       title={`${t.from_lands.toFixed(2)} from lands, `
                            + `${t.from_ramp.toFixed(2)} from ramp`}>
                    <div style={{ width: `${(t.from_lands / peak) * 100}%`,
                                  background: 'var(--vine)' }} />
                    <div style={{ width: `${(t.from_ramp / peak) * 100}%`,
                                  background: 'var(--heat-ink)' }} />
                  </div>
                </td>
                <td className="px-2 py-1 text-right tabular-nums">
                  {t.expected_mana.toFixed(2)}
                </td>
                <td className="px-2 py-1 text-right tabular-nums">
                  {(t.odds * 100).toFixed(0)}%
                </td>
                <td className="px-2 py-1 text-right tabular-nums"
                    style={{ color: 'var(--text-muted)' }}>
                  {(t.land_drop_odds * 100).toFixed(0)}%
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      <p className="text-xs" style={{ color: 'var(--text-secondary)' }}>
        <span style={{ color: 'var(--vine)' }}>■</span> lands{' '}
        <span style={{ color: 'var(--heat-ink)' }}>■</span> ramp.{' '}
        <strong>“All drops”</strong> is the chance of making a land drop every
        single turn up to that point, and it is the answer to the question
        everybody asks first
        {a.lands_for_every_drop !== null && <>
          {' '}— to get it to {(curve.target * 100).toFixed(0)}% on turn{' '}
          {a.target_turn} you would need{' '}
          <strong>{a.lands_for_every_drop} lands</strong>, which is why that is
          not the question worth asking
        </>}.
      </p>
    </section>
  )
}
