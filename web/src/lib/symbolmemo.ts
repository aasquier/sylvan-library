/**
 * Codes the official-symbol route has refused this session, remembered at
 * module level so one offline render of a 99 does not fire ~200 doomed
 * requests and re-fire them on every scroll. A symbol that comes back (the
 * network returns) is one reload away, which is what a browser already means
 * by "try again".
 *
 * Its own file rather than a corner of `manasymbol.tsx`, because the colour
 * wheel's vertices (`pentagram.tsx`) ask the same route and must share the
 * same memory — and a component file that exports a function stops being
 * fast-refreshable, which the lint refuses. Two verbs rather than the set,
 * so nothing outside can forget a failure or invent one.
 */
const FAILED = new Set<string>()

export function officialSymbolFailed(code: string): boolean { return FAILED.has(code) }
export function noteOfficialSymbolFailed(code: string): void { FAILED.add(code) }
