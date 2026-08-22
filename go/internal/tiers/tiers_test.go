package tiers

import "testing"

// TestEveryTiersModelLandsOnItsOwnLabel is the test LabelFor's own comment
// promises, and it is not decoration.
//
// The family table is deliberately NOT derived from All, even though the three
// labels coincide today — a ledger row can hold a model that is no tier at all
// (a model served after a fallback, an older id, an A/B via the environment
// override), and resolving those through Get would label them with the DEFAULT
// TIER, which is how a screen ends up naming the wrong Claude with total
// confidence. Two independent tables that must agree need a test saying so, or
// the day a tier's model id changes the picker and the ledger disagree
// silently.
func TestEveryTiersModelLandsOnItsOwnLabel(t *testing.T) {
	for _, tier := range All {
		if got := LabelFor(tier.Model); got != tier.Label {
			t.Errorf("tier %q serves %q, which LabelFor calls %q -- the picker "+
				"and the ledger would name different Claudes",
				tier.Key, tier.Model, got)
		}
	}
}

// TestAnUnknownModelIsNamedWithoutGuessing pins the deliberately
// uninformative answer.
//
// "Another Claude" is paired with the unpriced counter beside it on the Admin
// page: both mean "this build does not know that model". A guess here would be
// worse than the shrug, because a wrong family name looks exactly like a right
// one.
func TestAnUnknownModelIsNamedWithoutGuessing(t *testing.T) {
	for _, unknown := range []string{
		"", "gpt-4", "claude", "claude-", "Claude-Opus-5",
		"anthropic.claude-opus-5", "claude-opus", "opus-5",
	} {
		if got := LabelFor(unknown); got != "Another Claude" {
			t.Errorf("LabelFor(%q) = %q, want \"Another Claude\" -- an id from "+
				"no known family must not be guessed at", unknown, got)
		}
	}
}

// TestTheAggregatedMarkerIsNamedSeveral covers the one input that is not a
// model id at all. A roll-up writes "(various)" into the column it aggregated
// away; rendering that through the family table would call it "Another
// Claude", which reads as an unrecognised model rather than as several.
func TestTheAggregatedMarkerIsNamedSeveral(t *testing.T) {
	if got := LabelFor(Various); got != "Several" {
		t.Errorf("LabelFor(%q) = %q, want \"Several\"", Various, got)
	}
}

// TestNoLabelIsAModelId is commandment 10 at the one place it was still being
// broken: the Admin ledger used to render `claude-sonnet-5` in its "Answered
// by" column, which is a model id, which is technology.
func TestNoLabelIsAModelId(t *testing.T) {
	labels := []string{LabelFor(Various), LabelFor("nothing-known")}
	for _, tier := range All {
		labels = append(labels, LabelFor(tier.Model), tier.Label)
	}
	for _, label := range labels {
		for _, family := range families {
			if len(label) >= len(family.Prefix) && label[:len(family.Prefix)] == family.Prefix {
				t.Errorf("%q is a model id on a screen", label)
			}
		}
	}
	// And the roster, which is what the picker actually renders.
	for _, entry := range Roster() {
		if entry.Label == "" || entry.Blurb == "" {
			t.Errorf("tier %q renders with no label or blurb", entry.Key)
		}
	}
}

// TestTheDefaultTierIsAlwaysResolvable is Python's module-level assertion:
// `resolve` returning nothing for a key it was told is always valid would be a
// very quiet bug.
func TestTheDefaultTierIsAlwaysResolvable(t *testing.T) {
	if _, ok := byKey[DefaultKey]; !ok {
		t.Fatalf("the default tier %q is not in the table", DefaultKey)
	}
	// An unknown key READS as the default rather than erroring — the column is
	// data on a volume that outlives any deploy, so a key this build no longer
	// knows about will happen.
	for _, unknown := range []string{"", "nope", "OPUS", "sonnet "} {
		if got := Get(unknown).Key; got != DefaultKey {
			t.Errorf("Get(%q) = %q, want the default %q", unknown, got, DefaultKey)
		}
		// Writing one, though, must be refused, or the Admin page's next
		// release quietly grants a tier that does not exist.
		if Known(unknown) {
			t.Errorf("Known(%q) is true; a write path would accept it", unknown)
		}
	}
	for _, tier := range All {
		if !Known(tier.Key) {
			t.Errorf("Known(%q) is false for a real tier", tier.Key)
		}
	}
}
