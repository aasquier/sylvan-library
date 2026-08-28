# 48. Light lands on a card, and never through it

**Status:** Accepted · **Decided:** 2026-08-28 with Aaron · Widens
[ADR 32](0032-card-art-motion-is-derived-at-runtime-and-never-committed.md)'s
effect vocabulary from the motion tier to every surface in the app, and
**corrects a misquotation of Scryfall's guidelines that ADR 32,
`cardimagery_test.go` and `index.css` all repeated**.

Aaron opened the session with it: *"We are a free-use site, period, they are
our card art provider and we must be in compliance."*

## Context

### The rule, quoted rather than remembered

From <https://scryfall.com/docs/api>, "Use of Scryfall Data and Images",
fetched 2026-08-28 with a descriptive User-Agent (the generic fetcher gets a
403). The image clauses, complete and verbatim:

> - Do not cover, crop, or clip off the copyright or artist name on card
>   images.
> - Do not distort, skew, or stretch card images.
> - Do not blur, sharpen, desaturate, or color-shift card images.
> - Do not add your own watermarks, stamps, or logos to card images.
> - Do not place card images in a way that implies someone other than Wizards
>   of the Coast created the card or that it is from another game besides
>   Magic: The Gathering.
> - When using the `art_crop`, list the artist name and copyright elsewhere in
>   the same interface presenting the art crop, or use the full card image
>   elsewhere in the same interface. Users should be able to identify the
>   artist and source of the image somehow.

> Repeated mishandling or misrepresentation of data or images in your project
> may result in Scryfall restricting or blocking your API access.

**Three corrections fall out of reading it rather than recalling it**, and
they are the durable half of this ADR:

1. **Cropping is not banned. Cropping *the credit off* is.** ADR 32,
   `cardimagery_test.go` and the `.arena-gate` block in `index.css` all say
   flatly that the guidelines "forbid cropping". They do not — and Scryfall
   themselves publish `art_crop` and `art` endpoints that are nothing but a
   crop. This overstatement had a real cost: it is why `object-fit: cover`
   sat in the guard's comment for months as an unanswerable "policy call
   rather than a bug", when the rule gives it an answer.
2. **Blur is forbidden explicitly**, in the same breath as desaturate. It is
   not a lesser offence than the colour ones, which matters because the
   largest violation found was a blur, and because the blur was load-bearing.
3. **`brightness` and `contrast` are not enumerated by name.** "Color-shift"
   is the umbrella they sit under in any ordinary reading. ADR 32 already
   chose the strict line, and it stays strict — stricter than required is
   free, and commandment 9 asks for it. But the citation is now honest.

### What was actually live

A sweep on 2026-08-27 found ten colour, blur or distortion filters landing on
Scryfall images on the deployed site. Re-verified 2026-08-28; all ten were
still there. A second pass — **cross-referencing every altering filter in the
stylesheet against the JSX that puts card art on the class it names** — found
four more, including the two biggest, and a third pass of the same shape found
a fifteenth (`.field-fan-card`, the gear fan's covered cards, at
`brightness(0.72) saturate(0.9)` over a whole printed card). Fifteen in total,
on twelve surfaces, on a site whose ADR had said "never distortion, blur or
colour-shift of the artwork" for a fortnight.

**That the third pass found one is the finding, not a footnote.** Two careful
readings of a written audit missed it and one mechanical cross-reference
caught it in seconds. So the durable instruction is a *procedure* rather than a
list: read every altering `filter` in `index.css`, take the subject class of
each rule, and ask the JSX whether anything with that class holds a card image.
The next session should re-run that, not re-read this.

**How fifteen violations reached production is the part worth recording.**
None of them was careless. Every one is somebody reaching for the obvious CSS
for an obvious intention — *this card is dead*, *this painting is too dark
against a black page*, *this neighbour is behind the front one*, *this is a
vision in glass rather than a picture on a screen* — and `filter` is the
property CSS offers for all four. The rule existed, in a document. A document
does not stop that.

Two more shapes worth naming, because they are how the guard missed them:

- **An inline `style` filter in JSX is invisible to a stylesheet reader**, by
  construction. `routes/DeckDetail.tsx` carried
  `style={{ filter: 'grayscale(0.7)' }}` on a dead card's art, and every check
  in `cardimagery_test.go` read a clean stylesheet and said so.
- **The guard excused `grayscale(1)`.** Its single list of harmless arguments
  accepted `1` for every function, which is the identity for `saturate` and
  `brightness` and **full strength** for `grayscale`, `sepia` and `invert`. The
  strongest possible desaturation read as a no-op. `@keyframes entomb-sink`
  was carrying exactly that.

## Options considered

1. **Move every filter to a pseudo-element and change nothing else.** The
   shape `.field-card-leaf::after` already used, and the right answer for most
   of the fourteen. It fails on three of them for concrete reasons: an `<img>`
   is a replaced element and has no pseudo-element to move a filter to; a
   `saturate()` has no layer equivalent at all; and one filter *was* the
   effect rather than a treatment of it.

2. **Delete every filter and accept the visual loss.** Honest, cheap, and
   wrong where the filter was carrying something. The dark-mode lifts exist
   because a dark commander painting on a near-black page really is mud
   (commandment 7), and the tarot vision's shimmer is what made a card in a
   crystal ball read as a card in a crystal ball (commandment 15). Deleting
   those buys compliance with quality, which is the trade commandment 15
   forbids.

3. **Two layer primitives, plus a threshold, plus one real design change.**
   Chosen.

## Decision

**Nothing reaches through a card and changes Wizards' painting. Light on a
card is a layer of its own.**

### The two primitives

`web/src/index.css`, in one commented block ("light lands ON a card"):

- **the shade** — black over the art. This is what a `brightness(k < 1)` was
  doing, and the conversion is `alpha = 1 - k`. It is written as an `::after`
  on the box the card is already in — `.card-sheet-slide`, `.field-fan-card`,
  `.entombing` — rather than as a standalone class, because every site so far
  had a box, and a class nothing uses is a class the next session has to work
  out the purpose of.
- **`.art-lift`** — a warm near-white at `--art-lift` alpha under
  `mix-blend-mode: screen`. This is what a `brightness(k > 1)` was reaching
  for, and **it is strictly better at it**: a multiply scales every channel and
  so blows highlights first, which is the bug the `.hero-art` comment already
  recorded (`1.75` "blew the highlights out to a flat yellow-green haze" on a
  bright forest). A screen lifts shadows and leaves highlights nearly alone.
  Equivalence at a dark mid-tone: `alpha = (k - 1) * v / (1 - v)`, `v = 0.25`.

`.art-dimmed` is the shade with a box of its own, for the case where the card
has no container to hang an `::after` on — a thumbnail in a graveyard row that
has to read quieter than its neighbours.

**`saturate()` gets no primitive and does not get one.** It is the clause by
name, and every `saturate()` in the sweep was between 1.02 and 1.15 — a delta
no eye finds and no painting needed.

### The threshold

**Under ten per cent, a grade is deleted. Over ten per cent, it becomes a
layer.** A layer costs a DOM node and sometimes a stacking-context rewrite of
its siblings; below the threshold there is nothing an eye can find to justify
that. This is a stated rule rather than a case-by-case feel, so the next
session does not have to re-litigate a six per cent dim.

### The wash

`SceneBackdrop` — the page's own masthead painting washed across the whole
viewport, on **every mastheaded route** — was that painting again at
`blur(11px) saturate(1.02)`. It is the most-seen violation the sweep found and
the only one where the forbidden thing *was* the effect: there is no layer to
move a blur onto when the blurring is the idea, and deleting it leaves a sharp
faint clone of the painting bleeding across the page, which the rule's own
comment records as having read as a rendering mistake once already.

**So the picture leaves and its colours stay.** `web/src/lib/artwash.ts` reads
the already-hot-linked image into a 24x18 canvas, averages it to a 3x3 grid,
and hands back nine swatches the stylesheet lays out as overlapping lobes of
light. `cards.scryfall.io` answers with `access-control-allow-origin: *`
(checked against live headers 2026-08-28), which is what makes
`crossOrigin="anonymous"` readable rather than tainting. Nothing is rehosted
and nothing new is fetched — the sampling reads the browser's own cache of an
image the masthead is already showing.

Nine colours are a palette, not a card image, so there is nothing on screen to
violate anything. It is also simply better: a colour field has no edges to
catch, at any viewport size, so the sharp-clone problem cannot recur. If the
read ever fails — a tainted canvas, a changed CORS header, a browser without
one — the hook returns null, the element is not rendered, and the room falls
back to the procedural ambience loop already drawing underneath it. **A
missing wash is a quieter page, never a broken one.**

### `object-fit: cover`, answered

Under the corrected reading the question is narrow and checkable: it is a
violation exactly when it clips the artist or copyright line off a **full card
image**, and it is fine on an art crop credited elsewhere in the same
interface.

Measured across the app on 2026-08-28: every `object-fit: cover` on card art
is on an `art_crop`, except two, and both are full cards in boxes that match
the printed 488x680 — `.stage-face` is exactly `488 / 680`, and
`.field-card-turn` is `58 / 81`, which is 0.716 against 0.7176, a fifth of a
per cent. **Nothing in this app clips a credit.** No change was made, and the
"policy call" is closed rather than deferred.

The art-crop attribution clause was also checked end to end. Every art crop
the app renders is either credited in words in the same interface or sits
beside a full card image, and both alternatives satisfy the clause. The one
gap reported by the audit — `FirstRun`'s hero in `routes/Library.tsx` — was
**not a gap**: the credit has been in place since PR #18, as a `<p>` at the
foot of the same panel. The audit missed it because the credit is a distant
sibling of the `<img>` rather than adjacent to it. Nothing was changed there
either.

### The guard

`go/cmd/mtglab/cardimagery_test.go` keeps this, and gains four things:

- **It reads the bundle's script as well as its stylesheet.** An inline JSX
  `style` filter is banned outright, whatever element it is on. The
  alternative considered was a lint rule forbidding `filter` in a `style`
  prop; reading the artifact wins for the same reason it won for the CSS — a
  lint rule polices one spelling in one language in one directory, and the
  bundle is what the browser is handed however it got written. The flat ban
  costs nothing real, because an inline `style` is the wrong place for a
  filter anyway: no `:hover` reaches it and no media query arrests it, which
  is commandment 17's lesson one property over.
- **`filter: url(#…)` is altering, always.** Whatever is behind that id can do
  anything; there is no argument to inspect.
- **Per-function identities.** An omitted argument means 1 for `grayscale`,
  `sepia`, `invert`, `saturate`, `brightness`, `contrast` and `opacity`, and 0
  for `blur` and `hue-rotate` — and for the first three of those, 1 is full
  strength. The minifier writes both spellings, because it drops an argument
  that equals the default, so `grayscale()` and `saturate()` arrive looking
  identical and mean opposite things.
- **`artBearing` widened from six classes to twenty-one**, each still naming
  the component it was read out of. The anti-vacuity check now consults the
  script too, because a class can be real and have no CSS rule — which
  `.deck-hero-band` became the moment its filter moved to a sibling layer.

The commit that closes this also proves the script reader is not vacuous by
putting the graveyard's `grayscale(0.7)` back, rebuilding, and watching the
check name the file — which is how it was found that the minifier emits a
template literal rather than the quotes the source used.

## Consequences

**Every mastheaded page looks different**, because the room behind it is now a
colour field rather than a blurred painting. This is the change most wanting
Aaron's eye (commandment 16), and it landed while he was away from the laptop.

**Fourteen effects were rebuilt and one was redesigned.** The seance vision no
longer ripples — an `feDisplacementMap` at `scale="15"` is fifteen pixels of a
painting physically moved, and *"Do not distort, skew, or stretch"* is that
clause as literally as it can be broken. What replaced it is
`.seance-glass-caustic`: two soft lobes of light crawling on the sphere on a
thirteen-second period that never lines up with the vision's nine-second sway.
The depth turns out to have been in the light rather than in the picture, and
the card in the glass is now exactly the card. The tarot deck is 136 cards
with 58 Magic crossovers weighted 1.5x, so that card is a Magic painting about
half the time; commandment 15 is why this one was redesigned rather than
merely deleted.

**Two things were deliberately not touched**, both ruled on by Aaron on
2026-08-27 as genuine credited re-cuts rather than filters: the tarot
`CrossoverFace`, a landscape crop fitted into a portrait 1909 plate, and
`.wheel-heart-bloom`, a detail of Gelon's painted heart cut at 8.5x
magnification.

**But `.wheel-heart-bloom` is not only a crop, and that wants re-asking.** It
carries `saturate(1.7) brightness(1.3) contrast(1.08)` over
`background-image: url(WHEEL_ART)` — a 70% saturation boost on Wizards'
painting, the largest colour-shift left anywhere in the app and larger than
anything this ADR removed. The ruling that spared it was about *the cut*, and
the filter appears not to have been in front of Aaron when he made it. It was
left in place because the instruction to leave it was explicit; it is recorded
here because a compliance sweep that quietly steps around its own biggest
remaining number is not a compliance sweep. Under the crop reading it is fine
and under the colour-shift clause it is not, and only Aaron can say which one
he meant. **Open, for Aaron.**

**The strict reading is now a cost we are choosing.** `brightness` and
`contrast` are ours, not Scryfall's, and a future session that wants a grade
back has an argument available. It should make it here, in a superseding ADR,
rather than in a rule.

**Commandment 19 exists because of this.** Card-art integrity was implied by
commandment 9 and enforced only by a test most sessions never read, which is
exactly how fourteen violations reached production. It is now a commandment
pointing at `cardimagery_test.go`, the way commandment 17 points at the `.btn`
family.
