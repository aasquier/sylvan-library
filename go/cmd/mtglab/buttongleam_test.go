package main

import (
	"regexp"
	"strings"
	"testing"
)

// The gleam that walks a button's rim, held to its three promises.
//
// `.btn`'s family got a travelling edge light on 2026-09-26: a short arc of a
// conic gradient masked to the border ring, turned by a registered `<angle>`
// (`--btn-gleam`). The mechanism and the material table live in one comment
// above `.btn-felt` in `web/src/index.css`; this file pins the three parts of
// it that a person cannot see by looking at the page they are on.
//
// **Why a check of its own, when `reducedmotion_test.go` already sweeps every
// animation in the bundle.** That sweep calls a rule covered when any class in
// its selector appears inside a `prefers-reduced-motion: reduce` block — and
// `.btn` appears in one, several hundred lines away, where it turns off
// *transitions*. So an unarrested `animation` on `.btn::before` reads as
// covered by a guard that never touches it. Proven rather than assumed: the
// reduced-motion rule below was deleted, the bundle rebuilt, and
// `TestEveryAnimationInTheBundleCanBeArrested` stayed green. That is the sweep
// working exactly as its own doc comment says it does — "a tripwire, not a
// rendering engine" — and it is why this promise needs naming.
//
// All three read the **committed bundle** rather than `web/src`, in
// `cardimagery_test.go`'s idiom and for its reason: what a browser parses is
// the artifact. The minifier writes `::before` as `:before` and folds every
// `color-mix()` to a hex, so the patterns below are matched against what the
// bundle actually says, not against what the source does.

// gleamRing is the ring rule itself: the pseudo-element that carries the
// masked conic gradient. Anchored on the registered angle's `var()` because
// that is the one token nothing else in the sheet uses.
var gleamRing = regexp.MustCompile(`conic-gradient\(from var\(--btn-gleam,\s*34deg\)`)

// gleamArrested is the reduced-motion rule: `animation-name` on its own, on a
// selector naming both `.btn` and the pseudo-element. The longhand is
// deliberate — the hover rule raises `animation-duration` and
// `animation-play-state` at a higher specificity, and a shorthand here would
// lose that argument.
var gleamArrested = regexp.MustCompile(
	`@media \(prefers-reduced-motion:\s*reduce\)\{[^{}]*\.btn[^{}]*:before[^{}]*\{[^{}]*animation-name:\s*none`)

// TestTheGleamStopsWalkingUnderReducedMotion. The light may not keep orbiting
// for somebody who asked the room to hold still, and it may not go out either:
// `--btn-gleam` is registered with an initial value, so an arrested lap parks
// on the same shoulder on every control rather than wherever it happened to
// be. A frozen mid-sweep is a smudge; an arc at rest is a highlight.
func TestTheGleamStopsWalkingUnderReducedMotion(t *testing.T) {
	t.Parallel()
	css := bundleStylesheet(t)
	if !gleamRing.MatchString(css) {
		t.Fatalf("the committed bundle has no button gleam to check — no " +
			"`conic-gradient(from var(--btn-gleam, 34deg)` anywhere in it. " +
			"Either the ring was removed (delete this file with it) or " +
			"web_dist/ is stale; rebuild it.")
	}
	if !gleamArrested.MatchString(css) {
		t.Errorf("the button gleam keeps walking under " +
			"`prefers-reduced-motion: reduce`. The guard is one rule beside " +
			"the material table in `web/src/index.css`, and it must set " +
			"`animation-name: none` (the longhand, not the shorthand — the " +
			"hover rule raises duration and play-state at a higher " +
			"specificity and would win the argument). Then rebuild web_dist/.")
	}
}

// TestADisabledControlDoesNotGleam. A `.btn:disabled` is already at 0.4
// opacity, and a light travelling around something that faded out reads as a
// control still working on something — which is the one thing a disabled
// control must never say to a newcomer (commandment 2).
func TestADisabledControlDoesNotGleam(t *testing.T) {
	t.Parallel()
	css := bundleStylesheet(t)
	if !gleamRing.MatchString(css) {
		t.Skip("no button gleam in the committed bundle; nothing to disable")
	}
	rule := regexp.MustCompile(`\.btn:disabled:before[^{}]*\{[^{}]*display:\s*none`)
	if !rule.MatchString(css) {
		t.Errorf("a disabled button still carries the gleam ring. " +
			"`.btn:disabled::before { display: none }` is what stops it, " +
			"beside the gleam's own rules in `web/src/index.css`. Then " +
			"rebuild web_dist/.")
	}
}

// TestTheGleamRegistersItsAngleAndStillDrawsWithoutIt. Two halves of one
// promise. The registration is what makes the arc turn at all — an unregistered
// custom property is not an animatable value — and the `var()` fallback is what
// a browser older than this site's Safari 16.4 floor draws instead: a static
// gleam on the resting shoulder rather than a rule that fails to parse and
// takes the whole ring with it. `browserfloor_test.go` owns the floor itself;
// this owns the graceful half below it.
func TestTheGleamRegistersItsAngleAndStillDrawsWithoutIt(t *testing.T) {
	t.Parallel()
	css := bundleStylesheet(t)
	if !gleamRing.MatchString(css) {
		t.Skip("no button gleam in the committed bundle")
	}
	registered := regexp.MustCompile(
		`@property --btn-gleam\{[^{}]*syntax:\s*"<angle>"[^{}]*initial-value:\s*34deg`)
	if !registered.MatchString(css) {
		t.Errorf("`--btn-gleam` is not registered as an `<angle>` with an " +
			"initial value of 34deg, so either the arc cannot animate or an " +
			"arrested one parks somewhere nobody chose. The `@property` block " +
			"sits above the gleam's rules in `web/src/index.css`.")
	}
	// The masked ring is a conic gradient laid over the whole face until the
	// two masks subtract the middle out of it, so the rules are inside an
	// `@supports` that asks for the composite by name. A bundle that lost the
	// guard would draw a gradient plate on every button on the oldest browsers
	// that reach us.
	if !strings.Contains(css, "@supports (mask-composite:exclude) or (-webkit-mask-composite:xor)") {
		t.Errorf("the gleam's rules are no longer behind an `@supports` for " +
			"`mask-composite`. Without the subtraction the pseudo-element is " +
			"a conic gradient over the button's whole face rather than a ring " +
			"around its edge.")
	}
}
