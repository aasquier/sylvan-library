package main

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The two statues standing in the Coliseum's page margins, and the three
// promises about them that nothing else in this tree can see.
//
// The room's own alt text has said since the banner was built that Critchlow's
// arena is "crowned by two tall statues", and the page around it had none.
// Above 1280px the route's 1024-pixel column leaves a few hundred pixels of
// bare page on each side; two Roman Hercules now stand there, photographed by
// the Met and released CC0 (`web/src/assets/coliseum/custodes.recipe.yaml`).
//
// Three things about them are invisible from the page you are looking at, and
// each has already gone wrong somewhere else in this repository:
//
//  1. **A committed picture and the copy of it a browser fetches are two
//     files.** `mediaprovenance_test.go` exempts everything under `web_dist/`
//     from its accounting on the stated grounds that those are "build copies,
//     byte-identical to their accounted sources" — which is true, and until
//     now was an assumption rather than a check. For these two it is checked:
//     the bytes in the bundle are the bytes the recipe produced, so the
//     provenance entry is a record of what is actually served.
//
//  2. **A decoration that is 940 pixels tall must not reach a phone.** The
//     lane these stand in is the page's own margin, and below about 1280px
//     there is no margin — the column has eaten the page. The guard is not
//     "is there a media query" but the behavioural one: outside every media
//     block the statues *and the sentence that credits them* are both
//     `display: none`, and both come back inside one width-gated block, so
//     the two can never disagree about whether there is anything to credit.
//
//  3. **The dusk over them is a layer, not a filter.** These are not Wizards'
//     art and commandment 19 is not what forbids it here — `secutor.webp`
//     next door wears a `filter` legitimately. What forbids it is what the
//     layer is *for*: the sheet is masked to the plate's own alpha so the
//     umber lands on marble and not on the page around it, and a `filter` on
//     the plate would re-render the photograph instead of lying over it,
//     which is the reach ADR 48 spent a day undoing on fourteen surfaces.
//     A missing layer is not a violation, so the sweep in
//     `cardimagery_test.go` would say nothing at all about this one.
//
// All three read the **committed bundle** rather than `web/src`, in
// `cardimagery_test.go`'s idiom and for its reason: what a browser parses is
// the artifact. The minifier writes `::after` as `:after` and
// `(min-width: 80rem)` as `(width>=80rem)`, so every pattern below is matched
// against what the bundle actually says.

// custodes is the pair, by the basename the recipe gives each one.
var custodes = []string{"custos-iuvenis.webp", "custos-barbatus.webp"}

// TestTheArenasTwoGuardiansAreCommittedAndServed: promise 1.
func TestTheArenasTwoGuardiansAreCommittedAndServed(t *testing.T) {
	t.Parallel()
	root := repoRoot(t)
	recipe, err := os.ReadFile(filepath.Join(root, "web", "src", "assets",
		"coliseum", "custodes.recipe.yaml"))
	if err != nil {
		t.Fatalf("no recipe for the arena's guardians: %v — a committed asset "+
			"without one is what ADR 29 forbids", err)
	}
	scripts := bundleScripts(t)
	for _, name := range custodes {
		if !strings.Contains(string(recipe), "file: "+name) {
			t.Errorf("custodes.recipe.yaml does not name %s as a `file:` "+
				"output, so `animist verify` holds nothing to it", name)
		}
		source := filepath.Join(root, "web", "src", "assets", "coliseum", name)
		want, err := os.ReadFile(source)
		if err != nil {
			t.Errorf("%s: %v", source, err)
			continue
		}
		served := filepath.Join(root, "web_dist", "assets", name)
		got, err := os.ReadFile(served)
		if err != nil {
			t.Errorf("%s is committed but %s is not — the bundle is stale, "+
				"and the guardian the recipe accounts for is not the one a "+
				"browser would be handed: rebuild it", source, served)
			continue
		}
		if !bytes.Equal(want, got) {
			t.Errorf("%s and %s differ (%d bytes against %d). Every media file "+
				"under web_dist/ is exempt from mediaprovenance_test.go's "+
				"accounting *because* it is a byte-identical build copy of an "+
				"accounted source; a copy that has drifted is a served picture "+
				"with no record standing beside it.",
				source, served, len(want), len(got))
		}
		// A file in the directory the door serves is not yet a file any page
		// asks for. Vite rewrites the import to a path, and that path is what
		// must appear in the script that draws the room.
		referenced := false
		for _, body := range scripts {
			if strings.Contains(body, "/assets/"+name) {
				referenced = true
				break
			}
		}
		if !referenced {
			t.Errorf("no script in the committed bundle asks for /assets/%s: "+
				"the plate ships and nothing draws it", name)
		}
	}
}

// TestAGuardianStandsOnlyWhereThereIsAMarginToStandIn: promise 2.
func TestAGuardianStandsOnlyWhereThereIsAMarginToStandIn(t *testing.T) {
	t.Parallel()
	css := bundleStylesheet(t)
	if !strings.Contains(css, ".coliseum-custos") {
		t.Fatalf("the committed bundle has no `.coliseum-custos` rules at all " +
			"— either the guardians were taken out (delete this file with " +
			"them) or web_dist/ is stale; rebuild it.")
	}
	// Outside every media block, both the statues and their credit are gone.
	bare := withoutMediaBlocks(css)
	for _, subject := range []string{".coliseum-custos", ".coliseum-custos-credit"} {
		rules := rulesWithSubject(bare, subject)
		if len(rules) == 0 {
			t.Errorf("no unconditional rule in the bundle has `%s` as its own "+
				"subject, so nothing takes it away below the breakpoint: a "+
				"phone is asked to draw a 940-pixel plate into a margin that "+
				"does not exist there", subject)
			continue
		}
		for _, r := range rules {
			if !strings.Contains(r.body, "display:none") {
				t.Errorf("`%s` is drawn unconditionally (body: %s). The lane "+
					"these stand in is the page's own margin and below about "+
					"1280px there is none — the column has eaten the page.",
					subject, r.body)
			}
		}
	}
	// And both come back inside ONE width-gated block, which is what makes
	// "the credit cannot outlive the statues" a fact rather than a habit.
	together := 0
	for _, block := range widthGatedBlocks(css) {
		statues := rulesWithSubject(block, ".coliseum-custos")
		credit := rulesWithSubject(block, ".coliseum-custos-credit")
		posed := false
		for _, r := range statues {
			if strings.Contains(r.body, "position:absolute") {
				posed = true
			}
		}
		shown := false
		for _, r := range credit {
			if strings.Contains(r.body, "display:block") {
				shown = true
			}
		}
		if posed && shown {
			together++
		}
	}
	if together != 1 {
		t.Errorf("%d width-gated blocks in the bundle place the guardians AND "+
			"restore their credit, want exactly 1. Two blocks can disagree, "+
			"and none means either the statues stand at every width or the "+
			"page credits two figures nobody can see.", together)
	}
}

// TestTheDuskOnAGuardianIsASheetLaidOverThePlate: promise 3.
func TestTheDuskOnAGuardianIsASheetLaidOverThePlate(t *testing.T) {
	t.Parallel()
	css := bundleStylesheet(t)
	sheets := rulesWithSubject(css, ".coliseum-custos-figure:after")
	if len(sheets) == 0 {
		t.Fatalf("no `.coliseum-custos-figure::after` rule in the committed " +
			"bundle: the guardians take no light from the room they stand in, " +
			"and a museum-lit statue beside a banner travelling from noon to " +
			"midnight is a cut-out on a page. Either the dusk was removed " +
			"(delete this test with it) or web_dist/ is stale; rebuild it.")
	}
	lit, masked := false, false
	for _, r := range sheets {
		if strings.Contains(r.body, "mix-blend-mode:multiply") {
			lit = true
		}
		if strings.Contains(r.body, "mask-image:var(--custos-plate)") {
			masked = true
		}
	}
	if !lit {
		t.Errorf("the dusk sheet over a guardian does not multiply, so it is " +
			"paint rather than light: a flat wash over a photograph, not a " +
			"room getting dark around a statue in it.")
	}
	if !masked {
		t.Errorf("the dusk sheet is not masked to `--custos-plate`, so the " +
			"umber covers the whole lane instead of the marble in it — a " +
			"rectangle of weather in the page's margin. The component hands " +
			"the plate over twice for exactly this reason.")
	}
	// The plate itself is never re-rendered on its way to the screen.
	for _, r := range rulesWithSubject(css, ".coliseum-custos-art") {
		if strings.Contains(r.body, "filter:") {
			t.Errorf("`.coliseum-custos-art` declares a filter (body: %s). "+
				"The light on these two is a layer above them and masked to "+
				"their own alpha; a filter here re-renders the photograph "+
				"instead of lying over it, which is the obvious reach ADR 48 "+
				"exists to stop.", r.body)
		}
	}
}

// atBlock matches the head of any at-rule that carries a block of rules.
var atBlock = regexp.MustCompile(`@media[^{]*\{`)

// widthGated is the minifier's spelling as well as the author's: `esbuild`
// rewrites `(min-width: 80rem)` to `(width>=80rem)`, so a guard that only
// knew the source spelling would find nothing in the artifact.
var widthGated = regexp.MustCompile(`min-width:|width>=`)

// spanOf returns the text between the brace at `open` and its partner.
func spanOf(css string, open int) (string, int) {
	i, depth := open, 1
	for i < len(css) && depth > 0 {
		switch css[i] {
		case '{':
			depth++
		case '}':
			depth--
		}
		i++
	}
	return css[open : i-1], i
}

// withoutMediaBlocks is the stylesheet with every `@media` block cut out, so
// what is left is what a browser applies at every width there is.
func withoutMediaBlocks(css string) string {
	var out strings.Builder
	last := 0
	for _, m := range atBlock.FindAllStringIndex(css, -1) {
		if m[0] < last {
			continue // inside a block already skipped
		}
		out.WriteString(css[last:m[0]])
		_, end := spanOf(css, m[1])
		last = end
	}
	out.WriteString(css[last:])
	return out.String()
}

// widthGatedBlocks is the inside of every `@media` block whose condition names
// a width floor.
func widthGatedBlocks(css string) []string {
	var out []string
	for _, m := range atBlock.FindAllStringIndex(css, -1) {
		body, _ := spanOf(css, m[1])
		if widthGated.MatchString(css[m[0]:m[1]]) {
			out = append(out, body)
		}
	}
	return out
}

// rulesWithSubject is every rule in `css` one of whose comma-separated
// selectors is exactly `subject`.
//
// Exactly, because the looser test is the wrong one twice over: a substring
// match on `.coliseum-custos` also catches `.coliseum-custos-credit` and
// `:root[data-theme=light] .coliseum-custos:before`, and the claims above are
// about what happens to the element itself.
func rulesWithSubject(css, subject string) []rule {
	var out []rule
	for _, r := range legendlessRules(css) {
		for _, sel := range strings.Split(r.sel, ",") {
			if strings.TrimSpace(sel) == subject {
				out = append(out, r)
				break
			}
		}
	}
	return out
}
