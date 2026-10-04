package tier3

import (
	"fmt"
	"testing"
)

// The scribe's own quiet lines, and the one name-shape it has to refuse.
//
// `scribe_test.go` plays a recorded match back whole, which is the right way to
// hold the reading honest and leaves one class untouched: the line a real match
// happens not to contain. A curse, a life line with no total on it, an outcome
// sentence Forge left empty, a zone line that arrived with no id. Each of them
// is one missing key away from a line the recording *does* contain — which is
// what makes them worth holding, because by the time a line reaches this reader
// a missing key looks exactly like a zero.
//
// Fed as lines rather than folded by hand: the reader's own entry point is the
// contract, and a hand-folded line would be testing this file's spelling of
// Forge's JSON instead of the reader's.

// aScribedGame is the opening every line below needs: a game, and one seat to
// hang it on.
func aScribedGame() *ScribeParser {
	p := NewScribeParser(true)
	p.Feed(`{"t":"game","game":1}`)
	p.Feed(`{"t":"seat","game":1,"seat":1,"who":"alpha","life":40}`)
	return p
}

// **A curse is an attachment with no host**, and it is the one attach line that
// has to be narrated *because* the board did not move: it names a player rather
// than a card, so nothing on the battlefield changed and a reader of the board
// alone would never know it happened.
func TestACurseIsNarratedEvenThoughNoCardMoved(t *testing.T) {
	t.Parallel()
	p := aScribedGame()
	p.Feed(`{"t":"zone","game":1,"zone":"Battlefield","mode":"in","seat":1,` +
		`"id":500,"card":"Curse of Bloodletting","types":"Enchantment - Aura Curse"}`)
	p.Feed(`{"t":"attach","game":1,"seat":1,"id":500,` +
		`"card":"Curse of Bloodletting","types":"Enchantment - Aura Curse",` +
		`"against":"beta"}`)

	var attach *GameEvent
	for i, e := range p.events {
		if e.Kind == EventAttach {
			attach = &p.events[i]
		}
	}
	if attach == nil {
		t.Fatalf("a curse raised no attachment: %+v", p.events)
	}
	if attach.Target != "beta" {
		t.Errorf("the curse is aimed at %q, want the player it names", attach.Target)
	}
	if attach.Card != "Curse of Bloodletting" {
		t.Errorf("the attachment names %q", attach.Card)
	}
}

// A life line with no total on it is a line that says nothing: absent and zero
// are the same bytes by the time they reach here, and reading a missing key as
// "this player is at zero" would end the game.
func TestALifeLineWithNoTotalSaysNothing(t *testing.T) {
	t.Parallel()
	p := aScribedGame()
	p.Feed(`{"t":"life","game":1,"seat":1}`)
	if len(p.events) != 0 {
		t.Errorf("a life line with no total raised %+v", p.events)
	}
	if p.board.life[1] != 40 {
		t.Errorf("seat one is at %d after a line carrying no total", p.board.life[1])
	}

	// A life line belonging to no seat is not a blow against anybody either —
	// the killing blow is read off the pair of lines, and the second of them
	// has to name whose life it is.
	p.Feed(`{"t":"damage","game":1,"seat":1,"against_seat":2,"amount":40,` +
		`"card":"Bronzehide Lion"}`)
	p.Feed(`{"t":"life","game":1,"seat":0,"life":0}`)
	if p.killer != nil {
		t.Errorf("a life line with no seat promoted %+v as the killing blow", p.killer)
	}
}

// Forge's outcome sentence carries the seat, the verb and the reason at once, so
// an empty one is a line with nothing to say — and it still carries the turn
// number, which is a row's field and the reason the line is read on both paths.
func TestAnOutcomeWithNothingSaidStillCountsTheTurn(t *testing.T) {
	t.Parallel()
	p := aScribedGame()
	p.Feed(`{"t":"outcome","game":1,"turn":9,"said":"   "}`)
	if len(p.events) != 0 {
		t.Errorf("an empty outcome raised %+v", p.events)
	}
	if p.outcomeTurn != 9 {
		t.Errorf("the outcome's turn reads %d, want 9", p.outcomeTurn)
	}
}

// A zone line with no id is still worth folding for its zone and its seat, and
// it can never be the companion being bought: the companion is followed by id,
// and an id of zero is every card at once.
func TestAZoneLineWithNoIDIsNeverTheCompanion(t *testing.T) {
	t.Parallel()
	p := aScribedGame()
	p.Feed(`{"t":"zone","game":1,"zone":"Sideboard","mode":"out","seat":1}`)
	p.Feed(`{"t":"zone","game":1,"zone":"Command","mode":"in","seat":1}`)
	p.Feed(`{"t":"zone","game":1,"zone":"Hand","mode":"in","seat":1}`)
	for _, e := range p.events {
		if e.Kind == EventCompanion {
			t.Errorf("a line with no card in it bought a companion: %+v", e)
		}
	}
	if len(p.companions) != 0 {
		t.Errorf("a companion was remembered under id zero: %v", p.companions)
	}
}

// **The beat cap holds on this path too.** A game that outruns [EventCap] keeps
// the beats it has and says it was cut; the row is unaffected, because a row is
// a record and the beats are a picture of it.
func TestAScribedGameThatOutrunsTheBeatCapSaysSo(t *testing.T) {
	t.Parallel()
	p := aScribedGame()
	for i := range EventCap + 50 {
		p.Feed(fmt.Sprintf(`{"t":"life","game":1,"seat":1,"life":%d}`, 40-i%40))
	}
	if len(p.events) != EventCap {
		t.Fatalf("a game kept %d beats, want the cap of %d", len(p.events), EventCap)
	}
	if !p.truncated {
		t.Error("a game that outran the cap did not say it had been cut")
	}

	log, game := p.Feed(`{"t":"result","game":1,"winner_seat":1,"turns":9,` +
		`"milliseconds":1200}`)
	if game == nil {
		t.Fatal("the row never closed")
	}
	if log == nil || !log.Truncated {
		t.Errorf("the beats crossed as %+v, without the cut being named", log)
	}
}

// **Forge's own bookkeeping cards are recognised by shape, and the shape is
// exact.** `Name (id)'s Effect` is one of Forge's; anything else wearing part of
// that suffix is somebody's real card, and dropping a real card from the board
// is how a creature goes missing mid-game.
//
// The three refusals below had never run, and each is a card name somebody could
// genuinely print: a parenthesis in the name, an empty pair, a set code where
// the id goes.
func TestOnlyForgesOwnEffectNamesAreRecognisedAsItsOwn(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name  string
		forge bool
		why   string
	}{
		{"Gilded Goose (136)'s Effect", true, "the shape Forge writes"},
		{"Arahbo, Roar of the World (203)'s Effect", true, "a comma in the host"},
		{"Bronzehide Lion's Effect", false, "no id at all"},
		{"Gilded Goose 136)'s Effect", false, "no bracket opens the id"},
		{"Gilded Goose ()'s Effect", false, "an empty id"},
		{"Gilded Goose (1a6)'s Effect", false, "an id that is not a number"},
		{"(12)'s Effect", false, "an id and no host"},
		{"Gilded Goose", false, "a card, plainly"},
	} {
		if got := forgeEffectName(c.name); got != c.forge {
			t.Errorf("%q read as Forge's own = %v (%s)", c.name, got, c.why)
		}
	}
}

// And the whole guard, as the board asks it: an untyped card sitting in the
// command zone is Forge's bookkeeping, and a *commander* there is not — the
// second one cost a commander every line it had for the rest of a game.
func TestTheCommandZoneGuardKeepsRealCards(t *testing.T) {
	t.Parallel()
	if !isForgeEffect("Battlefield", "Gilded Goose (136)'s Effect", "") {
		t.Error("an effect name on the battlefield was read as a real card")
	}
	if !isForgeEffect("Command", "Mana Pool", "") {
		t.Error("an untyped card in the command zone was read as a real card")
	}
	if isForgeEffect("Command", "Arahbo, Roar of the World",
		"Legendary Creature - Cat Avatar") {
		t.Error("a commander in the command zone was read as Forge's own " +
			"bookkeeping, which costs it every line for the rest of the game")
	}
}

// The reader never raises a beat for a line it dropped, which is the one
// property every case above shares — asserted once over all of them so a new
// quiet line cannot add a beat without this failing.
func TestAQuietLineRaisesNothingAtAll(t *testing.T) {
	t.Parallel()
	for _, line := range []string{
		`{"t":"life","game":1,"seat":1}`,
		`{"t":"outcome","game":1,"said":""}`,
		`{"t":"counters","game":1,"seat":1,"id":11,"card":"Gilded Goose"}`,
		`not json at all`,
		`{"t":"nonsense","game":1}`,
		`{`,
	} {
		p := aScribedGame()
		log, game := p.Feed(line)
		if log != nil || game != nil {
			t.Errorf("%s closed a game", line)
		}
		if len(p.events) != 0 {
			t.Errorf("%s raised %+v", line, p.events)
		}
	}
	// And a line that is not JSON at all goes to the prose tally rather than
	// being dropped, because Forge's own complaints arrive on this stream.
	p := aScribedGame()
	p.Feed("Could not load deck - alpha, match cannot start")
	if len(p.prose.Output.DeckLoadFailures) == 0 {
		t.Error("Forge's own complaint about a deck it could not load was dropped")
	}
}
