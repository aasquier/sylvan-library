/**
 * The five mana symbols, drawn.
 *
 * Magic's colours have had icons since Alpha — a sun, a drop of water, a
 * skull, a flame and a tree — and every player reads them faster than they
 * read a letter. The app spelled them `W`, `U`, `B`, `R`, `G` instead, which
 * is legible and completely characterless: a lettered disc is a placeholder
 * that had stopped looking like one.
 *
 * **Why these are drawn rather than fetched.** Scryfall serves the real
 * symbols as SVGs and the app already hotlinks its card art from the same
 * CDN, so that was the obvious move and it is the wrong one here, for three
 * reasons that all point the same way:
 *
 * - A pip is **inline in a sentence**. The deck files carry 174 of them across
 *   `why`, `strategy` and `notes`, and the gate's own errors are the densest
 *   of the lot — "identity {GW} includes {W}, outside the commander's {G}".
 *   Card art is decorative and lazy-loaded; prose is not, and a sentence that
 *   reflows or shows five broken images while a CDN answers is a regression on
 *   the app's most load-bearing text.
 * - `/api/colors` is the one deck-facing page that works on a fresh clone
 *   before `data refresh` has ever run — no corpus, no network. It renders
 *   taglines through `ManaText`. Pips that need a CDN would take that property
 *   away for decoration.
 * - Checking Scryfall's own path data into a public repository would be
 *   redistributing their asset rather than hotlinking it, which is the
 *   distinction rule 5 and [ADR 6](docs/adr/0006-never-redistribute-scryfall-bulk-data.md)
 *   are careful about. These are our drawings of the iconography.
 *
 * So this is the branch's own rule — everything that is not card art is drawn
 * in SVG/CSS — applied to the mark the app uses more than any other. Roughly
 * 2 kB in the bundle, no requests, and it works offline.
 *
 * **Built for 13px.** That is the size in prose, and it is brutal: the whole
 * glyph is about nine pixels of ink. Every shape here is a bold silhouette
 * with no interior detail that would close up — the skull is the hard one and
 * is drawn as three holes in a slab rather than as a skull. Checked at 13, 16
 * and 34px, which are the three sizes the app actually asks for.
 *
 * The disc colours are unchanged: `--mtg-*` already matched Scryfall's own
 * `#F8F6D8` / `#C1D7E9` / `#E49977` / `#A3C095` exactly. Only `--mtg-b` differs
 * on purpose — Scryfall fills both black and colourless with `#CAC5C0`, and
 * this app darkens black so the two are not the same disc.
 */

/**
 * White: a sun. A core disc and eight tapered rays.
 *
 * Generated rather than typed, because eight triangles at 45° is exactly the
 * sort of thing that is wrong by two degrees somewhere and nobody notices.
 * `-90` starts the first ray at twelve o'clock, so the glyph is symmetric
 * about the vertical and does not read as slightly rotated.
 */
const SUN_RAYS = Array.from({ length: 8 }, (_, i) => {
  const a = ((i * 45 - 90) * Math.PI) / 180
  const spread = (13 * Math.PI) / 180
  const at = (r: number, angle: number) =>
    `${(50 + r * Math.cos(angle)).toFixed(1)} ${(50 + r * Math.sin(angle)).toFixed(1)}`
  // Bases at r=21 sit *inside* the r=24 core rather than against it. Flush at
  // r=25 they left a one-unit ring of disc colour showing between the core and
  // its own rays — invisible at 13px and a clear hairline seam at 300.
  return `M${at(21, a - spread)}L${at(47, a)}L${at(21, a + spread)}Z`
}).join('')

/**
 * The five glyphs, as path data on a 100x100 box.
 *
 * One path each, so a glyph is a single fill and cannot end up half-inked if a
 * colour fails to resolve. The skull's eyes and nose are subpaths closed the
 * same direction as the slab and rely on `evenodd` to punch through.
 */
const GLYPHS: Record<string, { d: string; evenOdd?: boolean; label: string }> = {
  // A sun.
  W: {
    d: `M50 26A24 24 0 1 1 50 74A24 24 0 1 1 50 26Z${SUN_RAYS}`,
    label: 'white mana',
  },
  // A drop of water: a point at the top, a half-circle at the bottom.
  U: {
    d: 'M50 11C50 11 79 45 79 61A29 29 0 0 1 21 61C21 45 50 11 50 11Z',
    label: 'blue mana',
  },
  // A skull. Cranium, two sockets, a nose, and a jaw with two gaps in it --
  // the gaps are what make it read as teeth at this size rather than a chin.
  B: {
    d:
      'M50 12C29 12 16 27 16 46C16 57 21 65 27 70L27 84C27 87 29 89 32 89'
      + 'L68 89C71 89 73 87 73 84L73 70C79 65 84 57 84 46C84 27 71 12 50 12Z'
      + 'M36 40A9 9 0 1 1 36 58A9 9 0 1 1 36 40Z'
      + 'M64 40A9 9 0 1 1 64 58A9 9 0 1 1 64 40Z'
      + 'M50 60L57 72L43 72Z'
      + 'M44 76L44 84L38 84L38 76Z'
      + 'M62 76L62 84L56 84L56 76Z',
    evenOdd: true,
    label: 'black mana',
  },
  // A flame: a tall tongue with a smaller one curling up inside it.
  R: {
    d:
      'M54 8C56 26 72 34 76 50C80 66 71 84 54 91C58 80 56 72 50 66'
      + 'C46 74 47 84 50 91C33 86 22 72 24 56C26 44 33 38 38 30'
      + 'C39 42 43 47 47 50C52 42 54 24 54 8Z',
    label: 'red mana',
  },
  // A tree: a broad canopy on a short trunk that flares into roots.
  //
  // Drawn as one outline rather than a canopy plus a trunk, which is what the
  // first attempt was and it read unmistakably as a wine glass — a circle on a
  // narrow stem with a flat foot is a drinking vessel, not a tree. The fix is
  // proportion, not detail: the canopy has to be much wider than the trunk is
  // long, and the trunk has to widen into the ground rather than stopping on
  // a base. Lobes were tried and dropped; the notches between them silt up at
  // 13px and a smooth crown survives the size better.
  G: {
    d:
      'M50 6C38 6 29 13 26 23C16 26 9 34 9 44C9 55 18 63 29 63'
      + 'L45 63L45 78C45 85 39 89 30 90L30 94L70 94L70 90'
      + 'C61 89 55 85 55 78L55 63L71 63C82 63 91 55 91 44'
      + 'C91 34 84 26 74 23C71 13 62 6 50 6Z',
    label: 'green mana',
  },
}

/** Does this symbol have a drawn glyph, as opposed to a letter or a number? */
export function hasGlyph(symbol: string): boolean {
  return symbol in GLYPHS
}

/**
 * The glyphs, as raw path data.
 *
 * Exported so both kinds of caller draw the identical shape: `ManaGlyph`
 * wraps a path in its own `<svg>` for use inline in prose, and the pentagram
 * needs the path itself because it is already inside an `<svg>` and places its
 * vertices in the diagram's own coordinate space.
 */
export const GLYPH_PATH: Record<string, { d: string; evenOdd?: boolean; label: string }> =
  GLYPHS
