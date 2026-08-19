/** The plain table that lives beside the charts.
 *
 * Its own module, and the reason is weight rather than tidiness. Every other
 * component in `charts.tsx` draws with recharts -- 113kB gzipped, the single
 * largest thing this app ships -- and a route that imported `DataTable` from
 * there pulled the whole charting library in to render a `<table>`. The
 * simulator did exactly that, three times over, before its first result
 * existed.
 *
 * So the rule this file exists to keep: nothing here may import a chart.
 */

/** One column of a `DataTable`, with `format` tied to the type of the value
 *  its own `key` selects.
 *
 *  The mapped type distributes over the row's keys, so `{ key: 'mulligan_rate',
 *  format: (v) => percent(v) }` infers `v` as `number` from `LandRow` rather
 *  than the `any` this used to be. It also makes a typo in `key` an error --
 *  previously `key: 'mulligan_rat'` type-checked and rendered `undefined`.
 */
export type Column<Row> = {
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
