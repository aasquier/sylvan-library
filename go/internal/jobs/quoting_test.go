package jobs

import (
	"strings"
	"testing"
	"time"
)

// The refusal a person reads, and the two guards behind the registry's own
// bookkeeping.
//
// `quoted` is the shape the served message has always used and it is not
// `strconv.Quote`: single quotes preferred, double ones only when the string
// holds a single quote and no double. Nothing was checking the escapes at
// all, which matters because the message is the *only* thing a caller sees
// when a planner names a lane this app does not have — a lane name carrying a
// newline would otherwise put a line break in the middle of a sentence and
// make the refusal read like two.

func TestAnUnknownLaneIsNamedInTheHouseQuotingRatherThanGosOwn(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct{ lane, want string }{
		"a plain name":                 {"ludicrous", "'ludicrous'"},
		"a name holding an apostrophe": {"the AI's lane", `"the AI's lane"`},
		"both quotes, so the single one wins and escapes": {
			`it's "fine"`, `'it\'s "fine"'`},
		"a backslash":       {`a\b`, `'a\\b'`},
		"a newline":         {"a\nb", `'a\nb'`},
		"a carriage return": {"a\rb", `'a\rb'`},
		"a tab":             {"a\tb", `'a\tb'`},
	} {
		if got := quoted(tc.lane); got != tc.want {
			t.Errorf("%s: quoted(%q) = %s, want %s", name, tc.lane, got, tc.want)
		}
	}

	// And the sentence it is for: the lane that was asked for, and the ones
	// that exist, all in one line a person can act on.
	err := &UnknownLaneError{Lane: Lane("a\nb")}
	message := err.Error()
	if strings.Count(message, "\n") != 0 {
		t.Fatalf("a lane name's newline reached the sentence raw: %q", message)
	}
	for _, lane := range Lanes {
		if !strings.Contains(message, quoted(string(lane))) {
			t.Errorf("the refusal does not offer %q: %s", lane, message)
		}
	}
}

// Two ids the same is a job silently overwriting another, so the registry
// re-rolls rather than trusting forty-eight bits. The loop is two lines and
// it had never run: a collision at that width would likely never fire on its
// own, which is precisely why nothing would notice it being wrong.
func TestATakenJobIdIsRolledAgainRatherThanOverwritingTheJobThatHasIt(t *testing.T) {
	t.Parallel()
	r := New(Config{})
	handed := []string{"aaaaaaaaaaaa", "aaaaaaaaaaaa", "bbbbbbbbbbbb"}
	next := 0
	r.newID = func() string {
		id := handed[min(next, len(handed)-1)]
		next++
		return id
	}
	first := r.Completed("kind", nil, "the first", 1)
	second := r.Completed("kind", nil, "the second", 1)
	if first.ID == second.ID {
		t.Fatalf("both jobs were filed under %s", first.ID)
	}
	if second.ID != "bbbbbbbbbbbb" {
		t.Fatalf("the re-roll landed on %s", second.ID)
	}
	if got := r.Get(first.ID, 1); got == nil || got.Label != "the first" {
		t.Fatalf("the first job was overwritten: %+v", got)
	}
}

// The registry is bounded, and when it is over the bound it evicts the
// oldest finished jobs. Two jobs filed in the same instant is the case the
// timestamp cannot settle, and the sequence number is what keeps the eviction
// order a fact rather than a map's whim — without it, which job survives a
// full registry is whatever Go's map iteration felt like that morning.
func TestTwoJobsFiledInTheSameInstantAreEvictedInTheOrderTheyArrived(t *testing.T) {
	t.Parallel()
	r := New(Config{Max: 2})
	frozen := time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)
	r.now = func() time.Time { return frozen }

	first := r.Completed("kind", nil, "the first", 1)
	second := r.Completed("kind", nil, "the second", 1)
	third := r.Completed("kind", nil, "the third", 1)
	if first.CreatedAt != second.CreatedAt || second.CreatedAt != third.CreatedAt {
		t.Fatal("the frozen clock did not freeze")
	}
	if r.Get(first.ID, 1) != nil {
		t.Fatal("the oldest job survived a registry over its bound")
	}
	for _, kept := range []*Job{second, third} {
		if r.Get(kept.ID, 1) == nil {
			t.Fatalf("%s was evicted ahead of an older job", kept.Label)
		}
	}
}
