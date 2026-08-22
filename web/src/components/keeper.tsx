/**
 * The keeper's dossier (second punch list of 2026-08-15, item 8): who
 * Ludevic actually is, behind an About button in the Laboratory.
 *
 * The prose is checked in, because a character's story is exactly the kind
 * of fact that stopped moving years ago (`lore.py`'s argument) — but it is
 * written *around the pool's own titles*. The exhibit case below the bio is
 * a live `searchCards` for his name: the four cards the pool holds — an
 * opus, a test subject, the man himself twice over, a hubris — each with
 * its real cost and text, hoverable to the full card. The names carry the
 * arc; the prose narrates and never contradicts what the pool ships beside
 * it. If the pool is missing, the case stands empty and says so.
 *
 * The room is Rafater's *Havengul Laboratory* — green vats, copper pipes —
 * with `VaporLayer` steaming from the vats at their measured points, and a
 * flurry of `BatFlurry` silhouettes released across the painting when the
 * dossier opens (and occasionally after: bats startle, they do not hover).
 * Hotlinked and credited, per every masthead's rules.
 */

import { useEffect, useRef, useState } from 'react'
import { api, type Card } from '../lib/api'
import { BatFlurry } from './bats'
import { CardHover, ManaCost } from './ui'
import { VaporLayer, type VaporSource } from './vapor'

/** Havengul Laboratory // Havengul Mystery, Rafater (Secret Lair x
 *  Innistrad). Looked up on Scryfall, not recalled. */
const HAVENGUL_ART =
  'https://cards.scryfall.io/art_crop/front/8/2/823b019e-10c0-4712-8167-d4f37a71e782.jpg'

/**
 * Steam off the vats. Measured on the full crop (green-glow blob centroids),
 * then remapped into the band below, which shows the slice of the painting
 * from 15.5% to 71.2% of its height (aspect 640/260 over 626/457 at
 * `object-position: center 35%`). A source outside the slice is simply not
 * listed.
 */
const HAVENGUL_VAPOR: VaporSource[] = [
  { x: 0.70, y: 0.13, hue: 'cool', size: 1.2 },
  { x: 0.69, y: 0.31, hue: 'cool', size: 0.9 },
  { x: 0.70, y: 0.80, hue: 'cool', size: 1.0 },
  { x: 0.49, y: 0.97, hue: 'cool', size: 1.3 },
]

/** The dossier's own copy. Checked-in prose; every card-shaped claim in it
 *  is carried by the exhibit case the pool fills in below. */
const BIO = [
  'Innistrad is a plane that buries its dead with silver and prayer, and '
  + 'still they do not always stay down. Some of that is the moon. Some of '
  + 'it is Ludevic. The plane’s most celebrated necro-alchemist — '
  + 'his own preferred billing — is a skaberen: a stitcher of skaabs, '
  + 'creatures assembled from donated material and persuaded, with alchemy '
  + 'and lightning, to disagree with their own funerals.',

  'What separates Ludevic from every other ghoulcaller and grave-robber on '
  + 'the plane is that he considers himself an artist. The laboratory, the '
  + 'showmanship, the signature on every creation — the man does not '
  + 'make monsters, he curates a body of work. Younger stitchers study his '
  + 'treatises the way painters study a master, and the loudest of them '
  + 'built whole careers on his methods. He has opinions about that, all '
  + 'of them flattering to Ludevic.',

  'The pool remembers him the way a gallery remembers anyone: by the '
  + 'work. There is an opus — a stitched, flying horror he billed as his '
  + 'masterpiece, and a partner: it can helm a deck beside its maker. '
  + 'There is a '
  + 'test subject, an egg whose hatching is somebody else’s problem. '
  + 'And there is a hubris, which is what happens when a man who signs his '
  + 'monsters finally makes one that answers back. The titles are his own '
  + 'arc, told in four cards.',

  'He keeps this Laboratory because research is what he has always done: '
  + 'read everything, cite the pages, follow the evidence wherever it '
  + 'twitches. He cannot see your decks. Given what he does with donated '
  + 'material, keep it that way.',
]

export function KeeperDossier({ open, onClose }: {
  open: boolean
  onClose: () => void
}) {
  const [works, setWorks] = useState<Card[] | null>(null)
  const [bells, setBells] = useState(0)
  const panel = useRef<HTMLDivElement>(null)

  // The exhibit case: the pool's own records of his name. Fetched when the
  // dossier first opens, not on page load — most visits never open it.
  useEffect(() => {
    if (!open || works !== null) return
    let live = true
    api.searchCards({ q: 'Ludevic', limit: 20 })
      .then((r) => {
        if (!live) return
        setWorks(r.cards.filter((c) => c.name.includes('Ludevic')))
      })
      .catch(() => { if (live) setWorks([]) })
    return () => { live = false }
  }, [open, works])

  // Release the bats when the dossier opens, and occasionally while it is
  // up. The interval is long on purpose.
  useEffect(() => {
    if (!open) return
    const id = window.setInterval(() => setBells((n) => n + 1), 16000)
    return () => window.clearInterval(id)
  }, [open])

  // The opening *is* the first bell, so the count is derived rather than
  // nudged by the effect above: `BatFlurry` releases on any change and reads
  // zero as "nothing yet", which is exactly what a closed dossier is.
  const flurry = open ? bells + 1 : 0

  useEffect(() => {
    if (!open) return
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [open, onClose])

  if (!open) return null

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4"
         style={{ background: 'rgba(0, 0, 0, 0.6)' }}
         onMouseDown={(e) => {
           if (!panel.current?.contains(e.target as Node)) onClose()
         }}>
      <div ref={panel} role="dialog" aria-modal="true"
           aria-label="About the keeper"
           className="card-surface max-h-[88vh] w-full max-w-2xl overflow-y-auto rounded-2xl">
        {/* The room: vats, steam, and the flurry. */}
        <div className="relative overflow-hidden rounded-t-2xl"
             style={{ aspectRatio: '640 / 260' }}>
          <img src={HAVENGUL_ART} alt=""
               className="absolute inset-0 h-full w-full object-cover"
               style={{ objectPosition: 'center 35%' }} />
          <VaporLayer sources={HAVENGUL_VAPOR}
                      className="absolute inset-0 h-full w-full" />
          <BatFlurry trigger={flurry}
                     className="absolute inset-0 h-full w-full" />
          <div className="absolute inset-x-0 bottom-0 p-4"
               style={{ background: 'linear-gradient(transparent, rgba(0,0,0,0.65))' }}>
            <p className="text-[10px] uppercase tracking-wide text-white/70">
              The keeper of the Laboratory
            </p>
            <h2 className="text-xl font-semibold text-white">
              Ludevic, Necro-Alchemist
            </h2>
          </div>
          <button type="button" onClick={onClose} aria-label="Close"
                  className="btn absolute right-3 top-3 h-8 w-8 rounded-full p-0 text-white"
                  style={{ background: 'rgba(0,0,0,0.45)', borderColor: 'transparent' }}>
            ✕
          </button>
        </div>

        <div className="space-y-3 p-5">
          {BIO.map((para) => (
            <p key={para.slice(0, 24)} className="text-sm leading-relaxed"
               style={{ color: 'var(--text-secondary)' }}>
              {para}
            </p>
          ))}

          {/* The exhibit case: what the pool holds under his name. */}
          <div className="border-t pt-3" style={{ borderColor: 'var(--hairline)' }}>
            <p className="text-[10px] uppercase tracking-wide"
               style={{ color: 'var(--text-muted)' }}>
              The collected works, as the pool records them
            </p>
            {works === null && (
              <p className="mt-1 text-xs" style={{ color: 'var(--text-muted)' }}>
                Opening the case…
              </p>
            )}
            {works !== null && works.length === 0 && (
              <p className="mt-1 text-xs" style={{ color: 'var(--text-muted)' }}>
                The case stands empty — this instance has no card pool to
                exhibit from.
              </p>
            )}
            {works !== null && works.length > 0 && (
              <ul className="mt-2 space-y-2">
                {works.map((card) => (
                  <li key={card.name} className="text-xs leading-relaxed">
                    <CardHover card={card}>
                      <span className="inline-flex cursor-help flex-wrap items-baseline gap-1.5">
                        <span className="font-medium underline decoration-dotted"
                              style={{ color: 'var(--text-primary)' }}>
                          {card.name}
                        </span>
                        {card.mana_cost && <ManaCost cost={card.mana_cost} size={12} />}
                        <span style={{ color: 'var(--text-muted)' }}>
                          {card.type_line}
                        </span>
                      </span>
                    </CardHover>
                  </li>
                ))}
              </ul>
            )}
          </div>

          <p className="text-[10px] leading-relaxed"
             style={{ color: 'var(--text-muted)' }}>
            <em>Havengul Laboratory</em> by Rafater — the vats steam and the
            bats startle, and the painting holds still for both. Bio written
            for this library; card facts beside it come from the pool.
          </p>
        </div>
      </div>
    </div>
  )
}
