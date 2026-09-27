package main

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/claude"
)

// The spend posture record: every Claude mode is covered by a cache, an
// in-flight dedupe key, or a written argument for neither.
//
// That sentence is the polish pass's Black rule and until now it was enforced
// by a person re-deriving it once a week. The cost of its going stale is
// paid twice over. A mode with neither cover pays for the same question twice
// whenever two tabs ask it inside the minutes a search takes, and **nothing
// anywhere reports that** -- the ledger records tokens spent, not tokens that
// need not have been, so the only instrument is somebody reading ten route
// files and asking the question of each. It has already been missed once at
// the other end: ADR 41 landed three new modes in one PR, and each one's
// posture was verified by hand in a night run's prose rather than by anything
// that would have failed.
//
// So the record below is the claim, and the two halves of this guard are the
// two ways it rots:
//
//   - **A new mode with no posture recorded.** The completeness check is
//     derived from `claude.ModeNames()` in both directions, so a mode that
//     lands without an answer to this question fails by name rather than
//     going out uncovered.
//   - **A posture whose mechanism moved.** Each entry names the file that
//     carries the cover and a token that must still be in it -- a key
//     expression for the keyed modes, the table name for the cached one, and
//     for the three that deliberately have neither, the sentence that argues
//     it. A reflow of one of those arguments fails this test, and that is the
//     intended contract: the argument *is* the cover, so somebody rewriting
//     it is exactly who should be told the record exists.
//
// What this guard deliberately does NOT do is decide whether a posture is the
// right one. Two of the three arguments below record a measured "no" (the
// interview's seconds-class cost, the description's single call) and one
// records a design refusal (a theme proposal must be re-askable, so caching it
// would deny somebody the one thing they clicked for). Re-opening any of them
// is a ledger question with numbers attached, never a test's business.

// spendCover is the three ways a mode may be covered. Named constants rather
// than bare strings so a typo in the record is a compile error.
const (
	coverCache  = "cache"  // an answer stored and re-served
	coverKey    = "key"    // `jobs.Plan.Key`: two identical asks in flight are one job
	coverArgued = "argued" // neither, with the reason written at the call site
)

// spendPosture is one mode's cover and where to check it.
type spendPosture struct {
	cover string
	// file is repository-relative; anchor is a literal substring of it.
	file, anchor string
}

// spendPostures is the record. One entry per mode, and the completeness check
// below is what keeps that true.
var spendPostures = map[string]spendPosture{
	// The dossier is the only mode with both halves: a stored answer keyed on
	// the commander's oracle id, and the same key as the in-flight dedupe.
	claude.ModeCommanderDossier: {coverCache,
		"go/internal/api/dossier.go", "dossier_cache"},

	// Research: nothing cached (ADR 26), deduplicated in flight because the
	// question text is the whole input.
	claude.ModeResearch: {coverKey,
		"go/internal/api/research.go", "Key: plan.Key"},

	// The slot argument's sweep is keyed on the slug plus the selection, so a
	// double-click joins the sweep already running. Its single-card twin is
	// synchronous and argued in the same file.
	claude.ModeSlotArgument: {coverKey,
		"go/internal/api/argue.go", `key := slug + ":" + hex.EncodeToString`},

	// The scan is keyed on the image itself, so the same photograph submitted
	// twice is transcribed once.
	claude.ModeScan: {coverKey,
		"go/internal/api/scan.go", `key := "scan:" + hex.EncodeToString`},

	// Both intake modes ride one job, keyed on the slug plus the actions asked
	// for, so a resubmitted import joins the filing in flight.
	claude.ModeRationaleDraft: {coverKey,
		"go/internal/api/intake.go", "intakeKey(actions)"},
	claude.ModeIntakeFiling: {coverKey,
		"go/internal/api/intake.go", "intakeKey(actions)"},

	// The interview is the smallest Claude surface and deliberately has no
	// job, no cache and nothing stored -- a measured "no", not an omission.
	claude.ModeRationaleInterview: {coverArgued,
		"go/internal/api/interview.go", "no job, no cache, nothing stored"},

	// Both halves of the theme interview take an empty key on purpose -- two
	// turns in flight are two people's evenings -- and neither is cached,
	// because the moment somebody wants a different answer is the moment they
	// click again on an unchanged conversation.
	claude.ModeThemeConversation: {coverArgued,
		"go/internal/api/theme.go", "**Nothing is cached.**"},
	claude.ModeThemeProposal: {coverArgued,
		"go/internal/api/theme.go", "**Nothing is cached.**"},

	// The description is one call about a whole deck, on the interview's
	// measured argument.
	claude.ModeDeckDescription: {coverArgued,
		"go/internal/api/describe.go", "A plain route rather than a job"},
}

// TestEveryModeIsCoveredBySomething is the completeness half: the record and
// the mode table are the same set, in both directions.
func TestEveryModeIsCoveredBySomething(t *testing.T) {
	t.Parallel()
	recorded := make([]string, 0, len(spendPostures))
	for name := range spendPostures {
		recorded = append(recorded, name)
	}
	sort.Strings(recorded)
	defined := claude.ModeNames()
	if strings.Join(recorded, " ") != strings.Join(defined, " ") {
		t.Errorf("the spend posture record covers\n  %v\nand the mode table holds"+
			"\n  %v\n-- a mode with no recorded posture is a paid surface nobody "+
			"asked the caching question about, which is the one thing this record "+
			"exists to refuse. Add its entry above, or take out the one that "+
			"names a mode that is gone.", recorded, defined)
	}
}

// TestEveryRecordedPostureStillHasItsMechanism is the anchor half: the cover
// each entry claims is still in the file it names.
func TestEveryRecordedPostureStillHasItsMechanism(t *testing.T) {
	t.Parallel()
	root := repoRoot(t)
	for _, name := range claude.ModeNames() {
		posture, ok := spendPostures[name]
		if !ok {
			continue // the completeness test above owns this failure
		}
		switch posture.cover {
		case coverCache, coverKey, coverArgued:
		default:
			t.Errorf("mode %q records the cover %q, which is not one of %q, %q, %q",
				name, posture.cover, coverCache, coverKey, coverArgued)
			continue
		}
		raw, err := os.ReadFile(filepath.Join(root, posture.file))
		if err != nil {
			t.Errorf("mode %q's %s posture names %s, which is not in the tree: %v",
				name, posture.cover, posture.file, err)
			continue
		}
		if !strings.Contains(string(raw), posture.anchor) {
			t.Errorf("mode %q is recorded as covered by a %s, anchored on\n  %s\n"+
				"in %s -- and that text is no longer there. Either the cover moved "+
				"(update the anchor) or it is gone (this mode now pays twice for the "+
				"same question, and the record is the only thing that would have "+
				"said so).", name, posture.cover, posture.anchor, posture.file)
		}
	}
}
