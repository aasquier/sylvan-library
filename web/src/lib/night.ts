/**
 * The Coliseum at Night's two switches, turned into words.
 *
 * The hall echoes the operator's own settings back verbatim (`01:00-04:00`,
 * `America/Los_Angeles`) so that a person comparing the room to the instance
 * compares like with like; what renders is this file's job. It lives apart
 * from the settings room because a route file that exports anything but its
 * component breaks fast refresh, and because the Coliseum's own pages will
 * want the same two sentences the day they say when the torches are lit.
 */

/** The zone switch as a place a person would name: the last segment of
 *  `America/Los_Angeles`, underscores returned to spaces. The wire form is a
 *  technology's name for a city and never renders (commandment 10). */
export function zoneWords(zone: string): string {
  const city = zone.split('/').pop() ?? zone
  return city.replace(/_/g, ' ')
}

/** `01:00-04:00` as the two clocks a sentence needs. A window in any other
 *  shape renders as itself rather than as nothing: a blank where a time
 *  should be would hide a misconfigured instance from the one person who
 *  would fix it. */
export function windowWords(window: string): string {
  const [open, close] = window.split('-')
  return open && close ? `from ${open} to ${close}` : window
}
