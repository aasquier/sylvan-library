/** The end of a bout: a wreath for the deck that took the match, and the
 *  gauntlet left on the sand by the one that did not.
 *
 *  **Why this is a panel in the page and not an overlay on the board.** A
 *  match here is something you watch, and the board keeps its scrubber
 *  afterwards precisely so somebody can go back through it. Anything drawn
 *  over that board is a thing between a person and the game they just watched,
 *  and it would have to be dismissed before the board could be used at all.
 *  So the verdict takes its own room, at the head of the result, below the
 *  field — it is passed on the way to the numbers rather than put in front of
 *  them.
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
import manicaArt from '../assets/coliseum/manica.webp'

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
            <div className="verdict-fall">
              <div className="verdict-fallen">
                <span className="verdict-label">Leaves the sand</span>
                <span className="verdict-name" title={fallen.name}>
                  {fallen.name}
                </span>
                <span className="verdict-tally">
                  {fallen.wins} of {result.played}
                </span>
              </div>
              <img className="verdict-gauntlet" src={manicaArt} alt=""
                   draggable={false} />
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
