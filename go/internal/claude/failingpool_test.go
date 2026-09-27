package claude

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	_ "github.com/duckdb/duckdb-go/v2" // registers "duckdb"

	"github.com/aasquier/sylvan-library/go/internal/claude/tools"
	"github.com/aasquier/sylvan-library/go/internal/pool"
)

// What every mode does when the card pool **fails a query** rather than being
// absent, which is the fault none of them had ever been driven through.
//
// The distinction is the one `internal/api`'s own failing-pool sweep is built
// on and it bites harder here, because these are the surfaces whose output a
// person reads as prose. A missing pool is a documented degraded answer: the
// deck still renders, the card facts are simply not there. A pool file DuckDB
// opens happily and then cannot answer -- a half-written refresh, a truncated
// restore, a schema older than the binary -- is a real error, and every one of
// these modes has an `if err != nil` for it that nothing had reached.
//
// What is asked is never the wording, which is DuckDB's. It is that the
// failure **comes back as a failure**. A dossier that swallowed a catalog
// error and wrote its competitors section from an empty lookup would publish
// a paragraph about Magic built on a database outage, with `answered_by:
// claude` over it and a source list under it -- which is precisely the
// blended, unfalsifiable page ADR 19 exists to prevent, arrived at from the
// other direction.

// schemalessConn leases a connection to a real DuckDB file with none of the
// pool's tables in it: `Use` succeeds, every `SELECT` fails with a catalog
// error.
//
// A corrupt file would NOT do -- `pool.Pool.Use` cannot open one, so it
// answers `ErrNoPool` and re-drives the degraded path that is already
// covered. It takes a database DuckDB is happy to open and a `SELECT` it
// cannot answer.
func schemalessConn(t *testing.T) *pool.Conn {
	t.Helper()
	path := filepath.Join(t.TempDir(), "mtg.duckdb")
	db, err := sql.Open("duckdb", path)
	if err != nil {
		t.Fatal(err)
	}
	// One table, so the file is a database rather than an empty stub -- and
	// deliberately not one of ours, so nothing resolves.
	if _, err := db.Exec(`CREATE TABLE half_a_refresh (name VARCHAR)`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	p := pool.New(path, nil)
	t.Cleanup(p.Close)

	var held *pool.Conn
	ready, done := make(chan struct{}), make(chan struct{})
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

// The premise the rest of this file rests on: the fault is a query error and
// not the answer an absent pool gives. If the two ever fold into one, every
// test below would keep passing while testing something else.
func TestASchemalessPoolFailsTheQueryRatherThanTheLease(t *testing.T) {
	t.Parallel()
	c := schemalessConn(t)
	if c == nil {
		t.Fatal("leasing a schemaless pool answered no connection")
	}
	_, err := c.GetCards(context.Background(), []string{"Sol Ring"})
	if err == nil {
		t.Fatal("a schemaless pool answered a card lookup; the fault this file " +
			"is about has stopped happening")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "catalog") {
		t.Logf("the failure is %v -- not a catalog error, which is fine as long "+
			"as it is still a failure", err)
	}
}

// The two briefs a per-card mode and the dossier are built on. Both are
// assembled from the pool *before* anything is asked of Anthropic, which is
// the good place for this to fail: nothing has been spent and the caller gets
// the fault rather than a report with holes in it.
func TestABriefOverAFailingPoolRefusesRatherThanAnsweringAnEmptyOne(t *testing.T) {
	t.Parallel()
	broken := schemalessConn(t)
	ctx := context.Background()
	mini, _ := miniDecks(t)

	if facts, err := Brief(ctx, broken, mini, "Swamp"); err == nil {
		t.Errorf("the card brief answered %v over a pool that cannot answer", facts)
	}
	if facts, err := DossierBrief(ctx, broken, "mini", mini); err == nil {
		t.Errorf("the commander brief answered %v over a pool that cannot answer", facts)
	}
	// And the free GET, which has a *different* obligation: a deck with no
	// commander the pool knows is a headless dossier rather than an error, so
	// this is the one place a query failure could plausibly be read as "no
	// commander" and must not be.
	got, err := ReadCachedDossier(ctx, broken, "mini", mini, nil, Endpoint{})
	if err == nil {
		t.Errorf("the cached GET answered %#v over a pool that cannot answer -- "+
			"a headless dossier here would say this deck has no commander", got)
	}
}

// The three modes that resolve card names *after* the model has answered.
//
// This is the expensive half and the dangerous one: the call has been made
// and paid for, the prose is in hand, and the only thing left is turning the
// names it used into pool rows. A silent failure here does not lose the
// answer -- it publishes it with every card quietly missing, which reads as
// "the model named nothing the pool has" rather than as an outage.
func TestEveryPostCallResolutionReportsAFailingPool(t *testing.T) {
	t.Parallel()
	broken := schemalessConn(t)
	ctx := context.Background()

	t.Run("the dossier's competitors", func(t *testing.T) {
		t.Parallel()
		corpus := loadDossierCorpus(t)
		mini, _ := miniDecks(t)
		refused := 0
		withPool(t, func(c *pool.Conn) {
			for _, row := range corpus.Reports {
				if row.Turn == nil {
					continue
				}
				// Planned against a pool that works, read back against one
				// that does not -- which is the shape of a refresh that broke
				// while a four-minute job was in flight.
				plan, err := CheckDossier(ctx, c, "mini", mini, DossierRequest{
					Requested: row.Requested, Refresh: true, Clock: frozenClock})
				if err != nil {
					t.Fatalf("%s: planning: %v", row.Note, err)
				}
				if _, err := readDossier(ctx, broken, plan,
					row.Turn.turn(ModeCommanderDossier)); err != nil {
					refused++
				}
			}
		})
		// A floor, not a count: the refusal rows (declined, unparseable, no
		// source) never reach a lookup and must not be expected to.
		if refused == 0 {
			t.Error("no dossier outcome noticed a pool that cannot answer a query")
		}
	})

	t.Run("research's cards", func(t *testing.T) {
		t.Parallel()
		corpus := loadResearchCorpus(t)
		refused := 0
		for _, row := range corpus.Reports {
			if row.Turn == nil {
				continue
			}
			plan, err := checkResearch(frozenClock, row.Question, row.Requested, "", nil, Endpoint{})
			if err != nil {
				t.Fatalf("%s: planning: %v", row.Note, err)
			}
			if _, err := readResearch(ctx, broken, plan, row.Turn.turn(ModeResearch)); err != nil {
				refused++
			}
		}
		if refused == 0 {
			t.Error("no research outcome noticed a pool that cannot answer a query")
		}
	})

	t.Run("the theme proposal's commanders", func(t *testing.T) {
		t.Parallel()
		corpus := loadThemeCorpus(t)
		refused := 0
		for _, row := range corpus.Proposals {
			if row.Turn == nil {
				continue
			}
			plan, err := CheckProposal(anyTranscript(corpus.Transcript), anySlots(row.Slots),
				row.Requested, row.Budget, row.Avoid, row.Persona, row.Seed, "", nil, Endpoint{})
			if err != nil {
				t.Fatalf("%s: planning: %v", row.Note, err)
			}
			if plan.Answer != nil {
				continue
			}
			who, err := GetPersona(plan.Persona)
			if err != nil {
				t.Fatal(err)
			}
			mode, err := themeMode(ModeThemeProposal, who)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := readProposal(ctx, broken, plan, who, mode.Name,
				row.Turn.turn(mode.Name)); err != nil {
				refused++
			}
		}
		if refused == 0 {
			t.Error("no proposal outcome noticed a pool that cannot answer a query")
		}
	})
}

// A tool whose handler fails for a reason that is OURS rather than the
// model's ends the turn instead of being handed back as a tool result.
//
// The distinction is `WireName`: a refusal the model can recover from
// declares one and comes back as `<Name>: <message>` for it to read and try
// again. A database that cannot answer declares nothing, and telling the
// model "that tool broke, carry on" would have it reason its way around an
// outage -- and bill for every turn it took doing so.
func TestAToolFailureWithNoWireNameEndsTheTurnRatherThanFeedingTheModel(t *testing.T) {
	t.Parallel()
	api := &scriptedAPI{replies: []string{
		reply{stop: "tool_use", in: 10, out: 5,
			content: toolUse("tu_1", "get_cards", `{"names":["Sol Ring"]}`)}.json(),
	}}
	ep := api.start(t)
	mode, err := GetMode(ModeResearch)
	if err != nil {
		t.Fatal(err)
	}
	_, err = Converse(context.Background(), mode, Request{
		Endpoint: ep,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("what is in the pool?")),
		},
		Stance:   SecondOpinion,
		Deps:     tools.Deps{Pool: schemalessConn(t)},
		MaxTurns: 4,
	})
	if err == nil {
		t.Fatal("the conversation carried on over a pool that cannot answer")
	}
	if !strings.Contains(err.Error(), mode.Name) {
		t.Errorf("the failure does not say which mode it came from: %v", err)
	}
	if api.served != 1 {
		t.Errorf("%d calls were made; the turn must end on the first failure, "+
			"not go round again", api.served)
	}
}
