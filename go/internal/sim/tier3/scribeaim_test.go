package tier3_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/sim/tier3"
)

// Two questions the bus could always answer and nobody had asked it: **who was
// cast, and what was aimed at**.
//
// Both were blocked on the same wrong belief — that a `CardView` is all the
// scribe can see. It is all the *bus hands over*, which is a different claim:
// `StackItemView` carries an ability's targets and always did, and
// `Card.wasCast()` is one `Game.findById` away from the view the event names.
// `docs/FORGE.md` records what was measured; this holds the reading to it.
//
// Every line below is verbatim from one real match — Atla Palani/Dinosaurs
// against Arahbo/Cats, seed 11, played on this laptop on 2026-08-27.

// The Atla Palani match: a creature cast, and a creature simply put there.
//
// Trimmed to the two creatures and the commander, and to nothing else: the
// whole game is fifteen hundred lines and the argument needs sixteen.
//
// Bronzehide Lion is cast — hand, stack, battlefield, `"entered":"cast"`.
// End-Raze Forerunners is a seven-mana Boar that never touches a hand or a
// stack at all: it goes **library to battlefield** off an Atla Palani egg and
// arrives `"entered":"put"`. Those two arrivals were drawn identically until
// now, and the second is the moment that deck exists for.
func puttingItThereMatch() []string {
	const cats = `"seat":2,"who":"Arahbo, Roar of the World — Cats"`
	const dinos = `"seat":1,"who":"Atla Palani, Nest Tender — Naya Dinosaurs"`
	const lion = `,"card":"Bronzehide Lion","power":3,"toughness":3` +
		`,"types":"Creature - Cat"`
	const boar = `,"card":"End-Raze Forerunners","power":7,"toughness":7` +
		`,"types":"Creature - Boar","keywords":"Vigilance,Trample,Haste"`
	const arahbo = `,"card":"Arahbo, Roar of the World","power":5,"toughness":5` +
		`,"types":"Legendary Creature - Cat Avatar"`
	return []string{
		`{"t":"game","game":1}`,
		`{"t":"zone","game":1,"zone":"Command","mode":"in",` + cats + `,"id":203` + arahbo + `}`,
		`{"t":"seat","game":1,"seat":1,"who":"Atla Palani, Nest Tender — Naya Dinosaurs","life":40}`,
		`{"t":"seat","game":1,"seat":2,"who":"Arahbo, Roar of the World — Cats","life":40}`,
		`{"t":"turn","game":1,"turn":1,` + dinos + `,"life":40}`,
		// Cast: out of the hand, onto the stack, onto the battlefield. The
		// stack lines are dropped by the board and are kept here anyway,
		// because the sequence is what makes the arrival a cast rather than a
		// claim about one.
		`{"t":"zone","game":1,"zone":"Hand","mode":"out",` + cats + `,"id":136` + lion + `}`,
		`{"t":"zone","game":1,"zone":"Stack","mode":"in","id":136` + lion + `}`,
		`{"t":"cast","game":1,` + cats + `,"id":136` + lion + `}`,
		`{"t":"zone","game":1,"zone":"Stack","mode":"out","id":136` + lion + `}`,
		`{"t":"zone","game":1,"zone":"Battlefield","mode":"in","entered":"cast",` +
			cats + `,"id":136` + lion + `}`,
		// Eminence: the commander is still in the command zone, has never been
		// cast, and is pumping the cat that just landed.
		`{"t":"ability","game":1,` + cats + `,"trigger":true,"target_id":136,` +
			`"target":"Bronzehide Lion","targets":"136","zone":"Command","id":203` + arahbo + `}`,
		// The same commander's *attack* pump, which picks its creature with
		// `Defined$` and therefore targets nothing at all.
		`{"t":"ability","game":1,` + cats + `,"trigger":true,"zone":"Battlefield","id":203` + arahbo + `}`,
		`{"t":"turn","game":1,"turn":9,` + dinos + `,"life":28}`,
		// Put there: library straight to battlefield, no hand and no stack.
		`{"t":"zone","game":1,"zone":"Library","mode":"out",` + dinos + `,"id":99` + boar + `}`,
		`{"t":"zone","game":1,"zone":"Battlefield","mode":"in","entered":"put",` +
			dinos + `,"id":99` + boar + `}`,
		`{"t":"result","game":1,"ms":1000,"seat":1,` +
			`"winner":"Atla Palani, Nest Tender — Naya Dinosaurs"}`,
	}
}

// A creature put onto the battlefield is told apart from one that was cast.
//
// **This is the whole point of the change.** Before it, every arrival looked
// the same, so Atla Palani flipping an egg into a seven-mana Boar was drawn
// exactly like somebody paying seven mana for it — which is the opposite of
// what that deck is interesting for. Nearly every non-token arrival really was
// cast a beat earlier and the room has already shown it; the handful that were
// not are the ones worth a scene.
func TestACreaturePutOntoTheBattlefieldIsToldApartFromOneCast(t *testing.T) {
	t.Parallel()
	log := fed(t, puttingItThereMatch())

	how := map[string]string{}
	for _, e := range log.Events {
		if e.Kind == tier3.EventEnters {
			how[e.Card] = e.Entered
		}
	}
	if got := how["Bronzehide Lion"]; got != "cast" {
		t.Errorf("Bronzehide Lion entered %q, want %q — it was cast out of a "+
			"hand and the room has already shown somebody paying for it",
			got, "cast")
	}
	if got := how["End-Raze Forerunners"]; got != "put" {
		t.Errorf("End-Raze Forerunners entered %q, want %q — it went from "+
			"library to battlefield off an egg and was never cast at all",
			got, "put")
	}
}

// An arrival nobody described is not a `put`, and the difference is a deploy.
//
// **The trap this guards is live, and ADR 45's eighth ruling is the same
// shape.** A worker image built before the scribe learned to ask sends a
// battlefield arrival with no `entered` on it at all, and so does every match
// already in the ledger. A reader that folded that absence into "put onto the
// battlefield" would hand every creature in every older match the scene that
// belongs to the four that earned it — a room confidently telling a newcomer
// that a Bronzehide Lion appeared out of thin air.
//
// The frozen `testdata/scribed-match.ndjson` is exactly that older stream and
// holds not one `entered`, so the corpus already runs this path. This says out
// loud what it has to produce.
func TestAnArrivalNobodyDescribedIsNotAPut(t *testing.T) {
	t.Parallel()
	// The same match with the field cut out of it, which is what an older
	// scribe sends.
	var older []string
	for _, line := range puttingItThereMatch() {
		older = append(older, strings.NewReplacer(
			`"entered":"cast",`, "", `"entered":"put",`, "").Replace(line))
	}
	if strings.Contains(strings.Join(older, "\n"), `"entered"`) {
		t.Fatal("the older stream still carries an entered field; the guard " +
			"would pass against the very thing it is written for")
	}
	log := fed(t, older)

	arrivals := 0
	for _, e := range log.Events {
		if e.Kind != tier3.EventEnters {
			continue
		}
		arrivals++
		if e.Entered != "" {
			t.Errorf("%s entered %q from a stream that never said — want the "+
				"empty string, because nobody answered the question",
				e.Card, e.Entered)
		}
	}
	if arrivals == 0 {
		t.Fatal("no permanent arrived at all; the guard read nothing")
	}
}

// An eminence trigger names the cat it made bigger.
//
// Zone alone said a commander in the command zone had done *something*. The
// target is the half that makes it a picture, and it was on `StackItemView` the
// whole time — which was only ever asked `isTrigger()`.
func TestAnEminenceTriggerNamesWhatItPumped(t *testing.T) {
	t.Parallel()
	log := fed(t, puttingItThereMatch())

	// The beat carries the name, because a sentence needs one.
	named := 0
	for _, e := range log.Events {
		if e.Kind != tier3.EventAbility || e.Zone != "Command" {
			continue
		}
		named++
		if e.Target != "Bronzehide Lion" {
			t.Errorf("the eminence beat was aimed at %q, want %q",
				e.Target, "Bronzehide Lion")
		}
	}
	if named != 1 {
		t.Fatalf("the account raised %d command-zone abilities, want 1", named)
	}

	// The board carries the id, because a name cannot say *which* Bronzehide
	// Lion and a room drawing an arrow has to point at one card.
	var aimed [][]int
	for _, step := range log.Board.Steps {
		for _, ability := range step.Abilities {
			if ability.Zone == "Command" {
				aimed = append(aimed, ability.Targets)
			}
		}
	}
	if len(aimed) != 1 || len(aimed[0]) != 1 || aimed[0][0] != 136 {
		t.Errorf("the board's command-zone abilities pointed at %v, want one "+
			"ability aimed at card 136", aimed)
	}
}

// Most abilities are aimed at nothing, and that is the data rather than a gap.
//
// **The half that would rot silently.** Seventeen of seventy-five abilities in
// the measured match carried a target; the other fifty-eight are surveil
// triggers, quest counters, and Arahbo's own *attack* pump, which picks its
// creature with `Defined$` rather than targeting it. A room that drew an arrow
// per ability would invent three of every four — so the empty case has to stay
// empty, and a reader that quietly defaulted it to the ability's own source
// would look completely reasonable.
func TestAnAbilityAimedAtNothingCarriesNoTarget(t *testing.T) {
	t.Parallel()
	log := fed(t, puttingItThereMatch())

	found := false
	for _, step := range log.Board.Steps {
		for _, ability := range step.Abilities {
			if ability.Zone != "Battlefield" {
				continue
			}
			found = true
			if len(ability.Targets) != 0 {
				t.Errorf("Arahbo's attack pump was aimed at %v; it uses "+
					"`Defined$` and targets nothing", ability.Targets)
			}
		}
	}
	if !found {
		t.Fatal("no battlefield ability was read at all; the guard read nothing")
	}
	for _, e := range log.Events {
		if e.Kind == tier3.EventAbility && e.Zone == "Battlefield" && e.Target != "" {
			t.Errorf("the untargeted beat named %q as its target", e.Target)
		}
	}
}

// An ability's targets cross the wire as ids, and a broken one is dropped.
//
// **Rendered rather than matched**, for the reason this package has already
// recorded once about `AttachedTo`: the field is `omitempty` on a slice, so a
// reader of the struct passes just as happily against a wire that dropped it.
// And zero is [tier3.BoardCard]'s own "no card", so a malformed entry folded
// leniently would point a room's arrow at a card that does not exist — which
// draws as a card having gone missing rather than as a bad line.
func TestAnAbilitysTargetsCrossTheWireAsIds(t *testing.T) {
	t.Parallel()
	log := fed(t, []string{
		`{"t":"game","game":1}`,
		`{"t":"seat","game":1,"seat":1,"who":"A","life":40}`,
		`{"t":"turn","game":1,"turn":1,"seat":1,"who":"A","life":40}`,
		// Two targets — never seen in a real match, and the reason the wire
		// carries a list rather than one field.
		`{"t":"ability","game":1,"seat":1,"who":"A","trigger":true,"zone":"Command",` +
			`"target_id":136,"target":"Bronzehide Lion","targets":"136,159",` +
			`"id":203,"card":"Arahbo, Roar of the World","power":5,"toughness":5,` +
			`"types":"Legendary Creature - Cat Avatar"}`,
		// A list with a nonsense entry and a zero in it. Both are dropped, and
		// neither takes the good id with it.
		`{"t":"ability","game":1,"seat":1,"who":"A","trigger":true,"zone":"Battlefield",` +
			`"targets":"0,nope,159","id":204,"card":"Qasali Slingers",` +
			`"power":4,"toughness":4,"types":"Creature - Cat Warrior"}`,
		`{"t":"result","game":1,"ms":1,"draw":true}`,
	})

	var got [][]int
	for _, step := range log.Board.Steps {
		for _, ability := range step.Abilities {
			got = append(got, ability.Targets)
		}
	}
	if len(got) != 2 {
		t.Fatalf("the board recorded %d abilities, want 2", len(got))
	}
	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	const want = `[[136,159],[159]]`
	if string(raw) != want {
		t.Errorf("the targets crossed as %s, want %s — a zero or an "+
			"unreadable id is dropped, never folded in", raw, want)
	}
}
