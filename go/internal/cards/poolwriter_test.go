package cards_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/cards"
	"github.com/aasquier/sylvan-library/go/internal/pool"
)

// The card queries, asked of a pool written the way the real one is written.
//
// # Why this exists, when `suggest_test.go` already asks all of this
//
// Because every other test in the repo builds its pool through `pooltest`,
// which binds each row with a prepared statement and an explicit `?::JSON`
// cast. **The refresh does not.** It writes through DuckDB's Appender, and
// the Appender takes a *value*: hand it a Go string for a JSON column and
// DuckDB stores a JSON string whose contents happen to be JSON --
// `"{\"commander\":\"legal\"}"` rather than `{"commander":"legal"}`.
//
// `json_extract_string` reads NULL out of that, and that expression is
// [isCard], which is the WHERE clause on every card query in the app. The
// live library answered "no card in the library is spelled anything like
// that" to `Sol Ring` for a week, holding all 35,393 of them, because
// `count(*)` carries no predicate and so the health check reported a
// perfectly healthy pool. `mtglab cards show 'Sol Ring'` kept working
// throughout, because `GetCards` matches on `name` and never asks about
// legality -- which is exactly why the fault read as "the fuzzy matching is
// broken" rather than as "the pool is unreadable".
//
// So the fixture here is the point, not the assertions: **the rows go in
// through [pool.LoadOracle]**, the same call `mtglab data refresh` makes.
// A test that cannot fail on the wrong encoding is a test that was always
// going to be green through this, and 1,500 of them were.
func TestAPoolWrittenByTheRealLoaderAnswersTheCardQueries(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	p := pool.New(writtenByTheLoader(t), nil)
	t.Cleanup(p.Close)

	// Aaron's two, verbatim from the report: "I can't match Sol Ring still,
	// let alone Kozilek, Butcher of Truth". The comma is the second half of
	// that sentence and gets its own case below.
	for _, tc := range []struct{ typed, want, via string }{
		{"Sol Ring", "Sol Ring", "exact"},
		{"sol ring", "Sol Ring", "exact"},
		{"Sol R", "Sol Ring", "holds"},
		{"Sol Rng", "Sol Ring", "near"},
		{"Kozilek, Butcher of Truth", "Kozilek, Butcher of Truth", "exact"},
		{"kozilek", "Kozilek, Butcher of Truth", "holds"},
		{"craterhof behemoth", "Craterhoof Behemoth", "near"},
		// A double-faced card, so `card_faces` is written by the same path
		// and the front-face half of the score has something to read.
		{"Etali, Primal Conqueror", "Etali, Primal Conqueror // Etali, Primal Sickness", "exact"},
	} {
		var found []cards.Suggestion
		if err := p.Use(ctx, func(c *pool.Conn) error {
			var err error
			found, err = cards.Suggest(ctx, c, tc.typed, 8)
			return err
		}); err != nil {
			t.Fatalf("%q: %v", tc.typed, err)
		}
		if len(found) == 0 {
			t.Errorf("%q offered NOTHING from a pool that holds it -- if the "+
				"whole table answers empty, the JSON columns went in as text "+
				"and json_extract_string is reading NULL out of `legalities`",
				tc.typed)
			continue
		}
		if found[0].Name != tc.want {
			names := make([]string, 0, len(found))
			for _, s := range found {
				names = append(names, s.Name)
			}
			t.Errorf("%q offered %q first, want %q (all: %v)",
				tc.typed, found[0].Name, tc.want, names)
			continue
		}
		if found[0].Via != tc.via {
			t.Errorf("%q offered %q via %q, want %q",
				tc.typed, found[0].Name, found[0].Via, tc.via)
		}
	}
}

// And the predicate itself, asked directly, so a failure names the cause
// rather than a symptom two layers up. A banned card is a card: [isCard] is
// `IN ('legal','banned')` because a Commander deck can contain one and the
// finder's whole argument is that it offers it and marks it.
func TestTheLoaderWritesLegalitiesAsADocumentAQueryCanRead(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db, err := sql.Open("duckdb", writtenByTheLoader(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	var stored, extracted sql.NullString
	if err := db.QueryRowContext(ctx,
		`SELECT CAST(legalities AS VARCHAR),
		        json_extract_string(legalities, 'commander')
		 FROM oracle_cards WHERE name = 'Sol Ring'`).Scan(&stored, &extracted); err != nil {
		t.Fatal(err)
	}
	if !extracted.Valid {
		t.Errorf("json_extract_string(legalities,'commander') is NULL; the "+
			"column holds %s -- a JSON string containing JSON, which is what "+
			"the Appender does with a Go string", stored.String)
	}
	if extracted.String != "legal" {
		t.Errorf("Sol Ring's commander legality reads %q from %s",
			extracted.String, stored.String)
	}

	// The banned card is present and reachable through the same predicate,
	// and the count is over the whole fixture so a half-broken encoding
	// cannot hide behind one lucky row.
	var matched, total int
	if err := db.QueryRowContext(ctx,
		`SELECT count(*) FILTER (
		          WHERE json_extract_string(legalities,'commander') IN ('legal','banned')),
		        count(*)
		 FROM oracle_cards`).Scan(&matched, &total); err != nil {
		t.Fatal(err)
	}
	if matched != total {
		t.Errorf("the commander predicate matched %d of %d rows -- every card "+
			"query in the app carries this as its WHERE clause, so the "+
			"difference is a library that reports itself healthy and answers "+
			"nothing", matched, total)
	}
}

// writtenByTheLoader is the fixture: a bulk file in Scryfall's own shape,
// loaded through the production writer. Returns the pool file's path.
//
// Card facts are read out of the pool, never recalled (rule 1) --
// `mtglab cards show 'Kozilek, Butcher of Truth'`, and the same for the rest.
func writtenByTheLoader(tb testing.TB) string {
	tb.Helper()
	dir := tb.TempDir()
	bulk := filepath.Join(dir, "oracle_cards.jsonl")
	if err := os.WriteFile(bulk, []byte(bulkCards), 0o600); err != nil {
		tb.Fatal(err)
	}
	path := filepath.Join(dir, "pool.duckdb")
	db, err := pool.OpenWriter(context.Background(), path)
	if err != nil {
		tb.Fatal(err)
	}
	n, err := pool.LoadOracle(context.Background(), db, bulk)
	if err != nil {
		_ = db.Close()
		tb.Fatal(err)
	}
	if err := db.Close(); err != nil {
		tb.Fatal(err)
	}
	if n != 5 {
		tb.Fatalf("loaded %d rows, want 5", n)
	}
	return path
}

// Five cards, in Scryfall's bulk shape. Kozilek is here because Aaron named
// it and because its comma is the `words` tier's edge; Etali because a
// double-faced card exercises `card_faces`, the other JSON column the
// Appender writes.
const bulkCards = `{"oracle_id":"90f1e8e7-8b32-4e6a-9c6f-000000000001","name":"Sol Ring","mana_cost":"{1}","cmc":1,"type_line":"Artifact","oracle_text":"{T}: Add {C}{C}.","colors":[],"color_identity":[],"keywords":[],"produced_mana":["C"],"legalities":{"commander":"legal","modern":"not_legal","legacy":"banned"},"layout":"normal","reserved":false,"edhrec_rank":1,"released_at":"1993-08-05","set":"lea","scryfall_uri":"https://scryfall.com/card/lea/270","image_uris":{"normal":"https://cards.example/sol-ring.jpg","art_crop":"https://cards.example/sol-ring-crop.jpg"},"artist":"Myles Wohl"}
{"oracle_id":"90f1e8e7-8b32-4e6a-9c6f-000000000002","name":"Kozilek, Butcher of Truth","mana_cost":"{10}","cmc":10,"type_line":"Legendary Creature — Eldrazi","oracle_text":"When you cast this spell, draw four cards.\nAnnihilator 4","colors":[],"color_identity":[],"keywords":["Annihilator"],"produced_mana":[],"legalities":{"commander":"legal","modern":"legal"},"layout":"normal","reserved":false,"edhrec_rank":2300,"released_at":"2009-10-02","set":"roe","scryfall_uri":"https://scryfall.com/card/roe/1","image_uris":{"normal":"https://cards.example/kozilek.jpg"},"power":"12","toughness":"12","artist":"Michael Komarck"}
{"oracle_id":"90f1e8e7-8b32-4e6a-9c6f-000000000003","name":"Craterhoof Behemoth","mana_cost":"{5}{G}{G}{G}","cmc":8,"type_line":"Creature — Beast","oracle_text":"Haste","colors":["G"],"color_identity":["G"],"keywords":["Haste"],"produced_mana":[],"legalities":{"commander":"legal"},"layout":"normal","reserved":false,"edhrec_rank":120,"released_at":"2012-10-05","set":"avr","scryfall_uri":"https://scryfall.com/card/avr/163","image_uris":{"normal":"https://cards.example/craterhoof.jpg"},"power":"5","toughness":"5","artist":"Aleksi Briclot"}
{"oracle_id":"90f1e8e7-8b32-4e6a-9c6f-000000000004","name":"Etali, Primal Conqueror // Etali, Primal Sickness","cmc":7,"type_line":"Legendary Creature — Elder Dinosaur // Legendary Creature — Phyrexian Elder Dinosaur","colors":["R"],"color_identity":["G","R"],"keywords":["Trample","Toxic"],"produced_mana":[],"legalities":{"commander":"legal"},"layout":"transform","card_faces":[{"name":"Etali, Primal Conqueror","mana_cost":"{4}{R}{R}","type_line":"Legendary Creature — Elder Dinosaur","power":"7","toughness":"7","colors":["R"],"loyalty":null,"image_uris":{"normal":"https://cards.example/etali-front.jpg","art_crop":"https://cards.example/etali-front-crop.jpg"}},{"name":"Etali, Primal Sickness","mana_cost":"","type_line":"Legendary Creature — Phyrexian Elder Dinosaur","power":"11","toughness":"11","colors":["G"],"image_uris":{"normal":"https://cards.example/etali-back.jpg"}}],"reserved":false,"edhrec_rank":600,"released_at":"2023-04-21","set":"mom","scryfall_uri":"https://scryfall.com/card/mom/163"}
{"oracle_id":"90f1e8e7-8b32-4e6a-9c6f-000000000005","name":"Black Lotus","mana_cost":"{0}","cmc":0,"type_line":"Artifact","oracle_text":"{T}, Sacrifice this artifact: Add three mana of any one color.","colors":[],"color_identity":[],"keywords":[],"produced_mana":["W","U","B","R","G"],"legalities":{"commander":"banned","vintage":"restricted"},"layout":"normal","reserved":true,"edhrec_rank":null,"released_at":"1993-08-05","set":"lea","scryfall_uri":"https://scryfall.com/card/lea/232","image_uris":{"normal":"https://cards.example/black-lotus.jpg"},"artist":"Christopher Rush"}
`
