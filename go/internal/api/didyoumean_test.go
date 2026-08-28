package api

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// The import's shortlist -- the "did you mean" beside every name the paste
// could not resolve.
//
// **This is commandment 2 with a keyboard in front of it.** A newcomer's first
// contact with the app is very often a decklist pasted out of somewhere else,
// and the difference between "3 cards were not recognised" and "3 cards were
// not recognised; by `Cultvatr Colosus` did you mean Cultivator Colossus?" is
// the difference between fixing it and giving up. Nothing was watching it:
// `didYouMean` is reachable only through an import whose paste actually
// missed, and no test had written one.
//
// **Every spelling below was measured, not invented**, and the band is
// narrow, which is the interesting part. The importer resolves a name on its
// own once it is close enough and clearly ahead of the field, so a mild typo
// never reaches this tier at all -- `Cultivator Colossis` scores 0.9789 and is
// simply read as the card. What lands here is the middle: wrong enough not to
// be recalled, close enough to be worth offering. A test written from a
// plausible-looking typo tests the resolver instead and passes while proving
// nothing, which is what the first draft of this file did.

// pastedLines is a decklist body, escaped the way a JSON request carries one.
func pastedLines(lines ...string) string {
	return strings.ReplaceAll(strings.Join(lines, "\n"), "\n", "\\n")
}

// importing runs one paste through the route as a dry run -- the identical
// code path, writing nothing -- and hands back the decoded answer.
func importing(t *testing.T, rig *writeRig, text string) map[string]any {
	t.Helper()
	status, body, raw := rig.do(t, alice, "POST", "/api/decks/import",
		`{"slug":"guessed","commander":["Goreclaw, Terror of Qal Sisma"],`+
			`"dry_run":true,"text":"`+text+`"}`)
	if status != 200 {
		t.Fatalf("%d %s", status, raw)
	}
	return body
}

// shortlists reads `did_you_mean` into a written-name -> candidate-names map,
// checking the two things every entry owes on the way past.
func shortlists(t *testing.T, body map[string]any) map[string][]string {
	t.Helper()
	raw, err := json.Marshal(body["did_you_mean"])
	if err != nil {
		t.Fatal(err)
	}
	var list []suggestion
	if err := json.Unmarshal(raw, &list); err != nil {
		t.Fatalf("did_you_mean is not the recorded shape: %v (%s)", err, raw)
	}
	out := map[string][]string{}
	for _, s := range list {
		if len(s.Candidates) == 0 {
			t.Errorf("%q has an entry with nothing in it, which renders as a "+
				"heading with a hole under it", s.Written)
		}
		if len(s.Candidates) > 3 {
			t.Errorf("%q was offered %d names; three is the cut", s.Written, len(s.Candidates))
		}
		names := []string{}
		for _, c := range s.Candidates {
			names = append(names, c.Name)
			if c.Score < mentionFloor {
				t.Errorf("%q was offered %q at %v, below the floor of %v",
					s.Written, c.Name, c.Score, mentionFloor)
			}
			// Rounded to four places, because the score renders next to a card
			// name and `0.9183673469387755` is not a thing to show somebody
			// who is trying to fix a typo.
			if c.Score != roundTo4(c.Score) {
				t.Errorf("%q -> %q scored %v, which is not rounded to four places",
					s.Written, c.Name, c.Score)
			}
		}
		out[s.Written] = names
	}
	return out
}

// roundTo4 is the rounding the shortlist applies, spelled independently of it.
func roundTo4(f float64) float64 {
	var scaled float64
	if _, err := fmt.Sscanf(fmt.Sprintf("%.4f", f), "%g", &scaled); err != nil {
		return f
	}
	return scaled
}

// A name that is nearly a card gets that card offered back, beside the
// refusal rather than instead of it.
func TestAMisspeltCardIsOfferedTheOneItNearlyIs(t *testing.T) {
	t.Parallel()
	rig := newWriteRig(t, noCredential)
	defer rig.close()

	// Measured against the 21-card fixture: 0.9184, 0.9046 and 0.9308 -- all
	// under the bar the importer resolves at and over the bar this offers at.
	body := importing(t, rig, pastedLines(
		"1 Cultvatr Colosus",
		"1 Cratrhof Behemt",
		"1 Rystc Stdy",
	))
	// All three are still unknown. A shortlist is an offer, never a decision:
	// a name quietly resolved to a near miss would be the app choosing
	// somebody's card for them, which is the fault the whole tier exists
	// below.
	unknown, _ := body["unknown"].([]any)
	if len(unknown) != 3 {
		t.Fatalf("%d unknown names, want 3 -- these spellings are being "+
			"resolved rather than offered, so this proves nothing: %v",
			len(unknown), body["unknown"])
	}

	lists := shortlists(t, body)
	for written, want := range map[string]string{
		"Cultvatr Colosus": "Cultivator Colossus",
		"Cratrhof Behemt":  "Craterhoof Behemoth",
		"Rystc Stdy":       "Rhystic Study",
	} {
		got, present := lists[written]
		if !present {
			t.Errorf("%q was offered nothing; the shortlists were %v", written, lists)
			continue
		}
		if len(got) == 0 || got[0] != want {
			t.Errorf("%q was offered %v, want %q first", written, got, want)
		}
	}
}

// A name nothing resembles is **absent** from the shortlists rather than
// present with an empty list, and a name that is close but not close enough
// is treated the same way.
//
// The second is the one with a number behind it. `Swrds to Plowshars` scores
// 0.8993 against `Swords to Plowshares` -- seven thousandths under the floor,
// measured -- so it is the case that proves the floor is a floor rather than
// a decoration. If a similarity change ever moves that score over 0.90 this
// test fails, and the fix is a worse spelling rather than a lower floor.
func TestANameNothingIsCloseEnoughToIsOfferedNothingAtAll(t *testing.T) {
	t.Parallel()
	rig := newWriteRig(t, noCredential)
	defer rig.close()

	body := importing(t, rig, pastedLines(
		"1 Swrds to Plowshars", // 0.8993: under the floor
		"1 Qqqqzzz Wwwwvvv",    // nothing like anything in the pool
		"1 Rystc Stdy",         // 0.9308: over it, so the cut is a cut
	))
	if unknown, _ := body["unknown"].([]any); len(unknown) != 3 {
		t.Fatalf("%d unknown names, want 3: %v", len(unknown), body["unknown"])
	}
	lists := shortlists(t, body)
	for _, written := range []string{"Swrds to Plowshars", "Qqqqzzz Wwwwvvv"} {
		if got, present := lists[written]; present {
			t.Errorf("%q was offered %v; it is below the %v floor and should "+
				"have no entry at all", written, got, mentionFloor)
		}
	}
	if _, present := lists["Rystc Stdy"]; !present {
		t.Errorf("nothing was offered for any name, so the two absences above "+
			"prove nothing: %v", lists)
	}
}

// TestOnlyTheFirstTwelveMissesAreScanned is the cap, and the count that
// admits to it.
//
// A paste with two hundred unrecognised lines is somebody who pasted the
// wrong thing entirely, and running two hundred similarity scans over the
// pool to tell them so is work nobody wanted. The number beside the
// shortlists is what keeps the cap honest: a client can say "and 188 more we
// did not check" rather than implying the rest were fine.
func TestOnlyTheFirstTwelveMissesAreScanned(t *testing.T) {
	t.Parallel()
	rig := newWriteRig(t, noCredential)
	defer rig.close()

	// Under the cap: nothing is skipped.
	under := importing(t, rig, pastedLines(nonsense(5)...))
	if got := under["did_you_mean_skipped"]; got != float64(0) {
		t.Errorf("five misses skipped %v, want 0", got)
	}

	// Exactly at it is the boundary, and it is not skipped: the break is on
	// `i >= nearMisses`, so the twelfth is the last one scanned rather than
	// the first one dropped.
	exact := importing(t, rig, pastedLines(nonsense(nearMisses)...))
	if got := exact["did_you_mean_skipped"]; got != float64(0) {
		t.Errorf("exactly %d misses skipped %v, want 0", nearMisses, got)
	}

	// Over it: the excess is counted exactly.
	const misses = nearMisses + 7
	over := importing(t, rig, pastedLines(nonsense(misses)...))
	unknown, _ := over["unknown"].([]any)
	if len(unknown) != misses {
		t.Fatalf("%d unknown names, want %d -- the fixture is not what this "+
			"test thinks it is", len(unknown), misses)
	}
	if got := over["did_you_mean_skipped"]; got != float64(misses-nearMisses) {
		t.Errorf("%d misses skipped %v, want %d", misses, got, misses-nearMisses)
	}
}

// nonsense is n decklist lines naming nothing, each distinct so the importer
// counts them separately.
func nonsense(n int) []string {
	out := make([]string, 0, n)
	for i := range n {
		out = append(out, fmt.Sprintf("1 Zzqqx%c Vvwwyz", 'a'+rune(i)))
	}
	return out
}
