/** The camera door: photograph a deck that exists nowhere online.
 *
 * Every other way into this library is text — a Moxfield export, an Archidekt
 * list, a paste. A deck that exists only as a stack of cards on a table has
 * nowhere to be typed from, and that is precisely the deck a first-time
 * player owns (commandment 2). This is the door for it.
 *
 * ## What decides, and what only offers
 *
 * A capture produces at most two facts: the set code and collector number
 * off the bottom-left corner, and the name off the title bar. The first pair
 * is a **lookup** and resolves outright. The second is a **similarity** and
 * resolves nothing at all, however certain it looks — the scores of right and
 * wrong answers overlap badly, which the server's card reader measured. So the
 * review list below has two kinds of row, and the difference between them is
 * the whole design: a card the pool *found*, and a shortlist somebody has to
 * choose from.
 *
 * Nothing is written until the button at the bottom is pressed, and what it
 * writes is decklist text into the box on the Import page — so the gate, the
 * draft, the counted rationales and ADR 13 all happen exactly as they do for
 * a pasted list. This door adds a way in; it does not add a way around.
 *
 * ## The photograph does not leave the browser
 *
 * The frame is drawn to a canvas, read by WebAssembly in this tab, and
 * discarded. What crosses the wire is `{set, number, title}` — three short
 * strings. No image is uploaded, stored, or logged.
 */

import { useCallback, useEffect, useRef, useState } from 'react'
import {
  api, errorMessage, followJob, type IdentifiedCard, type Reading,
  type ScanResult,
} from '../lib/api'
import { CARD_ASPECT } from '../lib/cardframe'
import { decklistLines, owingNote } from '../lib/decklist'
import { read, rest, warm, type Sighting } from '../lib/reader'
import { Badge, ErrorNote, Spinner } from './ui'

/** How much of the frame's height the card guide takes. Chosen so a phone
 *  held at a comfortable distance fills it without the edges clipping —
 *  provisional until it has been used on real cards in real light, which is
 *  what commandment 16 is for. */
const GUIDE_HEIGHT = 0.86

/** One photographed card, and what became of it. */
interface Shot {
  id: number
  /** A small dataURL of what was captured, so the review list can show the
   *  user their own photograph beside the card the pool proposed. Without it
   *  a wrong match is indistinguishable from a right one. */
  thumb: string
  state: 'reading' | 'read' | 'failed'
  sighting?: Sighting
  reading?: Reading
  /** What this shot will contribute. Set by the pool for a corner hit, and
   *  by a person for everything else. */
  chosen?: IdentifiedCard
  error?: string
  /** Claude is looking at this one (ADR 34). */
  scanning?: boolean
  /** What Claude read off the card, shown beside whatever it resolved to.
   *  A wrong match next to the words it came from can be caught. */
  transcribed?: { title?: string; corner?: string }
}


export default function CameraDoor({ onCards }: {
  /** Hands finished lines to whoever opened the door. The Import page drops
   *  them into its decklist box rather than importing behind anybody's back. */
  onCards: (lines: string[]) => void
}) {
  const video = useRef<HTMLVideoElement | null>(null)
  const stream = useRef<MediaStream | null>(null)
  const counter = useRef(0)

  const [open, setOpen] = useState(false)
  const [starting, setStarting] = useState(false)
  const [aspect, setAspect] = useState(4 / 3)
  const [shots, setShots] = useState<Shot[]>([])
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  const stop = useCallback(() => {
    stream.current?.getTracks().forEach((track) => { track.stop() })
    stream.current = null
    setOpen(false)
  }, [])

  // The camera and the six megabytes both have to be let go of. A viewfinder
  // left running behind a closed panel is a light on somebody's phone.
  useEffect(() => () => { stop(); void rest() }, [stop])

  async function start() {
    setError(null)
    setStarting(true)
    try {
      if (!navigator.mediaDevices?.getUserMedia) {
        // The honest cause is almost always an insecure context, and saying
        // "denied" there sends somebody to reset a permission they never set.
        throw new Error(
          'This browser will not open a camera here. A camera needs a secure '
          + 'connection — https, or localhost.')
      }
      const media = await navigator.mediaDevices.getUserMedia({
        // The back camera, on anything that has two.
        video: { facingMode: 'environment', width: { ideal: 1920 } },
        audio: false,
      })
      stream.current = media
      setOpen(true)
      // Fetch the reader now rather than at the first capture: it is six
      // megabytes, and it arrives while the first card is being lined up.
      warm()
    } catch (e) {
      setError(errorMessage(e))
    } finally {
      setStarting(false)
    }
  }

  // Attaching the stream is an effect rather than part of `start`, because
  // the <video> does not exist until `open` has rendered it.
  useEffect(() => {
    const element = video.current
    if (!open || !element || !stream.current) return
    element.srcObject = stream.current
    void element.play().catch(() => { /* autoplay refusals are not fatal */ })
  }, [open])

  async function capture() {
    const element = video.current
    if (!element || !element.videoWidth) return
    setBusy(true)

    // The guide's rectangle in the frame's own pixels. The container is given
    // the stream's aspect ratio, so what is drawn here is exactly what was
    // inside the brackets on screen — no object-fit arithmetic to get wrong.
    const height = element.videoHeight * GUIDE_HEIGHT
    const width = height * CARD_ASPECT
    const left = (element.videoWidth - width) / 2
    const top = (element.videoHeight - height) / 2

    const canvas = document.createElement('canvas')
    canvas.width = Math.round(width)
    canvas.height = Math.round(height)
    canvas.getContext('2d')?.drawImage(
      element, left, top, width, height, 0, 0, canvas.width, canvas.height)

    counter.current += 1
    const id = counter.current
    // One encode, used twice: rendered at 54px in the review list, and sent
    // whole if the fallback is ever asked for. 0.8 rather than 0.5 because
    // the second use is a reader, and JPEG mush is what it would be reading.
    const shot: Shot = {
      id, state: 'reading', thumb: canvas.toDataURL('image/jpeg', 0.8),
    }
    setShots((all) => [...all, shot])

    try {
      const sighting = await read(canvas, canvas.width, canvas.height)
      const result = await api.identifyCards([sighting])
      const reading = result.readings[0]
      setShots((all) => all.map((s) => s.id === id ? {
        ...s,
        state: 'read',
        sighting,
        reading,
        // A corner hit is the only thing that arrives already decided.
        ...(reading?.resolved ? { chosen: reading.resolved } : {}),
      } : s))
    } catch (e) {
      setShots((all) => all.map((s) => s.id === id
        ? { ...s, state: 'failed', error: errorMessage(e) } : s))
    } finally {
      setBusy(false)
    }
  }

  /**
   * Ask Claude to read this one (ADR 34).
   *
   * Deliberately per-card and never automatic. This is the only path in the
   * app that sends a photograph anywhere, so it happens because somebody
   * pressed a button on one specific card — the sentence beside the button
   * says so before it is pressed, not after.
   */
  async function askClaude(shot: Shot) {
    setShots((all) => all.map((s) =>
      s.id === shot.id ? { ...s, scanning: true, error: undefined } : s))
    try {
      // The data URL's prefix is not part of the image.
      const base64 = shot.thumb.slice(shot.thumb.indexOf(',') + 1)
      const job = await api.scanCard(base64)
      const done = await followJob(job.id, () => { /* no per-tick UI */ },
                                   400, job).promise
      const result = done.result as ScanResult | undefined
      setShots((all) => all.map((s) => s.id === shot.id ? {
        ...s,
        scanning: false,
        transcribed: result?.transcribed ?? {},
        reading: result?.reading ?? s.reading,
        // A corner Claude read still only resolves if the pool agrees, so
        // this is the same rule the local tier follows, not a shortcut.
        ...(result?.reading?.resolved ? { chosen: result.reading.resolved } : {}),
      } : s))
    } catch (e) {
      setShots((all) => all.map((s) => s.id === shot.id
        ? { ...s, scanning: false, error: errorMessage(e) } : s))
    }
  }

  function choose(id: number, card: IdentifiedCard) {
    setShots((all) => all.map((s) => s.id === id ? { ...s, chosen: card } : s))
  }

  function discard(id: number) {
    setShots((all) => all.filter((s) => s.id !== id))
  }

  function lines(): string[] {
    return decklistLines(
      shots.flatMap((shot) => shot.chosen ? [shot.chosen.name] : []))
  }

  const decided = shots.filter((s) => s.chosen).length
  const owing = shots.filter((s) => s.state === 'read' && !s.chosen).length

  return (
    <section className="space-y-3">
      {!open && (
        <div className="flex flex-wrap items-center gap-3">
          <button onClick={() => void start()} disabled={starting}
                  className="btn btn-primary btn-accent-2">
            {starting ? 'Opening the lens…' : 'Photograph the cards'}
          </button>
          <span className="text-xs" style={{ color: 'var(--text-muted)' }}>
            One card at a time. The photograph never leaves this browser.
          </span>
        </div>
      )}

      {error && <ErrorNote>{error}</ErrorNote>}

      {open && (
        <div className="space-y-3">
          <div className="relative overflow-hidden rounded-xl"
               style={{ aspectRatio: aspect, background: '#000',
                        border: '1px solid var(--hairline)' }}>
            <video
              ref={video}
              muted
              playsInline
              aria-label="Camera viewfinder"
              onLoadedMetadata={(e) => {
                const v = e.currentTarget
                if (v.videoWidth) setAspect(v.videoWidth / v.videoHeight)
              }}
              className="h-full w-full object-cover"
            />
            {/* The guide. Its height is the same fraction the capture uses,
                and the container carries the stream's own aspect ratio, so
                what is inside the brackets is what gets read. */}
            <div className="pointer-events-none absolute inset-0 flex items-center justify-center">
              <div className="camera-guide"
                   style={{ height: `${GUIDE_HEIGHT * 100}%`,
                            aspectRatio: CARD_ASPECT }} />
            </div>
          </div>

          <div className="flex flex-wrap items-center gap-3">
            <button onClick={() => void capture()} disabled={busy}
                    className="btn btn-primary btn-accent-1">
              {busy ? 'Reading…' : 'Capture this card'}
            </button>
            <button onClick={stop} className="btn btn-quiet">
              Close the lens
            </button>
            <span className="text-xs tabular" style={{ color: 'var(--text-muted)' }}>
              {shots.length} photographed · {decided} named
              {owing > 0 && ` · ${owing} waiting on you`}
            </span>
          </div>
        </div>
      )}

      {shots.length > 0 && (
        <ul className="space-y-2">
          {shots.map((shot) => (
            <ShotRow key={shot.id} shot={shot}
                     onChoose={(card) => { choose(shot.id, card) }}
                     onAskClaude={() => { void askClaude(shot) }}
                     onDiscard={() => { discard(shot.id) }} />
          ))}
        </ul>
      )}

      {decided > 0 && (
        <div className="flex flex-wrap items-center gap-3">
          <button className="btn btn-primary btn-accent-1"
                  onClick={() => { onCards(lines()) }}>
            Add {decided} card{decided === 1 ? '' : 's'} to the list
          </button>
          {owing > 0 && (
            <span className="text-xs" style={{ color: 'var(--status-warning)' }}>
              {owingNote(owing)}
            </span>
          )}
        </div>
      )}
    </section>
  )
}

/** One photographed card and what the pool made of it. */
function ShotRow({ shot, onChoose, onAskClaude, onDiscard }: {
  shot: Shot
  onChoose: (card: IdentifiedCard) => void
  onAskClaude: () => void
  onDiscard: () => void
}) {
  return (
    <li className="card-surface flex gap-3 rounded-lg p-3">
      <img src={shot.thumb} alt="" width={54}
           className="h-auto shrink-0 self-start rounded"
           style={{ border: '1px solid var(--hairline)' }} />

      <div className="min-w-0 flex-1 space-y-2">
        {shot.state === 'reading' && <Spinner label="Reading the card…" />}

        {shot.state === 'failed' && (
          <p className="text-xs" style={{ color: 'var(--status-critical)' }}>
            {shot.error}
          </p>
        )}

        {shot.state === 'read' && shot.chosen && (
          <div className="flex flex-wrap items-center gap-2">
            <strong className="text-sm">{shot.chosen.name}</strong>
            {shot.reading?.via === 'printing' && !shot.reading.candidates.length
              ? <Badge tone="good">read from the corner</Badge>
              : <Badge tone="neutral">you chose this</Badge>}
          </div>
        )}

        {/* The shortlist. Never pre-selected, however good the top score
            looks — a wrong card can outscore a right one, so the only thing
            that may decide here is a person. */}
        {shot.state === 'read' && !shot.chosen
          && (shot.reading?.candidates.length ?? 0) > 0 && (
          <div className="space-y-1">
            <p className="text-xs" style={{ color: 'var(--text-muted)' }}>
              The corner could not be read. Which of these is it?
            </p>
            <div className="flex flex-wrap gap-2">
              {shot.reading?.candidates.map((card) => (
                <button key={card.name} className="btn btn-quiet btn-xs"
                        onClick={() => { onChoose(card) }}>
                  {card.name}
                </button>
              ))}
            </div>
          </div>
        )}

        {shot.state === 'read' && !shot.chosen
          && !shot.reading?.candidates.length && !shot.scanning && (
          <p className="text-xs" style={{ color: 'var(--status-warning)' }}>
            Nothing legible. Photograph it again, or type this one into the
            list yourself.
          </p>
        )}

        {/* What Claude read, shown beside what it resolved to. A wrong match
            next to the words it came from is one somebody can catch. */}
        {shot.transcribed && (shot.transcribed.title ?? shot.transcribed.corner) && (
          <p className="font-mono text-[11px]" style={{ color: 'var(--text-muted)' }}>
            read: {[shot.transcribed.title, shot.transcribed.corner]
              .filter(Boolean).join(' · ')}
          </p>
        )}

        {shot.scanning && <Spinner label="Claude is looking at this one…" />}

        {/* The fallback door (ADR 34). Offered only where the local reader
            fell short — most often a card printed before 2015, which has no
            collector number on its face at all. */}
        {shot.state === 'read' && !shot.chosen && !shot.scanning
          && !shot.transcribed && (
          <div className="space-y-1">
            <button onClick={onAskClaude} className="btn btn-quiet btn-xs">
              Ask Claude to read it
            </button>
            <p className="text-[11px]" style={{ color: 'var(--text-muted)' }}>
              This one photograph is sent to Claude, and nothing else is.
            </p>
          </div>
        )}
      </div>

      <button onClick={onDiscard} className="btn btn-ghost btn-xs self-start"
              aria-label="Discard this photograph">
        Discard
      </button>
    </li>
  )
}
