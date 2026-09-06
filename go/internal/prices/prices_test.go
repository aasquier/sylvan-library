package prices

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/aasquier/sylvan-library/go/internal/wire"
)

type pricesFile struct {
	Table map[string]struct {
		Input, Output float64
		ThenInput     *float64 `json:"then_input"`
		ThenOutput    *float64 `json:"then_output"`
		Until         string
	}
	CacheReadFraction float64 `json:"cache_read_fraction"`
	Cases             []struct {
		Name string
		When string
		Rows []struct {
			Model           string
			Conversations   int64
			InputTokens     int64 `json:"input_tokens"`
			OutputTokens    int64 `json:"output_tokens"`
			CacheReadTokens int64 `json:"cache_read_tokens"`
		}
		Rendered string
	}
}

func load(t *testing.T) pricesFile {
	t.Helper()
	raw, err := os.ReadFile("testdata/prices.json")
	if err != nil {
		t.Fatalf("prices.json: %v (a frozen golden; never regenerated)", err)
	}
	var fx pricesFile
	if err := json.Unmarshal(raw, &fx); err != nil {
		t.Fatal(err)
	}
	return fx
}

// TestTheTableMatchesTheRecordedRates holds `Table` to the corpus's copy —
// an edit to the table fails here until it is faced against the recorded
// rates, which is the whole reason the corpus records the table and not
// only the answers.
func TestTheTableMatchesTheRecordedRates(t *testing.T) {
	t.Parallel()
	fx := load(t)
	if len(fx.Table) != len(Table) {
		t.Fatalf("the corpus prices %d models, the table %d", len(fx.Table), len(Table))
	}
	if fx.CacheReadFraction != CacheReadFraction {
		t.Fatalf("cache read fraction %v != %v", fx.CacheReadFraction, CacheReadFraction)
	}
	for model, want := range fx.Table {
		got, ok := Table[model]
		if !ok {
			t.Errorf("%s is priced in the corpus and absent here", model)
			continue
		}
		if got.Rate.Input != want.Input || got.Rate.Output != want.Output {
			t.Errorf("%s: rate %v/%v != %v/%v", model,
				got.Rate.Input, got.Rate.Output, want.Input, want.Output)
		}
		if (want.ThenInput != nil) != (got.Then != nil) {
			t.Errorf("%s: scheduled change disagreement", model)
			continue
		}
		if want.ThenInput != nil && (got.Then.Input != *want.ThenInput ||
			got.Then.Output != *want.ThenOutput || got.Until != want.Until) {
			t.Errorf("%s: window %v until %q != %v/%v until %q", model,
				got.Then, got.Until, *want.ThenInput, *want.ThenOutput, want.Until)
		}
	}
}

// afterEveryChange is a `now` past every boundary the table will ever have,
// so the segment tests read the whole table rather than whichever half of it
// today happens to fall in. A fixed string and not a clock: a test whose
// answer depends on the day it runs is a test that will one day disagree with
// itself for no reason anybody can reproduce.
const afterEveryChange = "2099-01-01"

// The boundaries are the table's own dates, moved on by one day -- `Until` is
// the last day the old rate applies, so the changeover is the morning after.
// Off by one here would price a whole day at the wrong rate and nothing else
// would notice.
func TestTheBoundariesAreTheDayAfterEachScheduledChange(t *testing.T) {
	t.Parallel()
	scheduled := 0
	for model, priced := range Table {
		if priced.Until == "" || priced.Then == nil {
			continue
		}
		scheduled++
		// The rate on the last day of the window is the old one; the rate on
		// the boundary itself is the new one. Both read off `On`, which is
		// what every caller uses.
		if got := priced.On(priced.Until); got != priced.Rate {
			t.Errorf("%s on its own last day %s is %v, want the old rate %v",
				model, priced.Until, got, priced.Rate)
		}
		day, err := time.Parse("2006-01-02", priced.Until)
		if err != nil {
			t.Fatalf("%s: Until %q is not a date", model, priced.Until)
		}
		changeover := day.AddDate(0, 0, 1).Format("2006-01-02")
		if got := priced.On(changeover); got != *priced.Then {
			t.Errorf("%s on %s is %v, want the new rate %v",
				model, changeover, got, *priced.Then)
		}
		found := false
		for _, boundary := range Boundaries() {
			if boundary == changeover {
				found = true
			}
		}
		if !found {
			t.Errorf("%s changes on %s and Boundaries() does not say so: %v",
				model, changeover, Boundaries())
		}
	}
	if scheduled == 0 && len(Boundaries()) > 0 {
		t.Errorf("no model has a scheduled change and yet Boundaries() is %v",
			Boundaries())
	}
}

// **The property the whole split rests on: inside a segment, nothing moves.**
// Every rate in the table is asked at the segment's first day, its last day
// and the date the segment says to price it at, and all three must agree --
// which is what makes "price the segment at `When`" a fact rather than a
// convention.
func TestNoRateChangesInsideASegment(t *testing.T) {
	t.Parallel()
	segments := Segments("", afterEveryChange)
	if len(segments) != len(Boundaries())+1 {
		t.Fatalf("%d boundaries produced %d segments", len(Boundaries()), len(segments))
	}
	for _, seg := range segments {
		first, last := seg.Since, seg.Until
		if first == "" {
			first = "0001-01-01"
		}
		if last == "" {
			last = afterEveryChange
		} else {
			last = dayBefore(last)
		}
		for model, priced := range Table {
			at := priced.On(seg.When)
			if got := priced.On(first); got != at {
				t.Errorf("%s costs %v on %s and %v at the segment's own date %s",
					model, got, first, at, seg.When)
			}
			if got := priced.On(last); got != at {
				t.Errorf("%s costs %v on %s and %v at the segment's own date %s",
					model, got, last, at, seg.When)
			}
		}
	}
}

// The segments tile the window: no gap, no overlap, and the ends are the ends.
// `since` inclusive and `until` exclusive is what makes the seam clean, and a
// seam that double-counted would inflate the bill by exactly one instant's
// worth of conversations -- which is nothing most days and a whole night's run
// on the wrong one.
func TestTheSegmentsTileTheWindowTheyWereGiven(t *testing.T) {
	t.Parallel()
	for _, since := range []string{"", "2020-01-01", "2026-08-30T12:00:00.000000+00:00"} {
		segments := Segments(since, afterEveryChange)
		if len(segments) == 0 {
			t.Fatalf("since=%q produced no segments at all", since)
		}
		if segments[0].Since != since {
			t.Errorf("since=%q: the first segment starts at %q", since, segments[0].Since)
		}
		if last := segments[len(segments)-1]; last.Until != "" {
			t.Errorf("since=%q: the last segment stops at %q rather than running on",
				since, last.Until)
		}
		for i := 1; i < len(segments); i++ {
			if segments[i-1].Until != segments[i].Since {
				t.Errorf("since=%q: a gap between %q and %q", since,
					segments[i-1].Until, segments[i].Since)
			}
		}
	}
	// A window that starts after every change is one segment -- the common
	// case on an instance whose rates have not moved lately, and the one that
	// must not pay for a second query.
	if got := Segments(afterEveryChange, afterEveryChange); len(got) != 1 {
		t.Errorf("a window past every boundary was cut into %d segments: %+v", len(got), got)
	}
	// And a boundary that has not arrived yet is not a segment: there is
	// nothing on the far side of it to price.
	if got := Segments("", "1999-01-01"); len(got) != 1 {
		t.Errorf("a window before every boundary was cut into %d segments: %+v", len(got), got)
	}
}

// **The regression itself.** A model with a scheduled change must be priced
// at BOTH its rates across an all-time window's segments -- which is exactly
// what did not happen when every window was handed `Today()`, and which cost
// the recorded bill ~50% the morning the introductory window closed.
func TestAScheduledChangeIsActuallyPricedTwice(t *testing.T) {
	t.Parallel()
	for model, priced := range Table {
		if priced.Then == nil {
			continue
		}
		seen := map[Rate]bool{}
		for _, seg := range Segments("", afterEveryChange) {
			seen[priced.On(seg.When)] = true
		}
		if !seen[priced.Rate] {
			t.Errorf("%s's own rate %v is priced in no segment of an all-time window",
				model, priced.Rate)
		}
		if !seen[*priced.Then] {
			t.Errorf("%s's later rate %v is priced in no segment of an all-time window",
				model, *priced.Then)
		}
	}
}

// Splitting a set of rows and summing the pieces is the same arithmetic as
// one sweep, when no rate moved between them.
//
// This is not a nicety: it is what lets a caller segment a window without
// changing any figure that did not need to change. `Over` accumulates term by
// term with the rounding guard after each product, and `Sum` continues that
// same accumulation rather than starting a different kind of addition -- so
// the last bit of the answer survives the split.
func TestSummingThePiecesIsTheSameArithmeticAsOneSweep(t *testing.T) {
	t.Parallel()
	rows := []Row{
		{Model: "claude-opus-5", Conversations: 3, InputTokens: 91_237,
			OutputTokens: 13_009, CacheRead: 1_400_311},
		{Model: "claude-sonnet-5", Conversations: 41, InputTokens: 7_777,
			OutputTokens: 313, CacheRead: 999_999},
		{Model: "claude-haiku-4-5", Conversations: 1, InputTokens: 13,
			OutputTokens: 7, CacheRead: 0},
	}
	const when = "2026-09-02"
	whole := Over(rows, when)
	for cut := 0; cut <= len(rows); cut++ {
		split := Sum([]Estimate{Over(rows[:cut], when), Over(rows[cut:], when)})
		if split.USD != whole.USD {
			t.Errorf("split at %d: %v, one sweep: %v", cut, split.USD, whole.USD)
		}
	}
	// And an unpriced model is counted in every piece it appears in but named
	// once, because one model nobody can price is one problem however many
	// windows it turns up in.
	orphan := []Row{{Model: "claude-nobody-knows", Conversations: 2},
		{Model: "claude-nobody-knows", Conversations: 5}}
	got := Sum([]Estimate{Over(orphan[:1], when), Over(orphan[1:], when)})
	if got.Unpriced != 7 {
		t.Errorf("unpriced conversations summed to %d, want 7", got.Unpriced)
	}
	if len(got.UnpricedModels) != 1 || got.UnpricedModels[0] != "claude-nobody-knows" {
		t.Errorf("the unpriced model is named %v", got.UnpricedModels)
	}
}

// TestEveryEstimateMatchesTheGolden is the corpus: `Over(...).AsDict()`
// compared as marshalled bytes — the half-to-even rounding, the window on
// both sides of Sonnet 5's changeover, and the unpriced accounting.
func TestEveryEstimateMatchesTheGolden(t *testing.T) {
	t.Parallel()
	fx := load(t)
	if len(fx.Cases) < 6 {
		t.Fatalf("only %d cases; the corpus has thinned", len(fx.Cases))
	}
	for _, tc := range fx.Cases {
		rows := make([]Row, 0, len(tc.Rows))
		for _, r := range tc.Rows {
			rows = append(rows, Row{Model: r.Model, Conversations: r.Conversations,
				InputTokens: r.InputTokens, OutputTokens: r.OutputTokens,
				CacheRead: r.CacheReadTokens})
		}
		got, err := wire.MarshalOrdered(Over(rows, tc.When).AsDict())
		if err != nil {
			t.Fatalf("%s: %v", tc.Name, err)
		}
		if string(got) != tc.Rendered {
			t.Errorf("%s diverged:\n got %s\nwant %s", tc.Name, got, tc.Rendered)
		}
	}
}
