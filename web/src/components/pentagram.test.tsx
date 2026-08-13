/**
 * The colour wheel, and the five mana symbols.
 *
 * What is worth pinning here is not the markup — it is a drawing and will be
 * adjusted — but the three claims the drawing makes, each of which can break
 * silently while the picture still looks fine:
 *
 * * **The lines are the ten guilds**, once each. A diagram that joined the
 *   wrong pair, or drew Azorius twice and Boros never, is a plausible-looking
 *   picture that teaches something false. `tests/test_colors.py` checks the
 *   same property from the Python side against `colors.py`; this checks that
 *   what reaches the DOM is that set.
 * * **No name is written here.** Every label comes from the taxonomy the page
 *   fetched, so `colors.py` stays the single authority. The test for that is
 *   to hand the component a deliberately wrong taxonomy and watch the wrong
 *   name come out — if it does not, something is hard-coded.
 * * **Only the five colours get a glyph.** A numeral, `{X}` and a hybrid keep
 *   their characters, because no single icon says what they mean.
 */

import { cleanup, fireEvent, render, screen, within } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ColorPentagram, TierGlyph } from './pentagram'
import { ManaGlyph } from './manasymbol'
import { hasGlyph } from '../lib/managlyphs'
import { ManaCost, ManaText } from './ui'
import type { Combination } from '../lib/api'

afterEach(cleanup)

/** A combination in the shape `/api/colors` serves. */
function combo(key: string, name: string, tier: string, colors: string[]): Combination {
  return {
    key, name, tier, colors, size: colors.length,
    tagline: `${name} tagline.`, history: `${name} history.`,
    aliases: [], verified_by: 'a card',
    // The teaching fields. Empty here on purpose: the wheel is geometry and
    // labels, and nothing about the diagram should start depending on whether
    // a slot has a story attached.
    lore: '', champions: [], signature: [],
  }
}

/** The five mono slots and the ten guilds — everything the wheel can name. */
const TAXONOMY: Combination[] = [
  ...['W', 'U', 'B', 'R', 'G'].map((c) => combo(c, `Mono-${c}`, 'mono', [c])),
  combo('WU', 'Azorius', 'guild', ['W', 'U']),
  combo('UB', 'Dimir', 'guild', ['U', 'B']),
  combo('BR', 'Rakdos', 'guild', ['B', 'R']),
  combo('RG', 'Gruul', 'guild', ['R', 'G']),
  combo('WG', 'Selesnya', 'guild', ['W', 'G']),
  combo('WB', 'Orzhov', 'guild', ['W', 'B']),
  combo('UR', 'Izzet', 'guild', ['U', 'R']),
  combo('BG', 'Golgari', 'guild', ['B', 'G']),
  combo('WR', 'Boros', 'guild', ['W', 'R']),
  combo('UG', 'Simic', 'guild', ['U', 'G']),
]

const ALLIED = ['Azorius', 'Dimir', 'Rakdos', 'Gruul', 'Selesnya']
const ENEMY = ['Orzhov', 'Izzet', 'Golgari', 'Boros', 'Simic']

function draw(onPick = vi.fn()) {
  const r = render(
    <ColorPentagram combinations={TAXONOMY} onPick={onPick} selected={null} />,
  )
  return { ...r, onPick }
}

/** Every interactive target, by the combination its label starts with. */
function target(name: string): HTMLElement {
  const hit = screen.getAllByRole('button')
    .find((el) => el.getAttribute('aria-label')?.startsWith(`${name} `))
  if (!hit) throw new Error(`no target for ${name}`)
  return hit
}

describe('the colour wheel', () => {
  it('draws five vertices and the ten guilds, each exactly once', () => {
    draw()
    const labels = screen.getAllByRole('button')
      .map((el) => el.getAttribute('aria-label')!.split(' —')[0])

    // Five mono and ten guilds, and the guilds are the ten real ones rather
    // than ten lines that happen to number ten.
    expect(labels.filter((l) => l.startsWith('Mono-'))).toHaveLength(5)
    const guilds = labels.filter((l) => !l.startsWith('Mono-'))
    expect(new Set(guilds)).toEqual(new Set([...ALLIED, ...ENEMY]))
    expect(guilds).toHaveLength(10)
  })

  it('draws the allied pairs solid and the enemy pairs dashed', () => {
    const { container } = draw()
    const dashOf = (name: string) =>
      target(name).querySelector('line')!.getAttribute('stroke-dasharray')

    // The distinction the legend describes, and the only thing separating a
    // shard from a wedge on the badges below.
    for (const name of ALLIED) expect(dashOf(name)).toBeNull()
    for (const name of ENEMY) expect(dashOf(name)).not.toBeNull()
    expect(container.querySelectorAll('.pentagram-edge')).toHaveLength(10)
  })

  it('picks the mono combination behind a vertex', () => {
    const { onPick } = draw()
    fireEvent.click(target('Mono-G'))
    expect(onPick).toHaveBeenCalledWith(
      expect.objectContaining({ key: 'G', tier: 'mono' }),
    )
  })

  it('picks the guild behind a line, which is in another tier', () => {
    const { onPick } = draw()
    fireEvent.click(target('Golgari'))
    // A chord, so this crosses out of the tier the diagram is drawn on. The
    // caller is what moves the tier selector; the wheel only has to hand over
    // the right combination.
    expect(onPick).toHaveBeenCalledWith(
      expect.objectContaining({ key: 'BG', tier: 'guild' }),
    )
  })

  it('is reachable and operable from the keyboard', () => {
    const { onPick } = draw()
    const azorius = target('Azorius')
    expect(azorius.getAttribute('tabindex')).toBe('0')
    fireEvent.keyDown(azorius, { key: 'Enter' })
    fireEvent.keyDown(target('Mono-W'), { key: ' ' })
    expect(onPick).toHaveBeenCalledTimes(2)
  })

  it('captions what the pointer is on, and says a line changes tier', () => {
    draw()
    fireEvent.mouseEnter(target('Golgari'))
    expect(screen.getByText('Golgari')).toBeTruthy()
    expect(screen.getByText(/enemy pair/)).toBeTruthy()
    // The click moves the tier selector underneath the user, so it says so.
    expect(screen.getByText(/cross to the guilds/)).toBeTruthy()

    fireEvent.mouseEnter(target('Mono-W'))
    expect(screen.getByText(/read Mono-W below/)).toBeTruthy()
  })

  it('says nothing about a combination it cannot draw', () => {
    // Found by running the Learn page, where `selected` is any of the 32
    // rather than only the fifteen shapes on the wheel. Artifice has no vertex
    // and no chord, so captioning it looked up a key that is in neither, found
    // no edge, and fell through to describing a four-colour identity as an
    // "enemy pair — opposite on the wheel" with a button offering to cross to
    // the guilds. The diagram only captions what it can point at.
    render(
      <ColorPentagram
        combinations={[...TAXONOMY,
                       combo('WUBR', 'Artifice', 'quad', ['W', 'U', 'B', 'R'])]}
        onPick={vi.fn()}
        selected="WUBR"
      />,
    )
    expect(screen.queryByText('Artifice')).toBeNull()
    expect(screen.queryByText(/enemy pair/)).toBeNull()
    // The resting caption instead, which is what "nothing is selected on this
    // diagram" has always looked like.
    expect(screen.getByText(/every pair of them has a name/)).toBeTruthy()
  })

  it('marks a guild it can draw when the page has one selected', () => {
    render(
      <ColorPentagram combinations={TAXONOMY} onPick={vi.fn()} selected="BG" />,
    )
    expect(screen.getByText('Golgari')).toBeTruthy()
    expect(screen.getByText(/enemy pair/)).toBeTruthy()
  })

  it('takes every name from the taxonomy rather than from itself', () => {
    // The guard against a second copy of `colors.py` growing in the frontend.
    // Rename a guild in the data and the diagram must rename it too.
    const lying = TAXONOMY.map((c) =>
      c.key === 'BG' ? { ...c, name: 'Notgolgari' } : c)
    render(<ColorPentagram combinations={lying} onPick={vi.fn()} selected={null} />)
    const labels = screen.getAllByRole('button')
      .map((el) => el.getAttribute('aria-label')!)
    expect(labels.some((l) => l.startsWith('Notgolgari'))).toBe(true)
    expect(labels.some((l) => l.startsWith('Golgari '))).toBe(false)
  })

  it('survives a taxonomy that is missing a combination', () => {
    // A fresh clone, or a trimmed payload. The line has nothing to name, so it
    // must not be clickable and must not throw.
    const thin = TAXONOMY.filter((c) => c.key !== 'BG')
    render(<ColorPentagram combinations={thin} onPick={vi.fn()} selected={null} />)
    const named = screen.getAllByRole('button')
      .map((el) => el.getAttribute('aria-label'))
    expect(named.some((l) => l?.startsWith('Golgari'))).toBe(false)
  })
})

describe('the tier badges', () => {
  it('lights one vertex for mono and all five for five', () => {
    const { container: mono } = render(<TierGlyph tier="mono" />)
    const { container: five } = render(<TierGlyph tier="five" />)
    // Lit vertices are the large discs; unlit ones stay as small dots so all
    // seven badges read as the same figure.
    const big = (c: HTMLElement) =>
      [...c.querySelectorAll('circle')].filter((n) => Number(n.getAttribute('r')) > 3)
    expect(big(mono)).toHaveLength(1)
    expect(big(five)).toHaveLength(5)
  })

  it('draws a shard as an arc and a wedge as a span', () => {
    const lines = (tier: string) => {
      const { container } = render(<TierGlyph tier={tier} />)
      return [...container.querySelectorAll('line')]
        .map((l) => l.getAttribute('stroke-dasharray'))
    }
    // Both are three dots and three lines. The textures are the difference,
    // and they are opposite: a shard is mostly neighbours, a wedge mostly
    // opposites. This is the whole reason the badges are worth drawing.
    const shard = lines('shard')
    const wedge = lines('wedge')
    expect(shard).toHaveLength(3)
    expect(wedge).toHaveLength(3)
    expect(shard.filter((d) => d === null)).toHaveLength(2)
    expect(wedge.filter((d) => d === null)).toHaveLength(1)
  })

  it('gives colourless a centre dot and no lit vertex', () => {
    const { container } = render(<TierGlyph tier="colorless" />)
    // Colourless is not a point on the wheel, so it lights none of them.
    expect(container.querySelectorAll('line')).toHaveLength(0)
    const filled = [...container.querySelectorAll('circle')]
      .filter((n) => Number(n.getAttribute('r')) > 3)
    expect(filled).toHaveLength(1)
    expect(filled[0].getAttribute('fill')).toBe('var(--mtg-c)')
  })
})

describe('the mana symbols', () => {
  it('has a glyph for the five colours and nothing else', () => {
    for (const c of ['W', 'U', 'B', 'R', 'G']) expect(hasGlyph(c)).toBe(true)
    // Deliberate omissions. A numeral and `{X}` are characters; a hybrid is
    // two colours that no single icon states. `{C}` is the one that is merely
    // *not done yet* rather than argued against — colourless has a real symbol
    // of its own and is not one of the five, so it stayed out of this pass and
    // keeps its letter.
    for (const s of ['2', 'X', 'T', 'C', 'G/W']) expect(hasGlyph(s)).toBe(false)
  })

  it('renders a glyph as one filled path with no stray text', () => {
    const { container } = render(<ManaGlyph symbol="G" size={16} />)
    const paths = container.querySelectorAll('path')
    expect(paths).toHaveLength(1)
    expect(paths[0].getAttribute('fill')).toBe('#141414')
    expect(container.textContent).toBe('')
  })

  it('draws a colour in a cost and leaves the generic part a numeral', () => {
    const { container } = render(<ManaCost cost="{2}{B}{G}" />)
    // Three pips, two of them drawn. The numeral has to survive as a numeral
    // or a cost stops being readable as a cost.
    expect(container.querySelectorAll('svg')).toHaveLength(2)
    expect(container.textContent).toBe('2')
    expect(within(container).getByLabelText('Black')).toBeTruthy()
    expect(within(container).getByLabelText('Green')).toBeTruthy()
  })

  it('names a drawn pip, since it no longer spells itself', () => {
    // A lettered pip reached the accessibility tree as the letter "G". A drawn
    // one contributes no text at all, so the name is stated explicitly —
    // otherwise the gate's densest sentence loses its colours to a reader.
    const { container } = render(<ManaText>{'identity {GW} includes {W}'}</ManaText>)
    expect(container.textContent).toBe('identity  includes ')
    expect(within(container).getAllByLabelText('White')).toHaveLength(2)
    expect(within(container).getAllByLabelText('Green')).toHaveLength(1)
  })

  it('leaves prose that is not a symbol alone', () => {
    const { container } = render(<ManaText>{'a {note} and {} stay text'}</ManaText>)
    expect(container.textContent).toBe('a {note} and {} stay text')
    expect(container.querySelectorAll('svg')).toHaveLength(0)
  })
})
