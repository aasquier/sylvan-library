/**
 * The forest stepping out of a room.
 *
 * The fireflies, the falling leaves (`ForestAmbience`) and the canopy over
 * the header are the forest the whole site stands in, and they mount in the
 * shell, above any prop a room could thread. Over a room that is footage —
 * the tavern, the campfire, the hut — they are two pictures at once, and
 * Aaron's first note on the first footage walk was that they pull the eye
 * off the film. So a room says it with a class on `<body>` for its lifetime,
 * and `body.in-footage-room` in index.css is what the forest reads.
 *
 * In `lib/` because two components need it (`roomplate.tsx` for the
 * full-bleed rooms, `cauldron.tsx` for the hut) and oxlint's fast-refresh
 * rule is right that a hook is not a component.
 */

import { useEffect } from 'react'

export const FOOTAGE_ROOM_CLASS = 'in-footage-room'

export function useForestStepsOut(): void {
  useEffect(() => {
    document.body.classList.add(FOOTAGE_ROOM_CLASS)
    return () => document.body.classList.remove(FOOTAGE_ROOM_CLASS)
  }, [])
}
