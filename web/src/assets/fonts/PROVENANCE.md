# Fonts

The tarot reading's letterforms (commandment 15). Every file in this
directory carries its licence argument here, the tarot deck's rule applied
to type.

Every face here is under the **SIL Open Font License 1.1**, whose text is at
`licenses/OFL-1.1.txt` with both copyright statements at its head. The OFL's
second condition asks that the copyright notice and the licence travel with
each copy; the notice travels *inside* each file — checked, not assumed, by
reading the `name` table of every woff2 here (nameID 0 carries the copyright
and, where the foundry reserved one, the reserved font name; nameID 14 the
OFL's URL) — and the licence text
travels beside them. `NOTICE.md` records that check.

## IM Fell English, IM Fell English SC

- `im-fell-english-regular.woff2`
- `im-fell-english-italic.woff2`
- `im-fell-english-sc.woff2`

Igino Marini's digitisation of the types cut for Dr John Fell at the Oxford
University Press in the 1670s — the Fell Types. Chosen for the fortune
teller's table because the Rider-Waite deck's own captions are hand-lettered
in exactly this tradition: seventeenth-century English book type, worn
edges and all. A period face that is also genuinely readable, which is what
commandment 2 requires of any costume this project puts on.

**Licence: SIL Open Font License 1.1**, with the reserved font name
"IM FELL". The OFL permits use, redistribution and bundling (including in
web form) provided the software is not sold by itself and the names are not
misused; both hold here — the fonts ship inside a free application and are
named only in `@font-face` declarations.

- Upstream: Igino Marini, <https://iginomarini.com/fell/> — the digitally
  revived Fell Types, released under the OFL.
- Fetched: 2026-08-16, as woff2 subsets via Google Fonts (families
  "IM Fell English" and "IM Fell English SC", latin subset), which serves
  the same OFL-licensed faces.
- Verified: the OFL 1.1 text and the reserved-name clause were read at
  fetch time; the licence file for the family is published with the family
  on Google Fonts and at the upstream site.

The 1909 Rider tarot scans these sit beside have their own argument in
`assets/tarot/PROVENANCE.md`.

## Caveat

- `caveat-variable.woff2`

The fortune-teller's handwriting (the séance question card), succeeding
Parisienne on 2026-09-05 — Aaron's call, on commandment 2 grounds. The
hand's history: Parisienne won the original board of five OFL script
faces on 2026-08-17 — over the two copperplates (Mrs Saint Delafield,
Herr Von Muellerhoff), whose hairlines go faint at reading size, and over
Allura on Aaron's taste — and its joins were the point: the ink animation
reveals each word left to right, and in a connected face that reveal
reads as the pen travelling, since the leading edge never leaves a
stroke; in the Fell italic it could only ever read as a wipe. But
Parisienne is a tall-looped cursive with a small x-height, and a newcomer
squinting at the fortune-teller's own question is commandment 2 failing
at the exact table built for them. Caveat keeps the joins — a connected
hand still, shaped by contextual alternates, so the reveal still reads as
writing — on a taller, more open semi-print body that stays legible at
reading size. One variable file carries the whole weight range (400–700),
so the hand's weight is a tuning knob rather than another download.

**Licence: SIL Open Font License 1.1** (The Caveat Project Authors;
designed by Impallari Type). Same terms as the Fell faces above; bundled
in a free application, named only in `@font-face`. No reserved font name.

- Upstream: Impallari Type, via Google Fonts, family "Caveat"
  (<https://github.com/googlefonts/caveat>).
- Fetched: 2026-09-05, as the latin variable woff2 (wght 400–700) from
  fonts.gstatic.com.
- Verified: the family's own OFL.txt read at fetch time, its licence body
  diffed identical to `licenses/OFL-1.1.txt`; the binary's `name` table
  read the way `NOTICE.md` records for its neighbours — nameID 0 carries
  the copyright ("Copyright 2014 The Caveat Project Authors"), nameID 14
  the OFL's URL. The head of `licenses/OFL-1.1.txt` now carries Caveat's
  copyright statement where Parisienne's stood, since the notice must
  travel with the faces actually shipped.
