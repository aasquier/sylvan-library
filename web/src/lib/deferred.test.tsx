/** The lazy-loading boundary the charts sit behind.
 *
 * recharts is 113kB gzipped and three routes were downloading it whether or
 * not a chart appeared. `deferred` is what defers it — and it is worth its own
 * test because nothing else covers it: no test in this suite asserts that a
 * chart renders (recharts draws into a `ResponsiveContainer`, which measures
 * zero in jsdom), so a broken boundary would show up as a blank Stats tab in
 * a browser and a green suite here.
 */

import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'

import { deferred } from './deferred'

function Chart({ label }: { label: string }) {
  return <div data-testid="chart">{label}</div>
}

describe('deferred', () => {
  // No `globals: true` in this project, so the automatic cleanup never
  // registers itself and one test's tree is still in the document for the next.
  afterEach(cleanup)

  it('holds space at the declared height, then renders the component', async () => {
    let release!: (c: typeof Chart) => void
    const Lazy = deferred<{ label: string }>(
      () => new Promise((r) => { release = r }), 240)

    const { container } = render(<Lazy label="Mana curve" />)

    // Before the chunk lands: a placeholder of exactly the chart's height, so
    // the page does not collapse and then jump when it arrives.
    const holding = container.querySelector<HTMLElement>('[aria-hidden]')
    expect(holding).not.toBeNull()
    expect(holding?.style.height).toBe('240px')
    expect(screen.queryByTestId('chart')).toBeNull()

    release(Chart)

    expect((await screen.findByTestId('chart')).textContent).toBe('Mana curve')
    expect(container.querySelector('[aria-hidden]')).toBeNull()
  })

  it('passes its props through to the real component', async () => {
    const Lazy = deferred<{ label: string }>(async () => Chart, 100)
    render(<Lazy label="Colored pips vs sources" />)
    expect((await screen.findByTestId('chart')).textContent)
      .toBe('Colored pips vs sources')
  })
})
