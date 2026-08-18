/** Charts.
 *
 * Palette: validated slots 1-4 from the reference instance, both modes, on the
 * adjacent pairlist. Light-mode aqua and yellow fall below 3:1 on the surface,
 * so every chart that uses them ships a legend plus a table view -- that is the
 * relief rule, not an optional extra.
 *
 * One y-axis per chart, always. Where two measures have different scales they
 * get two charts, never a second axis.
 */

import {
  Bar,
  BarChart,
  CartesianGrid,
  Cell,
  Legend,
  Line,
  LineChart,
  ReferenceArea,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts'
import type { TooltipContentProps } from 'recharts'
import type { CategoryRow, ColorNeed, CurveBucket, LandRow, TurnRow } from '../lib/api'
import { COLOR_NAMES, categoryLabel } from '../lib/mtg'

const AXIS = { fill: 'var(--text-muted)', fontSize: 11 }
const GRID = 'var(--gridline)'

/** Recharts injects `active`, `payload` and `label` by cloning the element
 *  passed to `content`, so every one of them is optional here -- `suffix` is
 *  the only prop a call site actually writes. `Partial` is what expresses
 *  that, and it is why the `payload?.length` guard below is load-bearing
 *  rather than defensive. */
type TooltipBoxProps = Partial<TooltipContentProps> & {
  suffix?: string
}

function TooltipBox({ active, payload, label, suffix = '' }: TooltipBoxProps) {
  if (!active || !payload?.length) return null
  return (
    <div
      className="rounded-lg px-3 py-2 text-xs shadow-lg"
      style={{
        background: 'var(--surface-1)',
        border: '1px solid var(--hairline)',
        color: 'var(--text-primary)',
      }}
    >
      <div className="mb-1 font-semibold">{label}</div>
      {/* `dataKey` is `string | number | ((obj) => unknown)` in Recharts, and a
          function is not a valid React key -- every chart here passes a string
          literal, so this only ever stringifies something already a string. */}
      {payload.map((p) => (
        <div key={String(p.dataKey)} className="flex items-center gap-2">
          <span className="inline-block h-2 w-2 rounded-full"
                style={{ background: p.color }} />
          <span style={{ color: 'var(--text-secondary)' }}>{p.name}</span>
          <span className="tabular ml-auto font-medium">
            {typeof p.value === 'number' ? p.value.toLocaleString() : p.value}{suffix}
          </span>
        </div>
      ))}
    </div>
  )
}

/* ------------------------------------------------------------------ curve */

export function CurveChart({ buckets }: { buckets: CurveBucket[] }) {
  const data = buckets.map((b) => ({ mv: b.mv === 0 ? '0' : String(b.mv), count: b.count }))
  return (
    <ResponsiveContainer width="100%" height={200}>
      <BarChart data={data} margin={{ top: 16, right: 8, bottom: 4, left: -18 }}>
        <CartesianGrid stroke={GRID} vertical={false} />
        <XAxis dataKey="mv" tick={AXIS} axisLine={{ stroke: 'var(--baseline)' }}
               tickLine={false} label={undefined} />
        <YAxis tick={AXIS} axisLine={false} tickLine={false} allowDecimals={false} />
        <Tooltip content={<TooltipBox />} cursor={{ fill: 'var(--gridline)', opacity: 0.4 }} />
        {/* Single series: magnitude, so one hue. No legend -- the title names it. */}
        <Bar dataKey="count" name="Cards" fill="var(--seq-450)" radius={[4, 4, 0, 0]}
             maxBarSize={44} />
      </BarChart>
    </ResponsiveContainer>
  )
}

/* -------------------------------------------------------------- by-turn */

const TURN_SERIES = [
  { key: 'lands', name: 'Lands', color: 'var(--series-1)' },
  { key: 'mana', name: 'Mana available', color: 'var(--series-2)' },
  { key: 'spells', name: 'Spells cast', color: 'var(--series-3)' },
  { key: 'unused', name: 'Mana wasted', color: 'var(--series-4)' },
]

export function ByTurnChart({ rows }: { rows: TurnRow[] }) {
  return (
    <ResponsiveContainer width="100%" height={260}>
      <LineChart data={rows} margin={{ top: 12, right: 12, bottom: 4, left: -20 }}>
        <CartesianGrid stroke={GRID} vertical={false} />
        <XAxis dataKey="turn" tick={AXIS} axisLine={{ stroke: 'var(--baseline)' }}
               tickLine={false} />
        <YAxis tick={AXIS} axisLine={false} tickLine={false} />
        <Tooltip content={<TooltipBox />}
                 cursor={{ stroke: 'var(--baseline)', strokeWidth: 1 }} />
        <Legend wrapperStyle={{ fontSize: 12, color: 'var(--text-secondary)' }} />
        {TURN_SERIES.map((s) => (
          <Line key={s.key} type="monotone" dataKey={s.key} name={s.name}
                stroke={s.color} strokeWidth={2} dot={false}
                activeDot={{ r: 4, strokeWidth: 2, stroke: 'var(--surface-1)' }} />
        ))}
      </LineChart>
    </ResponsiveContainer>
  )
}

export function CommanderCurve({ rows }: { rows: TurnRow[] }) {
  const data = rows.map((r) => ({ turn: r.turn, pct: +(r.commander_down * 100).toFixed(1) }))
  return (
    <ResponsiveContainer width="100%" height={220}>
      <LineChart data={data} margin={{ top: 12, right: 12, bottom: 4, left: -20 }}>
        <CartesianGrid stroke={GRID} vertical={false} />
        <XAxis dataKey="turn" tick={AXIS} axisLine={{ stroke: 'var(--baseline)' }}
               tickLine={false} />
        <YAxis tick={AXIS} axisLine={false} tickLine={false} domain={[0, 100]}
               tickFormatter={(v) => `${v}%`} />
        <Tooltip content={<TooltipBox suffix="%" />}
                 cursor={{ stroke: 'var(--baseline)' }} />
        <Line type="monotone" dataKey="pct" name="Commander on board"
              stroke="var(--series-1)" strokeWidth={2} dot={false}
              activeDot={{ r: 4, strokeWidth: 2, stroke: 'var(--surface-1)' }} />
      </LineChart>
    </ResponsiveContainer>
  )
}

/* ---------------------------------------------------------- land sweep */

/** Deployment across land counts.
 *
 * This is THE decision metric, and it is usually flat -- so the flat band is
 * drawn explicitly rather than leaving a reader to find a peak in noise.
 */
export function LandSweepChart({
  rows, flat,
}: {
  rows: LandRow[]
  flat: boolean
}) {
  const values = rows.map((r) => r.spells_through_t8)
  const lo = Math.min(...values)
  const hi = Math.max(...values)
  const pad = Math.max(0.4, (hi - lo) * 2)

  return (
    <ResponsiveContainer width="100%" height={240}>
      <LineChart data={rows} margin={{ top: 12, right: 16, bottom: 4, left: -16 }}>
        <CartesianGrid stroke={GRID} vertical={false} />
        {flat && (
          // Show the whole spread as one band: if every point sits inside it,
          // the differences are noise and there is no peak to pick.
          <ReferenceArea y1={lo} y2={hi} fill="var(--series-1)" fillOpacity={0.1}
                         stroke="none" />
        )}
        <XAxis dataKey="lands" tick={AXIS} axisLine={{ stroke: 'var(--baseline)' }}
               tickLine={false} />
        <YAxis tick={AXIS} axisLine={false} tickLine={false}
               domain={[+(lo - pad).toFixed(2), +(hi + pad).toFixed(2)]}
               tickFormatter={(v) => Number(v).toFixed(1)} />
        <Tooltip content={<TooltipBox />} cursor={{ stroke: 'var(--baseline)' }} />
        <Line type="monotone" dataKey="spells_through_t8" name="Spells through T8"
              stroke="var(--series-1)" strokeWidth={2}
              dot={{ r: 3, strokeWidth: 0, fill: 'var(--series-1)' }}
              activeDot={{ r: 5, strokeWidth: 2, stroke: 'var(--surface-1)' }} />
      </LineChart>
    </ResponsiveContainer>
  )
}

/** The two measures that DO move, on their own axes-free charts. */
export function LandTradeoffChart({ rows }: { rows: LandRow[] }) {
  const data = rows.map((r) => ({
    lands: r.lands,
    commander: +(r.commander_by_t5 * 100).toFixed(1),
    mulligans: +(r.mulligan_rate * 100).toFixed(1),
  }))
  return (
    <ResponsiveContainer width="100%" height={240}>
      <LineChart data={data} margin={{ top: 12, right: 16, bottom: 4, left: -16 }}>
        <CartesianGrid stroke={GRID} vertical={false} />
        <XAxis dataKey="lands" tick={AXIS} axisLine={{ stroke: 'var(--baseline)' }}
               tickLine={false} />
        {/* Both series are percentages, so one axis is honest here. */}
        <YAxis tick={AXIS} axisLine={false} tickLine={false}
               tickFormatter={(v) => `${v}%`} />
        <Tooltip content={<TooltipBox suffix="%" />} cursor={{ stroke: 'var(--baseline)' }} />
        <Legend wrapperStyle={{ fontSize: 12 }} />
        <Line type="monotone" dataKey="commander" name="Commander by T5"
              stroke="var(--series-1)" strokeWidth={2} dot={false} />
        <Line type="monotone" dataKey="mulligans" name="Mulligan rate"
              stroke="var(--series-2)" strokeWidth={2} dot={false} />
      </LineChart>
    </ResponsiveContainer>
  )
}

export function WastedManaChart({ rows }: { rows: LandRow[] }) {
  return (
    <ResponsiveContainer width="100%" height={240}>
      <LineChart data={rows} margin={{ top: 12, right: 16, bottom: 4, left: -16 }}>
        <CartesianGrid stroke={GRID} vertical={false} />
        <XAxis dataKey="lands" tick={AXIS} axisLine={{ stroke: 'var(--baseline)' }}
               tickLine={false} />
        <YAxis tick={AXIS} axisLine={false} tickLine={false} />
        <Tooltip content={<TooltipBox />} cursor={{ stroke: 'var(--baseline)' }} />
        <Line type="monotone" dataKey="wasted_through_t8" name="Mana wasted through T8"
              stroke="var(--series-4)" strokeWidth={2} dot={false} />
      </LineChart>
    </ResponsiveContainer>
  )
}

/* ------------------------------------------------------- deck composition */

export function CategoryCoverage({ rows }: { rows: CategoryRow[] }) {
  const data = rows
    .filter((r) => r.count > 0 || r.target_low)
    .map((r) => ({ ...r, label: categoryLabel(r.category) }))

  const statusColor = (s: CategoryRow['status']) =>
    s === 'low' ? 'var(--status-warning)'
      : s === 'high' ? 'var(--status-serious)'
        : 'var(--seq-450)'

  return (
    <ResponsiveContainer width="100%" height={Math.max(220, data.length * 30)}>
      <BarChart data={data} layout="vertical"
                margin={{ top: 8, right: 40, bottom: 4, left: 92 }}>
        <CartesianGrid stroke={GRID} horizontal={false} />
        <XAxis type="number" tick={AXIS} axisLine={false} tickLine={false}
               allowDecimals={false} />
        <YAxis type="category" dataKey="label" tick={AXIS} axisLine={false}
               tickLine={false} width={90} />
        <Tooltip content={<TooltipBox />} cursor={{ fill: 'var(--gridline)', opacity: 0.4 }} />
        <Bar dataKey="count" name="Cards" radius={[0, 4, 4, 0]} maxBarSize={18}>
          {data.map((row) => (
            <Cell key={row.category} fill={statusColor(row.status)} />
          ))}
        </Bar>
      </BarChart>
    </ResponsiveContainer>
  )
}

/** Colored pip demand against colored sources.
 *
 * Two series, so a legend is mandatory. Uses MTG's own colors for the axis
 * labels rather than the categorical palette, since the entity IS a color.
 */
export function ColorNeedsChart({ needs }: { needs: ColorNeed[] }) {
  const data = needs.map((n) => ({
    color: COLOR_NAMES[n.color] ?? n.color,
    key: n.color,
    pips: n.pips,
    sources: n.sources,
  }))
  return (
    <ResponsiveContainer width="100%" height={210}>
      <BarChart data={data} margin={{ top: 12, right: 8, bottom: 4, left: -20 }}
                barGap={2}>
        <CartesianGrid stroke={GRID} vertical={false} />
        <XAxis dataKey="color" tick={AXIS} axisLine={{ stroke: 'var(--baseline)' }}
               tickLine={false} />
        <YAxis tick={AXIS} axisLine={false} tickLine={false} allowDecimals={false} />
        <Tooltip content={<TooltipBox />} cursor={{ fill: 'var(--gridline)', opacity: 0.4 }} />
        <Legend wrapperStyle={{ fontSize: 12 }} />
        <Bar dataKey="pips" name="Pips required" fill="var(--series-2)"
             radius={[4, 4, 0, 0]} maxBarSize={30} />
        <Bar dataKey="sources" name="Sources" fill="var(--series-1)"
             radius={[4, 4, 0, 0]} maxBarSize={30} />
      </BarChart>
    </ResponsiveContainer>
  )
}

/** One column of a `DataTable`, with `format` tied to the type of the value
 *  its own `key` selects.
 *
 *  The mapped type distributes over the row's keys, so `{ key: 'mulligan_rate',
 *  format: (v) => percent(v) }` infers `v` as `number` from `LandRow` rather
 *  than the `any` this used to be. It also makes a typo in `key` an error --
 *  previously `key: 'mulligan_rat'` type-checked and rendered `undefined`.
 */
type Column<Row> = {
  [K in keyof Row & string]: {
    key: K
    label: string
    format?: (v: Row[K]) => string
  }
}[keyof Row & string]

/** The table view that discharges the light-mode contrast WARN. */
export function DataTable<Row extends object>({
  columns, rows,
}: {
  columns: Column<Row>[]
  rows: Row[]
}) {
  return (
    <div className="overflow-x-auto">
      <table className="tabular w-full text-sm">
        <thead>
          <tr style={{ color: 'var(--text-muted)' }}>
            {columns.map((c) => (
              <th key={c.key}
                  className="whitespace-nowrap px-3 py-2 text-left text-[11px] font-medium uppercase tracking-wide">
                {c.label}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {rows.map((row, i) => (
            <tr key={i} style={{ borderTop: '1px solid var(--gridline)' }}>
              {columns.map((c) => (
                <td key={c.key} className="whitespace-nowrap px-3 py-1.5">
                  {c.format ? c.format(row[c.key]) : String(row[c.key])}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}


/* ---------------------------------------------------------- deck edits */

/** The admin dashboard's one chart: edits to decks per day, last thirty.
 *  Counts of events over days — a bar per day, one hue, no legend. */
export function EditsChart({ rows }: { rows: { day: string; edits: number }[] }) {
  // "08-18" is enough on an axis whose window is thirty days.
  const data = rows.map((r) => ({ day: r.day.slice(5), edits: r.edits }))
  return (
    <ResponsiveContainer width="100%" height={160}>
      <BarChart data={data} margin={{ top: 8, right: 8, bottom: 4, left: -18 }}>
        <CartesianGrid stroke={GRID} vertical={false} />
        <XAxis dataKey="day" tick={AXIS} axisLine={{ stroke: 'var(--baseline)' }}
               tickLine={false} />
        <YAxis tick={AXIS} axisLine={false} tickLine={false} allowDecimals={false} />
        <Tooltip content={<TooltipBox />}
                 cursor={{ fill: 'var(--gridline)', opacity: 0.4 }} />
        <Bar dataKey="edits" name="Edits" fill="var(--seq-450)"
             radius={[4, 4, 0, 0]} maxBarSize={28} />
      </BarChart>
    </ResponsiveContainer>
  )
}

/* -------------------------------------------------------------- traffic */

/** The visitor ledger's chart: requests per day, stacked by status class.
 *  Reds are reserved for the classes that mean trouble — 4xx wears the
 *  warning tone and 5xx the critical one — so a bad day reads as a bad
 *  day from across the room. */
const TRAFFIC_SERIES = [
  { key: '2xx', name: 'Answered', color: 'var(--series-1)' },
  { key: '3xx', name: 'Redirected', color: 'var(--series-2)' },
  { key: '4xx', name: 'Refused', color: 'var(--status-warning)' },
  { key: '5xx', name: 'Failed', color: 'var(--status-critical)' },
]

export function TrafficChart({ days }: {
  days: { day: string; total: number }[]
}) {
  const data = days.map((d) => ({ ...d, day: d.day.slice(5) }))
  return (
    <ResponsiveContainer width="100%" height={200}>
      <BarChart data={data} margin={{ top: 8, right: 8, bottom: 4, left: -14 }}>
        <CartesianGrid stroke={GRID} vertical={false} />
        <XAxis dataKey="day" tick={AXIS} axisLine={{ stroke: 'var(--baseline)' }}
               tickLine={false} />
        <YAxis tick={AXIS} axisLine={false} tickLine={false} allowDecimals={false} />
        <Tooltip content={<TooltipBox />}
                 cursor={{ fill: 'var(--gridline)', opacity: 0.4 }} />
        <Legend wrapperStyle={{ fontSize: 12, color: 'var(--text-secondary)' }} />
        {TRAFFIC_SERIES.map((s) => (
          <Bar key={s.key} dataKey={s.key} name={s.name} stackId="day"
               fill={s.color} maxBarSize={28} />
        ))}
      </BarChart>
    </ResponsiveContainer>
  )
}
