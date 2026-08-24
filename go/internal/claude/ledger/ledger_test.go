package ledger

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/auth/authtest"
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
	r := scratch(t)
	ctx := context.Background()
	for _, row := range []Row{
		{"commander-dossier", "claude-opus-5", "end_turn", 3, 100, 20, 50},
		{"commander-dossier", "claude-opus-5", "end_turn", 2, 50, 10, 25},
		{"rationale-interview", "claude-sonnet-5", "end_turn", 1, 10, 5, 0},
	} {
		r.Record(ctx, row)
	}

	byMode, err := r.Summarise(ctx, "mode", "")
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

	byModel, err := r.Summarise(ctx, "model", "")
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
	if Various != tiers.Various {
		t.Fatalf("ledger says %q, tiers says %q", Various, tiers.Various)
	}
	if got := tiers.LabelFor(Various); got != "Several" {
		t.Errorf("the aggregated marker renders as %q, want \"Several\"", got)
	}
}

func TestSinceFiltersOnTheTextTimestamp(t *testing.T) {
	r := scratch(t)
	ctx := context.Background()
	r.Record(ctx, Row{"research", "claude-sonnet-5", "end_turn", 1, 10, 5, 0})

	// created_at is ISO-8601 UTC text, so string comparison is date
	// comparison. An instant in the past keeps the row; one in the future
	// drops it, and neither needs a date type.
	kept, err := r.Summarise(ctx, "mode", "2000-01-01T00:00:00.000000+00:00")
	if err != nil {
		t.Fatalf("since in the past: %v", err)
	}
	if len(kept) != 1 {
		t.Errorf("a past `since` dropped the row: %+v", kept)
	}
	dropped, err := r.Summarise(ctx, "mode", "2999-01-01T00:00:00.000000+00:00")
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
	r := scratch(t)
	for _, bad := range []string{
		"", "slug", "MODE", "mode; DROP TABLE claude_usage",
		"mode)--", "1", "created_at",
	} {
		out, err := r.Summarise(context.Background(), bad, "")
		if err == nil {
			t.Errorf("%q was accepted as a grouping axis, returning %+v", bad, out)
			continue
		}
		if !strings.HasPrefix(err.Error(), "cannot group by ") {
			t.Errorf("%q: refusal is not the recorded wording: %v", bad, err)
		}
	}
	// And the table is still there, which is the point of the paragraph above.
	if _, err := r.Summarise(context.Background(), "mode", ""); err != nil {
		t.Fatalf("the table did not survive the refusals: %v", err)
	}
}

// TestRecordNeverFailsTheConversationThatProducedIt is the property the whole
// module is shaped around. A dossier run costs four minutes and real money;
// losing it because the accounting could not be written would be strictly
// worse than having no accounting at all.
func TestRecordNeverFailsTheConversationThatProducedIt(t *testing.T) {
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
		got, err := r.Summarise(ctx, q.By, since)
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
