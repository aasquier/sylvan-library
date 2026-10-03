package tiers

import "testing"

// The guard over the roster, shown refusing.
//
// [Get] answers the default for anything it does not recognise, so a roster
// that does not contain [DefaultKey] would hand every unrecognised key — and
// every account whose column is NULL, which is most of them — a zero [Tier]:
// no label, no blurb, and an empty model id travelling onward as if somebody
// had chosen it. That is the quietest bug this package could have, and
// [indexTiers] refuses to build such a table at all.
//
// It is a guard rather than a path (docs/polish/COVERAGE.md, lever 10), so what
// is asserted is the refusal: a roster without the default panics, and one with
// it keys every entry.
func TestARosterWithoutTheDefaultTierIsRefusedRatherThanIndexed(t *testing.T) {
	t.Parallel()
	defer func() {
		if recover() == nil {
			t.Error("a roster with no default tier was indexed happily, " +
				"which would hand every unnamed account a tier with no model")
		}
	}()
	indexTiers([]Tier{{Key: DefaultKey + "-but-not-quite", Label: "Nearly"}})
}

// And the roster the package actually ships is keyed by key, every entry
// reachable — the half of [indexTiers] that runs at boot.
func TestTheRealRosterIsKeyedByEveryKeyItHolds(t *testing.T) {
	t.Parallel()
	keyed := indexTiers(All)
	if len(keyed) != len(All) {
		t.Fatalf("%d tiers keyed out of %d -- two share a key", len(keyed), len(All))
	}
	for _, want := range All {
		got, ok := keyed[want.Key]
		if !ok {
			t.Errorf("%q is in the roster and not in the index", want.Key)
			continue
		}
		if got != want {
			t.Errorf("%q indexed as %+v", want.Key, got)
		}
	}
	if _, ok := keyed[DefaultKey]; !ok {
		t.Errorf("the default tier %q is not in the roster", DefaultKey)
	}
}
