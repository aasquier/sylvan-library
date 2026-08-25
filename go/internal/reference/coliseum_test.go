package reference_test

import (
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/reference"
)

// The coliseum's roster is checked-in prose, so these are gates on the prose
// rather than on code: the file is edited by hand and a hand can drop a field.
// What cannot be checked here is whether a card name resolves — that is the
// pool's answer at serve time, and the route drops and counts what it cannot
// find, exactly as `/api/colors/{key}` does.

func TestTheColiseumHasSixHousesAndNoDuplicates(t *testing.T) {
	t.Parallel()

	arenas := reference.Arenas()
	if len(arenas) != 6 {
		t.Fatalf("the coliseum holds %d arenas, want 6", len(arenas))
	}
	seen := map[string]bool{}
	for _, a := range arenas {
		if seen[a.Key] {
			t.Errorf("two arenas answer to %q", a.Key)
		}
		seen[a.Key] = true
		if _, ok := reference.ArenaByKey(a.Key); !ok {
			t.Errorf("%q is not reachable by key", a.Key)
		}
	}
}

func TestEveryArenaIsFurnished(t *testing.T) {
	t.Parallel()

	// A backdrop, a palette, a motion and a stable of champions. An arena
	// missing any of them renders as a hole rather than as an error, which is
	// the failure mode worth a gate.
	motions := map[string]bool{"sand": true, "banners": true, "stone": true,
		"wind": true, "oil": true, "water": true}

	for _, a := range reference.Arenas() {
		if strings.TrimSpace(a.Name) == "" || strings.TrimSpace(a.Card) == "" {
			t.Errorf("%s: no name or no backdrop card", a.Key)
		}
		if strings.TrimSpace(a.Plane) == "" {
			t.Errorf("%s: names no plane", a.Key)
		}
		if !motions[a.Motion] {
			t.Errorf("%s: motion %q has no stylesheet behind it", a.Key, a.Motion)
		}
		if !strings.HasPrefix(a.Palette.Ink, "#") || !strings.HasPrefix(a.Palette.Glow, "#") {
			t.Errorf("%s: palette %+v is not two colours", a.Key, a.Palette)
		}
		if len(a.Champions) < 3 {
			t.Errorf("%s: %d champions is not a stable", a.Key, len(a.Champions))
		}
		for _, ch := range a.Champions {
			if strings.TrimSpace(ch.Card) == "" {
				t.Errorf("%s: a champion with no card", a.Key)
			}
			if ch.Card != strings.TrimSpace(ch.Card) {
				t.Errorf("%s: %q is padded and will never resolve", a.Key, ch.Card)
			}
			// The role is what makes a champion a gladiator rather than a
			// thumbnail. A card with no role is a card with nothing to say.
			if strings.TrimSpace(ch.Role) == "" {
				t.Errorf("%s: %s has no role", a.Key, ch.Card)
			}
		}
	}
}

func TestEveryFactSaysWhatItsKindPromises(t *testing.T) {
	t.Parallel()

	// The kind is what the frontend renders by, so a fact whose fields do not
	// match its kind is a slide that comes out blank on one side. Boot already
	// refuses an unknown kind; this checks the fields behind it.
	for _, a := range reference.Arenas() {
		for i, f := range a.Facts {
			where := a.Key + " fact " + string(rune('A'+i%26))
			if !reference.FactKinds[f.Kind] {
				t.Errorf("%s: unknown kind %q", where, f.Kind)
				continue
			}
			switch f.Kind {
			case "paired":
				if strings.TrimSpace(f.Rome) == "" || strings.TrimSpace(f.Magic) == "" {
					t.Errorf("%s: a paired fact needs both halves", where)
				}
			case "magic":
				if strings.TrimSpace(f.Magic) == "" {
					t.Errorf("%s: a magic fact with no Magic in it", where)
				}
				if strings.TrimSpace(f.Rome) != "" {
					t.Errorf("%s: a magic fact carrying Roman text", where)
				}
			default: // roman, gladiator, coliseum
				if strings.TrimSpace(f.Rome) == "" {
					t.Errorf("%s: a %s fact with no text", where, f.Kind)
				}
				if strings.TrimSpace(f.Magic) != "" {
					t.Errorf("%s: a %s fact carrying Magic text", where, f.Kind)
				}
			}
			if f.Card != strings.TrimSpace(f.Card) {
				t.Errorf("%s: card %q is padded and will never resolve", where, f.Card)
			}
		}
	}
}

func TestNoArenaRepeatsItselfForMinutes(t *testing.T) {
	t.Parallel()

	// **The variety gate.** A Tier 3 match takes minutes, and this is what a
	// person looks at for those minutes: a thin arena is a loop somebody
	// notices. Ten facts is roughly five minutes at thirty seconds a slide,
	// and three kinds is enough that the shape of the slide changes before the
	// subject does.
	for _, a := range reference.Arenas() {
		if len(a.Facts) < 10 {
			t.Errorf("%s: %d facts will loop inside one match", a.Key, len(a.Facts))
		}
		kinds := map[string]bool{}
		for _, f := range a.Facts {
			kinds[f.Kind] = true
		}
		if len(kinds) < 3 {
			t.Errorf("%s: only %d kinds of fact (%v); the rotation reads as one shape",
				a.Key, len(kinds), keysOf(kinds))
		}
	}
}

func TestTheColiseumTeachesRomeAndMagicBoth(t *testing.T) {
	t.Parallel()

	// Across the whole coliseum every kind is actually used. A kind declared
	// and never written is a renderer branch nobody will ever see run.
	total := map[string]int{}
	for _, a := range reference.Arenas() {
		for _, f := range a.Facts {
			total[f.Kind]++
		}
	}
	for kind := range reference.FactKinds {
		if total[kind] == 0 {
			t.Errorf("no fact anywhere is of kind %q", kind)
		}
	}
	sum := 0
	for _, n := range total {
		sum += n
	}
	if sum < 60 {
		t.Errorf("the coliseum holds %d facts; six arenas want more than that", sum)
	}
	t.Logf("%d facts: %v", sum, total)
}

func keysOf(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
