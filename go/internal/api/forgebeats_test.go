package api

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/sim/tier3"
)

// The narration half of a match, through the route: the beats Forge raises
// while it plays reach the job's `partial` beside the row they belong to.
//
// The properties worth pinning are all about *what a room can rely on*, and
// none of them is visible from the shaping function alone:
//
//   - narration is asked for, so a surface that only wants the tally never
//     pays for a hundred beats a game;
//   - a beat names a deck, never a seat number, because every other thing this
//     package hands a client names a deck;
//   - the beats and the row of the same game arrive in the same partial, so a
//     room never shows a game landing with its narration one poll behind;
//   - and the cut at the ceiling is announced rather than silent.

// theBeats is a small, real-shaped game: somebody takes a turn, plays a land,
// casts a thing, swings, and wins. Seat 1 and seat 2 are both spoken for, and
// one beat (the resolve) names no player at all — which is the common case in
// Forge's log and the one a naive seat-to-slug mapping gets wrong.
func theBeats() []tier3.GameEvent {
	life := 32
	return []tier3.GameEvent{
		{Kind: tier3.EventTurn, Turn: 4, Seat: 1},
		{Kind: tier3.EventLand, Turn: 4, Seat: 1, Card: "Bojuka Bog"},
		{Kind: tier3.EventCast, Turn: 4, Seat: 1, Card: "Gyome, Master Chef"},
		{Kind: tier3.EventResolve, Turn: 4, Card: "Gyome, Master Chef"},
		{Kind: tier3.EventAttack, Turn: 4, Seat: 1, Card: "Gyome, Master Chef",
			TargetSeat: 2},
		{Kind: tier3.EventDamage, Turn: 4, Card: "Gyome, Master Chef",
			Amount: 8, TargetSeat: 2},
		{Kind: tier3.EventLife, Turn: 4, Seat: 2, Life: &life},
		{Kind: tier3.EventOutcome, Turn: 4, Seat: 2,
			Note: "has lost because life total reached 0"},
	}
}

// A room that asked to watch is handed the beats, with the row of the same
// game, in one partial.
func TestTheBeatsReachThePartialBesideTheirOwnRow(t *testing.T) {
	t.Parallel()
	shim := &stubShim{stream: true, hold: make(chan struct{}),
		beats: theBeats(),
		games: []tier3.WireGame{won(1, 5421, 1, 11), won(2, 4000, 2, 9)}}
	a, reg, _ := forgeAPI(t, shim)
	srv := forgeServer(t, a)

	status, payload := postForge(t, srv,
		`{"a_slug":"kaheera","b_slug":"mono-green","games":2,"narrate":true}`)
	if status != 200 {
		t.Fatalf("%d %v", status, payload)
	}
	id := payload["id"].(string)

	first := awaitPartial(t, reg, id, 1)
	if first.Beats == nil {
		t.Fatal("the room asked to watch and was handed no beats")
	}
	// The same game, in one partial: the row that says game 1 finished and the
	// beats that say how.
	if first.Beats.Game != 1 || first.Rows[0].Game != 1 {
		t.Fatalf("beats for game %d arrived with the row for game %d",
			first.Beats.Game, first.Rows[0].Game)
	}
	if len(first.Beats.Beats) != len(theBeats()) {
		t.Fatalf("%d beats crossed, want %d", len(first.Beats.Beats), len(theBeats()))
	}
	if first.Beats.Truncated {
		t.Error("eight beats were reported as a cut game")
	}

	// **A beat names a deck.** Seat 1 is the deck submitted as `a`, seat 2 as
	// `b`, and a beat that named nobody names nobody still.
	land := first.Beats.Beats[1]
	if land.Who == nil || *land.Who != "kaheera" {
		t.Errorf("the land was played by %v, want kaheera", land.Who)
	}
	swing := first.Beats.Beats[4]
	if swing.Who == nil || *swing.Who != "kaheera" ||
		swing.Against == nil || *swing.Against != "mono-green" {
		t.Errorf("the attack reads %v -> %v, want kaheera -> mono-green",
			swing.Who, swing.Against)
	}
	resolve := first.Beats.Beats[3]
	if resolve.Who != nil {
		t.Errorf("a beat that named no player was given one: %v", *resolve.Who)
	}
	if resolve.Card != "Gyome, Master Chef" {
		t.Errorf("the resolving card came across as %q", resolve.Card)
	}
	outcome := first.Beats.Beats[7]
	if outcome.Note != "has lost because life total reached 0" {
		t.Errorf("the outcome's reason came across as %q", outcome.Note)
	}

	shim.hold <- struct{}{}

	// The next game replaces the last one's beats rather than accumulating
	// them: the partial is re-sent on every poll and a growing transcript
	// would be re-sent whole every time. The rows still accumulate.
	second := awaitPartial(t, reg, id, 2)
	if second.Beats == nil || second.Beats.Game != 2 {
		t.Fatalf("the second partial carries beats for %v, want game 2", second.Beats)
	}
	if len(second.Rows) != 2 {
		t.Errorf("the rows stopped accumulating at %d", len(second.Rows))
	}
	// And the partial a poll already read did not grow under it.
	if first.Beats.Game != 1 {
		t.Error("the first partial's beats were rewritten in place")
	}

	shim.hold <- struct{}{}
	reg.Wait()
	if job := reg.Get(id, alice.UserID); job.Status() != "done" {
		t.Fatalf("the match ended %q", job.Status())
	}
}

// Narration is asked for, never assumed — the measured rule (`events.go`): the
// flag is free in time and about a hundred beats a game in volume, so a
// surface that wants the tally and not the play-by-play must be able to run
// silent. The ask itself is checked, not only its effect: a client that
// quietly stopped sending the flag would still look green against a stub that
// narrated regardless.
func TestAMatchNobodyAskedToNarrateIsSilent(t *testing.T) {
	t.Parallel()
	shim := &stubShim{stream: true, hold: make(chan struct{}),
		beats: theBeats(),
		games: []tier3.WireGame{won(1, 5421, 1, 11)}}
	a, reg, _ := forgeAPI(t, shim)
	srv := forgeServer(t, a)

	status, payload := postForge(t, srv,
		`{"a_slug":"kaheera","b_slug":"mono-green","games":1}`)
	if status != 200 {
		t.Fatalf("%d %v", status, payload)
	}
	id := payload["id"].(string)

	first := awaitPartial(t, reg, id, 1)
	if first.Beats != nil {
		t.Errorf("a silent match narrated itself: %+v", first.Beats)
	}
	if narrate, _ := shim.askedFor()["narrate"].(bool); narrate {
		t.Error("the ask ordered narration nobody wanted")
	}
	shim.hold <- struct{}{}
	reg.Wait()
}

// Two asks that differ only in narration are two matches, not one.
//
// The FORGE lane dedupes on the plan's key so that a second identical request
// joins the live job instead of queueing a duplicate JVM. Narration has to be
// part of that key: without it, a room that asked to watch could be handed a
// running silent match and would sit mute for the whole of it, for a reason
// nothing on screen could explain.
func TestASilentMatchAndANarratedOneAreNotTheSameMatch(t *testing.T) {
	t.Parallel()
	// The stream is held for the same reason [TestTwoIdenticalAsksAreOneMatch]
	// holds it: the join under test is an *in-flight* one, and a stub that
	// finishes instantly turns "a second job was correct" into what looks like
	// a broken dedupe.
	hold := make(chan struct{})
	shim := &stubShim{stream: true, beats: theBeats(),
		games: []tier3.WireGame{won(1, 100, 1, 3)}, hold: hold}
	a, reg, _ := forgeAPI(t, shim)
	srv := forgeServer(t, a)

	silent := `{"a_slug":"kaheera","b_slug":"mono-green","games":1,"seed":5}`
	told := `{"a_slug":"kaheera","b_slug":"mono-green","games":1,"seed":5,` +
		`"narrate":true}`

	_, first := postForge(t, srv, silent)
	_, watching := postForge(t, srv, told)
	if first["id"] == watching["id"] {
		t.Errorf("the watcher joined the silent match (%v) and would never "+
			"hear a thing", first["id"])
	}
	// And the case dedupe exists for still holds: an identical narrated ask
	// joins the narrated match rather than queueing a third JVM.
	_, again := postForge(t, srv, told)
	if again["id"] != watching["id"] {
		t.Errorf("two identical narrated asks made two matches: %v and %v",
			watching["id"], again["id"])
	}

	close(hold)
	reg.Wait()
}

// The recorded key is untouched for a silent ask. `forge.json` froze this
// text, so narration is a suffix a narrated ask wears and a silent one does
// not — a `|false` on the end of every key would have rewritten a golden to
// buy nothing.
func TestTheSilentKeyIsStillTheRecordedText(t *testing.T) {
	t.Parallel()
	corpus := loadShapeCorpus(t)
	if len(corpus.Shape.Keys) == 0 {
		t.Fatal("the corpus records no keys")
	}
	for _, c := range corpus.Shape.Keys {
		if strings.Contains(c.Key, "narrated") {
			t.Errorf("a recorded key already carries narration: %q", c.Key)
		}
	}
}

// The ceiling on what crosses is announced, never silent — [tier3.EventLog]'s
// own rule one layer down, and for the same reason: a consumer handed a short
// list with no flag has no way to know it is reading half a game.
func TestTheBeatsAreCutAtTheCeilingAndSayThatTheyWere(t *testing.T) {
	t.Parallel()
	long := make([]tier3.GameEvent, ForgeBeatsMax+50)
	for i := range long {
		long[i] = tier3.GameEvent{Kind: tier3.EventCast, Turn: 1, Seat: 1,
			Card: fmt.Sprintf("Card %d", i)}
	}
	seats := map[int]string{1: "gyome", 2: "trostani"}

	cut := newForgeBeats(tier3.EventLog{Game: 3, Events: long}, seats, nil, nil)
	if len(cut.Beats) != ForgeBeatsMax {
		t.Errorf("%d beats crossed, want the ceiling of %d",
			len(cut.Beats), ForgeBeatsMax)
	}
	if !cut.Truncated {
		t.Error("the cut was silent")
	}
	// The beats kept are the *first* ones: a game's opening is what makes the
	// rest of it legible, so the cut takes the tail.
	if cut.Beats[0].Card != "Card 0" {
		t.Errorf("the cut kept %q first, want the opening", cut.Beats[0].Card)
	}

	// A game inside the ceiling is not marked, and a game the parser had
	// already truncated stays marked whatever this does.
	whole := newForgeBeats(tier3.EventLog{Game: 4, Events: long[:10]}, seats, nil, nil)
	if whole.Truncated || len(whole.Beats) != 10 {
		t.Errorf("a ten-beat game came out as %d beats, truncated=%v",
			len(whole.Beats), whole.Truncated)
	}
	already := newForgeBeats(
		tier3.EventLog{Game: 5, Events: long[:10], Truncated: true}, seats, nil,
		nil)
	if !already.Truncated {
		t.Error("the parser's own cut was dropped on the way to the browser")
	}
}

// A beat carries no seat number to the browser at all — the value a room could
// mis-map is simply not in the payload.
//
// **Rendered rather than matched**, which is this repo's recorded lesson about
// exactly this kind of claim: a test that read `beat.Who` and found "gyome"
// would pass just as happily with a stray `"seat": 1` riding along beside it,
// and the whole point of shaping is that the seat does not travel. So the
// assertion is made against the bytes.
func TestABeatOnTheWireNamesADeckAndCarriesNoSeat(t *testing.T) {
	t.Parallel()
	shaped := newForgeBeats(tier3.EventLog{Game: 1, Events: theBeats()},
		map[int]string{1: "gyome", 2: "trostani"}, nil, nil)
	raw, err := json.Marshal(shaped)
	if err != nil {
		t.Fatal(err)
	}
	rendered := string(raw)
	for _, gone := range []string{`"seat"`, `"target_seat"`} {
		if strings.Contains(rendered, gone) {
			t.Errorf("%s reached the browser:\n%s", gone, rendered)
		}
	}
	for _, wanted := range []string{`"who":"gyome"`, `"against":"trostani"`,
		`"card":"Bojuka Bog"`, `"kind":"attack"`, `"life":32`} {
		if !strings.Contains(rendered, wanted) {
			t.Errorf("the wire is missing %s:\n%s", wanted, rendered)
		}
	}
	// A beat nobody acted in says so with a null rather than by omitting the
	// field: `who` is the answer to "whose beat is this", and a missing key
	// and an explicit nobody read the same in JavaScript but not to anybody
	// reading the payload.
	if !strings.Contains(rendered, `"who":null`) {
		t.Errorf("the unattributed beat dropped its `who` entirely:\n%s", rendered)
	}
}
