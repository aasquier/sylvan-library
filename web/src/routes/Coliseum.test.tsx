/**
 * The Coliseum, and the properties a screenshot would not catch.
 *
 * Four of these are about honesty rather than layout:
 *
 * - a fact renders **the halves its kind promises** — a `magic` slide has no
 *   Roman paragraph and a `paired` slide has both, because a kind whose
 *   fields do not match is a slide that comes out blank on one side;
 * - with **no card pool** the room still teaches: every fact still reads, and
 *   the page says plainly that the paintings are what is missing;
 * - a champion the pool dropped simply is not there, and the page does not
 *   draw one from its name;
 * - the weather is **decoration** — out of the accessibility tree, and never
 *   the thing carrying the meaning.
 *
 * And the fifth is the one Aaron will notice first: walking into a different
 * arena starts that arena's rotation at its own first fact, rather than
 * halfway through the last one's.
 */

import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type {
  Coliseum, ColiseumArena, ColiseumChampion, ColiseumFact, DeckTile,
  ForgeBeats, ForgeResult, Job,
} from '../lib/api'
import ColiseumRoom from './Coliseum'


vi.mock('../lib/api', async () => {
  const actual = await vi.importActual<typeof import('../lib/api')>('../lib/api')
  return {
    ...actual,
    api: { coliseum: vi.fn(), decks: vi.fn(), forgeStatus: vi.fn(),
           simForge: vi.fn(), job: vi.fn(), glossary: vi.fn(),
           validate: vi.fn() },
  }
})

const { api } = await import('../lib/api')

const DECKS = [
  { slug: 'gyome', owner: 'aaron', name: 'Gyome, Master Chef — Food',
    pilot: '', writable: true },
  { slug: 'arahbo', owner: 'aaron', name: 'Arahbo, Roar of the World — Cats',
    pilot: '', writable: true },
] as unknown as DeckTile[]

function champion(name: string, role: string): ColiseumChampion {
  return {
    name, role, mana_cost: null, type_line: 'Legendary Creature',
    oracle_text: '', color_identity: [], image: null,
    art_crop: `https://cards.scryfall.io/art_crop/${name}.jpg`,
  }
}

function arena(over: Partial<ColiseumArena> & { key: string; name: string }): ColiseumArena {
  return {
    plane: 'Otaria', motion: 'sand',
    art: { url: 'https://cards.scryfall.io/art_crop/arena.jpg',
           artist: 'Carl Critchlow', printing: 'Onslaught' },
    palette: { ink: '#f3e2bd', glow: '#b5762c' },
    backdrop: champion(`${over.name} backdrop`, ''),
    champions: [champion('Jareth, Leonine Titan', 'Fights behind the shield alone.')],
    facts: [{ kind: 'roman', rome: 'The floor was strewn with harena.' }],
    ...over,
  }
}

/** The four paintings the board's own zones wear. Checked-in prose on the
 *  server, so this is present whether or not a pool answered. */
const ZONES: Coliseum['zones'] = [
  { key: 'command', card: 'Throne of Eldraine', why: '',
    art: { url: 'https://cards.scryfall.io/art_crop/throne.jpg',
           artist: 'Kieran Yanner', printing: 'Wilds of Eldraine Commander' } },
  { key: 'graveyard', card: 'Ancient Tomb', why: '',
    art: { url: 'https://cards.scryfall.io/art_crop/tomb.jpg',
           artist: 'Colin MacNeil', printing: 'Tempest' } },
  { key: 'exile', card: 'Path to Exile', why: '',
    art: { url: 'https://cards.scryfall.io/art_crop/path.jpg',
           artist: 'Torgeir Fjereide', printing: 'Tales of Middle-earth Commander' } },
  { key: 'ghost', card: 'Crypt Ghast', why: '',
    art: { url: 'https://cards.scryfall.io/art_crop/ghast.jpg',
           artist: 'Chris Rahn', printing: 'Gatecrash' } },
]

function room(over: Partial<Coliseum> = {}): Coliseum {
  return {
    pool: true, dropped: 0, zones: ZONES,
    arenas: [arena({ key: 'grand-coliseum', name: 'The Grand Coliseum' })],
    ...over,
  }
}

function show(path = '/coliseum') {
  return render(
    <MemoryRouter initialEntries={[path]}><ColiseumRoom /></MemoryRouter>)
}

/** One game's narration, as the job's `partial` carries it. */
function beats(over: Partial<ForgeBeats> = {}): ForgeBeats {
  return {
    // No board: this fixture is about the *account*, and a match played by a
    // worker without the scribe carries exactly this — which is the state
    // worth having a fixture for anyway.
    game: 1, truncated: false, board: null,
    beats: [
      { kind: 'turn', turn: 4, who: 'gyome', against: null },
      { kind: 'land', turn: 4, who: 'gyome', against: null,
        card: 'Bojuka Bog' },
      { kind: 'attack', turn: 4, who: 'gyome', against: 'arahbo',
        card: 'Gyome, Master Chef' },
      { kind: 'outcome', turn: 4, who: 'arahbo', against: null,
        note: 'has lost due to accumulation of 21 damage from generals' },
    ],
    ...over,
  }
}

function runningJob(partial: unknown): Job {
  return {
    id: 'j1', kind: 'sim.forge', status: 'running', done: 1, total: 3,
    percent: 33, label: '', result: null, partial, error: null,
    created_at: '2026-08-25T00:00:00Z',
  } as unknown as Job
}

const RESULT: ForgeResult = {
  // A silent match: nobody asked to narrate, so the result carries no beats
  // and no board — which is also every match a worker without the scribe
  // plays.
  beats: [],
  decks: [
    { slug: 'gyome', name: 'Gyome Food', address: 'aaron/gyome', wins: 2 },
    { slug: 'arahbo', name: 'Arahbo Cats', address: 'aaron/arahbo', wins: 1 },
  ],
  games: 3, played: 3, draws: 0, timed_out: 1,
  median_seconds: 5.4, max_seconds: 37.8,
  startup_seconds: 9.2, wall_seconds: 71.0,
  clock: 300, seed: 7,
  rows: [
    { game: 1, winner: 'gyome', seconds: 5.4, turns: 9, draw: false,
      timed_out: false },
    { game: 2, winner: null, seconds: 300.0, turns: null, draw: false,
      timed_out: true },
    { game: 3, winner: 'gyome', seconds: 8.0, turns: 12, draw: false,
      timed_out: false },
  ],
  caveat: 'server caveat text',
}

const DONE: Job = {
  id: 'j1', kind: 'sim.forge', status: 'done', done: 3, total: 3,
  percent: 100, label: '', result: RESULT, partial: null, error: null,
  created_at: '2026-08-25T00:00:00Z',
} as unknown as Job

beforeEach(() => {
  vi.mocked(api.coliseum).mockReset()
  vi.mocked(api.decks).mockResolvedValue(DECKS)
  vi.mocked(api.forgeStatus).mockResolvedValue({ available: true, why: null })
  vi.mocked(api.glossary).mockResolvedValue(
    { terms: [] } as unknown as Awaited<ReturnType<typeof api.glossary>>)
})

afterEach(() => {
  cleanup()
  vi.clearAllMocks()
})

describe('the Coliseum', () => {
  it('renders each kind of fact with the halves that kind promises', async () => {
    const facts: ColiseumFact[] = [
      { kind: 'paired', rome: 'Sand drank the blood.', magic: 'And the land still charges for it.', card: 'Grand Coliseum' },
    ]
    vi.mocked(api.coliseum).mockResolvedValue(room({
      arenas: [arena({ key: 'a', name: 'A', facts })],
    }))
    const { container } = show()
    await screen.findByText('Sand drank the blood.')
    // Both halves, and the card the Magic half is about — asserted *within
    // the slide*, because the masthead credit names Grand Coliseum too and a
    // bare query cannot tell the room's own nameplate from a fact about it.
    const slide = container.querySelector('.arena-slide')
    expect(slide).toBeTruthy()
    const inSlide = within(slide as HTMLElement)
    expect(inSlide.getByText('And the land still charges for it.')).toBeTruthy()
    expect(inSlide.getByText('Grand Coliseum')).toBeTruthy()
  })

  it('does not invent a Roman half for a Magic fact', async () => {
    vi.mocked(api.coliseum).mockResolvedValue(room({
      arenas: [arena({
        key: 'a', name: 'A',
        facts: [{ kind: 'magic', magic: 'Jareth is a 4/7.' }],
      })],
    }))
    show()
    await screen.findByText('Jareth is a 4/7.')
    expect(screen.getByText('Magic')).toBeTruthy()
    expect(screen.queryByText('Rome, and its echo')).toBeNull()
  })

  it('still teaches with no card pool, and says what is missing', async () => {
    vi.mocked(api.coliseum).mockResolvedValue(room({
      pool: false,
      arenas: [arena({
        key: 'a', name: 'A', backdrop: null, champions: [],
        art: { url: '', artist: '', printing: '' },
        facts: [{ kind: 'coliseum', rome: 'Eighty numbered arches.' }],
      })],
    }))
    show()
    // The prose is the point and it is all text: it survives a missing pool.
    await screen.findByText('Eighty numbered arches.')
    expect(screen.getByText(/showing without their/i)).toBeTruthy()
    // The stage is still legible rather than a hole.
    expect(screen.getByRole('img', { name: /without its painting/i })).toBeTruthy()
  })

  it('walks to another arena and starts that arena at its own first fact', async () => {
    vi.mocked(api.coliseum).mockResolvedValue(room({
      arenas: [
        arena({ key: 'a', name: 'The Grand Coliseum',
          facts: [
            { kind: 'roman', rome: 'First of the Coliseum.' },
            { kind: 'roman', rome: 'Second of the Coliseum.' },
          ] }),
        arena({ key: 'b', name: 'The Cephalid Coliseum', motion: 'water',
          facts: [{ kind: 'roman', rome: 'First of the drowned one.' }] }),
      ],
    }))
    show()
    await screen.findByText('First of the Coliseum.')

    // Advance the first arena's rotation, then leave it.
    fireEvent.click(screen.getByRole('button', { name: 'Next' }))
    await screen.findByText('Second of the Coliseum.')

    fireEvent.click(screen.getByRole('tab', { name: 'The Cephalid Coliseum' }))
    // Not "slide 2 of an arena that has one slide", and not a blank.
    await screen.findByText('First of the drowned one.')
  })

  it('keeps the weather out of the accessibility tree', async () => {
    vi.mocked(api.coliseum).mockResolvedValue(room())
    const { container } = show()
    await screen.findByText(/harena/)
    const motion = container.querySelector('.arena-motion')
    expect(motion).toBeTruthy()
    // Decoration announces nothing: the facts carry the meaning.
    expect(motion?.getAttribute('aria-hidden')).toBe('true')
    // And the arena's motion is named, so the stylesheet has something to
    // hang six different kinds of weather on.
    expect(motion?.getAttribute('data-motion')).toBe('sand')
  })

  it('shows only the champions the pool actually resolved', async () => {
    vi.mocked(api.coliseum).mockResolvedValue(room({
      dropped: 2,
      arenas: [arena({
        key: 'a', name: 'A',
        champions: [champion('Kamahl, Pit Fighter', 'The pits named him.')],
      })],
    }))
    show()
    await screen.findByText('Kamahl, Pit Fighter')
    // A dropped name is absent, never drawn from the name alone.
    expect(screen.queryByText('Jareth, Leonine Titan')).toBeNull()
  })

  it('shows the named printing and credits its painter', async () => {
    // The pool answers a card name with its *newest* printing, which put
    // Ninja Turtles art on the Grand Coliseum. The arena carries a chosen
    // painting instead, and the painter is named on the page.
    vi.mocked(api.coliseum).mockResolvedValue(room({
      arenas: [arena({ key: 'a', name: 'A',
        art: { url: 'https://cards.scryfall.io/art_crop/chosen.jpg',
               artist: 'Carl Critchlow', printing: 'Onslaught' } })],
    }))
    const { container } = show()
    await screen.findByText(/Carl Critchlow/)
    const art = container.querySelector('img.arena-art')
    expect(art?.getAttribute('src')).toContain('chosen.jpg')
    // Scoped to the stage's own caption: the masthead credit names Onslaught
    // too, and a bare query cannot tell the room's nameplate from the arena
    // standing in front of you.
    const stage = container.querySelector('.arena-stage')?.parentElement
    expect(stage).toBeTruthy()
    expect(within(stage as HTMLElement).getByText(/Onslaught/)).toBeTruthy()
    expect(within(stage as HTMLElement).getByText(/Carl Critchlow/)).toBeTruthy()
  })

  it('says so when the doors do not answer', async () => {
    vi.mocked(api.coliseum).mockRejectedValue(new Error('nope'))
    show()
    await waitFor(() => expect(screen.getByText(/did not answer/i)).toBeTruthy())
  })
})

/**
 * The gates: this room is the only one that starts a real match, and these are
 * the properties a green backend suite cannot see from its side of the wire.
 *
 * They moved here from the Simulator with the Forge itself (Aaron's call,
 * 2026-08-25), and one is new and load-bearing: **this room asks to be
 * narrated.** Narration is free in time and about a hundred beats a game in
 * volume, so it is asked for per run — and if this room ever stopped asking,
 * the play-by-play would simply be empty with nothing on screen to explain it.
 */
describe('the gates', () => {
  beforeEach(() => { vi.mocked(api.coliseum).mockResolvedValue(room()) })

  it('sends both decks in, and asks to be told the game', async () => {
    vi.mocked(api.simForge).mockResolvedValue(DONE)
    show()
    await screen.findByText('Send them in')
    fireEvent.click(screen.getByText('Send them in'))
    await waitFor(() => expect(api.simForge).toHaveBeenCalledWith(
      expect.objectContaining({
        a_slug: 'gyome', a_owner: 'aaron',
        b_slug: 'arahbo', b_owner: 'aaron',
        games: 10, narrate: true,
      })))
  })

  it('seats both fighters from a link', async () => {
    vi.mocked(api.simForge).mockResolvedValue(DONE)
    show('/coliseum?a=aaron/arahbo&b=aaron/gyome')
    await screen.findByText('Send them in')
    fireEvent.click(screen.getByText('Send them in'))
    await waitFor(() => expect(api.simForge).toHaveBeenCalledWith(
      expect.objectContaining({ a_slug: 'arahbo', b_slug: 'gyome' })))
  })

  it('does not open where Forge is not installed', async () => {
    vi.mocked(api.forgeStatus).mockResolvedValue(
      { available: false, why: 'no jar at /opt/forge' })
    show()
    await screen.findByText(/harena/)
    await waitFor(() => expect(api.forgeStatus).toHaveBeenCalled())
    // Absent, never greyed out with an excuse — and the maintainer-facing
    // reason never renders (commandment 10).
    expect(screen.queryByText('Send them in')).toBeNull()
    expect(screen.queryByText(/no jar/)).toBeNull()
    // The room is still worth walking through, which is why nothing else
    // depends on the gate.
    expect(screen.getByText(/harena/)).toBeTruthy()
  })

  it('treats a failed gate ask as absence, not as an error', async () => {
    vi.mocked(api.forgeStatus).mockRejectedValue(new Error('boom'))
    show()
    await waitFor(() => expect(api.forgeStatus).toHaveBeenCalled())
    expect(screen.queryByText('Send them in')).toBeNull()
    expect(screen.queryByText(/The match failed/)).toBeNull()
  })

  it('says the forge is lighting from the click, not from the job', async () => {
    // A submission that never comes back, which is the whole point: this is
    // the gap where a JVM is starting on another machine and there is no job
    // to read a status off yet.
    vi.mocked(api.simForge).mockReturnValue(new Promise(() => {}))
    show()
    const button = await screen.findByText('Send them in') as HTMLButtonElement
    expect(button.disabled).toBe(false)
    fireEvent.click(button)
    await waitFor(() =>
      expect(screen.getByText('Lighting the forge…')).toBeTruthy())
    expect((screen.getByText('Lighting the forge…') as HTMLButtonElement)
      .disabled).toBe(true)
  })
})

/**
 * The play-by-play. What it must do is *name decks* — the wire carries a slug
 * and the room owns the shelf that turns one into a name — and what it must
 * never do is fold Forge's own outcome sentence into something tidier: "has
 * lost due to accumulation of 21 damage from generals" is the loss condition
 * Commander is named for, and it only reads correctly if nothing improves it.
 */
describe('the play-by-play', () => {
  beforeEach(() => { vi.mocked(api.coliseum).mockResolvedValue(room()) })

  /** Start a match whose job is parked mid-stream holding `partial`. */
  async function watching(partial: unknown) {
    vi.mocked(api.simForge).mockResolvedValue(runningJob(partial))
    vi.mocked(api.job).mockReturnValue(new Promise(() => {}))
    show()
    fireEvent.click(await screen.findByText('Send them in'))
    // "The account", not "The game": the game itself is the board above,
    // and this column is the words beside it.
    return screen.findByRole('tab', { name: 'The account' })
  }

  it('tells the game in words, naming the decks rather than the seats',
     async () => {
    await watching({ rows: [], beats: beats() })
    // Short names, because a beat is one line and a deck's full name is most
    // of it — "Gyome, Master Chef — Food" is the shelf's name for it and
    // "Gyome" is what anybody calls it out loud.
    await waitFor(() => expect(screen.getAllByText('Gyome').length)
      .toBeGreaterThan(0), { timeout: 3000 })
    await waitFor(() => expect(screen.getByText('plays Bojuka Bog'))
      .toBeTruthy(), { timeout: 3000 })
    // No seat number reaches the page.
    expect(screen.queryByText(/seat 1|seat 2/)).toBeNull()
  })

  it('keeps Forge’s own outcome sentence whole', async () => {
    await watching({ rows: [], beats: beats() })
    await waitFor(() => expect(screen.getByText(
      'has lost due to accumulation of 21 damage from generals')).toBeTruthy(),
    { timeout: 4000 })
  })

  it('says when a game outran what crosses', async () => {
    await watching({ rows: [], beats: beats({ truncated: true }) })
    await waitFor(() => expect(screen.getByText(/ran long/)).toBeTruthy(),
                  { timeout: 3000 })
  })

  it('leaves the house within reach while a match plays', async () => {
    await watching({ rows: [], beats: beats() })
    // Both places are offered, and the facts are still one click away — the
    // room existed before it could run anything and does not forget itself
    // the moment it can.
    fireEvent.click(screen.getByRole('tab', { name: 'The house' }))
    await screen.findByText(/harena/)
  })

  it('is quiet, not broken, when nobody narrated', async () => {
    await watching({ rows: [] })
    // No beats on the wire is the shape of an older worker, or of a match
    // nobody asked to narrate. The stage says so in its own words.
    await waitFor(() => expect(screen.getByText(/being played/)).toBeTruthy())
  })
})

describe('the tale of the tape', () => {
  beforeEach(() => { vi.mocked(api.coliseum).mockResolvedValue(room()) })

  it('keeps clock-outs apart from draws', async () => {
    vi.mocked(api.simForge).mockResolvedValue(DONE)
    show()
    fireEvent.click(await screen.findByText('Send them in'))
    await screen.findByText('Gyome Food wins')
    expect(screen.getByText('Arahbo Cats wins')).toBeTruthy()
    // The tile pair the distinction lives in — a game called off at the clock
    // is the measurement giving up, not a game that ended level.
    expect(screen.getByText('Hit the clock')).toBeTruthy()
    expect(screen.getByText('Draws')).toBeTruthy()
    // And per game: the clocked row is neither a draw nor a win.
    expect(screen.getByText('hit the clock')).toBeTruthy()
    // The wall-clock line speaks Magic, not machinery.
    expect(screen.getByText(/lighting\s+the forge/)).toBeTruthy()
  })
  /** The house's voices — the `.btn-*` classes in `index.css` that actually
   *  define a hover, a press and a pressed state. Written down here rather
   *  than derived from the stylesheet, and that is a limitation rather than a
   *  preference: vitest does not process CSS, so `import '../index.css?raw'`
   *  hands a test the **empty string** and every class in it looks undressed.
   *  A guard reading nothing reports nothing wrong, which is the worst kind,
   *  so this list is explicit and will fail loudly if a voice is renamed —
   *  the stylesheet is still the authority, this is only the roll-call. */
  const VOICES = [
    'btn-primary', 'btn-quiet', 'btn-danger', 'btn-danger-solid',
    'btn-ghost', 'btn-felt', 'btn-arena',
  ]

  it('gives every control in the gates and the walk a voice to answer with',
     async () => {
    // `.btn` on its own is a transparent border and a cursor: no hover, no
    // press, no reply of any kind. `Back` and `Next` wore exactly that while
    // carrying the whole thirteen-slide walk through an arena's lore, and the
    // gate wore the *chart* accent. Commandment 17 is the rule; this is the
    // gate on it. The suite's own beforeEach already stands a forge up and a
    // shelf of decks behind it, which is what puts the gates on the page.
    show()
    await screen.findByText(/harena/)

    for (const name of ['Back', 'Next', 'Send them in']) {
      const button = await screen.findByRole('button', { name })
      const answers = [...button.classList].filter((c) => VOICES.includes(c))
      expect(answers, `${name} wears no voice from index.css`)
        .not.toHaveLength(0)
    }
  })

  it('never lets the two deck pickers shrink below a usable width',
     async () => {
    // Three rounds of this bug. `flex-1` is `flex: 1 1 0%`, and with
    // `min-w-0` the two selects were the only items on the row that could
    // give — so they absorbed every deficit by shrinking, and `flex-wrap`
    // never rescued them, because an item with `min-width: 0` always fits and
    // the row therefore never has a reason to break. Measured on the deployed
    // room with the `sm:` branch live: 32px at a 390px container, 19px at
    // 520px, one row at every width.
    //
    // Rounds one and two both hung the fix on a *breakpoint*, which is only
    // ever a guess about how wide the room is — and it guessed wrong for the
    // person who reported it, whose phone reports a layout width at or above
    // 640. jsdom has no layout and cannot measure a width, but it can hold
    // the property that makes the crush impossible: a floor these two cannot
    // shrink through. `min-w-0` here is the bug, by name.
    show()
    await screen.findByText(/harena/)

    const pickers = ['Champion', 'Challenger'].map((label) =>
      screen.getByRole('combobox', { name: new RegExp(label, 'i') }))
    expect(pickers).toHaveLength(2)

    for (const picker of pickers) {
      const cls = picker.closest('label')?.className ?? ''
      expect(cls, 'a deck picker may not be allowed to shrink to nothing')
        .not.toMatch(/\bmin-w-0\b/)
      expect(cls, 'a deck picker needs a floor it cannot shrink through')
        .toMatch(/\bmin-w-\[/)
    }
  })

  it('keeps a heading for the outline while the banner does the talking',
     async () => {
    // The visible title is gone — the banner says "coliseum" better than the
    // word does — and this is what stops that from costing anything a reader
    // needs. A screen reader has no picture to be spoken for by, and a page
    // whose outline starts nowhere is a page nobody can navigate.
    //
    // It also retires the collision this test used to guard: the title and
    // the credit were two absolutes anchored to the same edge, twelve pixels
    // apart on a laptop and straight through each other on a phone (Aaron
    // photographed it, 2026-08-25). One of them no longer occupies the frame
    // at all, so there is nothing left to collide with.
    const { container } = show()
    await screen.findByText(/harena/)

    const heading = screen.getByRole('heading', { level: 1, name: /coliseum/i })
    expect(heading.className, 'the heading is read, never drawn')
      .toContain('sr-only')
    // And the credit is under the frame rather than washed across the foot of
    // it. The scrim that used to make white ink legible over Critchlow's pale
    // stone was paid for by the picture; underneath, it is ink on page.
    const hero = container.querySelector('.coliseum-hero')
    const credit = container.querySelector('.coliseum-footnote')
    expect(hero).toBeTruthy()
    expect(credit).toBeTruthy()
    expect(hero?.contains(credit!), 'the credit sits under the frame, not on it')
      .toBe(false)
  })

  it('credits the painting, and says so differently once it moves',
     async () => {
    // **The two lines are different claims and must not be the same words.**
    // Over Critchlow's plate the footnote is a credit. Over the generated
    // loop it is an acknowledgement: what is on screen was made *from* that
    // painting and is not his work, so "art by" alone would put his name on
    // something he did not make.
    const still = show()
    await screen.findByText(/harena/)
    const credit = () => still.container
      .querySelector('.coliseum-footnote')?.textContent ?? ''
    expect(credit()).toMatch(/^Grand Coliseum, Onslaught — art by Carl Critchlow$/)
    expect(still.container.querySelector('video.coliseum-hero-art')).toBeNull()
    cleanup()

    // And with the derivative on the shelf, the banner is the loop.
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({
        ready: true, effect: 'daynight', fingerprint: 'abc',
        urls: { mp4: '/api/art/motion/x/daynight/loop.mp4?v=abc' },
      }),
    }))
    const { container } = show()
    await screen.findByText(/harena/)
    await waitFor(() => {
      expect(container.querySelector('video.coliseum-hero-art')).toBeTruthy()
    })
    expect(container.querySelector('.coliseum-footnote')?.textContent)
      .toMatch(/^Motion inspired by Grand Coliseum, Onslaught — art by Carl Critchlow$/)
    // The still is not left underneath it: one banner, not two stacked.
    expect(container.querySelector('img.coliseum-hero-art')).toBeNull()
  })
})
