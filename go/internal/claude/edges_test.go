package claude

import (
	"context"
	"encoding/json"
	"math"
	"math/big"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/deckread"
	"github.com/aasquier/sylvan-library/go/internal/pool"
	"github.com/aasquier/sylvan-library/go/internal/wire"
)

// The edges the corpora do not reach, gathered rather than scattered.
//
// Everything here is a branch that exists for a caller inside this process
// rather than for a request: a plan that already carries its answer, a cap
// that trims a list, a lookup that has nothing to look up. They are one to
// three statements each and they were all unrun, which is the usual shape of
// a tail -- and the usual reason a tail is worth taking is here too, because
// several of them are the difference between an honest answer and a
// plausible one.

// A plan that already carries its answer is a **job born finished**, and the
// four Run functions must hand that answer straight back rather than calling.
//
// This is the whole point of the check/run split (a dossier took 236 seconds
// on the deployed instance): the free half decides, and a stored row or a
// stance of `off` becomes a result rather than a refusal so the client's
// response shape never forks. A Run that called anyway would spend a search
// on an answer it already had -- and the endpoint here is the zero
// [Endpoint], which has no credential, so anything that reached the wire
// would fail rather than pass quietly.
func TestAPlanThatAlreadyAnsweredIsNeverCalledFor(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	dossier := &DossierPlan{Answer: &DossierReport{Slug: "mini", Reason: "stored"}}
	research := &ResearchPlan{Answer: &ResearchReport{Question: "q", Reason: "off"}}
	ask := &AskPlan{Answer: &AskReport{Reason: "off"}}
	proposal := &ProposalPlan{Answer: &ProposalReport{Reason: "off"}}

	for _, p := range []interface{ NeedsCall() bool }{dossier, research, ask, proposal} {
		if p.NeedsCall() {
			t.Errorf("%T says it still needs a call with an answer in hand", p)
		}
	}
	for _, p := range []interface{ NeedsCall() bool }{
		&DossierPlan{}, &ResearchPlan{}, &AskPlan{}, &ProposalPlan{},
	} {
		if !p.NeedsCall() {
			t.Errorf("%T says it needs no call with no answer at all", p)
		}
	}

	got, err := RunDossier(ctx, nil, dossier, DossierRun{})
	if err != nil || got.Reason != "stored" {
		t.Errorf("RunDossier on a finished plan: %+v / %v", got, err)
	}
	gotResearch, err := RunResearch(ctx, nil, research, ResearchRun{})
	if err != nil || gotResearch.Reason != "off" {
		t.Errorf("RunResearch on a finished plan: %+v / %v", gotResearch, err)
	}
	gotAsk, err := RunAsk(ctx, nil, ask, ThemeRun{})
	if err != nil {
		t.Errorf("RunAsk on a finished plan: %v", err)
	} else if report, ok := gotAsk.(AskReport); !ok || report.Reason != "off" {
		t.Errorf("RunAsk answered %#v", gotAsk)
	}
	gotProposal, err := RunProposal(ctx, nil, proposal, ThemeRun{})
	if err != nil {
		t.Errorf("RunProposal on a finished plan: %v", err)
	} else if report, ok := gotProposal.(ProposalReport); !ok || report.Reason != "off" {
		t.Errorf("RunProposal answered %#v", gotProposal)
	}
}

// The store never fails the feature, and that promise has three edges nobody
// had driven: a nil store handed a clock, a row whose stored bytes are not
// JSON, and a value that will not marshal at all.
//
// The middle one is the one that matters. A `result_json` that is not JSON
// can only come from outside this code -- a hand-edited row, a restore, a
// write that was interrupted -- and it is served **raw** into the response,
// so passing it through would put bytes the client cannot parse into a page
// that otherwise looks fine. A miss is the only honest answer: the dossier
// gets written again.
func TestTheStoreMissesRatherThanServingBytesThatAreNotADossier(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	var none *DossierStore
	if none.WithClock(frozenClock) != nil {
		t.Error("a nil store grew a clock")
	}

	rec, _ := scratchLedger(t)
	store := NewDossierStore(rec.DB(), nil).WithClock(frozenClock)
	if _, err := rec.DB().ExecContext(ctx,
		"INSERT INTO dossier_cache (key, oracle_id, commander, result_json, created_at) "+
			"VALUES (?, ?, ?, ?, ?)",
		"torn", "o", "c", "{not json at all", frozenNow); err != nil {
		t.Fatal(err)
	}
	if hit := store.Get(ctx, "torn"); hit != nil {
		t.Errorf("a row whose bytes are not JSON was served as %s -- it goes "+
			"into the response raw", hit.Result)
	}

	// A value the canonical encoder refuses is a warning and a dropped write,
	// never a failed dossier: the answer is already in the caller's hand.
	store.Put(ctx, "unstorable", "o", "c", map[string]any{"nan": math.NaN()})
	if hit := store.Get(ctx, "unstorable"); hit != nil {
		t.Errorf("an unencodable dossier was stored as %s", hit.Result)
	}
}

// A mode is its tool set (ADR 15), and that holds at the point the schemas
// are BUILT and not only at the door: a mode naming a tool the registry does
// not have is refused rather than quietly offered one tool fewer.
func TestAModeCannotNameAToolTheRegistryDoesNotHave(t *testing.T) {
	t.Parallel()
	if _, err := (Mode{Name: "invented", ToolNames: []string{"set_card_field"}}).Schemas(); err == nil {
		t.Error("a mode built schemas for a tool that is not in the registry")
	}
	// A real mode's schemas build, which is the floor that stops the check
	// above passing for the wrong reason.
	mode, err := GetMode(ModeSlotArgument)
	if err != nil {
		t.Fatal(err)
	}
	schemas, err := mode.Schemas()
	if err != nil {
		t.Fatalf("%s: %v", mode.Name, err)
	}
	if len(schemas) == 0 {
		t.Errorf("%s built no schemas at all", mode.Name)
	}
	// And an unknown mode name is a refusal that lists what there is, because
	// the registry is data and data can be asked about.
	if _, err := GetMode("no-such-mode"); err == nil {
		t.Error("an invented mode name resolved")
	} else if !strings.Contains(err.Error(), ModeSlotArgument) {
		t.Errorf("the refusal does not say what there is: %v", err)
	}
}

// `toolParam` is where a schema becomes the SDK's own type, and its two
// guards are about a shape that arrives rather than a value: a tool with no
// `input_schema` object is a load-time fault, and an absent `required` list
// becomes an empty one rather than a null.
//
// The null matters more than it looks. `required: null` on the wire is not
// `required: []`, and the tools block is hashed for the prompt cache -- so a
// tool that rendered one where the other was recorded would invalidate the
// cache on every turn, for free and invisibly.
func TestABuiltToolFillsItsRequiredListRatherThanSendingANull(t *testing.T) {
	t.Parallel()
	if _, err := toolParam(map[string]any{"name": "get_deck"}); err == nil {
		t.Error("a schema with no input_schema was built into a tool")
	} else if !strings.Contains(err.Error(), "get_deck") {
		t.Errorf("the refusal does not name the tool: %v", err)
	}

	param, err := toolParam(map[string]any{
		"name": "get_deck", "description": "a deck",
		"input_schema": map[string]any{"properties": map[string]any{}},
	})
	if err != nil {
		t.Fatalf("a schema with no required list: %v", err)
	}
	if param.OfTool == nil {
		t.Fatal("the built parameter carries no tool")
	}
	if param.OfTool.InputSchema.Required == nil {
		t.Error("`required` was left nil, which crosses the wire as null " +
			"rather than as the recorded empty list")
	}
}

// A persona may arrive as raw JSON bytes rather than as a decoded value --
// which is what a route holding an undecoded body field hands over -- and it
// is read rather than refused.
func TestAPersonaArrivesAsRawBytesAsWellAsAsAString(t *testing.T) {
	t.Parallel()
	// The default's own key, so the name is the roster's rather than one this
	// test invented -- and the read is what is in question, not the name.
	byName, err := GetPersona(DefaultPersona)
	if err != nil {
		t.Fatalf("the default persona does not resolve: %v", err)
	}
	byBytes, err := GetPersona(json.RawMessage(`"` + DefaultPersona + `"`))
	if err != nil {
		t.Fatalf("a persona named in raw bytes: %v", err)
	}
	if byBytes.Key != byName.Key {
		t.Errorf("raw bytes resolved to %q where the string resolved to %q",
			byBytes.Key, byName.Key)
	}
	// Bytes that are not a name at all fall through to the same refusal a bad
	// string gets, rather than to a decoder's complaint.
	if _, err := GetPersona(json.RawMessage(`{`)); err == nil {
		t.Error("truncated bytes resolved to a persona")
	}
	// And no persona at all is the house default rather than a refusal.
	fallback, err := GetPersona(nil)
	if err != nil {
		t.Fatalf("no persona: %v", err)
	}
	if fallback.Key != DefaultPersona {
		t.Errorf("no persona resolved to %q, want the default %q", fallback.Key, DefaultPersona)
	}
}

// An all-padding capture is refused, which is the edge `len(s)%4` and the
// padding count between them do not catch: `====` is four characters, two of
// which would be the padding, and the body it leaves is empty.
//
// It decodes to nothing, so accepting it would send the API an image that is
// zero bytes long and wait for a model to say something about it.
func TestACaptureThatIsNothingButPaddingIsRefused(t *testing.T) {
	t.Parallel()
	for _, s := range []string{"==", "====", "========"} {
		if got, err := strictB64Decode(s); err == nil {
			t.Errorf("%q decoded to %d byte(s) rather than being refused", s, len(got))
		}
	}
	// The floor: a real capture still decodes, and an empty string is empty
	// rather than refused -- the caller's own check is what rejects that.
	if got, err := strictB64Decode("YQ=="); err != nil || string(got) != "a" {
		t.Errorf("a real base64 string decoded to %q / %v", got, err)
	}
	if got, err := strictB64Decode(""); err != nil || len(got) != 0 {
		t.Errorf("an empty string decoded to %q / %v", got, err)
	}
}

// The number grammar's edges, which are all about a separator or a digit that
// is not ASCII -- the two things `strconv` would either accept differently or
// not accept at all.
//
// An underscore is a separator **only between digits**, which is the rule
// `int()` applies and the reason `1_0.5` is ten and a half while `1._5` is
// not a number. Getting that wrong does not fail loudly: it widens what a
// budget field accepts, silently.
func TestTheNumberGrammarRefusesASeparatorThatIsNotBetweenDigits(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{"1._5", "1_.5", "1.5_e3", "١é2", "1é2"} {
		if got, err := floatFromString(raw); err == nil {
			t.Errorf("%q read as the float %v", raw, got)
		}
	}
	// The floor: the spellings the grammar does take, including a separator
	// that IS between digits and a non-ASCII digit.
	for _, tc := range []struct {
		raw  string
		want float64
	}{
		{"1_0.5", 10.5}, {"١٢", 12}, {"-inf", math.Inf(-1)}, {"1e400", math.Inf(1)},
	} {
		got, err := floatFromString(tc.raw)
		if err != nil {
			t.Errorf("%q: %v", tc.raw, err)
			continue
		}
		if got != tc.want {
			t.Errorf("%q read as %v, want %v", tc.raw, got, tc.want)
		}
	}

	// And the integer side: a `json.Number` whose literal is not a number at
	// all fails both readings -- the exact big.Int one and the float
	// fallback -- rather than landing on a zero.
	if got, err := IntValue(json.Number("not-a-number")); err == nil {
		t.Errorf("an unreadable number literal read as the integer %v", got)
	}
	if got, err := IntValue(json.Number("42")); err != nil || got.Cmp(big.NewInt(42)) != 0 {
		t.Errorf("a real number literal read as %v / %v", got, err)
	}
}

// Both card resolvers short-circuit when there is nothing to look up, and the
// two of them answer **differently** on purpose -- which is ADR 26 in two
// functions, and the reason this asserts the counts and not just the lists.
//
// A dossier's competitors are dropped and counted, so a name the pool lacks
// disappears from a section that is prose about the meta. A research answer's
// cards are LABELLED, `in_pool: false`, because a card spoiled ahead of the
// next refresh is a third of what that surface is for.
func TestTheTwoCardResolversAgreeOnNothingToLookUpAndDisagreeOnWhy(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	allowed := map[string]bool{"s1": true}

	// Well-formed items with no usable card name: every one counts as dropped,
	// and the count is the ITEM count rather than the count of resolvable
	// names.
	items := []any{
		map[string]any{"why": "no card key at all"},
		map[string]any{"card": "   "},
		map[string]any{"card": nil},
		"not an object at all, and not counted",
	}
	got, dropped, err := Competitors(ctx, nil, items, allowed)
	if err != nil {
		t.Fatalf("competitors with nothing to look up: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("competitors resolved %d rows from names that are not there", len(got))
	}
	if dropped != 3 {
		t.Errorf("dropped %d, want the three well-formed items -- the count is "+
			"the item count, not the resolvable-name count", dropped)
	}

	cards, unresolved, err := ResolveCards(ctx, nil, []any{"  ", nil}, MaxResearchCards)
	if err != nil {
		t.Fatalf("resolving nothing: %v", err)
	}
	if len(cards) != 0 || unresolved != 0 {
		t.Errorf("resolving nothing gave %d card(s) and %d unresolved", len(cards), unresolved)
	}
	if cards == nil {
		t.Error("the card list is null rather than empty -- the client iterates it")
	}
}

// The research card list is capped and de-duplicated before the pool is asked,
// because both are the model's to get wrong and the reader pays for either.
//
// The cap is spelled twice -- the schema tells the model and this is the
// rule -- for the same reason `search_cards` caps its own limit: a schema is
// advice.
func TestTheResearchCardListIsCappedAndDeduplicatedBeforeTheLookup(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	withPool(t, func(c *pool.Conn) {
		many := []any{}
		for i := 0; i < MaxResearchCards*3; i++ {
			many = append(many, "Invented Card "+string(rune('A'+i)))
		}
		cards, unresolved, err := ResolveCards(ctx, c, many, MaxResearchCards)
		if err != nil {
			t.Fatalf("resolving a long list: %v", err)
		}
		if len(cards) != MaxResearchCards {
			t.Errorf("a list of %d resolved to %d cards, want the cap of %d",
				len(many), len(cards), MaxResearchCards)
		}
		if unresolved != MaxResearchCards {
			t.Errorf("%d unresolved, want all %d -- these names are invented",
				unresolved, MaxResearchCards)
		}

		// The same name three ways is one card. A model repeating itself
		// would otherwise fill the reader's list with one card.
		cards, _, err = ResolveCards(ctx, c,
			[]any{"Sol Ring", "sol ring", "SOL RING"}, MaxResearchCards)
		if err != nil {
			t.Fatalf("resolving repeats: %v", err)
		}
		if len(cards) != 1 {
			t.Errorf("the same card named three ways resolved to %d entries", len(cards))
		}
	})
}

// `kv` is how every ordered payload in this package is read, and a key that
// is not there answers nil rather than panicking -- which is what lets a
// brief be assembled from a payload whose optional halves are absent.
func TestReadingAKeyThatIsNotThereAnswersNothing(t *testing.T) {
	t.Parallel()
	rows := wire.OrderedMap{{Key: "card", Value: "Sol Ring"}}
	if got := kv(rows, "card"); got != "Sol Ring" {
		t.Errorf("reading a key that is there gave %#v", got)
	}
	if got := kv(rows, "commander_card"); got != nil {
		t.Errorf("reading a key that is not there gave %#v", got)
	}
	if got := kv(nil, "card"); got != nil {
		t.Errorf("reading a key out of nothing gave %#v", got)
	}
}

// The brief's mana value prefers the POOL's number over the deck file's,
// because the deck file records what somebody typed and the pool records what
// the card costs -- and falls back to the deck's rather than to nothing,
// because a card the pool lacks still has a curve position worth naming.
func TestAManaValueComesFromThePoolAndFallsBackToTheDeckFile(t *testing.T) {
	t.Parallel()
	typed := 7.0
	entry := deckread.CardJSON{Name: "Invented Card", CMC: &typed}

	if got, ok := manaValue(entry, deckread.NamedCard{CMC: 2}); !ok || got != 2 {
		t.Errorf("with a pool row the mana value is %v (%v), want the pool's 2", got, ok)
	}
	if got, ok := manaValue(entry, nil); !ok || got != typed {
		t.Errorf("with no pool row the mana value is %v (%v), want the deck file's %v",
			got, ok, typed)
	}
	if _, ok := manaValue(deckread.CardJSON{Name: "Invented Card"}, nil); ok {
		t.Error("a card with no mana value anywhere reported one")
	}
}

// The sibling list is capped, because the interview's opening message carries
// it and a hundred-card category would put the whole deck in the prompt --
// paid for on every turn, and read by nobody.
func TestTheSiblingRationalesAreCappedBeforeTheyReachThePrompt(t *testing.T) {
	t.Parallel()
	rows := []deckread.CardJSON{}
	for i := 0; i < MaxSiblings*2; i++ {
		rows = append(rows, deckread.CardJSON{
			Name: "Sibling " + string(rune('A'+i)), Category: "ramp", Why: "ramp",
		})
	}
	// The card the interview is about is excluded from its own sibling list.
	rows = append(rows, deckread.CardJSON{Name: "Subject", Category: "ramp", Why: "the subject"})
	payload := wire.OrderedMap{{Key: "cards", Value: rows}}

	got := siblingRationales(payload, "ramp", "Subject")
	if len(got) != MaxSiblings {
		t.Errorf("%d siblings reached the prompt, want the cap of %d", len(got), MaxSiblings)
	}
	for _, row := range got {
		if name, _ := kv(row, "name").(string); name == "Subject" {
			t.Error("the card being interviewed is in its own sibling list")
		}
	}
	// A category nothing else is in is an empty list rather than a null.
	if alone := siblingRationales(payload, "land", "Subject"); alone == nil || len(alone) != 0 {
		t.Errorf("a category of one gave %#v, want an empty list", alone)
	}
}

// `contains` is the list membership the dropped-name bookkeeping rests on,
// and its false arm is the one nothing had run -- which is the arm that
// decides whether a name is recorded as dropped at all.
func TestANameThatIsNotInAListIsNotInIt(t *testing.T) {
	t.Parallel()
	list := []string{"Sol Ring", "Arcane Signet"}
	if !contains(list, "Arcane Signet") {
		t.Error("a name in the list read as absent")
	}
	if contains(list, "Invented Card") {
		t.Error("a name that is not in the list read as present")
	}
	if contains(nil, "Sol Ring") {
		t.Error("a name was found in no list at all")
	}
}
