/** Load a heavy component when it is first about to be drawn.
 *
 * Written for recharts, which is 113kB gzipped — larger than the whole rest
 * of the application — and was being downloaded by three routes whether or
 * not a chart ever appeared on the screen.
 *
 * Its own module because `components/lazycharts.tsx` exports nothing but
 * components, which is what lets fast refresh work there; a factory and a
 * placeholder living alongside them would cost that.
 */

import { lazy, Suspense, type ComponentType } from 'react'

/** Wrap one chart so its module loads on first render.
 *
 * The loader is passed in rather than looked up by name, so each call infers
 * one concrete prop type: a single generic factory over every chart at once
 * cannot be spread into JSX without TypeScript losing the correspondence
 * between the name and its props, and the whole value of this file is that a
 * screen importing `CurveChart` from here still gets `CurveChart`'s props.
 */
/** The space the component is about to occupy, held open while it loads.
 *
 * Sized rather than empty, and that is the point: a fallback with no height
 * lets the page collapse and then jump when the chunk lands. It borrows the
 * surface's own gridline colour rather than drawing a spinner, because what
 * is arriving is a picture and a pulsing box reads as one.
 *
 * A plain function returning an element rather than a component, so this
 * module exports no component at all and fast refresh keeps working in the
 * files that import it.
 */
function space(height: number) {
  return (
    <div aria-hidden className="w-full animate-pulse rounded-lg"
         style={{ height, background: 'var(--gridline)', opacity: 0.35 }} />
  )
}


export function deferred<P extends object>(
  load: () => Promise<ComponentType<P>>, height: number,
): ComponentType<P> {
  const Inner = lazy(async () => ({ default: await load() }))
  return function Deferred(props: P) {
    return (
      <Suspense fallback={space(height)}>
        <Inner {...props} />
      </Suspense>
    )
  }
}
