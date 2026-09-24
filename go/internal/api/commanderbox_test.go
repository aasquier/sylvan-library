package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/aasquier/sylvan-library/go/internal/auth"
	"github.com/aasquier/sylvan-library/go/internal/cards"
	"github.com/aasquier/sylvan-library/go/internal/pool"
	"github.com/aasquier/sylvan-library/go/internal/pool/pooltest"
)

// The commander box on the import page, read as the person typing in it reads
// it.
//
// `GET /api/cards/commander` answers one field while somebody types, and every
// answer it gives is a **sentence** rather than a code -- which is exactly the
// kind of surface `docs/polish/COVERAGE.md` names as cheap to reach, load
// bearing for commandment 2, and almost entirely unreached. The verdicts that
// had never run are the ones a newcomer is most likely to meet: a Background
// typed on its own, a pair that does pair, and three names in a field that
// takes one or two.
//
// The fixture pool carries no pairing ability at all, which is why: the rows
// are doctored into the test's own copy (`pairedPool`, argued in
// `createcompanion_test.go`), and the Background is invented because what is
// asserted is that the check reads a type line.

// boxPool is `pairedPool` plus a Background, which is the one card type that
// is a legal commander in a pair and never alone.
func boxPool(t *testing.T) *pool.Pool {
	t.Helper()
	return pooltest.OpenWith(t,
		pooltest.Card{Name: "Fixture Partnered Knight", ManaCost: "{2}{W}", CMC: 3,
			TypeLine: "Legendary Creature — Human Knight", ColorIdentity: []string{"W"},
			OracleText: "Partner (You can have two commanders if both have partner.)"},
		pooltest.Card{Name: "Fixture Partnered Mage", ManaCost: "{1}{U}", CMC: 2,
			TypeLine: "Legendary Creature — Human Wizard", ColorIdentity: []string{"U"},
			OracleText: "Partner (You can have two commanders if both have partner.)"},
		pooltest.Card{Name: "Fixture Lonely Baron", ManaCost: "{3}{B}", CMC: 4,
			TypeLine: "Legendary Creature — Vampire Noble", ColorIdentity: []string{"B"},
			OracleText: "Flying"},
		pooltest.Card{Name: "Fixture Cellar Background", ManaCost: "{W}", CMC: 1,
			TypeLine: "Legendary Enchantment — Background", ColorIdentity: []string{"W"},
			OracleText: "Commander creatures you own have vigilance."},
	)
}

// box asks the commander field one question and hands back the decoded answer.
func box(t *testing.T, a *API, typed string) map[string]any {
	t.Helper()
	_, payload, raw := callAs(t, a, alice, http.MethodGet,
		"/api/cards/commander?q="+url.QueryEscape(typed), "")
	if payload == nil {
		t.Fatalf("the commander box answered %s for %q", raw, typed)
	}
	return payload
}

// Every verdict the box can reach, and the sentence each one carries.
//
// The sentence is the assertion rather than the state, because the state is
// what the border turns red and the sentence is the only part that tells
// somebody what to do about it. A `trouble` with no way out of it is the
// failure commandment 2 names.
func TestTheCommanderBoxSaysWhatIsWrongAndWhatToDoAboutIt(t *testing.T) {
	t.Parallel()
	a := New(Config{Pool: boxPool(t), DecksDir: decksDir(t)})

	for _, row := range []struct {
		typed, state string
		says         []string
	}{
		// Two that carry the keyword: the one verdict that greets a pair.
		{"Fixture Partnered Knight + Fixture Partnered Mage", "ready",
			[]string{"Fixture Partnered Knight", "Fixture Partnered Mage", "together"}},
		// Two that do not: refused, with the rule named rather than "invalid".
		{"Fixture Partnered Knight + Fixture Lonely Baron", "trouble",
			[]string{"cannot lead together", "pairing ability"}},
		// A Background alone. It IS a command-zone card, so "that is not a
		// commander" would be a flat lie about it, and the sentence says the
		// one thing that gets the person moving: write both names.
		{"Fixture Cellar Background", "trouble",
			[]string{"is a Background", "+"}},
		// Three names in a field that takes one or two.
		{"Fixture Partnered Knight + Fixture Partnered Mage + Fixture Lonely Baron", "trouble",
			[]string{"one commander, or by two", "3 names"}},
		// One that leads alone.
		{"Goreclaw, Terror of Qal Sisma", "ready", []string{"can lead this deck"}},
		// A real card that cannot sit there.
		{"Sol Ring", "trouble", []string{"cannot sit", "command zone"}},
		// Nothing typed at all: no verdict, no red box.
		{"", "blank", nil},
	} {
		payload := box(t, a, row.typed)
		if payload["state"] != row.state {
			t.Errorf("%q read as %v, not %s", row.typed, payload["state"], row.state)
			continue
		}
		sentence, _ := payload["sentence"].(string)
		for _, want := range row.says {
			if !strings.Contains(sentence, want) {
				t.Errorf("%q answered %q, which never says %q",
					row.typed, sentence, want)
			}
		}
	}
}

// A pair's seats each say which pairing ability they carry, because "these two
// cannot lead together" is answerable only if the person can see what each one
// actually has.
func TestAPairedSeatSaysWhichPairingAbilityItCarries(t *testing.T) {
	t.Parallel()
	a := New(Config{Pool: boxPool(t), DecksDir: decksDir(t)})

	payload := box(t, a, "Fixture Partnered Knight + Fixture Partnered Mage")
	seats, _ := payload["commanders"].([]any)
	if len(seats) != 2 {
		t.Fatalf("a pair seated %d commanders: %v", len(seats), payload)
	}
	for _, seat := range seats {
		entry, _ := seat.(map[string]any)
		if entry["pairing"] != "Partner" {
			t.Errorf("%v carries pairing %v, not the ability printed on it",
				entry["name"], entry["pairing"])
		}
		if entry["may_command"] != true {
			t.Errorf("%v may not command: %v", entry["name"], entry)
		}
	}
}

// A name the pool does not hold is answered with a shortlist rather than a
// dead end, and the cards that could actually lead are listed first.
func TestAnUnknownCommanderIsOfferedNamesRatherThanRefused(t *testing.T) {
	t.Parallel()
	a := New(Config{Pool: boxPool(t), DecksDir: decksDir(t)})

	payload := box(t, a, "Goreclaww, Terror of Qal Sisma")
	if payload["state"] != "unknown" {
		t.Fatalf("a misspelled commander read as %v: %v", payload["state"], payload)
	}
	offers, _ := payload["did_you_mean"].([]any)
	if len(offers) == 0 {
		t.Fatalf("a near-miss was offered nothing: %v", payload)
	}
	first, _ := offers[0].(map[string]any)
	candidates, _ := first["candidates"].([]any)
	if len(candidates) == 0 {
		t.Fatalf("the offer carries no candidates: %v", first)
	}
	// The ones that can lead come first: an offer list headed by Sol Ring is
	// a list somebody scrolls past.
	head, _ := candidates[0].(map[string]any)
	if head["may_command"] != true {
		t.Errorf("the shortlist is headed by a card that cannot lead: %v", head)
	}
	if sentence, _ := payload["sentence"].(string); !strings.Contains(sentence, "Check the spelling") {
		t.Errorf("the unknown sentence does not offer a way forward: %q", sentence)
	}
}

// The box over a pool that opens and then cannot answer. It must not read as
// `unknown`, which would put a red box over a correctly-typed commander and
// send somebody to fix a spelling that is fine.
func TestTheCommanderBoxDoesNotCallACommanderUnknownWhenTheQueryFails(t *testing.T) {
	t.Parallel()
	a := failingPoolAPI(t)

	status, payload, raw := callAs(t, a, alice, http.MethodGet,
		"/api/cards/commander?q=Goreclaw%2C+Terror+of+Qal+Sisma", "")
	if status == http.StatusOK {
		if payload["state"] == "unknown" {
			t.Fatalf("a failed query was reported as an unknown card: %s", raw)
		}
		return
	}
	if !saysSomething(payload) {
		t.Errorf("the box answered %d with nothing to read: %s", status, raw)
	}
}

// The typeahead's deck annotation, and the three ways it can fail to get one.
//
// `deck=<owner>/<slug>` is what makes the finder mark a card its commander
// allows, and every one of its refusals is deliberately silent: a typeahead
// that 500s because a deck reference was stale is a typeahead that stops
// working while somebody types. What it must never do is stop answering.
func TestTheFinderStillAnswersWhenTheDeckItWasPointedAtCannotBeRead(t *testing.T) {
	t.Parallel()
	good := New(Config{Pool: pooltest.Open(t), DecksDir: decksDir(t)})

	for _, row := range []struct {
		name string
		api  *API
		deck string
	}{
		{"a deck that is not there", good, "alice/no-such-deck"},
		{"an owner that is not there", good, "nobody/kaheera"},
		{"a pool that will not answer the commander", failingPoolAPI(t), "alice/kaheera"},
		{"a database that has gone", goneDB(t), "alice/kaheera"},
	} {
		status, payload, raw := callAs(t, row.api, alice, http.MethodGet,
			"/api/cards/suggest?q=sol&deck="+url.QueryEscape(row.deck), "")
		if status >= http.StatusInternalServerError && row.api == good {
			t.Errorf("%s: the finder answered %d: %s", row.name, status, raw)
			continue
		}
		if status == http.StatusOK {
			if _, ok := payload["cards"]; !ok {
				t.Errorf("%s: a 200 with no cards key: %s", row.name, raw)
			}
			continue
		}
		if !saysSomething(payload) {
			t.Errorf("%s: answered %d with nothing to read: %s", row.name, status, raw)
		}
	}
}

// The camera door's three edges: more sightings than it will look at, a pool
// that answers an error, and a body that stops arriving part-way.
func TestTheCameraDoorStopsAtItsCapAndSaysSoWhenItCannotRead(t *testing.T) {
	t.Parallel()

	// The cap. One more than it will take, so the break is the thing under
	// test rather than the loop's own end.
	over := make([]string, 0, cards.MaxSightings+1)
	for i := range cards.MaxSightings + 1 {
		over = append(over, fmt.Sprintf(`{"title":"Fixture Card %d"}`, i))
	}
	a := New(Config{Pool: pooltest.Open(t)})
	status, payload, raw := callAs(t, a, alice, http.MethodPost, "/api/cards/identify",
		`{"sightings":[`+strings.Join(over, ",")+`]}`)
	if status != http.StatusOK {
		t.Fatalf("a full camera roll answered %d: %s", status, raw)
	}
	readings, _ := payload["readings"].([]any)
	if len(readings) != cards.MaxSightings {
		t.Errorf("%d sightings were read, and the door takes %d -- the cap is "+
			"not being applied", len(readings), cards.MaxSightings)
	}

	// A pool that answers an error. The camera's degraded answer is for a
	// pool that is absent; a pool that fails a query is a fault, and reporting
	// it as "nothing was read" would tell somebody their cards are unknown.
	failing := failingPoolAPI(t)
	status, payload, raw = callAs(t, failing, alice, http.MethodPost, "/api/cards/identify",
		`{"sightings":[{"title":"Sol Ring"}]}`)
	if status == http.StatusOK {
		t.Errorf("a failed query was answered as a successful read: %s", raw)
	} else if !saysSomething(payload) {
		t.Errorf("the camera answered %d with nothing to read: %s", status, raw)
	}

	// A body that stops arriving. Driven at the handler because no rig can
	// build a request whose body fails half way, and it is the one branch
	// between a dropped connection and a nil map.
	req := httptest.NewRequest(http.MethodPost, "/api/cards/identify",
		iotest.ErrReader(errTruncatedBody)).WithContext(
		auth.WithScope(t.Context(), alice))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	a.identify(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("a body that stopped arriving answered %d, not 400: %s",
			rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "could not read") {
		t.Errorf("the answer does not say what went wrong: %s", rec.Body.String())
	}
}

// errTruncatedBody is a connection that went away mid-body.
var errTruncatedBody = fmt.Errorf("the connection went away")
