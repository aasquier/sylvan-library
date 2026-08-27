/** The end of a bout: a wreath for the deck that took the match, and a stone
 *  raised over the one that did not.
 *
 *  **Why this is a panel in the page and not an overlay on the board.** A
 *  match here is something you watch, and the board keeps its scrubber
 *  afterwards precisely so somebody can go back through it. Anything drawn
 *  over that board is a thing between a person and the game they just watched,
 *  and it would have to be dismissed before the board could be used at all.
 *  So the verdict takes its own room rather than the field's.
 *
 *  **Where that room is** (Aaron, 2026-08-27: *"our results block is wrong, it
 *  currently sits below the coliseum facts panel, it should sit right below
 *  the sandbox"*). It used to say here that the verdict was "passed on the way
 *  to the numbers", which was true of the two-inch journey from the crown to
 *  the tiles and quietly false about everything above it: the whole result —
 *  verdict, tale of the tape, every bout — was rendered *after* the arena
 *  slide and after the champions, so the way to your own match's numbers ran
 *  past six houses of Roman prose. Somebody who had just watched twelve games
 *  end had to walk the length of the room to find out what happened.
 *
 *  It now sits directly under the sand, in `routes/Coliseum.tsx`: the board,
 *  then this, then the tape. The half of that argument which survives is the
 *  ordering *within* the result — the crowning is the first thing under the
 *  field and the arithmetic follows it, because who won is the answer and the
 *  rest is the evidence.
 *
 *  **Once per match, not once per game.** A series is up to a dozen games and
 *  a crowning that fires on each of them stops meaning anything by the third.
 *  This renders from the finished `ForgeResult`, which exists exactly once,
 *  when every game has landed.
 *
 *  A drawn match gets the wreath and nobody under it. That is deliberate and
 *  it is the honest picture: the object is there, it went unclaimed, and
 *  nothing fell. Inventing a loser out of a tie would be the room taking a
 *  view the games did not support.
 */

import { useState } from 'react'

import type { ForgeResult } from '../lib/api'
import coronaArt from '../assets/coliseum/corona.webp'
import cippusArt from '../assets/coliseum/cippus.webp'

export function MatchVerdict({ result }: { result: ForgeResult }) {
  const [cleared, setCleared] = useState(false)
  if (cleared) return null

  // Sorted rather than searched for a maximum, because the second place is
  // needed too and a tie has to be visible rather than resolved arbitrarily.
  const standing = [...result.decks].sort((a, b) => b.wins - a.wins)
  const champion = standing[0]
  const fallen = standing[1]
  if (!champion) return null
  const drawn = !fallen || champion.wins === fallen.wins

  return (
    <section aria-label="The verdict"
             className={`verdict${drawn ? ' is-drawn' : ''}`}>
      <div className="verdict-field">
        <div className="verdict-side">
          <div className="verdict-crown">
            <img className="verdict-wreath" src={coronaArt} alt=""
                 draggable={false} />
            <div className="verdict-crowned">
              {drawn ? (
                <span className="verdict-label">Unclaimed</span>
              ) : (
                <>
                  <span className="verdict-label">Takes the wreath</span>
                  <span className="verdict-name" title={champion.name}>
                    {champion.name}
                  </span>
                  <span className="verdict-tally">
                    {champion.wins} of {result.played}
                  </span>
                </>
              )}
            </div>
          </div>
          {drawn && (
            <p className="verdict-drawn-line">
              Neither deck took the match — the wreath goes back on its hook.
            </p>
          )}
        </div>

        {!drawn && fallen && (
          <div className="verdict-side">
            {/* **The stone stands and its name is at its foot**, where the
                wreath hangs above one. That is the whole shape of the panel:
                a crown comes down onto a head, a grave marker goes up out of
                the ground, and the two names end up at different heights
                because the two things happen in opposite directions.

                The object is a *cippus* — a Roman marble funerary altar, cut
                for a woman named Cominia Tyche around A.D. 90 — rather than
                the gauntlet that stood here before it. The gauntlet was a
                good idea nobody could read (Aaron, 2026-08-27: "mega lame, I
                can't even tell what it is"); a grave marker needs no caption
                in any language. `cippus.recipe.yaml` argues the swap. */}
            <div className="verdict-fall">
              <div className="verdict-plot">
                <img className="verdict-stone" src={cippusArt} alt=""
                     draggable={false} />
                <span className="verdict-ground" aria-hidden="true" />
              </div>
              <div className="verdict-fallen">
                <span className="verdict-label">Left on the sand</span>
                <span className="verdict-name" title={fallen.name}>
                  {fallen.name}
                </span>
                <span className="verdict-tally">
                  {fallen.wins} of {result.played}
                </span>
              </div>
            </div>
          </div>
        )}
      </div>

      <div className="verdict-foot">
        <button type="button" className="btn btn-quiet btn-xs"
                onClick={() => setCleared(true)}>
          Clear the sand
        </button>
      </div>
    </section>
  )
}
