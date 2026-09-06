package ledger

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aasquier/sylvan-library/go/internal/auth/authtest"
	"github.com/aasquier/sylvan-library/go/internal/prices"
	"github.com/aasquier/sylvan-library/go/internal/tiers"
)

func scratch(t *testing.T) *Recorder {
	t.Helper()
	path := filepath.Join(t.TempDir(), "app.db")
	if err := authtest.NewScratchDB(path); err != nil {
		t.Fatalf("seeding a scratch app.db: %v", err)
	}
	r, err := NewRecorder(path, nil)
	if err != nil {
		t.Fatalf("opening the ledger: %v", err)
	}
	t.Cleanup(func() { r.Close() })
	return r
}

func TestARecordedConversationCanBeRolledUp(t *testing.T) {
	t.Parallel()
	r := scratch(t)
	ctx := context.Background()
	for _, row := range []Row{
		{"commander-dossier", "claude-opus-5", "end_turn", 3, 100, 20, 50},
		{"commander-dossier", "claude-opus-5", "end_turn", 2, 50, 10, 25},
		{"rationale-interview", "claude-sonnet-5", "end_turn", 1, 10, 5, 0},
	} {
		r.Record(ctx, row)
	}

	byMode, err := r.Summarise(ctx, "mode", "", "")
	if err != nil {
		t.Fatalf("rolling up by mode: %v", err)
	}
	if len(byMode) != 2 {
		t.Fatalf("expected two modes, got %d", len(byMode))
	}
	// Most expensive first: input + output, so the dossier's 170 beats 15.
	if byMode[0].Mode != "commander-dossier" {
		t.Errorf("rows are not most-expensive-first: %+v", byMode)
	}
	top := byMode[0]
	if top.Conversations != 2 || top.Requests != 5 ||
		top.InputTokens != 150 || top.OutputTokens != 30 || top.CacheReadTokens != 75 {
		t.Errorf("dossier totals are wrong: %+v", top)
	}
	// The axis that was NOT grouped on holds the marker, never a winner
	// SQLite happened to pick. That is the whole reason the marker exists: a
	// caller pricing a per-mode roll-up must get "unpriced" rather than a
	// number computed from an arbitrary model.
	if top.Model != Various {
		t.Errorf("grouping by mode left %q in the model column, want %q",
			top.Model, Various)
	}

	byModel, err := r.Summarise(ctx, "model", "", "")
	if err != nil {
		t.Fatalf("rolling up by model: %v", err)
	}
	if len(byModel) != 2 || byModel[0].Model != "claude-opus-5" {
		t.Fatalf("by-model roll-up is wrong: %+v", byModel)
	}
	if byModel[0].Mode != Various {
		t.Errorf("grouping by model left %q in the mode column", byModel[0].Mode)
	}
}

// TestTheMarkerIsTheSameWordInBothPackages pins a coincidence that must not
// become a divergence. The ledger writes "(various)" into the aggregated
// column and tiers.LabelFor turns that exact string into "Several" on screen;
// if either moved, the Admin panel would render "Another Claude" for every
// rolled-up row and look like a model nobody recognised.
func TestTheMarkerIsTheSameWordInBothPackages(t *testing.T) {
	t.Parallel()
	if Various != tiers.Various {
		t.Fatalf("ledger says %q, tiers says %q", Various, tiers.Various)
	}
	if got := tiers.LabelFor(Various); got != "Several" {
		t.Errorf("the aggregated marker renders as %q, want \"Several\"", got)
	}
}

func TestSinceFiltersOnTheTextTimestamp(t *testing.T) {
	t.Parallel()
	r := scratch(t)
	ctx := context.Background()
	r.Record(ctx, Row{"research", "claude-sonnet-5", "end_turn", 1, 10, 5, 0})

	// created_at is ISO-8601 UTC text, so string comparison is date
	// comparison. An instant in the past keeps the row; one in the future
	// drops it, and neither needs a date type.
	kept, err := r.Summarise(ctx, "mode", "2000-01-01T00:00:00.000000+00:00", "")
	if err != nil {
		t.Fatalf("since in the past: %v", err)
	}
	if len(kept) != 1 {
		t.Errorf("a past `since` dropped the row: %+v", kept)
	}
	dropped, err := r.Summarise(ctx, "mode", "2999-01-01T00:00:00.000000+00:00", "")
	if err != nil {
		t.Fatalf("since in the future: %v", err)
	}
	if len(dropped) != 0 {
		t.Errorf("a future `since` kept rows: %+v", dropped)
	}
}

// TestAnUnknownAxisIsRefusedRatherThanInterpolated is a security property, not
// a validation nicety: `by` is spliced into the SQL because a column name
// cannot be bound as a parameter, so the safety comes entirely from the value
// never being caller-controlled.
func TestAnUnknownAxisIsRefusedRatherThanInterpolated(t *testing.T) {
	t.Parallel()
	r := scratch(t)
	for _, bad := range []string{
		"", "slug", "MODE", "mode; DROP TABLE claude_usage",
		"mode)--", "1", "created_at",
	} {
		out, err := r.Summarise(context.Background(), bad, "", "")
		if err == nil {
			t.Errorf("%q was accepted as a grouping axis, returning %+v", bad, out)
			continue
		}
		if !strings.HasPrefix(err.Error(), "cannot group by ") {
			t.Errorf("%q: refusal is not the recorded wording: %v", bad, err)
		}
	}
	// And the table is still there, which is the point of the paragraph above.
	if _, err := r.Summarise(context.Background(), "mode", "", ""); err != nil {
		t.Fatalf("the table did not survive the refusals: %v", err)
	}
}

// TestRecordNeverFailsTheConversationThatProducedIt is the property the whole
// module is shaped around. A dossier run costs four minutes and real money;
// losing it because the accounting could not be written would be strictly
// worse than having no accounting at all.
func TestRecordNeverFailsTheConversationThatProducedIt(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	row := Row{"commander-dossier", "claude-opus-5", "end_turn", 1, 1, 1, 1}

	// A nil recorder is the no-ledger case, not a crash.
	var absent *Recorder
	absent.Record(ctx, row)

	// A closed handle is the "app.db went away underneath us" case. Record
	// has no error to return and must not panic.
	r := scratch(t)
	if err := r.db.Close(); err != nil {
		t.Fatalf("closing: %v", err)
	}
	r.Record(ctx, row)

	// A cancelled context, which is what a shutting-down job hands it.
	live := scratch(t)
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	live.Record(cancelled, row)
}

// TestTheRowIsCountersAndNeverAChatLog pins the aggregate-on-purpose decision
// at the schema, where it is enforceable.
//
// If a slug, a user id or a question ever lands in this table, ADR 17's
// who-may-read-what argument has to be made for it — and the honest answer
// would be that a table of what everybody asked Claude is a chat log. Keeping
// the columns to counters is what makes that conversation unnecessary.
func TestTheRowIsCountersAndNeverAChatLog(t *testing.T) {
	t.Parallel()
	r := scratch(t)
	rows, err := r.db.Query("SELECT name FROM pragma_table_info('claude_usage')")
	if err != nil {
		t.Fatalf("reading the schema: %v", err)
	}
	defer rows.Close()
	got := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		got[name] = true
	}
	want := []string{"id", "created_at", "mode", "model", "stop_reason",
		"requests", "input_tokens", "output_tokens", "cache_read_tokens"}
	for _, name := range want {
		if !got[name] {
			t.Errorf("claude_usage has no %q column", name)
		}
		delete(got, name)
	}
	for name := range got {
		t.Errorf("claude_usage grew a %q column. This table is counters; a "+
			"slug, an account or a question here makes it a chat log, and "+
			"ADR 17's argument would have to be made for it.", name)
	}
}

// at writes one row with a timestamp of the test's choosing. `Record` stamps
// `now()`, which is right for the app and useless for a question about
// windows, so the seam tests insert directly -- exactly as the corpus test
// does, and for the same reason.
func at(t *testing.T, r *Recorder, stamp string, row Row) {
	t.Helper()
	if _, err := r.db.ExecContext(context.Background(),
		"INSERT INTO claude_usage (created_at, mode, model, stop_reason,"+
			" requests, input_tokens, output_tokens, cache_read_tokens)"+
			" VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		stamp, row.Mode, row.Model, row.StopReason, row.Requests,
		row.InputTokens, row.OutputTokens, row.CacheReadTokens); err != nil {
		t.Fatalf("seeding a row at %s: %v", stamp, err)
	}
}

// scheduledChange is a model whose rate is known to move, and the two days
// that straddle the move -- read off the price table rather than typed here,
// so this test still asks the right question after the next rate change lands
// and the current one is history.
func scheduledChange(t *testing.T) (model, lastOldDay, firstNewDay string) {
	t.Helper()
	for name, priced := range prices.Table {
		if priced.Then == nil || priced.Until == "" {
			continue
		}
		day, err := time.Parse("2006-01-02", priced.Until)
		if err != nil {
			t.Fatalf("%s: Until %q is not a date", name, priced.Until)
		}
		return name, priced.Until, day.AddDate(0, 0, 1).Format("2006-01-02")
	}
	t.Skip("no rate in the table is scheduled to move; nothing to straddle")
	return "", "", ""
}

// **The finding this window was built for.** Tokens spent under one rate and
// tokens spent under the next are two different bills, and a roll-up that
// prices the whole window at today's rate answers the second one twice.
//
// Both halves here are identical in every way but the day they happened on, so
// the only thing that can make the total differ from double-one-half is the
// rate -- and the test computes both candidate answers from the price table
// rather than restating a figure, which is what keeps it true when the table
// moves.
func TestAWindowThatCrossesARateChangeIsPricedOnBothSidesOfIt(t *testing.T) {
	t.Parallel()
	model, before, after := scheduledChange(t)
	r := scratch(t)
	spend := Row{Mode: "commander-dossier", Model: model, StopReason: "end_turn",
		Requests: 4, InputTokens: 1_000_000, OutputTokens: 100_000,
		CacheReadTokens: 2_000_000}
	at(t, r, before+"T09:00:00.000000+00:00", spend)
	at(t, r, after+"T09:00:00.000000+00:00", spend)

	// A `now` past the change, so the window genuinely spans it.
	roll, err := r.Window(context.Background(), "", after)
	if err != nil {
		t.Fatalf("rolling the window up: %v", err)
	}

	half := prices.Row{Model: model, Conversations: 1,
		InputTokens: int64(spend.InputTokens), OutputTokens: int64(spend.OutputTokens),
		CacheRead: int64(spend.CacheReadTokens)}
	honest := prices.Sum([]prices.Estimate{
		prices.Over([]prices.Row{half}, before),
		prices.Over([]prices.Row{half}, after),
	})
	if roll.Cost.USD != honest.USD {
		t.Errorf("the window came to %v; the two halves at their own rates come "+
			"to %v", roll.Cost.USD, honest.USD)
	}

	// And the answer the old shape gave -- everything at the later rate -- is a
	// different number, which is what makes this a fix rather than a
	// rearrangement. If the two ever agree the test is asking nothing.
	flat := prices.Over([]prices.Row{half, half}, after)
	if flat.USD == honest.USD {
		t.Fatalf("both rates price this spend at %v, so nothing here is being "+
			"asked -- pick a model whose rates actually differ", flat.USD)
	}
	if roll.Cost.USD == flat.USD {
		t.Errorf("the window was priced entirely at %s's rate (%v)", after, flat.USD)
	}

	// The per-model figure is the same arithmetic, since one model spent all
	// of it -- which is what the shell's `est. USD` column renders.
	if got := roll.CostOf[model]; got.USD != roll.Cost.USD {
		t.Errorf("%s's own figure is %v and the window's is %v",
			model, got.USD, roll.Cost.USD)
	}
}

// `until` is exclusive and `since` inclusive, so two abutting windows tile:
// every row lands in exactly one of them. A seam that included both ends would
// bill the instant twice, which is precisely the arithmetic the segments do.
func TestAbuttingWindowsTileRatherThanOverlap(t *testing.T) {
	t.Parallel()
	r := scratch(t)
	ctx := context.Background()
	const seam = "2026-09-01T00:00:00.000000+00:00"
	at(t, r, "2026-08-31T23:59:59.999999+00:00", Row{"research", "claude-sonnet-5", "end_turn", 1, 10, 1, 0})
	at(t, r, seam, Row{"research", "claude-sonnet-5", "end_turn", 1, 100, 1, 0})
	at(t, r, "2026-09-01T00:00:00.000001+00:00", Row{"research", "claude-sonnet-5", "end_turn", 1, 1000, 1, 0})

	below, err := r.Summarise(ctx, "mode", "", seam)
	if err != nil {
		t.Fatal(err)
	}
	above, err := r.Summarise(ctx, "mode", seam, "")
	if err != nil {
		t.Fatal(err)
	}
	whole, err := r.Summarise(ctx, "mode", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(below) != 1 || len(above) != 1 || len(whole) != 1 {
		t.Fatalf("one mode should give one row each: %d/%d/%d",
			len(below), len(above), len(whole))
	}
	if below[0].Conversations != 1 {
		t.Errorf("the window below the seam holds %d conversations, want 1 -- "+
			"the row AT the seam belongs above it", below[0].Conversations)
	}
	if below[0].InputTokens+above[0].InputTokens != whole[0].InputTokens {
		t.Errorf("%d + %d tokens across the seam, %d in the whole window",
			below[0].InputTokens, above[0].InputTokens, whole[0].InputTokens)
	}
	if below[0].Conversations+above[0].Conversations != whole[0].Conversations {
		t.Errorf("%d + %d conversations across the seam, %d in the whole window",
			below[0].Conversations, above[0].Conversations, whole[0].Conversations)
	}
}

// A window with no ledger under it is nothing rather than an error: an
// instance before its first boot has an honest answer to "what has this cost",
// and it is zero.
func TestAWindowWithNoLedgerIsEmptyRatherThanAFailure(t *testing.T) {
	t.Parallel()
	var absent *Recorder
	roll, err := absent.Window(context.Background(), "", "2026-09-05")
	if err != nil {
		t.Fatalf("a missing ledger answered with an error: %v", err)
	}
	if len(roll.ByMode) != 0 || len(roll.ByModel) != 0 || roll.Cost.USD != 0 {
		t.Errorf("a missing ledger answered %+v", roll)
	}
	// Empty rather than nil, because the panel serialises these straight to
	// the wire and `null` where a list belongs is a different payload.
	if roll.ByMode == nil || roll.ByModel == nil || roll.CostOf == nil {
		t.Error("a missing ledger answered with nil collections, which reach " +
			"the wire as null rather than as an empty list")
	}
}

// A model the table cannot price is counted, never charged at nothing --
// and the count reaches the roll-up, which is what the Admin panel's warning
// and the shell's `unpriced` line both read.
func TestAModelWithNoRateIsCountedRatherThanPricedAtZero(t *testing.T) {
	t.Parallel()
	r := scratch(t)
	at(t, r, "2026-09-02T09:00:00.000000+00:00",
		Row{"research", "claude-from-the-future", "end_turn", 1, 10_000, 500, 0})
	roll, err := r.Window(context.Background(), "", "2026-09-05")
	if err != nil {
		t.Fatal(err)
	}
	if roll.Cost.Unpriced != 1 {
		t.Errorf("%d unpriced conversations, want 1", roll.Cost.Unpriced)
	}
	if len(roll.Cost.UnpricedModels) != 1 ||
		roll.Cost.UnpricedModels[0] != "claude-from-the-future" {
		t.Errorf("the unpriced model is named %v", roll.Cost.UnpricedModels)
	}
	if roll.Cost.USD != 0 {
		t.Errorf("a model with no rate contributed %v to the total", roll.Cost.USD)
	}
}

type ledgerCorpus struct {
	Columns []string `json:"columns"`
	Rows    [][]any  `json:"rows"`
	Queries []struct {
		By    string    `json:"by"`
		Since *string   `json:"since"`
		Rows  []Summary `json:"rows"`
	} `json:"queries"`
}

// TestTheRollUpAgreesWithTheCorpus drives both axes and four `since` bounds
// against the recorded answers.
//
// The SQL is short and the temptation is to call it obvious. Three things in
// it are not, and each is a shape a careful rewrite still gets wrong. **Scan
// order depends on the axis** — the grouped column is SELECTed first, so mode
// and model swap positions between the two queries, and a fixed scan order
// puts the right numbers under the wrong names. **The marker fills the column
// that was not grouped on**, where SQLite would otherwise hand back an
// arbitrary winner. And **`since` is a TEXT comparison**, inclusive at `>=`,
// which the corpus probes at exactly a row's timestamp and one microsecond
// past it.
func TestTheRollUpAgreesWithTheCorpus(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile(filepath.Join("testdata", "ledger.json"))
	if err != nil {
		t.Fatalf("reading the ledger corpus: %v", err)
	}
	var c ledgerCorpus
	if err := json.Unmarshal(raw, &c); err != nil {
		t.Fatalf("decoding the ledger corpus: %v", err)
	}
	if len(c.Queries) == 0 || len(c.Rows) == 0 {
		t.Fatal("the ledger corpus is empty")
	}

	r := scratch(t)
	ctx := context.Background()
	// Inserted with the corpus's own timestamps rather than through Record,
	// which stamps `now()` — the `since` bounds are only meaningful against
	// fixed instants.
	for _, row := range c.Rows {
		if _, err := r.db.ExecContext(ctx,
			"INSERT INTO claude_usage (created_at, mode, model, stop_reason,"+
				" requests, input_tokens, output_tokens, cache_read_tokens)"+
				" VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
			row[0], row[1], row[2], row[3], int(row[4].(float64)),
			int(row[5].(float64)), int(row[6].(float64)), int(row[7].(float64)),
		); err != nil {
			t.Fatalf("seeding %v: %v", row, err)
		}
	}

	for _, q := range c.Queries {
		since := ""
		if q.Since != nil {
			since = *q.Since
		}
		got, err := r.Summarise(ctx, q.By, since, "")
		if err != nil {
			t.Errorf("by=%s since=%q: %v", q.By, since, err)
			continue
		}
		if len(got) != len(q.Rows) {
			t.Errorf("by=%s since=%q: go %d rows, python %d",
				q.By, since, len(got), len(q.Rows))
			continue
		}
		for i := range got {
			if got[i] != q.Rows[i] {
				t.Errorf("by=%s since=%q row %d:\n go     %+v\n python %+v",
					q.By, since, i, got[i], q.Rows[i])
			}
		}
	}
}
