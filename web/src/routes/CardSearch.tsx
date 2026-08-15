import { useCallback, useEffect, useRef, useState } from 'react'
import { api, errorMessage, type Card } from '../lib/api'
import { money } from '../lib/mtg'
import {
  Badge, CardArt, CardHover, ErrorNote, ManaCost, NumberField, PageMasthead,
  Select, Spinner, TextField,
} from '../components/ui'

/**
 * Demonic Tutor, Anato Finnstark, Strixhaven Mystical Archive (2021) — the
 * library search every other search is measured against. The Mystical Archive
 * is Strixhaven's vault of the game's definitive spells, which is why the
 * page mastheads draw on it: an archive of the whole printed history is what
 * this screen queries. Hotlinked, never committed (rule 5, ADR 6); the
 * printing is named because the pool only knows the default one, and the
 * `?<timestamp>` cache-buster is dropped as it is on the library's masthead.
 */
const DEMONIC_TUTOR_ART =
  'https://cards.scryfall.io/art_crop/front/3/0/3009ba46-c9f8-46dc-8ffc-2aa4cef7b17c.jpg'

const IDENTITIES = [
  { value: '', label: 'Any identity' },
  { value: 'BG', label: 'Golgari (BG)' },
  { value: 'GW', label: 'Selesnya (GW)' },
  { value: 'RGW', label: 'Naya (RGW)' },
  { value: 'WUB', label: 'Esper (WUB)' },
  { value: 'G', label: 'Mono-green' },
  { value: 'B', label: 'Mono-black' },
  { value: 'WUBRG', label: 'Five-color' },
]

const TYPES = ['', 'Creature', 'Instant', 'Sorcery', 'Artifact', 'Enchantment',
  'Land', 'Planeswalker', 'Battle']

export default function CardSearch() {
  const [q, setQ] = useState('')
  const [identity, setIdentity] = useState('')
  const [typeLine, setTypeLine] = useState('')
  const [cmcMax, setCmcMax] = useState(0)
  const [priceMax, setPriceMax] = useState(0)
  const [sort, setSort] = useState('edhrec')
  const [cards, setCards] = useState<Card[] | null>(null)
  const [message, setMessage] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const seq = useRef(0)

  const search = useCallback(async () => {
    const mine = ++seq.current
    setBusy(true)
    setError(null)
    try {
      const body = await api.searchCards({
        q, identity, type_line: typeLine, sort, limit: 60,
        ...(cmcMax > 0 ? { cmc_max: cmcMax } : {}),
        ...(priceMax > 0 ? { price_max: priceMax } : {}),
      })
      // Ignore a response that a newer query has already superseded.
      if (mine !== seq.current) return
      setCards(body.cards)
      setMessage(body.message ?? null)
    } catch (e) {
      if (mine === seq.current) setError(errorMessage(e))
    } finally {
      if (mine === seq.current) setBusy(false)
    }
  }, [q, identity, typeLine, cmcMax, priceMax, sort])

  // Debounced so typing does not fire a query per keystroke.
  useEffect(() => {
    const timer = setTimeout(search, 250)
    return () => clearTimeout(timer)
  }, [search])

  return (
    <div className="space-y-6">
      <PageMasthead
        art={DEMONIC_TUTOR_ART}
        alt="Demonic Tutor, painted by Anato Finnstark: a robed scholar bent
             over a glowing book whose roots spill across the floor, while a
             horned demon looms in the radiance above."
        title="Card search"
        credit={<>
          <em>Demonic Tutor</em> by Anato Finnstark, Strixhaven Mystical
          Archive — search your library for a card.
        </>}>
        <p>
          The whole printed history, queried locally. Identity is a subset filter:
          asking for Golgari returns colorless and mono-black cards too, because
          those are legal in the deck.
        </p>
      </PageMasthead>

      <div className="card-surface flex flex-wrap items-end gap-3 rounded-xl p-4">
        <TextField label="Text" value={q} onChange={setQ}
                   placeholder="name or rules text, e.g. create a Food token" />
        <Select label="Identity" value={identity} onChange={setIdentity}
                options={IDENTITIES} />
        <Select label="Type" value={typeLine} onChange={setTypeLine}
                options={TYPES.map((t) => ({ value: t, label: t || 'Any type' }))} />
        <NumberField label="Max MV" value={cmcMax} onChange={setCmcMax} min={0} max={16}
                     suffix={cmcMax === 0 ? 'any' : ''} />
        <NumberField label="Max price" value={priceMax} onChange={setPriceMax} min={0}
                     step={1} suffix={priceMax === 0 ? 'any' : 'USD'} />
        <Select label="Sort" value={sort} onChange={setSort}
                options={[
                  { value: 'edhrec', label: 'EDHREC rank' },
                  { value: 'cmc', label: 'Mana value' },
                  { value: 'name', label: 'Name' },
                  { value: 'newest', label: 'Newest' },
                ]} />
      </div>

      {error && <ErrorNote>{error}</ErrorNote>}
      {message && (
        <div className="card-surface rounded-lg px-4 py-3 text-sm"
             style={{ color: 'var(--status-warning)' }}>
          {message}
        </div>
      )}
      {busy && <Spinner label="Searching…" />}

      {cards && !busy && (
        <>
          <p className="text-xs" style={{ color: 'var(--text-muted)' }}>
            {cards.length} result{cards.length === 1 ? '' : 's'}
            {cards.length === 60 && ' (capped — narrow the filters for more)'}
          </p>
          <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
            {cards.map((card) => (
              <article key={card.name} className="card-surface overflow-hidden rounded-xl">
                <CardHover card={card}>
                  <CardArt src={card.art_crop} alt={card.name} ratio="aspect-[626/280]"
                           className="rounded-none" />
                </CardHover>
                <div className="space-y-1.5 p-3">
                  <div className="flex items-start justify-between gap-2">
                    <h3 className="text-sm font-semibold leading-tight">{card.name}</h3>
                    <ManaCost cost={card.mana_cost} size={14} />
                  </div>
                  <p className="text-[11px]" style={{ color: 'var(--text-muted)' }}>
                    {card.type_line}
                  </p>
                  <div className="flex flex-wrap items-center gap-1.5 pt-1">
                    <Badge>{money(card.price_usd)}</Badge>
                    {card.edhrec_rank && <Badge>#{card.edhrec_rank.toLocaleString()}</Badge>}
                    {card.reserved && <Badge tone="warning">reserved list</Badge>}
                  </div>
                </div>
              </article>
            ))}
          </div>
          {cards.length === 0 && (
            <div className="card-surface rounded-lg px-4 py-8 text-center text-sm"
                 style={{ color: 'var(--text-secondary)' }}>
              Nothing matched. Try loosening a filter.
            </div>
          )}
        </>
      )}
    </div>
  )
}
