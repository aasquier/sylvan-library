package decklog

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/auth/authtest"
)

// describeCase is one row of the oracle (testdata/describe.json, a frozen
// golden): the keywords one recorded edit carried, and the verb and
// sentence rendered for it.
type describeCase struct {
	Extra   map[string]any `json:"extra"`
	Action  string         `json:"action"`
	Summary string         `json:"summary"`
}

// editFor turns one recorded `extra` back into the typed Edit this package
// takes. The corpus discriminates by which key is present; the switch below
// asks the same question in the same order, which is what makes the rows
// comparable at all.
func editFor(extra map[string]any) Edit {
	str := func(key string) string { s, _ := extra[key].(string); return s }
	switch {
	case has(extra, "added"):
		return Edit{Kind: EditAdd, Card: str("added"), Category: str("category"), Into: str("into")}
	case has(extra, "entombed"):
		if names, ok := extra["entombed"].([]any); ok {
			return Edit{Kind: EditEntomb, Cards: strings2(names)}
		}
		return Edit{Kind: EditEntomb, Card: str("entombed")}
	case has(extra, "removed"):
		return Edit{Kind: EditRemove, Card: str("removed")}
	case has(extra, "returned"):
		return Edit{Kind: EditReturn, Card: str("returned")}
	case has(extra, "exiled"):
		return Edit{Kind: EditExile, Card: str("exiled")}
	case has(extra, "swapped_out"):
		return Edit{Kind: EditSwap, Card: str("swapped_out"), SwapIn: str("swapped_in")}
	case has(extra, "note"):
		return Edit{Kind: EditNote, Note: str("note")}
	case has(extra, "card"):
		return Edit{Kind: EditSetCard, Card: str("card"), Field: str("field")}
	case has(extra, "field"):
		return Edit{Kind: EditSetDeck, Field: str("field"), Value: extra["value"]}
	}
	return Edit{}
}

func has(m map[string]any, key string) bool { _, ok := m[key]; return ok }

func strings2(items []any) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		s, _ := item.(string)
		out = append(out, s)
	}
	return out
}

// TestDescribeWritesTheRecordedSentences is the log's half of the family's
// gate. The sentence is rendered once, at write time, so a route that wrote
// a different one would change the History panel's wording from that day on
// -- silently, and only for edits made after the change.
func TestDescribeWritesTheRecordedSentences(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("testdata/describe.json")
	if err != nil {
		t.Fatalf("reading the oracle: %v", err)
	}
	var cases []describeCase
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatalf("decoding the oracle: %v", err)
	}
	if len(cases) < 30 {
		t.Fatalf("the oracle has %d cases; testdata/describe.json is a frozen golden and should hold at least 30", len(cases))
	}
	for _, c := range cases {
		action, summary := Describe(editFor(c.Extra))
		if action != c.Action || summary != c.Summary {
			t.Errorf("%v\n  golden: %q / %q\n     got: %q / %q",
				c.Extra, c.Action, c.Summary, action, summary)
		}
	}
}

// TestNoRationaleReachesASentence pins ADR 28's hardest rule, which no
// equality check can hold on its own: the oracle only proves the sentences
// match the recording, and a corpus recorded from a leaking renderer would
// agree all the way into the table.
func TestNoRationaleReachesASentence(t *testing.T) {
	t.Parallel()
	const secret = "A rationale that must not appear in any log line."
	raw, err := os.ReadFile("testdata/describe.json")
	if err != nil {
		t.Fatalf("reading the oracle: %v", err)
	}
	if !strings.Contains(string(raw), secret) {
		t.Fatal("the oracle no longer passes a `why` to describe, so this " +
			"proves nothing; put one back")
	}
	var cases []describeCase
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	for _, c := range cases {
		_, summary := Describe(editFor(c.Extra))
		if strings.Contains(summary, secret) || strings.Contains(c.Summary, secret) {
			t.Errorf("a rationale reached the log: %q", summary)
		}
	}
	// And the type cannot carry one: an Edit has no field for it, which is
	// ADR 25's technique reused -- the shape refuses rather than the code.
	if _, _ = Describe(Edit{Kind: EditSwap, Card: "A", SwapIn: "B"}); false {
		t.Fatal("unreachable")
	}
}

// TestAnUnknownOperationStillSaysSomething is the fallback, and it is the
// branch most likely to be "simplified" away by somebody who notices nothing
// reaches it. The tenth edit operation is the one somebody adds in a year.
func TestAnUnknownOperationStillSaysSomething(t *testing.T) {
	t.Parallel()
	action, summary := Describe(Edit{})
	if action != "edit" || summary != "edited the deck" {
		t.Errorf("an unrecognised operation said %q / %q; silence is the one "+
			"failure mode a history cannot have", action, summary)
	}
}

// newScratchDB builds an `app.db` from the ladder's recorded schema -- a
// reading of the real ladder, never a second copy of it.
//
// The bytes lived in this package's own `testdata` until 2026-08-22, when the
// accounts flip found two other packages had each transcribed the ladder by
// hand and frozen it at a different rung. They are `authtest`'s now, which is
// where the story is written down.
func newScratchDB(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "app.db")
	if err := authtest.NewScratchDB(path); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRecordWritesAnEntryTheReaderFinds(t *testing.T) {
	t.Parallel()
	path := newScratchDB(t)
	recorder, err := NewRecorder(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer recorder.Close()

	ctx := context.Background()
	recorder.Record(ctx, "goreclaw", nil, "", Edit{
		Kind: EditAdd, Card: "Sol Ring", Category: "ramp", Into: "cards"})
	actor := "aasquier"
	// `deck_log.owner_id` references `users(id)`, and foreign keys are on --
	// which is the point of turning them on, so an entry cannot outlive the
	// account it belongs to. The owned tier needs a real account to hang off.
	owner := newScratchUser(t, path, actor)
	recorder.Record(ctx, "goreclaw", &owner, actor, Edit{
		Kind: EditEntomb, Card: "Primeval Titan"})

	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// The file tier is `owner_id IS NULL`, and reading it back through the
	// same query the route uses is the point: an equality test would return
	// nothing at all for the curated six, forever, without erroring.
	entries, err := Entries(ctx, db, nil, "goreclaw", DefaultLimit)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("the file tier has %d entries, expected 1", len(entries))
	}
	if entries[0].Action != "add" || entries[0].Summary != "added Sol Ring as ramp" {
		t.Errorf("wrote %q / %q", entries[0].Action, entries[0].Summary)
	}
	if entries[0].Actor != nil {
		t.Errorf("an empty actor should be null, got %q", *entries[0].Actor)
	}
	// The stamp is ISO-8601 with microseconds and an offset, and the panel
	// renders it, so the shape is part of the contract.
	if got := entries[0].CreatedAt; !strings.HasSuffix(got, "+00:00") || len(got) != 32 {
		t.Errorf("created_at is %q; the contract is `2026-08-22T01:23:45.678901+00:00`", got)
	}

	owned, err := Entries(ctx, db, &owner, "goreclaw", DefaultLimit)
	if err != nil {
		t.Fatal(err)
	}
	if len(owned) != 1 || owned[0].Actor == nil || *owned[0].Actor != actor {
		t.Fatalf("the owned tier read back as %+v", owned)
	}
}

// TestRecordNeverFailsTheEdit is the property the whole module is built
// around: the deck write has already happened by the time this runs.
func TestRecordNeverFailsTheEdit(t *testing.T) {
	t.Parallel()
	// No database at all -- a laptop with auth off that nothing has yet
	// created `app.db` on.
	missing := filepath.Join(t.TempDir(), "nothing", "app.db")
	if _, err := NewRecorder(missing, nil); err == nil {
		t.Error("opening a missing app.db read-write should fail loudly at " +
			"startup; it is Record that must stay quiet")
	}
	// ... and a nil recorder, which is what a door with no app.db holds.
	var recorder *Recorder
	recorder.Record(context.Background(), "goreclaw", nil, "", Edit{Kind: EditExile, Card: "X"})

	// A database whose table was dropped from under a live handle.
	path := newScratchDB(t)
	live, err := NewRecorder(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer live.Close()
	db, err := sql.Open("sqlite", "file:"+path+"?mode=rw")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("DROP TABLE deck_log"); err != nil {
		t.Fatal(err)
	}
	db.Close()
	// The assertion is that this returns at all.
	live.Record(context.Background(), "goreclaw", nil, "", Edit{Kind: EditAdd, Card: "X"})
}

// TestTheRecorderDoesNotCreateTheDatabase pins the one-creator rule: the
// ladder (`auth.Migrate`) runs at boot, so an `app.db` minted here would be
// a database at version zero with no tables in it.
func TestTheRecorderDoesNotCreateTheDatabase(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "app.db")
	if _, err := NewRecorder(path, nil); err == nil {
		t.Fatal("NewRecorder created app.db; only the boot ladder may make the file")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("NewRecorder left a file at %s", path)
	}
}

// newScratchUser inserts one account and hands back its id, because the log's
// owned tier is a foreign key into `users` and a test that skipped it would be
// testing an insert the real database refuses.
func newScratchUser(t *testing.T, path, username string) int64 {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+path+"?mode=rw")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	result, err := db.Exec(
		"INSERT INTO users (username, email, password_hash, created_at)"+
			" VALUES (?, ?, ?, ?)",
		username, username+"@example.invalid", "x", "2026-08-22T00:00:00+00:00")
	if err != nil {
		t.Fatalf("seeding a user: %v", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// The intake's entry (ADR 41): one row for a whole pass, naming the cards and
// never the text.
//
// The never-the-text half is the one that matters and it is structural rather
// than careful -- `Edit` has no field a rationale could travel in, so this
// tests that the sentence stays a sentence about which cards, at every size.
func TestTheIntakeEntrySaysWhichCardsAndNeverWhatWasWritten(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		edit Edit
		want string
		why  string
	}{
		{
			name: "one card", why: "the field is named in the words a sentence wants",
			edit: Edit{Kind: EditIntake, Field: "why", Cards: []string{"Sol Ring"}},
			want: "drafted the rationale on Sol Ring",
		},
		{
			name: "the filing pass", why: "the same row shape, keyed by which field",
			edit: Edit{Kind: EditIntake, Field: "category", Cards: []string{"Sol Ring", "Cultivate"}},
			want: "drafted the category on Sol Ring, Cultivate",
		},
		{
			name: "more than a handful", why: "the whole 99 must not become the log",
			edit: Edit{Kind: EditIntake, Field: "why", Cards: []string{
				"A", "B", "C", "D", "E", "F", "G", "H"}},
			want: "drafted the rationale on 8 cards: A, B, C, D, E, F, and 2 more",
		},
		{
			name: "a pass that wrote nothing", why: "silence is the one thing a history cannot have",
			edit: Edit{Kind: EditIntake, Field: "why"},
			want: "ran the intake and wrote nothing",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			action, summary := Describe(tc.edit)
			if action != "intake" {
				t.Errorf("the row is filed under %q, so it is not queryable as an "+
					"intake", action)
			}
			if summary != tc.want {
				t.Errorf("summary:\n got  %q\n want %q\n(%s)", summary, tc.want, tc.why)
			}
		})
	}
}

// The bulk edit's one entry (Lane B). It says what a pass did in counts, names
// the cards it buried, and -- the rule this file is oldest about -- never
// carries a word of what any rationale says.
func TestTheBulkEntryCountsThePassAndNamesOnlyTheBurials(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		edit Edit
		want string
		why  string
	}{
		{
			name: "the ordinary pass",
			why:  "every clause that happened, in the order somebody cares about",
			edit: Edit{Kind: EditBulk, Cards: []string{"Sol Ring", "Cultivate"},
				Bulk: &BulkTally{Added: 4, Rewrote: 6, Requantified: 2, Entombed: 2}},
			want: "rewrote the 99 from a pasted list: 4 cards added, 6 reasons " +
				"rewritten, 2 quantities changed, 2 cards entombed (Sol Ring, Cultivate)",
		},
		{
			name: "one of each",
			why:  "the singular is written out; `plural` would say 'quantitys'",
			edit: Edit{Kind: EditBulk, Cards: []string{"Sol Ring"},
				Bulk: &BulkTally{Added: 1, Rewrote: 1, Requantified: 1, Entombed: 1}},
			want: "rewrote the 99 from a pasted list: 1 card added, 1 reason " +
				"rewritten, 1 quantity changed, 1 card entombed (Sol Ring)",
		},
		{
			name: "reasons only",
			why:  "a clause for something that did not happen is noise, not a fact",
			edit: Edit{Kind: EditBulk, Bulk: &BulkTally{Rewrote: 12}},
			want: "rewrote the 99 from a pasted list: 12 reasons rewritten",
		},
		{
			name: "more burials than a panel wants",
			why:  "the whole 99 must not become the log; the same handful EditEntomb names",
			edit: Edit{Kind: EditBulk,
				Cards: []string{"A", "B", "C", "D", "E", "F", "G", "H"},
				Bulk:  &BulkTally{Entombed: 8}},
			want: "rewrote the 99 from a pasted list: 8 cards entombed " +
				"(A, B, C, D, E, F, and 2 more)",
		},
		{
			name: "no tally at all",
			why:  "silence is the one failure a history cannot have",
			edit: Edit{Kind: EditBulk},
			want: "rewrote the 99 from a pasted list",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			action, summary := Describe(tc.edit)
			if action != "bulk" {
				t.Errorf("the row is filed under %q, so it is not queryable as a "+
					"bulk edit", action)
			}
			if summary != tc.want {
				t.Errorf("summary:\n got  %q\n want %q\n(%s)", summary, tc.want, tc.why)
			}
		})
	}
}

// A bulk edit is not an intake, and the two must never be filed as one: an
// intake says a model drafted these sentences (ADR 41), and a bulk edit says a
// person typed them into a box.
func TestABulkEditIsNotFiledAsAnIntake(t *testing.T) {
	t.Parallel()
	bulk, _ := Describe(Edit{Kind: EditBulk, Bulk: &BulkTally{Rewrote: 3}})
	intake, _ := Describe(Edit{Kind: EditIntake, Field: "why", Cards: []string{"A"}})
	if bulk == intake {
		t.Fatalf("both operations file under %q, so the history cannot tell a "+
			"person's words from a drafted sentence", bulk)
	}
	_, summary := Describe(Edit{Kind: EditBulk, Bulk: &BulkTally{Rewrote: 3}})
	if strings.Contains(summary, "drafted") {
		t.Errorf("the bulk entry claims something was drafted: %q", summary)
	}
}
