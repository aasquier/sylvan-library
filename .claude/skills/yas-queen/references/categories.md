# The ten categories

The ball's floor plan. Each category has an eye, its chop tells, what a ten
looks like, how to check it, and **the house exemplar** — a place in this repo
that already does it right, because a standard with no example is a mood.

Walk the categories in order for a full ball. Name one for a spot check —
*"Category is: Runway, on the reading room"* — and give it the whole eye.

---

## 1. Category is: Hand Answers Hand

**The eye.** Every control answers the hand that reaches for it, and it
answers **three** separate ways: hover, focus, press. Commandment 17 is the
law; this category is whether the reply is *worth* hearing.

**Chop tells.**
- **A fade.** `hover:opacity-90`, `hover:opacity-80`, any hover that *removes*
  light. Dimming is the universal language of **disabled** and, in Magic, of
  **tapped**. Hover adds light. Press takes it away. That is the grammar.
- `:hover` with no `:focus-visible` anywhere in the family. Keyboard users get
  no hover, ever, so the control is invisible to them. Two-thirds done at best.
- Styling that lives in `style={{…}}` — a `:hover` cannot reach an inline
  style, so the control *cannot* be fixed where it stands.
- A press with no reply. Hover lifts it and click does nothing? The hand
  pushed and the room did not move.
- A control that keeps accepting clicks while it works. On a **write** that is
  a double edit, not a double read.
- `cursor: pointer` doing the entire job. A cursor is a promise, not a reply.

**What a ten looks like.** Hover *adds* — a lift, a light, a warmed hairline,
a glint that sweeps once. Press *settles* — the lift collapses, the shadow
goes, the thing feels physically depressed. Focus is a real ring that respects
the composition. Busy is `disabled` **and** visibly pending, both halves. And
all of it lives in **one named place** so the next session finds it.

**How to check.** In the browser, with a mouse and then with only the Tab key.
Hover it, tab to it, press and hold it, click it twice fast. Then run the
undressed dragnet in `SKILL.md` for anything you have not laid a hand on.

**The house exemplar.** `.btn` + `.btn-primary`, `web/src/index.css:3995` —
the transition list names five properties on purpose, `:focus-visible` sits on
the parent so every variant inherits it, `:disabled` drops to 0.4 and kills
the cursor, and the glint sweeps **only** on hover and **never** under reduced
motion. And `.card-action[aria-pressed='true']` (`:3890`) for a toggle that
says out loud that it is a toggle.

---

## 2. Category is: Face

**The eye.** Typography. The face is the first thing read and the last thing
noticed. Scale, weight, tracking, line length, numerals, and the one `h1`.

**Chop tells.**
- Four type sizes inside one card, none of them a step apart in the scale.
- Weight doing hierarchy's whole job — `font-medium` on everything, so
  nothing is emphasised because everything is.
- Numbers in a table that are not tabular, so the column wobbles as it
  updates. `.tabular` exists (`web/src/index.css:174`); use it on every
  figure that changes.
- Body text wider than about 75 characters, or a two-word orphan under a
  heading.
- `letter-spacing` untouched on all-caps. Caps *need* tracking; that is not a
  preference.
- Two `h1`s, or a route with none — `PageMasthead` owns the page's `h1`
  (`web/src/components/ui.tsx:434`) and it is the answer, not a suggestion.
- A serif borrowed into a dense data table because it looked romantic in
  isolation. It is not romantic at 12px in a column of 40 rows.

**What a ten looks like.** A visible step between levels. One weight change
per level, not three. Figures aligned. The display face where the room wants
atmosphere, the workhorse where the eye wants *data* — and the seam between
them chosen, not inherited.

**How to check.** Screenshot at desktop and at the mobile preset, then squint
— literally blur your eye at the image. Hierarchy survives blur; decoration
does not.

**The house exemplar.** The seam itself, and it is deliberate: **IM Fell
English** (a 17th-century face, licence recorded in
`web/src/assets/fonts/PROVENANCE.md`) carries the rooms — the séance, the
reading room, the room signs — with **Parisienne** for a hand-written moment,
while the workhorse stays `system-ui` (`web/src/index.css:126`) where the eye
is reading numbers. That is a real typographic decision, not laziness: an
old-face at 13px in the simulator's grid would be atmosphere bought with
legibility, and commandment 2 does not permit that trade. **Where you find
that seam, check it was chosen** — a room in system-ui, or a table in IM Fell,
is where it slipped.

---

## 3. Category is: Body

**The eye.** Layout: rhythm, alignment, density, and breathing room. The
silhouette. Where a garment is *cut*.

**Chop tells.**
- Spacing values from nowhere: 13px beside 15px beside 18px. Pick the scale
  and stay on it.
- Optical misalignment — an icon centred by its box rather than by its ink.
- A card with generous padding beside a card with none, in the same grid.
- Everything the same width, so the eye has no idea what matters.
- A control bar that reflows into a ladder at 375px and nobody looked.
- Dense-by-default on the newcomer's route. Density is for the *expert*
  surfaces; the front door gets air.

**What a ten looks like.** One spacing scale, visible rhythm, a real focal
point, and generosity where the reader has to make a decision. Whitespace is
not empty; it is where the eye rests before choosing.

**How to check.** The blur test again for silhouette. Then the mobile preset
with a **reload** (device gates run at load), then a mid-width drag to catch
the breakpoint nobody designed for.

**The house exemplar.** `PageMasthead` — the painting **whole at its own
ratio** beside the title, never a cropped band behind it. `art_crop` is
1.37:1, a full-bleed band keeps less than half of it, and this project learned
that the hard way four separate times. That is a layout rule bought with
scars; do not re-buy it.

---

## 4. Category is: Realness

**The eye.** Materials. Whether a thing looks *made of something*. This is the
category where "best of the best" is either true or a slogan, and it is
commandment 5's enforcement arm — no clip art, nothing cutesy, nothing
vector-cartoon.

**Chop tells.**
- **Flat gold.** A `#FFD700` rectangle is not gold, it is a sticker. Gold is a
  gradient plus a highlight plus a shadow, minimum.
- One `box-shadow` doing the work of a bevel. Real hardware has an inset
  hairline, a top highlight, an inner shade and a drop — four, not one.
- A "glass" panel with no edge. Glass is 90% edge.
- Felt with no nap, wood with no grain, brass with no tarnish in the crease.
- Emoji or a stock vector doing an icon's job on a surface a user pays
  attention to.
- A gradient between two hues that muddies through grey in the middle. Mix in
  a better space or add a stop.
- Card art *simulated* rather than used. There are hundreds of thousands of
  real paintings one hotlink away (rule 5, ADR 6 — hotlink, never commit).

**What a ten looks like.** You can name the material out loud without being
told, and it behaves like that material when light moves across it.

**How to check.** Zoom to 200% and look at the edges — cheapness always lives
in the edges. Then both themes: a bevel tuned for dark is often mud in light.

**The house exemplar.** `.wheel-spin-btn`, `web/src/index.css:3694` — brass
inset hairline, top highlight, deep inner shade for a concave face, drop
shadow above the felt, ink at `#f3e2b8` because candlelight is not white. Also
the anvil's own three tokens (`--anvil-iron`, `--anvil-brass`,
`--anvil-glint`): a material given *named* values so it stays the same
material everywhere it appears.

---

## 5. Category is: Runway

**The eye.** Motion. Commandment 6 wants the site alive; this category is
whether the movement has **intention** or is just a page that fidgets.

**Chop tells.**
- `transition: all`. Name the properties. `all` animates things you have not
  thought about, including layout.
- `linear` on anything a hand touches. Hands expect ease-out.
- 400ms on a hover. A hover is 120–180ms; longer feels like syrup.
- A loop with no reason, or a loop so long the user never sees it fire.
- Something that animates *in* and then never acknowledges leaving.
- Ambience **frozen** under `prefers-reduced-motion` rather than removed. That
  ruling is settled in `web/README.md`: frozen weather is a smudge.
- Motion carrying meaning nowhere else stated — if the animation is the only
  thing saying "saved", a reduced-motion user was never told.
- Ten things easing in at once with no stagger. A ball walks *one* girl at a
  time.

**What a ten looks like.** Every movement says what happened or where a thing
came from. Enter is staggered, exit is faster than enter, easing is chosen per
property, and the whole thing has a reduced-motion path that is *designed*
rather than switched off.

**How to check.** In the browser, at real speed, then again with reduced
motion on. Count the loop and **tell Aaron the cycle time** — commandment 16
is explicit that nobody should stare at a hole waiting for a snake that comes
out once a minute.

**The house exemplar.** The forest layer (`web/src/components/forest.tsx`):
inline SVG on `var(--vine)`, no assets at all, and the two themes get
genuinely *different weather* — fireflies at night, falling leaves by day —
display-gated in CSS, `aria-hidden`, `pointer-events: none`, never
load-bearing, and removed outright under reduced motion. Decoration that
knows it is decoration. And `.btn-primary::after`: one sweep, hover only,
gone under reduced motion, and its own comment says it stays subtler than the
wheel's button on purpose. That sentence is a *house* thinking.

---

## 6. Category is: Labels

**The eye.** Copy as design. The words on the controls, and whose language
they are in. Commandment 3 wants Magic to permeate; commandment 10 forbids
naming the machinery; commandment 2 outranks both if a newcomer would be lost.

**Chop tells.**
- A button labelled with a noun. Buttons are verbs. "Confirmation" is a sign;
  "Confirm" is a button.
- Conversational English where Magic has its own word: *shuffle, mulligan,
  untap, cast, mana, sideboard, commander, pod, tapped out*. Say ours.
- A wire token rendered raw — `second-opinion`, `on-request`. `lib/claudecopy.ts`
  is the only place a token becomes a label, and it exists for exactly this.
- Any technology named on a user surface: a language, a database, a framework,
  a seed, a model id. Claude is the *only* nameable name, and by name only.
- An error that reports a status code and no next step.
- An empty state that says "No items."
- Magic jargon at the front door with no teaching hand. A `Term`/`HelpTip`
  that teaches on hover is beginner-safe where a bare word is not — and its
  key has to exist in `go/internal/reference/data/glossary.json`, which
  **nothing checks**: a typo'd key costs the tooltip silently, in both the
  suite and the browser. Confirm a new key against that file by hand.
- Flavour that costs clarity. A cryptic label is a worse flavour failure than
  a plain one, because it teaches a newcomer that the site is not for them.

**What a ten looks like.** The verb is on the button, the game's word is used,
the newcomer has a hand offered, and nothing on screen admits a computer
exists.

**How to check.** Read every string on the route out loud in a beginner's
voice. Then grep the surface for wire tokens and technology names.

**The house exemplar.** `web/src/lib/claudecopy.ts` — commandment 10
implemented as code rather than as vigilance, which is the only way an
absolute claim survives a session with no memory of this conversation.

---

## 7. Category is: The Card Back

**The eye.** The parts only some of your people ever see. Every Magic card
ever printed has the identical back, because one card that looks different
**marks the whole deck** — the face nobody examines is the one that has to be
perfect. Here that is the focus path, the accessible name, the contrast ratio,
the touch target, the reduced-motion route. **This is where the house's real
standard lives**, because nobody is watching and it gets done anyway.

**Chop tells.**
- A tab order that jumps, or a stop with no visible ring.
- An icon-only control with no `aria-label` — a drawing contributes *nothing*
  to the accessibility tree on its own.
- `--text-muted` on `--surface-1` at 11px for something that matters. Muted is
  for the third thing on the page, never the second.
- A drawn pip with no `role="img"` and no colour name
  (`web/src/components/manasymbol.tsx` gets this right — read it before
  drawing anything).
- A dialog that opens and leaves focus behind it, or traps focus with no way
  back.
- Colour as the only carrier of meaning. Ask what a colourblind reader sees;
  ask what a screen reader *says*.
- A 30px tap target on a phone.

**What a ten looks like.** The keyboard walk is as pleasant as the mouse walk.
Every control has a name a screen reader can say. Nothing means anything by
colour alone.

**How to check.** Tab the whole route. Then read the DOM with `read_page` and
check every interactive node has an accessible name. She owns the whole of it
here — the systematic sweep **and** whether the finish is beautiful as well as
merely present.

**The house exemplar.** The castability heatmap's palette
(`web/src/index.css`, the `--heat-*` block): a **sequential ramp of one hue**
rather than red-to-green, chosen because the grid's values are not
good-versus-bad and because red/green is the commonest way to build a chart a
colourblind reader cannot use — **and every cell prints its own number over
the wash**, so colour finds the shape and never carries the value alone. Read
that comment. That is the lining, hand-finished.

---

## 8. Category is: Both Themes, Both Hands

**The eye.** Light and dark are two designs. Phone and desktop are two
designs. Four rooms, not one.

**Chop tells.**
- A colour hard-coded where a token exists. There are 54 tokens in
  `index.css`; the odds that your hex is the right one are poor.
- Decorative art treated *identically* in both themes. It needs **opposite**
  treatment — dark mode brightens, light mode dims — and the reasoning is
  written beside `.hero-art`.
- A shadow tuned on white that vanishes on near-black, or a scrim tuned for
  light that turns a dark forest painting to mud.
- A hover state visible in one theme only.
- A mechanism that needs hover, on a surface a phone will reach.
- A new visual defined inline when the two themes need different treatment —
  that belongs in `index.css` (the rule is in `web/README.md`).

**What a ten looks like.** Both themes look *designed*, not inverted. The
phone layout is a considered layout, not a squeezed one.

**How to check.** `resize_window` with `colorScheme` for both, plus the
in-app gear (`web/src/components/settings.tsx` — the one place a preference
about *you* lives). Mobile preset, then **reload**. Screenshot all four.

**The house exemplar.** The two-selector `data-theme` dance used throughout
`index.css` (35 occurrences), and `.hero-art`'s comment explaining *why* the
treatment inverts. A rule with its argument attached beside it survives a
session that never read this file.

---

## 9. Category is: The Pack

**The eye.** Everything before the content: first paint, loading, skeleton,
empty, disabled, error, 404, and the seat of a user who has *nothing yet*.
Magic players love the wrapper and the crack as much as the card that comes
out of it. **A beginner meets the pack first, and sometimes meets nothing
else.**

**Chop tells.**
- A spinner where a skeleton belongs — a skeleton keeps the layout, a spinner
  throws it away and then bangs it back down.
- "No decks yet." That is a *stub*. The empty library is the single best
  invitation surface in the app and it should be gorgeous.
- A disabled control with no reason given. A newcomer assumes they broke it.
  A `title`, a helper line, anything that says *what would enable this*.
- An error that offers no way forward.
- Layout shift when data lands.
- A 404 that is a bare word.
- Never having *looked* — the repo hands you the doors:
  `mtglab-ui-empty-library` (8768) and `mtglab-ui-no-pool` (8766) exist for
  exactly this and cost nothing to open.

**What a ten looks like.** The empty state teaches and invites. Loading holds
the shape. Every disabled thing says why. The 404 is in the house's voice.

**How to check.** Open the empty-library door and the no-pool door and walk
them like a first-time visitor. Then throttle the network and watch the real
route load.

**The house exemplar.** `Spinner` (`web/src/components/ui.tsx:383`) takes a
`label` — a loading state that can *say what it is waiting for* rather than
just twirling. Use the parameter; that is what it is for.

---

## 10. Category is: House Codes

**The eye.** Coherence. Does this route belong to the same house as the
others? A Magic set holds one look across three hundred cards and four art
directors; a house style holds it across thirty years of sets. Ours has
to survive across routes **and across sessions with no memory of each other**.

**Chop tells.**
- A control's states defined in the route rather than in a named place. That
  is how a vocabulary dies — one honest little convenience at a time.
- A second control doing a job the shared one already does. Every preference
  about *you* is one gear (`components/settings.tsx`); a new preference
  belongs there and in `lib/prefs.ts`, not in a second panel.
- A route with no signature on it — no vine, no felt, no masthead painting, no
  wordmark. Unsigned is not neutral.
- A palette that appears once. One-off colour is a rented look.
- A component that reimplements `PageMasthead`, `Term`, `Spinner`, `CardHover`
  or `VideoBackdrop` slightly differently.
- A deck link built by hand instead of through `deckUrl` — one place builds
  those (`lib/api.ts`), and transposed positional strings become somebody
  else's 404.
- An absolute claim in prose that nothing enforces. "Always", "never",
  "every" — ask what fails if it stops being true. If the answer is nothing,
  it has already drifted. **Make it machine-checked; never just reword it.**

**What a ten looks like.** A stranger could tell two routes came from the same
house without being told, and a fresh Claude session finds the one named place
before inventing a second one.

**How to check.** Walk three routes back-to-back and screenshot them side by
side. Coherence failures are invisible one page at a time and obvious in a
row of three.

**The house exemplar.** The wardrobe itself — one `.btn` family, one
`.chip-toggle`, one `.strip-tab`, each with `:hover`, `:active` and
`:focus-visible` in a single named place, with `--btn-accent` and `--btn-ink`
letting the Simulator keep its series colour and the Laboratory its own
**without forking the class**. That is one point of view, worn in different
colours. That is a house.

---

## Working a category

1. Pick the category and the surface. Open the door; do not read the diff.
2. Walk it: mouse, keyboard, both themes, phone, reduced motion.
3. Write each finding in the four-part Read from `SKILL.md` — read, receipt,
   serve, score. No receipt, no finding.
4. Give the tens honestly and name the file, so the next session inherits the
   standard instead of the complaint.
5. If the chops share a cause, **stop chopping instances**. Name the system,
   fix it in one place, and record which instances were examined and
   deliberately left alone, **with the reason**. That last note is the only
   thing that stops a finding from being rediscovered forever.
