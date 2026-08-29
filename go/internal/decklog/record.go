package decklog

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// The write side of ADR 28, and its three argued properties.
//
// **Record never returns an error.** A history that can fail an edit is worse
// than no history: the deck write has already happened by the time this is
// called, so a failure here would report a failure for work that succeeded. A
// failed write is a logged warning, and nothing more.
//
// **One call site.** The commit path is the single place every deck write
// passes through, so "an edit nobody logged" is not something a new route can
// produce by forgetting.
//
// **No rationale text ever lands here.** Describe builds its sentence out of
// card names, categories and field *names*; where a swap carries the `why` the
// user typed, it is dropped. The log records that a rationale changed and
// never what it says. Rule 4's text lives in `deck.yaml`, and a second copy in
// a table nobody edits would go stale and would be a place a rationale could
// be read back out of by something that is not allowed to write one.
//
// One decision worth stating: Record never **creates** `app.db`. The
// ladder (`auth.Migrate`) runs once at the serving command's boot and is
// the only creator, so by the time an edit reaches this handle the file
// exists — a missing one means the process was assembled without the
// ladder, which is exactly the loud, warned drop this stays. The Phase 4
// gap this note used to record (a laptop whose `app.db` nothing had ever
// created) closed when the boot migration landed.

// Recorder writes entries. It is a struct rather than a bare function because
// the read-write handle is opened once and shared, and because a nil Recorder
// is the honest representation of "this instance has no app.db" -- which
// answers by warning, as a broken write does.
type Recorder struct {
	db  *sql.DB
	log *slog.Logger
}

// NewRecorder opens `app.db` read-write for the log's inserts.
//
// `mode=rw` and not `rwc`: see the note above. The busy timeout matches
// the auth side's (5000ms) so two writers collide as a short wait rather
// than as an error, and foreign keys are on because SQLite keeps that per
// connection and `deck_log` has one.
func NewRecorder(path string, logger *slog.Logger) (*Recorder, error) {
	db, err := openReadWrite(path)
	if err != nil {
		return nil, err
	}
	// `sql.Open` connects lazily, so the missing file and the missing table
	// both surface here or not at all. They surface **here on purpose**:
	// Record must never fail an edit, so the loud failure belongs at startup,
	// where a wrong MTGLAB_DATA_DIR is a refusal to boot rather than a warning
	// on every write.
	if err := ping(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Recorder{db: db, log: logger}, nil
}

// DB is the read-write handle, shared with the SQL deck tier so one process
// keeps one writer to `app.db` rather than two pools racing for its write
// lock. Nil on a Recorder that never opened one.
func (r *Recorder) DB() *sql.DB {
	if r == nil {
		return nil
	}
	return r.db
}

// Close releases the handle.
func (r *Recorder) Close() error {
	if r == nil || r.db == nil {
		return nil
	}
	return r.db.Close()
}

// Record writes one entry, and never fails the edit that produced it.
//
// `ownerID` is nil for the file-backed curated library, of which there is one
// per instance -- not the owner segment out of the URL, which is `local` on a
// laptop and a username deployed, so a history keyed on it would split in two
// the day `MTGLAB_ADMIN_EMAIL` was set.
//
// `actor` is a username, or empty for whoever is at this machine. Never an
// email address, which must not reach a log line at all (CLAUDE.md, rule 5).
func (r *Recorder) Record(ctx context.Context, slug string, ownerID *int64, actor string, edit Edit) {
	action, summary := Describe(edit)
	if r == nil || r.db == nil {
		slog.Default().Warn("deck log record dropped: no app.db",
			"slug", slug, "action", action)
		return
	}
	var owner any
	if ownerID != nil {
		owner = *ownerID
	}
	var who any
	if actor != "" {
		who = actor
	}
	_, err := r.db.ExecContext(ctx,
		"INSERT INTO deck_log (created_at, owner_id, slug, actor, action, summary)"+
			" VALUES (?, ?, ?, ?, ?, ?)",
		now(), owner, slug, who, action, summary)
	if err != nil {
		r.log.Warn("deck log record failed", "slug", slug, "action", action, "err", err)
	}
}

func now() string {
	// The recorded stamp shape is `2026-08-22T01:23:45.678901+00:00` --
	// microseconds, and an offset rather than a `Z`. The column is text and
	// the panel renders it, so the shape is part of the contract.
	//
	// One hair of licence, taken rather than chased: a recorded stamp may
	// omit the microseconds entirely when they are exactly zero, and this
	// always writes six digits. Both are ISO-8601 and both parse; the case
	// arrives about once in a million entries.
	return time.Now().UTC().Format("2006-01-02T15:04:05.000000-07:00")
}

// EditKind names which of `Describe`'s branches an operation takes --
// stated as a shape rather than sniffed from which fields are set, because
// a zero value cannot tell an absent field from an empty one, and clearing
// a deck field to empty is a real edit this has to report as one.
type EditKind string

const (
	EditAdd     EditKind = "add"
	EditEntomb  EditKind = "entomb"
	EditRemove  EditKind = "remove"
	EditReturn  EditKind = "return"
	EditExile   EditKind = "exile"
	EditSwap    EditKind = "swap"
	EditNote    EditKind = "note"
	EditSetCard EditKind = "set-card"
	// The intake's write (ADR 41). One entry for a whole pass rather than one
	// per card: the pass is the thing that happened, and ninety-nine rows
	// saying "drafted a rationale" would bury the history somebody actually
	// comes here to read.
	//
	// It records WHICH cards and never WHAT was written, which is this file's
	// oldest rule (ADR 28) and matters more here than anywhere else: the text
	// is a rationale, and a log that carried rationale text would be the
	// undo-by-transcript this log exists not to be.
	EditIntake  EditKind = "intake"
	EditSetDeck EditKind = "set-deck"
)

// Edit is one operation's own description -- the keywords `_commit(**extra)`
// has always assembled and thrown away with the response.
type Edit struct {
	Kind EditKind

	Card     string   // the card an operation names
	Cards    []string // the bulk sweep's names
	Category string   // what an added card was filed under
	Into     string   // "cards" or "swap_board"
	SwapIn   string   // the card a swap brings in
	Field    string   // which field a set operation touched
	Value    any      // and what it was set to; never a rationale
	Note     string   // which note changed
}

// fieldWords are the field names `SetCardField` and `SetDeckField` accept, in
// the words a sentence wants. Anything absent falls through as itself, so a
// field added to the editor reads acceptably here before anybody remembers
// this table.
var fieldWords = map[string]string{
	"why":           "rationale",
	"qty":           "quantity",
	"commander_art": "commander art",
}

// Describe turns one operation into a verb and a sentence.
//
// Two rules, both load-bearing. The sentence is rendered **here**, once, rather
// than at read time: the CLI and the deck panel would otherwise be two
// renderers of the same row in two languages, and they would drift. And the
// verb is returned beside it so the row stays queryable without parsing prose.
//
// The fallback is `("edit", "edited the deck")` rather than nothing, which is
// the load-bearing case: an operation whose shape this has never seen is an
// operation somebody added, and the log must say it happened even when it
// cannot say what it was. Silence is the one failure mode a history cannot
// have.
func Describe(e Edit) (action, summary string) {
	switch e.Kind {
	case EditAdd:
		where := ""
		if e.Into == "swap_board" {
			where = " on the swap board"
		}
		of := ""
		if e.Category != "" {
			of = " as " + e.Category
		}
		return "add", fmt.Sprintf("added %s%s%s", e.Card, of, where)

	case EditEntomb:
		if e.Cards == nil {
			return "entomb", "entombed " + e.Card
		}
		// The bulk sweep. Named in full up to a handful, because "entombed 12
		// cards" is the entry somebody comes to this panel to expand.
		if len(e.Cards) > 6 {
			return "entomb", fmt.Sprintf("entombed %s: %s, and %d more",
				plural(len(e.Cards), "card"), strings.Join(e.Cards[:6], ", "),
				len(e.Cards)-6)
		}
		return "entomb", "entombed " + strings.Join(e.Cards, ", ")

	case EditRemove:
		return "remove", "removed " + e.Card + " from the swap board"
	case EditReturn:
		return "return", "returned " + e.Card + " from the graveyard"
	case EditExile:
		return "exile", "exiled " + e.Card + " from the graveyard"

	case EditSwap:
		// The `why` this operation carries is the one thing not carried
		// across. See the note at the top of this file.
		in := e.SwapIn
		if in == "" {
			in = "another card"
		}
		return "swap", fmt.Sprintf("swapped %s out for %s", e.Card, in)

	case EditNote:
		return "note", "changed the " + e.Note + " note"

	case EditIntake:
		// `Field` carries which pass this was -- `why` or `category` -- in the
		// same slot `EditSetCard` uses, so the row stays queryable by field.
		word := fieldWord(e.Field)
		if word == "" {
			word = "entry"
		}
		if len(e.Cards) == 0 {
			return "intake", "ran the intake and wrote nothing"
		}
		if len(e.Cards) > 6 {
			return "intake", fmt.Sprintf("drafted the %s on %s: %s, and %d more",
				word, plural(len(e.Cards), "card"), strings.Join(e.Cards[:6], ", "),
				len(e.Cards)-6)
		}
		return "intake", fmt.Sprintf("drafted the %s on %s",
			word, strings.Join(e.Cards, ", "))

	case EditSetCard:
		word := fieldWord(e.Field)
		if word == "" {
			word = "entry"
		}
		return "set-card", fmt.Sprintf("changed the %s for %s", word, e.Card)

	case EditSetDeck:
		word := fieldWord(e.Field)
		text := valueText(e.Value)
		// Emptiness is checked first, and that ordering is the whole of this
		// branch: clearing the art back to the default printing is a real
		// edit, and the art rule below would have reported it as a change to
		// some new picture nobody chose.
		if text == "" {
			return "set-deck", "cleared the " + word
		}
		// An art id is a Scryfall UUID. Nobody reads one, and printing it
		// would be the longest entry in the panel saying the least.
		if e.Field == "commander_art" || e.Field == "art" || len([]rune(text)) > 40 {
			return "set-deck", "changed the " + word
		}
		return "set-deck", fmt.Sprintf("set %s to %s", word, text)
	}

	return "edit", "edited the deck"
}

func fieldWord(field string) string {
	if word, ok := fieldWords[field]; ok {
		return word
	}
	return field
}

// valueText renders a set value for the sentence. A list is joined rather
// than dumped, because `themes` is a list and a container's default
// rendering would put brackets and quotes in a sentence people read.
func valueText(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(v)
	case []string:
		return strings.Join(v, ", ")
	case []any:
		parts := make([]string, 0, len(v))
		for _, item := range v {
			parts = append(parts, fmt.Sprint(item))
		}
		return strings.Join(parts, ", ")
	default:
		return strings.TrimSpace(fmt.Sprint(v))
	}
}

func plural(n int, word string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, word)
	}
	return fmt.Sprintf("%d %ss", n, word)
}
