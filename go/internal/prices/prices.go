// Package prices is the Claude ledger's price table: what the tokens cost,
// in money.
//
// The standing rules, all load-bearing: rates are dated and the date
// renders beside every figure; a rate known to move is modelled as a window
// rather than flattened; **a window is priced at the rates that were in force
// during it**, not at this morning's; a model the table does not know is
// priced at nothing and **counted**, because charging it zero silently would
// read as "cheap"; and the figure stays a floor, since cache writes bill at
// 1.25x input and are recorded nowhere. Re-check rates by reading the pricing
// page and editing `Table`; the corpus test holds the table to the recorded
// rates, so a rate never drifts silently.
//
// The third rule is the one that was broken by a caller rather than by this
// file, which is why `Segments` lives here now. `Over` prices a set of rows at
// one date, and every caller passed `Today()` — correct until the day a rate
// moved, then wrong forever for everything spent before it, by exactly the
// size of the move. Sonnet 5's introductory window closed on 2026-08-31 and
// the whole recorded bill before it started reading ~50% high the next
// morning. A caller with a date range asks `Segments` to cut it at the
// boundaries the table itself knows, prices each piece at the rate that was in
// force, and adds them with `Sum`.
package prices

import (
	"sort"
	"time"

	"github.com/aasquier/sylvan-library/go/internal/floats"
	"github.com/aasquier/sylvan-library/go/internal/wire"
)

// Checked is when a human last read the pricing page — rendered beside every
// figure, the only honest substitute for a freshness guarantee.
const Checked = "2026-08-18"

// Source is where to re-check. Not fetched, deliberately.
const Source = "https://platform.claude.com/docs/en/pricing"

// CacheReadFraction prices a prompt-cache read as a fraction of input. One
// number rather than a per-model column: it is the same ratio across the
// family, and a copy per model would be that many chances to mistype a tenth.
const CacheReadFraction = 0.1

// Rate is dollars per million tokens.
type Rate struct {
	Input, Output float64
}

// Priced is one model's rate, and the date any of it is known to change:
// `Then` takes over after `Until`, both set or neither.
type Priced struct {
	Rate  Rate
	Then  *Rate
	Until string // ISO date, the last day Rate applies
}

// On is the rate in force on `when` (an ISO date; string comparison is date
// comparison).
func (p Priced) On(when string) Rate {
	if p.Until != "" && p.Then != nil && when > p.Until {
		return *p.Then
	}
	return p.Rate
}

// Table is the rate per model. Sonnet 5 is the one with a scheduled
// change: $2/$10 introductory through 2026-08-31, $3/$15 after.
var Table = map[string]Priced{
	"claude-fable-5":    {Rate: Rate{10.00, 50.00}},
	"claude-mythos-5":   {Rate: Rate{10.00, 50.00}},
	"claude-opus-5":     {Rate: Rate{5.00, 25.00}},
	"claude-opus-4-8":   {Rate: Rate{5.00, 25.00}},
	"claude-opus-4-7":   {Rate: Rate{5.00, 25.00}},
	"claude-opus-4-6":   {Rate: Rate{5.00, 25.00}},
	"claude-sonnet-5":   {Rate: Rate{2.00, 10.00}, Then: &Rate{3.00, 15.00}, Until: "2026-08-31"},
	"claude-sonnet-4-6": {Rate: Rate{3.00, 15.00}},
	"claude-haiku-4-5":  {Rate: Rate{1.00, 5.00}},
}

const perMillion = 1_000_000

// Cost is what one conversation came to, or false for a model with no rate
// — a conversation that cost nothing and one nobody can price are different
// facts. Every product is fenced with `floats.Rounded`, because `a*b + c`
// is the shape arm64 fuses into one FMADDD and the recorded arithmetic
// rounds after every product.
func Cost(model string, inputTokens, outputTokens, cacheReadTokens int64,
	when string) (float64, bool) {
	priced, ok := Table[model]
	if !ok {
		return 0, false
	}
	rate := priced.On(when)
	total := floats.Rounded(float64(inputTokens) * rate.Input)
	total = floats.Rounded(total + floats.Rounded(float64(outputTokens)*rate.Output))
	cache := floats.Rounded(floats.Rounded(float64(cacheReadTokens)*rate.Input) * CacheReadFraction)
	total = floats.Rounded(total + cache)
	return total / perMillion, true
}

// Row is one ledger summary row's pricing-relevant half.
type Row struct {
	Model         string
	Conversations int64
	InputTokens   int64
	OutputTokens  int64
	CacheRead     int64
}

// Estimate is the total, and what could not be
// priced — which is not a footnote, since an unpriced row contributes
// nothing to `usd` and a caller rendering the figure without it is showing a
// number that is wrong downward.
type Estimate struct {
	USD            float64
	Unpriced       int64
	UnpricedModels []string // sorted, "(none recorded)" for a blank model
}

// Over totals rows in order — a plain running sum, never a compensated
// one, deliberately: the recorded totals accumulate term by term, and the
// order of additions is part of the answer's last bit.
func Over(rows []Row, when string) Estimate {
	out := Estimate{}
	seen := map[string]bool{}
	for _, row := range rows {
		conversations := row.Conversations
		if conversations == 0 {
			conversations = 1
		}
		amount, ok := Cost(row.Model, row.InputTokens, row.OutputTokens,
			row.CacheRead, when)
		if !ok {
			out.Unpriced += conversations
			name := row.Model
			if name == "" {
				name = "(none recorded)"
			}
			if !seen[name] {
				seen[name] = true
				out.UnpricedModels = append(out.UnpricedModels, name)
			}
			continue
		}
		out.USD = floats.Rounded(out.USD + amount)
	}
	sort.Strings(out.UnpricedModels)
	return out
}

// AsDict is the wire shape, in the recorded key order: usd (rounded
// half-to-even to four places, rendered canonically), unpriced,
// unpriced_models, complete, checked.
func (e Estimate) AsDict() wire.OrderedMap {
	models := e.UnpricedModels
	if models == nil {
		models = []string{}
	}
	return wire.OrderedMap{
		{Key: "usd", Value: floats.Float(floats.RoundTo(e.USD, 4))},
		{Key: "unpriced", Value: e.Unpriced},
		{Key: "unpriced_models", Value: models},
		{Key: "complete", Value: e.Unpriced == 0},
		{Key: "checked", Value: Checked},
	}
}

// Today is the local day as an ISO date — on the instance, UTC.
func Today() string { return time.Now().Format("2006-01-02") }

// ---- pricing a window that crosses a rate change --------------------------

// Segment is one stretch of time over which every rate in Table held still:
// `[Since, Until)`, and `When` is a date inside it, which is all `Cost` needs
// to pick the rate that was in force.
//
// Both bounds are compared as text against `created_at`, which is ISO-8601
// UTC — so a bare date is a perfectly good instant here, and midnight is where
// it lands. An empty `Since` is "from the beginning" and an empty `Until` is
// "up to now"; both are the absence of a bound rather than a bound at zero.
type Segment struct {
	Since string
	Until string
	When  string
}

// Boundaries are the dates the table's rates change, sorted, deduplicated —
// the first day each new rate applies, which is the day AFTER the `Until` a
// Priced records as the last day of the old one.
//
// Derived from Table rather than written down beside it, because a second
// list of the same dates is a second thing to forget when a rate moves.
func Boundaries() []string {
	seen := map[string]bool{}
	out := []string{}
	for _, priced := range Table {
		if priced.Until == "" || priced.Then == nil {
			continue
		}
		day, err := time.Parse("2006-01-02", priced.Until)
		if err != nil {
			// An unparseable Until cannot be turned into a boundary, and
			// guessing one would price a window at the wrong rate silently.
			// `Priced.On` still compares it as text, so the table keeps working;
			// what is lost is only the split, which is the honest failure.
			continue
		}
		changeover := day.AddDate(0, 0, 1).Format("2006-01-02")
		if !seen[changeover] {
			seen[changeover] = true
			out = append(out, changeover)
		}
	}
	sort.Strings(out)
	return out
}

// Segments splits `[since, now]` at every boundary it crosses.
//
// **This is what "estimated spend" means.** Pricing a whole window at today's
// rate is not an estimate of what was spent, it is an estimate of what the
// same traffic would cost if it were bought again this morning — and when a
// rate moves by half, as Sonnet 5's introductory window did on 2026-09-01,
// the two answers differ by half. The ledger records when each conversation
// happened; this is what lets a roll-up use that.
//
// `now` bounds the split rather than the sum: a boundary that has not arrived
// yet cannot have any rows past it, so a segment beginning after today is left
// out instead of costing a query that can only answer zero.
func Segments(since, now string) []Segment {
	cuts := []string{since}
	for _, boundary := range Boundaries() {
		// `>` and not `>=`: a boundary at exactly the window's start has
		// already taken effect, so there is nothing before it to price.
		if boundary > since && boundary <= now {
			cuts = append(cuts, boundary)
		}
	}
	out := make([]Segment, 0, len(cuts))
	for i, start := range cuts {
		seg := Segment{Since: start, When: now}
		if i+1 < len(cuts) {
			seg.Until = cuts[i+1]
			// A date inside the segment: the day before the next change, which
			// is the last day this segment's rates were in force. Any date in
			// the range would pick the same rate; this one always exists.
			seg.When = dayBefore(seg.Until)
		}
		out = append(out, seg)
	}
	return out
}

func dayBefore(date string) string {
	day, err := time.Parse("2006-01-02", date)
	if err != nil {
		return date
	}
	return day.AddDate(0, 0, -1).Format("2006-01-02")
}

// Sum folds one estimate per segment into the window's own.
//
// The dollars are added in segment order with `Over`'s rounding, so the sum is
// the same arithmetic done in the same sequence rather than a second kind of
// addition. The unpriced counts add; the unpriced model names are a set, since
// a model the table cannot price is unpriceable in every segment it appears in
// and naming it three times would read as three problems.
func Sum(parts []Estimate) Estimate {
	out := Estimate{}
	seen := map[string]bool{}
	for _, part := range parts {
		out.USD = floats.Rounded(out.USD + part.USD)
		out.Unpriced += part.Unpriced
		for _, model := range part.UnpricedModels {
			if !seen[model] {
				seen[model] = true
				out.UnpricedModels = append(out.UnpricedModels, model)
			}
		}
	}
	sort.Strings(out.UnpricedModels)
	return out
}
