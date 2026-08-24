package tools

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/library"
	"github.com/aasquier/sylvan-library/go/internal/pool"
	"github.com/aasquier/sylvan-library/go/internal/pool/pooltest"
)

// The tool handlers themselves, run rather than only registered.
//
// `tools_test.go` holds the structural guards -- no write function exists, a
// mode cannot widen its own tool set, no schema accepts a `why` (ADR 8: no
// path passes a model response into a deck's rationale). What this holds is
// what happens when a tool actually runs, and the two degraded shapes that
// decide whether the model can recover.
//
// The distinction that matters: **a missing library is a refusal and a
// missing card pool is not**. A deck answered without a pool is still a deck,
// and `pool_available` says which happened -- so a laptop with no pool can
// still hold a conversation about a deck's shape. Only the two card tools
// require one, because a card lookup with no pool has nothing to say.
//
// The other is that every refusal **names the slug**. What the model reads is
// `DeckNotFound: gyome`, and its recovery is to call `list_decks` and ask
// again -- which it can only do if it was told which name failed.

// deps builds the tool dependencies over a real file library and, optionally,
// the tiny pool.
func deps(t *testing.T, withPool bool, slugs ...string) Deps {
	t.Helper()
	root := t.TempDir()
	for _, slug := range slugs {
		dir := filepath.Join(root, slug)
		if err := os.MkdirAll(dir, 0o750); err != nil {
			t.Fatal(err)
		}
		text := "slug: " + slug + "\nname: " + slug + "\nstatus: draft\n" +
			"commander:\n  - Sol Ring\ncards:\n  - name: Forest\n    why: a land\n"
		if err := os.WriteFile(filepath.Join(dir, "deck.yaml"), []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	out := Deps{Source: library.NewFileSource(root, false)}
	if withPool {
		out.Pool = leasedConn(t)
	}
	return out
}

// leasedConn holds one pool connection open for the length of the test, the
// way a Claude conversation holds one for the length of a turn.
func leasedConn(t *testing.T) *pool.Conn {
	t.Helper()
	p := pooltest.Open(t)
	var held *pool.Conn
	ready := make(chan struct{})
	done := make(chan struct{})
	go func() {
		err := p.Use(context.Background(), func(c *pool.Conn) error {
			held = c
			close(ready)
			<-done
			return nil
		})
		if err != nil {
			panic(err)
		}
	}()
	<-ready
	t.Cleanup(func() { close(done) })
	return held
}

// run dispatches a tool the way `converse` does.
func run(t *testing.T, name string, args map[string]any, d Deps) (any, error) {
	t.Helper()
	return Run(context.Background(), name, args, d, nil)
}

// Every deck tool answers without a card pool, because a deck answered
// without one is still a deck. This is the laptop's shape and the shape a
// fresh instance has before its first refresh.
func TestEveryDeckToolAnswersWithoutACardPool(t *testing.T) {
	t.Parallel()
	d := deps(t, false, "gyome")

	if out, err := run(t, "list_decks", map[string]any{}, d); err != nil || out == nil {
		t.Errorf("list_decks refused an instance with no pool: %v", err)
	}
	for _, name := range []string{"get_deck", "validate_deck", "deck_stats"} {
		out, err := run(t, name, map[string]any{"slug": "gyome"}, d)
		if err != nil {
			t.Errorf("%s refused an instance with no pool: %v", name, err)
			continue
		}
		if out == nil {
			t.Errorf("%s answered nothing", name)
		}
	}

	// The suggestion tool says so in the payload rather than refusing: the
	// model needs to know the difference between "no replacements" and "I
	// could not look".
	out, err := run(t, "suggest_replacements", map[string]any{"slug": "gyome"}, d)
	if err != nil {
		t.Fatalf("suggest_replacements refused: %v", err)
	}
	payload, ok := out.(map[string]any)
	if !ok {
		t.Fatalf("suggest_replacements answered %T", out)
	}
	if payload["pool_available"] != false {
		t.Errorf("pool_available is %v on an instance with no pool", payload["pool_available"])
	}
	if payload["slug"] != "gyome" {
		t.Errorf("the answer names %v", payload["slug"])
	}
	targets, ok := payload["targets"].([]any)
	if !ok || len(targets) != 0 {
		t.Errorf("targets is %#v, want an empty list -- the model iterates it", payload["targets"])
	}
}

// The two card tools DO require a pool, because a card lookup with no pool
// has nothing to say -- and they say that rather than answering an empty list
// the model would read as "no such card".
func TestTheCardToolsRefuseWithoutAPool(t *testing.T) {
	t.Parallel()
	d := deps(t, false)

	for _, tc := range []struct {
		name string
		args map[string]any
	}{
		{"get_cards", map[string]any{"names": []any{"Sol Ring"}}},
		{"search_cards", map[string]any{"q": "ramp"}},
	} {
		out, err := run(t, tc.name, tc.args, d)
		if err == nil {
			t.Errorf("%s answered %v with no pool -- the model would read that "+
				"as 'no such card'", tc.name, out)
			continue
		}
		if !strings.Contains(err.Error(), "card pool is not available") {
			t.Errorf("%s said %q", tc.name, err)
		}
	}
}

// With a pool, the card tools answer real cards -- and rule 1 lives here:
// what the model is handed is what the pool holds, never what it remembers.
func TestTheCardToolsAnswerFromThePool(t *testing.T) {
	t.Parallel()
	d := deps(t, true, "gyome")

	out, err := run(t, "get_cards", map[string]any{"names": []any{"Sol Ring"}}, d)
	if err != nil {
		t.Fatalf("get_cards: %v", err)
	}
	if out == nil {
		t.Fatal("get_cards answered nothing")
	}

	// A name list with non-strings in it drops them rather than failing the
	// whole lookup: the model is generating this argument.
	if _, err := run(t, "get_cards",
		map[string]any{"names": []any{"Sol Ring", 7, nil}}, d); err != nil {
		t.Errorf("a mixed name list failed the lookup: %v", err)
	}
	// `names` is required, and the check happens before dispatch -- so a
	// model that forgets it is told which argument is missing rather than
	// being handed an empty answer it would read as "no such card".
	_, err = run(t, "get_cards", map[string]any{}, d)
	if err == nil {
		t.Error("get_cards answered with no names at all")
	} else if !strings.Contains(err.Error(), "names") {
		t.Errorf("the refusal does not name the argument: %q", err)
	}

	if _, err := run(t, "search_cards", map[string]any{"q": "Forest"}, d); err != nil {
		t.Errorf("search_cards: %v", err)
	}
}

// **Every refusal names the slug**, because the model's recovery is to call
// `list_decks` and ask again, and it can only do that if it is told which
// name failed. The message is bare -- `converse` prefixes the class name, so
// what the model reads is `DeckNotFound: gyome`.
func TestADeckThatIsNotThereIsRefusedByName(t *testing.T) {
	t.Parallel()
	d := deps(t, true, "gyome")

	for _, name := range []string{"get_deck", "validate_deck", "deck_stats", "suggest_replacements"} {
		_, err := run(t, name, map[string]any{"slug": "no-such-deck"}, d)
		if err == nil {
			t.Errorf("%s answered for a deck that is not there", name)
			continue
		}
		var missing *ErrDeckNotFound
		if !errors.As(err, &missing) {
			t.Errorf("%s refused with %T, want *ErrDeckNotFound", name, err)
			continue
		}
		if missing.Slug != "no-such-deck" {
			t.Errorf("%s named %q", name, missing.Slug)
		}
		// Bare: converse adds the class name, and a second copy would read
		// as `DeckNotFound: DeckNotFound: x`.
		if strings.Contains(err.Error(), "DeckNotFound") {
			t.Errorf("%s's message already carries its class: %q", name, err)
		}
	}

	// An omitted slug is caught **before** dispatch, and that is a
	// different and better refusal: a misspelled name needs `list_decks`,
	// while a missing argument needs the schema. The model is told which.
	_, err := run(t, "get_deck", map[string]any{}, d)
	if err == nil {
		t.Fatal("get_deck answered with no slug at all")
	}
	var missing *ErrDeckNotFound
	if errors.As(err, &missing) {
		t.Error("an omitted argument was reported as a missing deck")
	}
	if !strings.Contains(err.Error(), "slug") {
		t.Errorf("the refusal does not name the argument: %q", err)
	}
}

// A surface with no library at all refuses every deck tool -- this is the
// research mode's shape, where ADR 26 says the model cannot see a deck.
func TestASurfaceWithNoLibraryRefusesEveryDeckTool(t *testing.T) {
	t.Parallel()
	empty := Deps{}

	for _, name := range []string{"list_decks", "get_deck", "validate_deck",
		"deck_stats", "suggest_replacements"} {
		args := map[string]any{"slug": "gyome"}
		if name == "list_decks" {
			args = map[string]any{}
		}
		_, err := run(t, name, args, empty)
		if err == nil {
			t.Errorf("%s answered on a surface with no library", name)
			continue
		}
		var refused *ErrNotAllowed
		if !errors.As(err, &refused) {
			t.Errorf("%s refused with %T, want *ErrNotAllowed", name, err)
		}
		if !strings.Contains(err.Error(), "no deck library is reachable") {
			t.Errorf("%s said %q", name, err)
		}
	}
}

// The validation tool hands back the gate's own verdict in three parts, so
// the model can tell "this deck is fine" from "this deck has warnings" --
// which is the difference between advising a change and reporting a problem
// (ADR 14: the gate is reproducible, an opinion is not).
func TestTheValidationToolReportsTheGatesOwnVerdict(t *testing.T) {
	t.Parallel()
	d := deps(t, true, "gyome")

	out, err := run(t, "validate_deck", map[string]any{"slug": "gyome"}, d)
	if err != nil {
		t.Fatalf("validate_deck: %v", err)
	}
	report, ok := out.(map[string]any)
	if !ok {
		t.Fatalf("validate_deck answered %T", out)
	}
	if _, present := report["ok"]; !present {
		t.Error("the verdict has no `ok`")
	}
	if _, ok := report["ok"].(bool); !ok {
		t.Errorf("`ok` is %T, not a verdict", report["ok"])
	}
	for _, key := range []string{"errors", "warnings"} {
		if _, present := report[key]; !present {
			t.Errorf("the verdict has no %q", key)
		}
	}
}

// The listing answers a list even when the library is empty, because the
// model iterates it -- and an empty library is a real state on a fresh
// instance.
func TestListingAnEmptyLibraryAnswersAList(t *testing.T) {
	t.Parallel()
	out, err := run(t, "list_decks", map[string]any{}, deps(t, true))
	if err != nil {
		t.Fatalf("list_decks: %v", err)
	}
	if out == nil {
		t.Fatal("an empty library listed nothing at all")
	}
}

// The suggestion limit is the model's to set, and a nonsense one falls back
// rather than becoming zero -- a limit of zero would answer no targets and
// read as "there is nothing to improve".
func TestTheSuggestionLimitFallsBackRatherThanBecomingZero(t *testing.T) {
	t.Parallel()
	d := deps(t, true, "gyome")

	for _, tc := range []struct {
		name string
		args map[string]any
	}{
		{"absent", map[string]any{"slug": "gyome"}},
		{"zero", map[string]any{"slug": "gyome", "limit": float64(0)}},
		{"negative", map[string]any{"slug": "gyome", "limit": float64(-5)}},
		{"the wrong type", map[string]any{"slug": "gyome", "limit": "five"}},
		{"a real limit", map[string]any{"slug": "gyome", "limit": float64(2)}},
	} {
		out, err := run(t, "suggest_replacements", tc.args, d)
		if err != nil {
			t.Errorf("%s: %v", tc.name, err)
			continue
		}
		payload, _ := out.(map[string]any)
		if payload["pool_available"] != true {
			t.Errorf("%s: pool_available is %v with a pool", tc.name, payload["pool_available"])
		}
		if payload["targets"] == nil {
			t.Errorf("%s: targets is nil -- the model iterates it", tc.name)
		}
	}
}

// `conn` pulls the pool out of untyped Deps, and anything that is not a
// pool connection is no pool rather than a panic -- Deps is `any` on both
// fields, so this is the only guard.
func TestTheDependencyAccessorsRefuseToPanicOnTheWrongType(t *testing.T) {
	t.Parallel()
	for _, bad := range []any{nil, "a string", 7, struct{}{}, (*pool.Conn)(nil)} {
		if got := conn(Deps{Pool: bad}); got != nil && bad != nil {
			if _, isConn := bad.(*pool.Conn); !isConn {
				t.Errorf("%#v read as a pool connection", bad)
			}
		}
		if _, err := source(Deps{Source: bad}); err == nil {
			t.Errorf("%#v read as a deck library", bad)
		}
	}
}
