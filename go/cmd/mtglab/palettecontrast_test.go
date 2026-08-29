package main

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// A token that is read as words is measured against the page it is read on.
//
// # What went wrong, and why nobody saw it
//
// The stylesheet's palette is written as two columns, light and dark, and the
// comment at its head has always said the dark column is "the same hues
// re-stepped for the dark surface". For seven tokens it was not re-stepped at
// all. Four of them -- `--text-muted`, `--status-good`, `--status-warning`,
// `--status-serious` -- were the *same hex* in both themes, chosen against a
// near-black page and handed unchanged to a near-white one. Two more,
// `--series-1` and `--series-2`, had a light value that had been stepped for a
// chart's 3:1 and never for a sentence's 4.5:1. Measured on 2026-08-29 against
// the light `--page`:
//
//	--text-muted     #898781  3.41:1   the ink of ~325 places
//	--status-warning #fab219  1.74:1
//	--series-2       #eb6834  3.04:1
//	--status-good    #0ca30c  3.18:1
//	--status-serious #ec835a  2.50:1
//	--series-1       #2a78d6  4.19:1
//
// **The dark theme was fine, which is exactly why this lasted.** Every one of
// those clears AA against `#0d0d0d`; a developer working at night sees a
// perfectly legible app. And the light theme's failure never presents as a
// failure -- nothing throws, nothing looks broken, the words are simply
// fainter than a person with ordinary eyes on an ordinary laptop in ordinary
// daylight can comfortably read. Commandment 2 is the one this costs: the
// muted ink is what captions carry, and a caption is where a newcomer is told
// what they are looking at.
//
// The stylesheet had even *found* it and worked around it three times --
// `.tarot-table-felt`, `.seance-note` and `.token-shop-note` each re-scope or
// avoid `--text-muted` locally, and one of them quotes the 3.41 exactly. Three
// local patches and an unmoved token is a design telling you where it hurts.
// A comment saying "measure this" would have been a fourth patch. This is the
// thing that measures it.
//
// # Why this lives in Go and reads the bundle
//
// Two reasons, both learned here the hard way.
//
// **The web suite cannot see a stylesheet.** `index.css?raw` resolves to the
// empty string under vitest, so a guard written over there reads nothing,
// finds nothing wrong, and passes -- the worst possible failure for a gate,
// because it is indistinguishable from success. jsdom has no layout and no
// cascade either, so even a mounted component could not be asked what colour
// it came out.
//
// **What a browser applies is the artifact**, which is `cardimagery_test.go`'s
// argument one facet over. Reading `web/src/index.css` would miss a token that
// arrives through Tailwind or a dependency, and would read a source the
// deployed app does not serve. Reading `web_dist/` reads the thing the
// deployed app serves, and CI already proves that bundle rebuilds from source.
//
// # What it checks
//
// Three things, and the second is the one that will earn its keep.
//
//  1. **Every ink clears 4.5:1 in its own theme**, against the worse of
//     `--page` and `--surface-1` -- because text sits on cards as often as on
//     the page, and in the dark theme a card is the *lighter* of the two, so
//     the page alone is not the worst case. That distinction is not academic:
//     dark `--series-2` measured 4.50 against the page and 4.48 against a
//     card.
//
//  2. **Every token in the palette must be classified here**, and an unknown
//     one fails. This is `configrecord_test.go`'s trick -- hold the list equal
//     to the code in both directions -- and it is what stops the exemptions
//     below from rotting into a list nobody re-reads. A new token cannot be
//     added to the palette without somebody deciding, in writing, whether
//     words will ever be made of it.
//
//  3. **The two dark blocks say the same thing.** The dark column is written
//     twice, once under `@media (prefers-color-scheme: dark)` for a reader who
//     has never touched the toggle and once under `[data-theme='dark']` for
//     one who has. They had already drifted: `--heat-ink` and `--heat-on` were
//     corrected only in the toggle's copy, so the castability grid drew itself
//     one way for a reader who picked dark and another way for a reader whose
//     machine picked it for them. Nobody could report that, because no single
//     reader ever saw both.
//
// # What it does not check, said plainly
//
// **Text on a tint is a different sum and this does not do it.** A `Badge`
// draws its ink on `color-mix(in srgb, <the same token> 16%, transparent)`,
// which is neither the page nor a card; measured by hand, the `good` and
// `critical` tones come out at 3.66 and 3.64 even with the inks fixed, because
// the tint pulls the ground toward the ink. That is a component's arithmetic
// rather than a token's and it wants a different fix (the `warning` tone
// already took it: `--text-primary` on the tint). Recorded here so the next
// reader knows this gate's green does not cover it.
//
// **The usage scan under-reports and is a floor, not a census.** It finds
// `color: var(--x)` in the stylesheet and `color:"var(--x)"` in the scripts,
// which is how the great majority of this app writes a text colour. It cannot
// follow an indirection -- `deckedit.tsx` keeps a `STRENGTH_COLOUR` map and
// hands the lookup to `style`, so `--status-serious` is text and no pattern
// adjacent to `color:` says so. That one is classified `ink` from a reading
// rather than from the scan. The scan's job is the other direction: to catch
// the day somebody writes words in a colour classified as a wash.
func TestThePaletteIsLegibleInTheThemeItIsReadIn(t *testing.T) {
	t.Parallel()

	css := bundleStylesheet(t)
	light := paletteBlock(t, css, rootLightPalette, "the light `:root` palette")
	darkMedia := paletteBlock(t, css, rootDarkMediaPalette,
		"the `@media (prefers-color-scheme: dark)` palette")
	darkToggle := paletteBlock(t, css, rootDarkTogglePalette,
		"the `:root[data-theme='dark']` palette")

	// (3) The two dark columns are one column written twice.
	for _, name := range sortedPaletteKeys(darkMedia, darkToggle) {
		media, inMedia := darkMedia[name]
		toggle, inToggle := darkToggle[name]
		switch {
		case !inMedia:
			t.Errorf("%s is set to %s under [data-theme='dark'] and not set at "+
				"all under @media (prefers-color-scheme: dark).\n\nThe dark "+
				"column is written twice on purpose -- once for a reader who "+
				"picked dark and once for a reader whose machine picked it -- "+
				"and a token in only one copy renders differently for the two "+
				"of them. Add it to both blocks in web/src/index.css and "+
				"rebuild web_dist/.", name, toggle)
		case !inToggle:
			t.Errorf("%s is set to %s under @media (prefers-color-scheme: dark) "+
				"and not set at all under [data-theme='dark'].\n\nThe theme "+
				"toggle would then fail to reach it: a reader who explicitly "+
				"picks dark gets the light theme's value for this one token. "+
				"Add it to both blocks in web/src/index.css and rebuild "+
				"web_dist/.", name, media)
		case media != toggle:
			t.Errorf("%s is %s under @media (prefers-color-scheme: dark) but %s "+
				"under [data-theme='dark']. The two dark blocks must agree; "+
				"they are the same theme reached two ways.", name, media, toggle)
		}
	}

	// (2) Every token in the palette carries a decision. Once, over the union
	// of both columns rather than once per theme: an unclassified token is a
	// fact about this list, not about a page it renders on.
	for _, name := range sortedPaletteKeys(light, darkToggle) {
		if _, known := paletteRoles[name]; known {
			continue
		}
		t.Errorf("%s is in the palette and this test does not classify it.\n\n"+
			"Every token here needs a decision in writing: will words ever be "+
			"made of it? Add it to paletteRoles -- `ink` if a sentence is ever "+
			"drawn in it, `wash` if it is only ever a fill, a rule or a swatch, "+
			"`groundRole` if it is a ground other things are measured against -- "+
			"and make the reason an argument rather than a shrug. An "+
			"unclassified token fails on purpose, so that the exemptions in "+
			"that list cannot quietly grow one nobody re-read.", name)
	}

	// (1), per theme. The dark theme is the light one with the dark column laid
	// over it, which is what a browser does.
	for _, theme := range []struct {
		name   string
		tokens map[string]string
	}{
		{"light", light},
		{"dark", overlay(light, darkToggle)},
	} {
		page, ok := theme.tokens["--page"]
		if !ok {
			t.Fatalf("the %s palette declares no --page; there is nothing to "+
				"measure against and this check is measuring nothing", theme.name)
		}
		card, ok := theme.tokens["--surface-1"]
		if !ok {
			t.Fatalf("the %s palette declares no --surface-1", theme.name)
		}

		for _, name := range sortedPaletteKeys(theme.tokens) {
			if strings.HasPrefix(name, "--lightningcss-") {
				continue // the bundler's own light/dark switch, never a colour
			}
			role, known := paletteRoles[name]
			if !known {
				continue // already reported once, above
			}
			floor := role.floor
			if floor == 0 {
				continue
			}
			for _, ground := range []struct {
				what string
				hex  string
			}{{"--page", page}, {"--surface-1", card}} {
				got, err := contrast(theme.tokens[name], ground.hex)
				if err != nil {
					// A token that is not a plain colour (a gradient, a
					// reference) cannot be measured and must not be classified
					// as if it could.
					t.Errorf("%s in the %s theme is %q, which is not a colour "+
						"this test can measure, but it is classified %s: %v",
						name, theme.name, theme.tokens[name], role.what, err)
					break
				}
				if got < floor {
					t.Errorf("%s is %s in the %s theme and measures %.2f:1 "+
						"against %s (%s) -- under the %.1f:1 this token needs "+
						"as %s.\n\nWhy: %s\n\nFix it in the palette at the head "+
						"of web/src/index.css, in that theme's column only, and "+
						"rebuild web_dist/. Darken (or lighten) along the "+
						"token's own hue by the least that clears the bar; a "+
						"muted grey that becomes body-black has destroyed the "+
						"hierarchy it exists to express.",
						name, theme.tokens[name], theme.name, got, ground.what,
						ground.hex, floor, role.what, role.why)
				}
			}
		}
	}

	// (2), the other direction: a token classified below ink, found being used
	// as one. See the caveat in the doc comment -- this is a floor, not a
	// census, and it is here to catch the new mistake rather than to certify
	// the old ones.
	inked := textTokens(t, css, bundleScripts(t))
	if len(inked) == 0 {
		t.Fatal("found no `color: var(--token)` anywhere in the bundle; the " +
			"bundle's shape has moved and this half is measuring nothing")
	}
	for _, name := range inked {
		role, known := paletteRoles[name]
		if !known || role.floor >= 4.5 {
			continue
		}
		// A ground drawn as text is a chip: `.help-pip[aria-expanded='true']`
		// fills its mark with `--vine` and inks it in `--page`. The ground for
		// that sum is the vine, not the page, so measuring it here would be
		// answering a question nobody asked -- and answering it wrong.
		if role.ground {
			continue
		}
		t.Errorf("%s is classified %s (%s) and the bundle draws text in it.\n\n"+
			"Either it is an ink and must be measured as one -- give it a 4.5 "+
			"floor in paletteRoles and step its value in whichever theme then "+
			"fails -- or the text is wrong and wants a token that was chosen "+
			"to be read.", name, role.what, role.why)
	}
}

// paletteRole is what a token is for, and therefore what it has to clear.
//
// The vocabulary is deliberately small: a token here is an *ink*, a *ground*,
// or a *wash*. A fourth class was drafted -- "a mark", held to WCAG's non-text
// 3:1 -- and dropped for having no members: every non-text token in this
// palette turned out to be a ground, or one of the game's own colours, or a
// chart slot with an argued mitigation of its own. Add it back the day
// something is genuinely a standalone mark; the unclassified-token failure
// will make somebody decide.
type paletteRole struct {
	what string
	// floor is the ratio this token must clear against both its theme's --page
	// and its --surface-1. Zero means "not measured against a palette ground",
	// and then `why` has to carry an argument rather than a shrug.
	floor float64
	// ground marks the two tokens everything else is measured *against*. They
	// are also drawn as ink now and then -- a filled chip inks itself in the
	// page colour -- and the usage scan skips them for that reason.
	ground bool
	why    string
}

func ink(why string) paletteRole  { return paletteRole{what: "an ink", floor: 4.5, why: why} }
func wash(why string) paletteRole { return paletteRole{what: "a wash", why: why} }
func groundRole(why string) paletteRole {
	return paletteRole{what: "a ground", ground: true, why: why}
}

// paletteRoles is the decision, in writing, for every token in the palette.
// The test fails on a token that is not here, which is the whole point: the
// list cannot fall behind the stylesheet without going red.
var paletteRoles = map[string]paletteRole{
	"--page":      groundRole("the ground everything else is measured against"),
	"--surface-1": groundRole("a card's ground, and the second floor every ink clears"),

	"--text-primary":   ink("body text and every headline"),
	"--text-secondary": ink("captions and the qualifying line under a figure"),
	"--text-muted":     ink("the quietest text there is, in ~325 places"),

	"--gridline": wash("a chart's gridlines and a card's rule. WCAG exempts the " +
		"decorative, and a gridline legible at 3:1 is a gridline competing with " +
		"its own data. The values it separates are what must be read."),
	"--baseline": wash("a chart's zero line, for --gridline's reason"),
	"--hairline": wash("a 10% edge on a surface; decoration by construction"),

	"--series-1": ink("a chart slot, and links and figures in eighteen places"),
	"--series-2": ink("a chart slot, and the accent on the dossier and research rooms"),
	// Slots 3, 4 and 5 sit below 3:1 in the light theme and that is an argued
	// decision rather than an oversight -- the stylesheet's own head comment
	// makes it: *"light-mode aqua and yellow sit below 3:1, so every chart
	// using them ships direct labels and a table view."* A categorical palette
	// is separated by hue and by the labels beside it, and dragging five hues
	// up to 3:1 against paper collapses them toward each other, which costs the
	// colourblind reader the separation the labels were protecting. Nothing
	// writes words in them; the scan above is what holds that true.
	"--series-3": wash("a chart slot below 3:1 by argument, with direct labels " +
		"and a table view carrying the reading. See the head of index.css."),
	"--series-4": wash("a chart slot below 3:1 by argument, with direct labels " +
		"and a table view carrying the reading. See the head of index.css."),
	"--series-5": wash("a chart slot below 3:1 by argument, with direct labels " +
		"and a table view carrying the reading. See the head of index.css."),

	"--seq-200": wash("a step in the sequential ramp; a bar fill only"),
	"--seq-400": wash("a step in the sequential ramp; a bar fill only"),
	"--seq-450": wash("a step in the sequential ramp; a bar fill only"),
	"--seq-600": wash("a step in the sequential ramp; a bar fill only"),

	"--heat-ink": wash("the castability wash's hue, never drawn above 62% and " +
		"never carrying a value on its own -- every cell prints its own number " +
		"over it in --text-primary or --heat-on"),
	"--heat-on": wash("the ink for a cell whose wash swallowed body text; it is " +
		"measured against that cell's wash, which is not a palette ground"),

	"--status-good":     ink("a stat tile's figure, and the deck page's all-clear"),
	"--status-warning":  ink("eighteen warnings, and the clock mark on the tape"),
	"--status-serious":  ink("a charge's strength at 10px, via deckedit's STRENGTH_COLOUR"),
	"--status-critical": ink("twenty error lines"),

	// Magic's own five colours plus colourless. These are washes by definition
	// -- white mana is #f8f6d8, which is 1.04:1 against paper because it is
	// *white* -- and the stylesheet already records what happens when one is
	// used as type: *"a route once shipped a whole page of muted text rendering
	// pure white because a colour that was never meant to be type was used as
	// type"* (the argument is at `.guild-creed`). A pip built from one carries
	// its meaning in its ring and its glyph; the wash is the light behind them.
	"--mtg-w": wash("a mana pip's wash; the game's own fixed semantics, never type"),
	"--mtg-u": wash("a mana pip's wash; the game's own fixed semantics, never type"),
	"--mtg-b": wash("a mana pip's wash; the game's own fixed semantics, never type"),
	"--mtg-r": wash("a mana pip's wash; the game's own fixed semantics, never type"),
	"--mtg-g": wash("a mana pip's wash; the game's own fixed semantics, never type"),
	"--mtg-c": wash("a mana pip's wash; the game's own fixed semantics, never type"),

	"--vine": ink("the forest's green: the help pip's mark, and its focus ring"),
}

// The three blocks the palette is written in. Each is matched by its selector
// *and* by declaring --page: the bundle carries dozens of other `:root` and
// `[data-theme=dark]` blocks (the arena's palette, the trench's, the seance's)
// and only one of each is this one.
var (
	rootLightPalette      = regexp.MustCompile(`(?s):root\{([^}]*?--page:[^}]*)\}`)
	rootDarkMediaPalette  = regexp.MustCompile(`(?s)prefers-color-scheme:dark\)\{:root:where\(:not\(\[data-theme=light\]\)\)\{([^}]*?--page:[^}]*)\}`)
	rootDarkTogglePalette = regexp.MustCompile(`(?s):root\[data-theme=dark\]\{([^}]*?--page:[^}]*)\}`)
)

var declRe = regexp.MustCompile(`(--[a-z0-9-]+)\s*:\s*([^;}]*)`)

// paletteBlock is one palette block's declarations. It fatals rather than
// returning empty: a block this test cannot find is a block it is not
// checking, and a silent skip is how a guard comes to certify nothing.
func paletteBlock(t *testing.T, css string, re *regexp.Regexp, what string) map[string]string {
	t.Helper()
	m := re.FindStringSubmatch(css)
	if m == nil {
		t.Fatalf("cannot find %s in the committed bundle. The stylesheet's "+
			"shape has moved and this check is measuring nothing -- re-read "+
			"web_dist/assets/index.css and fix the pattern here rather than "+
			"letting it pass by finding nothing.", what)
	}
	out := map[string]string{}
	for _, d := range declRe.FindAllStringSubmatch(m[1], -1) {
		out[d[1]] = strings.TrimSpace(d[2])
	}
	if len(out) < 10 {
		t.Fatalf("%s parsed to only %d declarations; the parse is wrong", what, len(out))
	}
	return out
}

// overlay is what the cascade does: the dark column laid over the light one.
func overlay(base, over map[string]string) map[string]string {
	out := make(map[string]string, len(base))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range over {
		out[k] = v
	}
	return out
}

func sortedPaletteKeys(maps ...map[string]string) []string {
	seen := map[string]bool{}
	for _, m := range maps {
		for k := range m {
			if !strings.HasPrefix(k, "--lightningcss-") {
				seen[k] = true
			}
		}
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// textTokens is every palette token the bundle draws text in, from the
// stylesheet's `color:var(--x)` and the scripts' `color:"var(--x)"`. See the
// doc comment: a floor, not a census.
var (
	cssTextRe = regexp.MustCompile(`(?:^|[;{])color:\s*var\((--[a-z0-9-]+)`)
	jsTextRe  = regexp.MustCompile(`color:\s*"var\((--[a-z0-9-]+)`)
)

func textTokens(t *testing.T, css string, scripts map[string]string) []string {
	t.Helper()
	seen := map[string]bool{}
	for _, m := range cssTextRe.FindAllStringSubmatch(css, -1) {
		seen[m[1]] = true
	}
	for _, body := range scripts {
		for _, m := range jsTextRe.FindAllStringSubmatch(body, -1) {
			seen[m[1]] = true
		}
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// contrast is WCAG 2.x, verbatim: relative luminance from linearised sRGB, and
// the ratio of the lighter to the darker with the 0.05 flare term.
//
//	L = 0.2126 R + 0.7152 G + 0.0722 B
//	ratio = (Llighter + 0.05) / (Ldarker + 0.05)
func contrast(a, b string) (float64, error) {
	la, err := luminance(a)
	if err != nil {
		return 0, err
	}
	lb, err := luminance(b)
	if err != nil {
		return 0, err
	}
	hi, lo := math.Max(la, lb), math.Min(la, lb)
	return (hi + 0.05) / (lo + 0.05), nil
}

func luminance(hex string) (float64, error) {
	r, g, b, err := parseHex(hex)
	if err != nil {
		return 0, err
	}
	return 0.2126*channel(r) + 0.7152*channel(g) + 0.0722*channel(b), nil
}

func channel(v int) float64 {
	c := float64(v) / 255
	if c <= 0.04045 {
		return c / 12.92
	}
	return math.Pow((c+0.055)/1.055, 2.4)
}

// parseHex takes the three spellings the bundler emits: #rgb, #rrggbb, and
// #rrggbbaa (lightningcss rewrites `rgba(...)` into the last one). An alpha
// channel is refused rather than ignored -- a translucent ink's contrast
// depends on what is behind it, and answering as if it were opaque would be a
// confident wrong number, which is worse than no answer.
func parseHex(s string) (r, g, b int, err error) {
	s = strings.TrimSpace(strings.ToLower(s))
	if !strings.HasPrefix(s, "#") {
		return 0, 0, 0, fmt.Errorf("%q is not a hex colour", s)
	}
	h := s[1:]
	if len(h) == 3 {
		h = string([]byte{h[0], h[0], h[1], h[1], h[2], h[2]})
	}
	if len(h) == 8 {
		return 0, 0, 0, fmt.Errorf("%q carries an alpha channel; its contrast "+
			"depends on what is behind it", s)
	}
	if len(h) != 6 {
		return 0, 0, 0, fmt.Errorf("%q is not a hex colour", s)
	}
	if _, err := fmt.Sscanf(h, "%02x%02x%02x", &r, &g, &b); err != nil {
		return 0, 0, 0, fmt.Errorf("%q is not a hex colour: %w", s, err)
	}
	return r, g, b, nil
}

// The arithmetic is the gate's whole load-bearing half, so it is pinned to
// values computed by hand from the specification rather than trusted because
// the other test passes. Black on white is WCAG's own worked example (21:1);
// a colour against itself is 1:1 by construction; and the two real values this
// change turned on are here so that a wrong rewrite of the luminance curve
// cannot pass by moving both the gate and its evidence at once.
func TestTheContrastArithmeticMatchesTheSpecification(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		a, b string
		want float64
	}{
		{"black on white, WCAG's own example", "#000000", "#ffffff", 21.00},
		{"a colour against itself", "#2a78d6", "#2a78d6", 1.00},
		{"shorthand is the same colour", "#fff", "#ffffff", 1.00},
		{"the muted ink that failed, on the light page", "#898781", "#f9f9f7", 3.41},
		{"the muted ink that replaced it", "#73716c", "#f9f9f7", 4.62},
		{"the warning that could not be read", "#fab219", "#f9f9f7", 1.74},
		{"critical on the dark page, which also failed", "#d03b3b", "#0d0d0d", 4.05},
	} {
		got, err := contrast(tc.a, tc.b)
		if err != nil {
			t.Errorf("%s: %v", tc.name, err)
			continue
		}
		if math.Abs(got-tc.want) > 0.005 {
			t.Errorf("%s: %s on %s measured %.4f, want %.2f",
				tc.name, tc.a, tc.b, got, tc.want)
		}
	}

	// An alpha channel is refused, not guessed at.
	if _, err := contrast("#0b0b0b1a", "#f9f9f7"); err == nil {
		t.Error("a translucent colour was measured as if it were opaque; its " +
			"contrast depends on what is behind it and there is no honest " +
			"answer without that")
	}
}
