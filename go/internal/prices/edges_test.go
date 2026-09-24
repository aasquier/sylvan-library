package prices

// Three places the roll-up meets something it cannot read.
//
// What binds them is the same rule: an estimate that is quietly wrong is
// worse than one that says it is short. A row with no conversation count
// still happened, so it counts as one rather than as none; a rate change
// whose date cannot be read is dropped from the split rather than guessed
// at, because guessing prices a window at the wrong rate and nothing fails.

import (
	"testing"
)

func TestARowWithNoConversationCountStillCountsAsOne(t *testing.T) {
	t.Parallel()
	// The ledger's older rows carry no count. A row that could not be priced
	// has to be reported as one unpriced conversation and not as zero --
	// zero would read as "nothing is missing from this total", which is the
	// one thing the caller most needs to know is false.
	got := Over([]Row{
		{Model: "a-model-nobody-prices", InputTokens: 1000, OutputTokens: 100},
	}, "2026-09-24")
	if got.Unpriced != 1 {
		t.Fatalf("an uncounted row reported %d unpriced conversations", got.Unpriced)
	}
	if got.USD != 0 {
		t.Fatalf("an unpriced row contributed %v", got.USD)
	}
	if len(got.UnpricedModels) != 1 || got.UnpricedModels[0] != "a-model-nobody-prices" {
		t.Fatalf("unpriced models %v", got.UnpricedModels)
	}
	// A row that carries its own count keeps it, so the fill-in above is the
	// zero case and not a cap.
	many := Over([]Row{
		{Model: "a-model-nobody-prices", Conversations: 9, InputTokens: 1000},
	}, "2026-09-24")
	if many.Unpriced != 9 {
		t.Fatalf("nine conversations reported as %d", many.Unpriced)
	}
}

func TestARateChangeWithNoReadableDateIsDroppedRatherThanGuessedAt(t *testing.T) {
	t.Parallel()
	// A boundary is the first day a new rate applies, derived from the last
	// day the old one did. An `Until` that is not a date cannot be turned
	// into one -- and inventing a changeover would price a whole window at
	// the wrong rate with nothing failing. `Priced.On` still compares it as
	// text, so the table keeps working; only the split is lost.
	rate := Rate{Input: 1, Output: 2}
	then := Rate{Input: 3, Output: 4}
	got := boundariesIn(map[string]Priced{
		"readable":      {Rate: rate, Then: &then, Until: "2026-08-31"},
		"unreadable":    {Rate: rate, Then: &then, Until: "the end of August"},
		"no change":     {Rate: rate},
		"half a change": {Rate: rate, Until: "2026-08-31"},
	})
	if len(got) != 1 || got[0] != "2026-09-01" {
		t.Fatalf("boundaries %v, want just the day after the readable one", got)
	}
	// The committed table is the real question, and it has to produce at
	// least one boundary or the split above is being tested against nothing.
	if len(Boundaries()) == 0 {
		t.Fatal("the committed table produced no boundaries at all")
	}
	for _, b := range Boundaries() {
		if len(b) != len("2026-09-01") {
			t.Fatalf("the committed table produced the boundary %q", b)
		}
	}
}

func TestADateThatCannotBeReadIsHandedBackUnchanged(t *testing.T) {
	t.Parallel()
	// The segment's `When` is a date inside the range, picked as the day
	// before the next change. Handed something that is not a date it returns
	// it as it stands: the rate lookup compares dates as text, so an
	// unreadable one simply picks the earlier rate rather than crashing a
	// roll-up somebody is waiting on.
	if got := dayBefore("2026-09-01"); got != "2026-08-31" {
		t.Fatalf("the day before 2026-09-01 is %q", got)
	}
	for _, in := range []string{"", "the end of August", "2026-13-45"} {
		if got := dayBefore(in); got != in {
			t.Fatalf("dayBefore(%q) = %q", in, got)
		}
	}
}
