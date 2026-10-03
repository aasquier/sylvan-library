package api

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/cards"
	"github.com/aasquier/sylvan-library/go/internal/pool"
	"github.com/aasquier/sylvan-library/go/internal/pool/pooltest"
)

// The pool that **moves between two queries**, and the three routes that say so.
//
// `pooltest.OpenFaultyPool` is the library that stops answering: every read
// after the budget fails, and the reader's job is to refuse rather than shorten.
// This file is the other half of the same volume fault and the half no fixture
// in the tree could reach: the pool keeps answering perfectly, and the cards it
// answered about a moment ago are **gone**. Three reads in this package shortlist
// names out of `oracle_cards` and then hydrate them in a second query, and each
// one carries the same four-line comment at the same arm —
//
//	// The pool moved under the shortlist between the two queries.
//	// Dropped rather than half-rendered: a row with no card
//	// behind it is a row somebody would click.
//
// — `suggest`, `commanderNearby` and `identifyAgainst`. Not one of those arms had
// ever run. They matter for the reason the comments give and for commandment 2
// besides: the alternative to dropping a name is a tile with no card behind it,
// and a newcomer who clicks one is told their own typing was the problem.
//
// **The fault is a deploy state, not a contrivance.** A pool file is rebuilt
// table by table — the schema first, the rows appended after — and the volume it
// lives on is the same one the app reads under a lease (ADR 23). A lease that
// spans a swap for a file whose oracle pass has not landed yet reads a full
// shortlist and then hydrates it against an empty table, which is this fixture
// exactly: the first query answers, the table empties, the second answers
// honestly about nothing.

// batchLookup is the opening of [pool.Conn.GetCards]' query, which is the one
// statement in the tree that hydrates a shortlist. The mover below watches for
// it rather than counting statements, so that a read which grows a query keeps
// its fixture: a marker that stopped matching leaves `dropped` at nought and
// fails the test by name, where a counted budget would quietly move the fault
// somewhere else and stay green.
const batchLookup = "WITH wanted(w)"

// mover is the shared state behind a moving pool: how many hydrations to let
// past untouched, and whether the cards have gone yet.
type mover struct {
	past  atomic.Int64
	moved atomic.Bool
}

// movingPoolOver is a [pool.Pool] over a pool file the caller built, whose
// `oracle_cards` rows are deleted in front of the (past+1)'th batch card
// lookup. Everything before that answers out of the whole fixture.
//
// `past` is not a convenience: `commanderCheck` hydrates the name it was asked
// about before it ever builds a shortlist, so the fault has to land on the
// second hydration there and on the first everywhere else.
func movingPoolOver(tb testing.TB, path string, past int) *pool.Pool {
	tb.Helper()
	m := &mover{}
	m.past.Store(int64(past))
	p := pool.NewOver(path, nil, func(context.Context, string) (*sql.DB, error) {
		return movingHandle(path, m)
	})
	tb.Cleanup(p.Close)
	return p
}

func movingHandle(path string, m *mover) (*sql.DB, error) {
	// The driver is borrowed off a throwaway handle rather than named, exactly
	// as `pooltest`'s faulty handle borrows it: `sql.OpenDB` wants a connector.
	plain, err := sql.Open("duckdb", path)
	if err != nil {
		return nil, fmt.Errorf("moving pool: %w", err)
	}
	inner := plain.Driver()
	if err := plain.Close(); err != nil {
		return nil, fmt.Errorf("moving pool: %w", err)
	}
	db := sql.OpenDB(&movingConnector{dsn: path, driver: inner, mover: m})
	// One connection, so the delete and the query it lands in front of are the
	// same session and the order is the order this file wrote down.
	db.SetMaxOpenConns(1)
	return db, nil
}

type movingConnector struct {
	dsn    string
	driver driver.Driver
	mover  *mover
}

func (c *movingConnector) Driver() driver.Driver { return c.driver }

func (c *movingConnector) Connect(context.Context) (driver.Conn, error) {
	real, err := c.driver.Open(c.dsn)
	if err != nil {
		return nil, err
	}
	// The same three calls `pooltest`'s faulty connector forwards, checked for
	// the same reason: without the context forms `database/sql` takes the
	// prepared-statement road, where nothing here watches for the hydration,
	// and without the named-value check a Go `[]string` bound as `?::VARCHAR[]`
	// is refused before any query runs.
	exec, execOK := real.(driver.ExecerContext)
	query, queryOK := real.(driver.QueryerContext)
	check, checkOK := real.(driver.NamedValueChecker)
	if !execOK || !queryOK || !checkOK {
		_ = real.Close()
		return nil, fmt.Errorf("api: %T does not speak the driver calls a "+
			"moving pool is built on", real)
	}
	return &movingConn{real: real, exec: exec, query: query, check: check,
		mover: c.mover}, nil
}

type movingConn struct {
	real  driver.Conn
	exec  driver.ExecerContext
	query driver.QueryerContext
	check driver.NamedValueChecker
	mover *mover
}

func (c *movingConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("api: a moving pool prepares nothing; use the context forms")
}

func (c *movingConn) Begin() (driver.Tx, error) {
	return nil, errors.New("api: a moving pool begins nothing")
}

func (c *movingConn) Close() error { return c.real.Close() }

func (c *movingConn) CheckNamedValue(v *driver.NamedValue) error {
	return c.check.CheckNamedValue(v)
}

func (c *movingConn) ExecContext(ctx context.Context, query string,
	args []driver.NamedValue) (driver.Result, error) {
	return c.exec.ExecContext(ctx, query, args)
}

func (c *movingConn) QueryContext(ctx context.Context, query string,
	args []driver.NamedValue) (driver.Rows, error) {
	if strings.Contains(query, batchLookup) && c.mover.past.Add(-1) < 0 &&
		c.mover.moved.CompareAndSwap(false, true) {
		if _, err := c.exec.ExecContext(ctx, "DELETE FROM oracle_cards", nil); err != nil {
			return nil, err
		}
	}
	return c.query.QueryContext(ctx, query, args)
}

// ---- the camera ------------------------------------------------------------

// **A reading whose card has gone is counted, never rendered.** The camera's
// engine reads the corner out of `printings` and the title out of
// `oracle_cards`, then hydrates both in one lookup; a name that no longer
// resolves is dropped and added to `dropped`, which is the one number on that
// answer whose whole job is to say "the library moved while we were looking".
// The two arms that do the dropping sit either side of the resolved/offered
// split and neither had been entered.
func TestTheCameraCountsTheNamesThePoolLostBetweenTheReadingAndTheHydration(t *testing.T) {
	t.Parallel()

	// `lea` 269 is Sol Ring in the recorded fixture, and the corner tier
	// resolves it because every printing of that number in that set names the
	// one card. The second sighting carries only a title, so it offers a
	// shortlist instead — one reading down each road of the engine.
	sightings := []cards.Sighting{
		{SetCode: "lea", CollectorNumber: "269"},
		{Title: "Sol Ring"},
	}

	// The whole answer first, so "dropped" has a number to be measured against.
	var whole identifyAnswer
	if err := pooltest.Open(t).Use(t.Context(), func(c *pool.Conn) error {
		var err error
		whole, err = identifyAgainst(t.Context(), c, sightings)
		return err
	}); err != nil {
		t.Fatalf("the healthy camera: %v", err)
	}
	if whole.Resolved != 1 || whole.Offered != 1 || whole.Dropped != 0 {
		t.Fatalf("the fixture reads %+v -- this test needs one corner resolved "+
			"and one title offered, with nothing dropped", whole)
	}
	offered := len(whole.Readings[1].Candidates)
	if offered < 1 {
		t.Fatalf("the title tier offered nothing; there is no shortlist to lose")
	}

	var got identifyAnswer
	p := movingPoolOver(t, pooltest.Build(t), 0)
	if err := p.Use(t.Context(), func(c *pool.Conn) error {
		var err error
		got, err = identifyAgainst(t.Context(), c, sightings)
		return err
	}); err != nil {
		t.Fatalf("a pool that moved mid-read refused the whole reading: %v", err)
	}
	// Every name the engine had is counted as lost: the resolved one and each
	// candidate of the shortlist.
	if got.Dropped != 1+offered {
		t.Errorf("the camera lost %d names and counted %d", 1+offered, got.Dropped)
	}
	if got.Resolved != 0 || got.Offered != 0 || got.Unread != 2 {
		t.Errorf("a reading of nothing was reported as %+v", got)
	}
	for i, rd := range got.Readings {
		if rd.Resolved != nil || len(rd.Candidates) != 0 || rd.Via != "nothing" {
			t.Errorf("reading %d still renders a card the pool no longer has: %+v", i, rd)
		}
	}
}

// ---- the typeahead and the commander field ---------------------------------

// **The typeahead offers no tile it cannot put a card behind.** `suggest`
// shortlists names and then hydrates them, and a name that vanished in between
// is dropped: the alternative is an offer in the list with no mana cost, no
// type line and no picture, which is a thing somebody clicks.
func TestTheTypeaheadOffersNothingThePoolLostUnderTheShortlist(t *testing.T) {
	t.Parallel()

	a := New(Config{Logger: quietLogger(), Pool: movingPoolOver(t, pooltest.Build(t), 0)})
	status, payload, raw := call(t, a, "GET", "/api/cards/suggest?q=sol&limit=5", "")
	if status != http.StatusOK {
		t.Fatalf("the typeahead refused a pool that answered it: %d %s", status, raw)
	}
	offers, isList := payload["cards"].([]any)
	if !isList {
		t.Fatalf("the typeahead answered without a list of cards: %s", raw)
	}
	if len(offers) != 0 {
		t.Errorf("the typeahead offered %d cards it has nothing behind: %s",
			len(offers), raw)
	}
}

// **The commander field's "did you mean" offers no seat it cannot fill.** The
// field hydrates the name it was asked about, finds nothing, builds a shortlist
// — and this is where the pool goes, so every candidate on that shortlist is a
// name with no card behind it. A seat rendered from one would carry no colours
// and no legality, which is the field telling somebody a real commander cannot
// lead.
func TestTheCommanderFieldSeatsNobodyThePoolLostUnderItsShortlist(t *testing.T) {
	t.Parallel()

	// One hydration let past — the field's own lookup of the misspelling — and
	// the fault lands on the shortlist's.
	a := New(Config{Logger: quietLogger(), Pool: movingPoolOver(t, pooltest.Build(t), 1)})
	status, payload, raw := call(t, a, "GET", "/api/cards/commander?q=Sol+Rng", "")
	if status != http.StatusOK {
		t.Fatalf("the commander field refused a pool that answered it: %d %s", status, raw)
	}
	if payload["state"] != "unknown" {
		t.Fatalf("a name no card carries was not called unknown: %s", raw)
	}
	offers, isList := payload["did_you_mean"].([]any)
	if !isList || len(offers) != 1 {
		t.Fatalf("the field answered without one shortlist: %s", raw)
	}
	offer, isObject := offers[0].(map[string]any)
	if !isObject {
		t.Fatalf("the shortlist is not an offer: %s", raw)
	}
	seats, isList := offer["candidates"].([]any)
	if !isList {
		t.Fatalf("the offer carries no candidate list: %s", raw)
	}
	if len(seats) != 0 {
		t.Errorf("the field seated %d candidates it has no cards for: %s", len(seats), raw)
	}
}
