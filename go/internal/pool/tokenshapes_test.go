package pool

import (
	"context"
	"database/sql"
	"strings"
	"testing"
)

// The token sheet against pools that are not the pool this binary expects.
//
// **Every one of these shapes is a real deploy state**, which is why they are
// worth a fixture rather than an inspection. Merging is deploying (ADR 23), so
// a binary carrying a new column meets a library built before it; a rebuild
// that died mid-flight leaves one table loaded and another empty; a restore
// from an old snapshot is a schema several rungs back. `TokensMade` already
// has two deliberate "nobody has looked yet" answers for the shapes it can
// recognise, and the point of these is the other side of that line: a pool
// that **fails the query** must say so, because the sheet's own empty answer
// reads as *this deck makes nothing*, and telling somebody that about a deck
// full of Food is worse than telling them nothing at all.
//
// The fixtures are hand-built tables rather than doctored copies of the tiny
// pool, because the subject is the *shape of the pool* and a table with two
// columns in it says that in two lines.

// onTables is a Conn over an in-memory database carrying exactly the tables
// the statements make -- no schema, no lease, nothing but the question.
//
// The Pool behind it is a bare one: a Conn only consults its pool for the
// memo, and a pool that has never opened anything matches no handle, so every
// lookup here goes to the database. That is what makes these fixtures honest
// about the query rather than about the cache.
func onTables(t *testing.T, statements ...string) *Conn {
	t.Helper()
	db, err := sql.Open("duckdb", "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	for _, stmt := range statements {
		if _, err := db.ExecContext(context.Background(), stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	return &Conn{db: db, pool: New("", nil)}
}

// onAClosedPool is the handle a shutdown leaves behind: every query refused,
// starting with the one that asks what columns there are.
func onAClosedPool(t *testing.T) *Conn {
	t.Helper()
	db, err := sql.Open("duckdb", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	return &Conn{db: db, pool: New("", nil)}
}

// A Gyome deck, as `all_parts` spells one: one token printing, by its id.
const fixtureFood = `[{"id":"tok-food","component":"token","name":"Food",` +
	`"type_line":"Token Artifact — Food"}]`

func TestATokenSheetRefusesAPoolItCannotEvenAskAboutColumns(t *testing.T) {
	t.Parallel()
	sheet, err := onAClosedPool(t).TokensMade(context.Background(),
		[]string{"Fixture Chef"})
	if err == nil {
		t.Fatalf("a closed pool answered with %+v", sheet)
	}
	if sheet.Read {
		t.Error("the refusal came back marked as a reading; a page showing " +
			"that would say the deck makes nothing")
	}
}

// A pool with the column but without the name beside it: the probe passes and
// the real query cannot bind. This is the shape a schema several rungs back
// produces, and it is the one the two "nobody has looked yet" guards cannot
// recognise -- they ask whether `all_parts` is there and filled, and it is.
func TestATokenSheetRefusesAPoolWhoseCardsHaveNoNameToCredit(t *testing.T) {
	t.Parallel()
	c := onTables(t,
		`CREATE TABLE oracle_cards (all_parts JSON)`,
		`INSERT INTO oracle_cards VALUES ('`+fixtureFood+`')`)
	sheet, err := c.TokensMade(context.Background(), []string{"Fixture Chef"})
	if err == nil {
		t.Fatalf("a pool with no name column answered with %+v", sheet)
	}
	if !strings.Contains(err.Error(), "tokens_made") {
		t.Errorf("the refusal does not say which read broke: %v", err)
	}
}

// The cards read and the printings gone: a rebuild that died between its two
// loads. The deck's cards name token printings and there is nowhere to look
// them up, which is a fault rather than a deck that makes nothing.
func TestATokenSheetRefusesAPoolWithNoPrintingsToIdentifyTokensBy(t *testing.T) {
	t.Parallel()
	c := onTables(t,
		`CREATE TABLE oracle_cards (name VARCHAR, all_parts JSON)`,
		`INSERT INTO oracle_cards VALUES ('Fixture Chef', '`+fixtureFood+`')`)
	sheet, err := c.TokensMade(context.Background(), []string{"Fixture Chef"})
	if err == nil {
		t.Fatalf("a pool with no printings answered with %+v", sheet)
	}
	if !strings.Contains(err.Error(), "token_identities") {
		t.Errorf("the refusal does not say which read broke: %v", err)
	}
}

// A printings table that can say *which* Spirit this is and nothing about the
// painting: the ladder added the art columns and no load has filled them, or
// the file predates them entirely. The identity resolves, the picture cannot
// be fetched, and the sheet must not quietly arrive with a plate and no
// painting on it -- that is the contract the absent-art case already owns, and
// two different truths must not share one rendering.
func TestATokenSheetRefusesAPoolTooOldToCarryThePaintings(t *testing.T) {
	t.Parallel()
	c := onTables(t,
		`CREATE TABLE oracle_cards (name VARCHAR, all_parts JSON)`,
		`INSERT INTO oracle_cards VALUES ('Fixture Chef', '`+fixtureFood+`')`,
		`CREATE TABLE printings (id VARCHAR, oracle_id VARCHAR)`,
		`INSERT INTO printings VALUES ('tok-food', 'oracle-food')`)
	sheet, err := c.TokensMade(context.Background(), []string{"Fixture Chef"})
	if err == nil {
		t.Fatalf("a pool with no art columns answered with %+v", sheet)
	}
	if !strings.Contains(err.Error(), "token_art_by_oracle") {
		t.Errorf("the refusal does not say which read broke: %v", err)
	}
}

// The same pool, and the token printing it has never heard of. The identity
// lookup comes back empty, so the grouping falls back to the token's *name* --
// and the fallback has to fail the same way, rather than being the one road
// into that blank plate.
func TestAnUnknownTokenPrintingFallsBackToTheNameAndStillRefuses(t *testing.T) {
	t.Parallel()
	c := onTables(t,
		`CREATE TABLE oracle_cards (name VARCHAR, all_parts JSON)`,
		`INSERT INTO oracle_cards VALUES ('Fixture Chef', '`+fixtureFood+`')`,
		`CREATE TABLE printings (id VARCHAR, oracle_id VARCHAR)`)
	sheet, err := c.TokensMade(context.Background(), []string{"Fixture Chef"})
	if err == nil {
		t.Fatalf("a pool with no art columns answered with %+v", sheet)
	}
	if !strings.Contains(err.Error(), "card_art") {
		t.Errorf("the refusal does not say which read broke: %v", err)
	}
}

// A deck file with a blank line in its list is a list of the cards that are
// there, not a query for the empty name -- and a list that is *all* blanks is
// a deck that was read and makes nothing, which is the one empty answer that
// is true.
func TestBlankNamesAreDroppedRatherThanAsked(t *testing.T) {
	t.Parallel()
	c := onTables(t,
		`CREATE TABLE oracle_cards (name VARCHAR, all_parts JSON)`,
		`INSERT INTO oracle_cards VALUES ('Fixture Chef', '`+fixtureFood+`')`,
		`CREATE TABLE printings (id VARCHAR, oracle_id VARCHAR, name VARCHAR,
		  set_code VARCHAR, set_name VARCHAR, released_at DATE, promo BOOLEAN,
		  image_normal VARCHAR, artist VARCHAR)`,
		`INSERT INTO printings VALUES ('tok-food', 'oracle-food', 'Food',
		  'teld', 'Throne of Eldraine', DATE '2019-10-04', false,
		  'https://example.invalid/food.jpg', 'Fixture Painter')`)
	ctx := context.Background()

	// A blank among real names changes nothing about the answer.
	sheet, err := c.TokensMade(ctx, []string{"Fixture Chef", "", "   "})
	if err != nil {
		t.Fatal(err)
	}
	if !sheet.Read || len(sheet.Tokens) != 1 || sheet.Tokens[0].Name != "Food" {
		t.Fatalf("a blank name in the list changed the answer: %+v", sheet)
	}
	if sheet.Tokens[0].Art == nil || sheet.Tokens[0].Art.Set != "TELD" {
		t.Errorf("the Food came back with %+v, want the painting the pool has",
			sheet.Tokens[0].Art)
	}

	// A list of nothing but blanks was still read, and the honest answer is
	// that it makes nothing -- never "nobody has looked yet".
	blank, err := c.TokensMade(ctx, []string{"", "  ", "\t"})
	if err != nil {
		t.Fatal(err)
	}
	if !blank.Read || len(blank.Tokens) != 0 {
		t.Errorf("a list of blanks answered %+v", blank)
	}
}

// **Two tokens with one name are two rows, in a settled order.** "Spirit" is a
// dozen different bodies and the identity is what tells them apart, so a deck
// making two of them has two piles to find -- and the order they are listed in
// may not come out of a map walk, or the same deck renders differently on two
// page loads for no reason a reader could ever explain.
func TestTwoTokensSharingANameAreOrderedByWhatSeparatesThem(t *testing.T) {
	t.Parallel()
	part := func(id, typeLine string) string {
		return `[{"id":"` + id + `","component":"token","name":"Fixture Spirit",` +
			`"type_line":"` + typeLine + `"}]`
	}
	c := onTables(t,
		`CREATE TABLE oracle_cards (name VARCHAR, all_parts JSON)`,
		`INSERT INTO oracle_cards VALUES ('Fixture Abbot', '`+
			part("tok-cleric", "Token Creature — Spirit Cleric")+`')`,
		`INSERT INTO oracle_cards VALUES ('Fixture Widow', '`+
			part("tok-plain", "Token Creature — Spirit")+`')`,
		`CREATE TABLE printings (id VARCHAR, oracle_id VARCHAR, name VARCHAR,
		  set_code VARCHAR, set_name VARCHAR, released_at DATE,
		  image_normal VARCHAR, artist VARCHAR)`,
		`INSERT INTO printings VALUES ('tok-cleric', 'oracle-cleric',
		  'Fixture Spirit', 'fix', 'Fixture Set', DATE '2019-01-01',
		  'https://example.invalid/cleric.jpg', 'Fixture Painter')`,
		`INSERT INTO printings VALUES ('tok-plain', 'oracle-plain',
		  'Fixture Spirit', 'fix', 'Fixture Set', DATE '2019-01-01',
		  'https://example.invalid/plain.jpg', 'Fixture Painter')`)

	sheet, err := c.TokensMade(context.Background(),
		[]string{"Fixture Abbot", "Fixture Widow"})
	if err != nil {
		t.Fatal(err)
	}
	if len(sheet.Tokens) != 2 {
		t.Fatalf("two different Spirits came back as %+v", sheet.Tokens)
	}
	// One maker each and the same name, so the only thing left to sort on is
	// the type line -- which is the one thing that tells a reader which pile
	// they are looking at.
	if sheet.Tokens[0].TypeLine != "Token Creature — Spirit" ||
		sheet.Tokens[1].TypeLine != "Token Creature — Spirit Cleric" {
		t.Errorf("listed as %q then %q; a tie has to settle on something a "+
			"reader can see, not on a map walk",
			sheet.Tokens[0].TypeLine, sheet.Tokens[1].TypeLine)
	}
}

// Nothing to look up is answered without asking, which is what keeps an
// oracle-only deck from paying for a query -- and is checkable precisely
// because a closed handle would refuse any query at all.
func TestAnIdentityLookupWithNothingToLookUpAsksNothing(t *testing.T) {
	t.Parallel()
	c := onAClosedPool(t)
	found, err := c.tokenIdentities(context.Background(), nil)
	if err != nil {
		t.Fatalf("an empty lookup reached the database: %v", err)
	}
	if len(found) != 0 {
		t.Errorf("an empty lookup invented %d identities", len(found))
	}
}

// `all_parts` arrives in three encodings and has to read in all of them.
//
// **The third one is not a detail.** A JSON-typed column comes back from the
// driver *already decoded*, as a list of maps, and a reader that only knew the
// text shapes answered every deck in the library with "makes nothing" -- with
// `Read` true, which is the sentence that is worse than silence. The text
// shapes stay because a pool that stored the column as VARCHAR still reads.
func TestRelatedPartsReadHoweverTheDriverHandsThemOver(t *testing.T) {
	t.Parallel()
	decoded := []any{map[string]any{
		"id": "tok-food", "component": "token", "name": "Food",
		"type_line": "Token Artifact — Food",
	}}
	for _, one := range []struct {
		how   string
		given any
	}{
		{"a JSON column, decoded by the driver", decoded},
		{"a VARCHAR column, as text", fixtureFood},
		{"a BLOB column, as bytes", []byte(fixtureFood)},
	} {
		parts := relatedParts(one.given)
		if len(parts) != 1 {
			t.Errorf("%s: read %d parts, want one", one.how, len(parts))
			continue
		}
		if parts[0].ID != "tok-food" || parts[0].Component != "token" ||
			parts[0].Name != "Food" {
			t.Errorf("%s: read %+v", one.how, parts[0])
		}
	}

	// And the shapes that are not an answer are nothing, rather than a panic
	// or a part with empty fields that would list a nameless token.
	for _, one := range []struct {
		how   string
		given any
	}{
		{"a NULL column", nil},
		{"a column of some other type entirely", int64(7)},
		{"text that is not the document", "{ this is not json"},
		{"a document of the wrong shape", `{"component":"token"}`},
	} {
		if parts := relatedParts(one.given); parts != nil {
			t.Errorf("%s: read %+v, want nothing at all", one.how, parts)
		}
	}
}

// The answer credits a card the way the deck file spells it, by the whole name
// or by either face -- and a row that matches neither is credited to nobody
// rather than to the empty string.
func TestACardIsCreditedByTheSpellingTheDeckUsed(t *testing.T) {
	t.Parallel()
	asked := map[string]string{
		"fixture chef":   "Fixture Chef",
		"fixture pariah": "Fixture Pariah",
	}
	for _, one := range []struct{ poolName, want string }{
		{"Fixture Chef", "Fixture Chef"},
		{"FIXTURE CHEF", "Fixture Chef"},
		{"Fixture Pariah // Fixture Ascendant", "Fixture Pariah"},
		{"Fixture Ascendant // Fixture Pariah", "Fixture Pariah"},
		{"Fixture Stranger", ""},
	} {
		if got := deckSpelling(asked, one.poolName); got != one.want {
			t.Errorf("the pool's %q was credited to %q, want %q",
				one.poolName, got, one.want)
		}
	}
}
