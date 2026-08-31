package api

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"sort"
	"strings"

	"github.com/aasquier/sylvan-library/go/internal/pool"

	"github.com/aasquier/sylvan-library/go/internal/claude"
	"github.com/aasquier/sylvan-library/go/internal/deck"
	"github.com/aasquier/sylvan-library/go/internal/floats"
	"github.com/aasquier/sylvan-library/go/internal/jobs"
	"github.com/aasquier/sylvan-library/go/internal/sim/tier3"
	"github.com/aasquier/sylvan-library/go/internal/sim/tier3/ledger"
	"github.com/aasquier/sylvan-library/go/internal/wire"
)

// The two Forge routes: Tier 3 matches, shaped
// for the UI (ADR 35).
//
// Tier 3 reaches the app the way every slow thing does — a job — with the
// division `themeruns` argued and this inherits deliberately: **everything
// refusable is refused in the request**, and only the JVM run is queued. A
// missing deck is a 404, a bad games count is a 422, an absent Forge is a 503,
// and a deck Forge cannot fully play is a 422 that names the cards — the
// pre-flight reads a zip on the request thread and was designed to.
//
// Unlike the sim family, planning failures here are *not* deferred into the
// job. That deferral is the sim family's recorded contract; this surface was
// new when it was written, so it got the honest shape from day one: distinct
// refusals with status codes, and a job that only ever fails for runtime
// reasons.
//
// **The lane is FORGE, and one worker is load-bearing.** The work waits on a
// Forge subprocess, so CPU (it would block Tier 1 for minutes) and NET (two
// JVMs at once race the shared `.dck` directory) are both wrong. Serialising
// the lane also makes the dedupe story simple: a second identical request
// joins the live job via the plan's key, and a *different* match queues
// honestly behind the first.
//
// Nothing is cached. Forge is seeded here the way Tier 1 is (same default,
// same doctrine — an unseeded sample is not reproducible), but a cache needs a
// key that names the engine's behaviour and the distribution can be upgraded
// under us; until someone measures that a repeat ask is common, in-flight
// dedupe is the whole memory.
//
// ForgeKind is what `/api/jobs` calls one of these.
const ForgeKind = "sim.forge"

// ForgeCaveat rides with the numbers rather than near them, because CLAUDE.md
// requires it quoted with them.
const ForgeCaveat = "Forge's AI is best with aggro and midrange, poor with " +
	"control, and bad with most combo — read results per archetype, never as " +
	"a single ranking. Games that hit the clock are reported apart from " +
	"draws: a clock-out is the measurement giving up, not a game outcome."

// ForgeClock is Forge's `-c`: seconds before a game is called a draw. 300
// rather than Forge's 120, because CLAUDE.md says so and a measured Trostani
// game ran 134 seconds — a shorter clock turns real games into fake draws.
const ForgeClock = 300

const (
	// ForgeGamesDefault is what a request that does not say gets.
	ForgeGamesDefault = 10
	// ForgeGamesMax caps the ask. Twenty games of the slowest measured
	// heads-up pairing is ~10 minutes of wall clock; the lane is serial, so
	// the cap is what keeps one enthusiastic request from parking the Forge
	// for an hour.
	ForgeGamesMax = 20
)

// forgeStatus answers: is the Forge reachable from this process?
// A fact about the environment.
//
// Two environments, one contract. With the hosted worker configured (ADR 35's
// second half), the answer is yes on configuration alone — no network, no
// machine woken to ask, exactly as `/api/claude` answers on the presence of a
// key rather than by calling Anthropic. Otherwise this probes the two things a
// local run needs — the distribution's jar and a JVM new enough — without
// booting either. `why` is maintainer-facing prose (it names paths and version
// floors); the client renders its own words, which is commandment 10 doing its
// usual work.
func (a *API) forgeStatus() (bool, *string) {
	if a.forge.Configured() {
		return true, nil
	}
	if _, err := a.forge.DesktopJar(); err != nil {
		why := err.Error()
		return false, &why
	}
	if _, err := a.forge.JavaBinary(); err != nil {
		why := err.Error()
		return false, &why
	}
	return true, nil
}

// forgeGate is `GET /api/forge` — the gate the Simulator asks before it offers
// real games.
func (a *API) forgeGate(w http.ResponseWriter, r *http.Request) {
	available, why := a.forgeStatus()
	wire.JSON(w, http.StatusOK, wire.OrderedMap{
		{Key: "available", Value: available},
		{Key: "why", Value: why},
	})
}

// forgeGames is the games dial: clamped to the cap, and never below one.
//
// The count goes through the recorded integer grammar
// (`claude.IntValue`), which is not `strconv.Atoi` — `"1_0"` is ten, a float
// truncates, and a bool is 0 or 1. A value the grammar refuses raises,
// and that is a **wart, recorded rather than tidied**: the plan
// runs in the request with no handler for it, so `{"games": "many"}`
// is an uncaught 500 rather than the 422 it should be. Pinned by
// `TestAGamesCountThatIsNotANumberIsTheRecordedFiveHundred`.
func forgeGames(body map[string]any) (int, error) {
	raw, ok := body["games"]
	if !ok {
		raw = ForgeGamesDefault
	}
	n, err := claude.IntValue(raw)
	if err != nil {
		return 0, err
	}
	// Clamped into [1, ForgeGamesMax] over an unbounded integer.
	//
	// **The upper clamp is now unreachable from the route**, which refuses an
	// over-cap ask in words before this runs (see [forgeGamesAsked]). It stays
	// because this is the last line of defence on a number that reaches
	// Forge's command line and every clock in [tier3.MatchBudget]: a caller
	// that found another way in still cannot ask the arena for a thousand
	// games. A silent clamp is a poor answer to a person and a fine answer to
	// a bad request.
	if n.Cmp(big.NewInt(ForgeGamesMax)) > 0 {
		return ForgeGamesMax, nil
	}
	if n.Cmp(big.NewInt(1)) < 0 {
		return 1, nil
	}
	return int(n.Int64()), nil
}

// forgeGamesAsked is the games count as it was *asked for*, before any
// clamping — what the route needs to tell "twenty-five" from "twenty".
//
// A number the grammar cannot read is not this function's business: it returns
// the error and the route lets the recorded 500 happen further down, exactly
// as before. Above `math.MaxInt` the answer is "more than the cap" without
// narrowing, because a `*big.Int` asked to be an `int` is a different bug.
func forgeGamesAsked(body map[string]any) (int, error) {
	raw, ok := body["games"]
	if !ok {
		return ForgeGamesDefault, nil
	}
	n, err := claude.IntValue(raw)
	if err != nil {
		return 0, err
	}
	if !n.IsInt64() || n.Int64() > int64(ForgeGamesMax) {
		return ForgeGamesMax + 1, nil
	}
	return int(n.Int64()), nil
}

// forgeSeed is the seed dial: the default when absent or empty, otherwise
// the recorded integer grammar.
//
// A `*big.Int` because the recorded grammar is unbounded and the seed is
// echoed
// into the result, the dedupe key and Forge's own command line as text. An
// int64 would silently answer a different number for a seed past 2**63, and a
// seed is a promise.
func forgeSeed(body map[string]any) (*big.Int, error) {
	raw, ok := body["seed"]
	if !ok || raw == nil {
		return big.NewInt(DefaultSeed), nil
	}
	if s, isString := raw.(string); isString && s == "" {
		return big.NewInt(DefaultSeed), nil
	}
	return claude.IntValue(raw)
}

// forgeRow is one game as the client renders it, whichever moment it arrives
// in.
//
// The same shape serves twice: inside the finished result's `rows`, and on the
// job's `partial` while the match is still playing (the match theater). One
// builder is what makes a streamed row and its final self identical — a
// theater that showed one shape live and another in the tale of the tape would
// be the drift the wire codec exists to prevent, one layer up.
type forgeRow struct {
	Game   int     `json:"game"`
	Winner *string `json:"winner"`
	// Seconds is a `floats.Float` and not a `float64`, which is the one
	// thing about this struct that has to be decided rather than typed:
	// four seconds rounded to one place is recorded as `4.0`, and
	// `encoding/json` writes `4`. Same number to a client, different bytes
	// in DevTools and in anything that ever hashes this payload.
	Seconds  floats.Float `json:"seconds"`
	Turns    *int         `json:"turns"`
	Draw     bool         `json:"draw"`
	TimedOut bool         `json:"timed_out"`
}

func newForgeRow(g tier3.GameResult, slug *string) forgeRow {
	row := forgeRow{Game: g.Index,
		Seconds:  floats.Float(floats.RoundTo(float64(g.Milliseconds)/1000, 1)),
		Turns:    g.Turns,
		Draw:     g.Draw && !g.TimedOut,
		TimedOut: g.TimedOut}
	if !g.TimedOut {
		row.Winner = slug
	}
	return row
}

// ForgeBeatsMax is how many beats of one game reach the browser.
//
// The parser's own [tier3.EventCap] is 10,000, which bounds a runaway game in
// *memory*; this is the tighter bound on what crosses to a person, and it
// exists because the job's `partial` is re-fetched every poll. A measured
// nine-turn game raises about a hundred beats (`events.go` did the counting),
// so four hundred is roughly four times the real thing and lands the partial
// near 30KB in the worst case and near 8KB in the ordinary one — which, at the
// client's poll interval, is the ~20KB/s a watched match costs. Stated rather
// than left to be discovered, because it is the one number here that anybody
// paying for bandwidth would want to know.
//
// The cut is announced, never silent: `truncated` says a game outran it, the
// same way [tier3.EventLog] does one layer down.
const ForgeBeatsMax = 400

// forgeBeat is one beat of a game as the client renders it.
//
// **Seats become slugs here**, which is the one decision this shape makes:
// [tier3.GameEvent] carries `seat` and `target_seat` because the parser reads
// them off Forge's own lines, and every other thing this file hands a client
// names a deck instead (`forgeRow.Winner` is a slug, not a seat). A browser
// that had to map a seat number to a deck would be re-deriving something the
// job already knows, and getting it wrong the first time two decks were
// submitted in the other order.
type forgeBeat struct {
	Kind string `json:"kind"`
	Turn int    `json:"turn,omitempty"`
	// Who is the deck that acted, or that an outcome is about. Null when the
	// line named no player, which is most of them — Forge usually names the
	// card instead.
	Who  *string `json:"who"`
	Card string  `json:"card,omitempty"`
	// ID is that card's board id, so a beat and the picture can be pointed at
	// each other. Zero on a match played by the prose parser, which has no ids
	// to give — a consumer must read zero as "not said" rather than as a card.
	//
	// A name cannot answer "which one": two Egg Tokens are one string between
	// them, and a room lighting up the card a beat is about would light the
	// wrong one as often as not.
	ID int `json:"id,omitempty"`
	// Zone is where an `ability` beat's source was standing — `Command` for an
	// eminence trigger, whose card never moves and so has no other sign.
	Zone string `json:"zone,omitempty"`
	// Trigger is whether an `ability` beat was raised by the game rather than
	// activated by a player: "triggers" against "activates".
	Trigger bool `json:"trigger,omitempty"`
	// Target is the card on the other end: blocked, damaged, or — on an
	// `ability` beat — the creature an eminence trigger made bigger.
	Target string `json:"target,omitempty"`
	// Entered is how an `enters` beat's permanent reached the battlefield —
	// `cast`, or `put` there by something else.
	//
	// **Only `put` is news.** A creature that was cast has already had its
	// moment a beat earlier, and the room showed somebody paying for it; a
	// creature *put* onto the battlefield — Atla Palani cracking an egg into a
	// seven-mana Boar — arrived with nothing said, and looked identical.
	//
	// **The empty string is a third state and it is not `put`.** A match played
	// by the prose parser cannot answer this at all, and neither can a worker
	// image built before the scribe learned to ask, so every game already in
	// the ledger sends nothing here. A room that read absence as `put` would
	// tell a newcomer that every creature in every older match appeared out of
	// thin air — which is the exact distinction this field exists to draw, run
	// backwards. See [tier3.GameEvent.Entered].
	Entered string `json:"entered,omitempty"`
	// Against is the deck on the other end: attacked, or damaged.
	Against *string `json:"against"`
	Amount  int     `json:"amount,omitempty"`
	Life    *int    `json:"life,omitempty"`
	Note    string  `json:"note,omitempty"`
}

// forgeBeats is one game's beats, as the room watching them receives them.
type forgeBeats struct {
	Game  int         `json:"game"`
	Beats []forgeBeat `json:"beats"`
	// Truncated is set when the game outran [ForgeBeatsMax] or the parser's
	// own cap. Either way the beats kept are the first ones, because a game's
	// opening is what makes the rest of it legible.
	Truncated bool `json:"truncated"`
	// Board is the battlefield, and it is null for a match played by a worker
	// without the scribe (ADR 42's fourth decision: the parser stays as the
	// floor). A room handed no board draws the account alone, which is what
	// every room did before there was one.
	Board *forgeBoard `json:"board"`
}

// forgeBoardSeat is one side of the table.
//
// The slug is here and nowhere else in the board, deliberately. Everything
// below refers to a seat by its number, because a board is a *place* — a
// browser lays out two sides and needs a stable index for them, and threading
// a slug through three hundred changes would be the same fact three hundred
// times. This is the one row that turns the number into a deck.
type forgeBoardSeat struct {
	Seat int     `json:"seat"`
	Slug *string `json:"slug"`
	// Name is what Forge called the player, which is the deck's own title. It
	// is the fallback for a seat whose slug the room cannot resolve.
	Name string `json:"name"`
	Life int    `json:"life"`
	// Commanders is this seat's commanders **by board id, in the deck's own
	// order** — one for most decks and two for a pairing.
	//
	// It is here rather than derived in the browser because the browser cannot
	// derive it. Everything that begins in a command zone looks alike from the
	// stream: a commander, a partner, a background, and a *companion* all
	// arrive as a card in the `command` zone on step zero, and only the deck
	// knows which is which. The order matters too — a pairing's two thrones
	// keep the order `deck.yaml` lists them in, so the same commander is on
	// the same side of the rail every game.
	//
	// Ids rather than names, because the browser is holding a dictionary keyed
	// on ids and a name it would have to match twice: once against Forge's
	// spelling and once against a front face.
	Commanders []int `json:"commanders,omitempty"`
	// Shape is what this seat's command zone *is* — [commandSolo] or
	// [commandPartners] — and it is empty for a board shaped before any deck
	// was known.
	//
	// **The zone has exactly three legal shapes and a list of ids names none
	// of them** (Aaron, 2026-08-26: *"at most it should just be two slots for
	// partners, one for a singular commander, or a second companion devoted
	// slot… those are the only combinations possible in that zone"*). Two ids
	// in [forgeBoardSeat.Commanders] and one commander that happens to have
	// been cloned read alike from a browser, and a room that wants to say
	// "partners" out loud — which commandment 3 wants — had no word to say it
	// with.
	//
	// Read off `deck.yaml`'s own declaration rather than worked out from the
	// cards: `internal/gate` has already refused a deck whose pairing is not
	// legal (`CheckPair`) and whose companion is not one (`CheckCompanion`),
	// so the deck's list of commanders is a validated answer and re-deriving
	// it here would be a second place for the rules to live. Counted from the
	// declared *names*, not from the ids resolved below, so a pairing whose
	// second half Forge could not implement is still a pairing.
	//
	// The companion is beside this rather than in it, because it can come with
	// either shape and is not part of what leads the deck.
	Shape string `json:"shape,omitempty"`
	// Companion is this seat's companion by board id, or zero for the decks
	// that brought none.
	//
	// **It is not a commander and it must not be counted as one.** A companion
	// really does sit in the command zone — `Player.assignCompanion` moves it
	// there at setup, checked in Forge's own bytecode — so the rule the board
	// used to identify commanders by ("nothing but a commander begins in the
	// command zone") is false for exactly these decks, and it was charging
	// commander tax for a card that has never had any.
	Companion int `json:"companion,omitempty"`
}

// forgeCommandZone is what a seat's command zone holds before a card has
// moved, read off the deck rather than off the game.
//
// The deck is the only honest source. Forge announces a card arriving in the
// command zone and says nothing about *why* it is there — `CardView` does
// carry `isCommander()`, but reaching it means another field on every zone
// line and another worker image, to learn something `deck.yaml` already
// states outright.
// The shapes a command zone can have — [forgeBoardSeat.Shape].
//
// Two, not three: a companion is a card *beside* whatever leads the deck, so
// it rides its own field and pairs with either of these. Words rather than a
// count, because "two" is not the fact — "partners" is, and a browser drawing
// two chairs should not have to know that two is what partners means.
const (
	commandSolo     = "commander"
	commandPartners = "partners"
)

type forgeCommandZone struct {
	// Commanders in the deck's own order. `deck.Deck.Commander` is a list
	// because a pairing is two cards, and this keeps that order.
	Commanders []string
	// Companion, or empty. `deck.Deck.Companion` is a pointer for the same
	// reason; flattened here because "no companion" and "the empty name" are
	// the same thing to a lookup.
	Companion string
}

// forgeBoardCard is one card, named and painted once.
type forgeBoardCard struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Token bool   `json:"token,omitempty"`
	Types string `json:"types,omitempty"`
	Seat  int    `json:"seat,omitempty"`
	// Image is the whole card face, which is what a permanent looks like lying
	// on a table. Art is the painting alone, for the cards a room wants to
	// show large. Either may be empty: a board draws a plate with no painting
	// on it rather than refusing to draw.
	Image string `json:"image,omitempty"`
	Art   string `json:"art,omitempty"`
	// Artist is carried for tokens, whose painting is chosen here rather than
	// looked up — somebody painted it, and rule 9 says name them.
	Artist string `json:"artist,omitempty"`
	// Faces is every name this card answers to, for the cards with more than
	// one. Absent for everything else, which is nearly every card.
	//
	// **It exists because Forge renames a card when it is cast as its other
	// half, and this board only ever learns a card's name once.** Bonecrusher
	// Giant is drawn into a hand as *Bonecrusher Giant*, and the moment its
	// Adventure is cast Forge's view calls it *Stomp* — so the beat says Stomp,
	// the middle of the arena goes looking for a card called Stomp among cards
	// called Bonecrusher Giant, finds nothing, and sets the name in type on a
	// dark plate. A whole black card with a title on it, in the one moment the
	// room exists to show a spell (Aaron, 2026-08-28).
	//
	// **This was once only the layouts printing both names on one picture, and
	// a modal double-faced card drew that same black plate for it** (Aaron,
	// 2026-08-29: *"MDF cards are not rendering intelligently... blanked out
	// black card with just the text when it is played as a creature... and same
	// deal when played as a land"*). The old rule was right while the record
	// carried one painting: answering to a back face's name with a front face's
	// picture looks correct and is worse than the plate. The record carries both
	// paintings now — see `FaceImages` — so the two questions are separate, and
	// `facesOf` and `facePicturesOf` are them.
	Faces []string `json:"faces,omitempty"`
	// FaceTypes is each face's own type line, index-aligned with Faces.
	//
	// **Because a card's two halves are two different kinds of spell**, and the
	// moment the room could find the card by its second name it started naming
	// the first one's type: Locthwain Scorn is a Sorcery and the plate under it
	// read *"casts Enchantment"*, off Virtue of Persistence's type line — a
	// confident sentence about the wrong half, caught on a real board before it
	// landed. Taken from the same `A // B` split the names are, so the two
	// lists cannot fall out of step.
	FaceTypes []string `json:"face_types,omitempty"`
	// FaceImages is each face's own painting, index-aligned with Faces, and
	// absent for the cards whose faces share one — an Adventure, a split card,
	// a flip card, which is what `Image` already is.
	//
	// **Present is the room's permission to change the picture.** A card with
	// this is a card the room may hold up by either face: the back of a modal
	// double-faced card played as a land is a painting of a land, and drawing
	// the sorcery on its front instead is the quiet version of the same bug the
	// black plate was the loud version of. A card without it either has one
	// picture, in which case `Image` is right for both names, or comes from a
	// pool that cannot say — and both are answered by drawing what we have
	// rather than by guessing which half we are looking at.
	//
	// **The credit rides the picture.** These are whole card images, so each
	// carries its own artist and copyright line printed on it, which is how a
	// card on this board has always been credited — see `Artist`, which is
	// carried only for tokens, whose printing this program chose rather than
	// looked up. Swapping to a back face therefore swaps in that face's own
	// credit, which matters: eight double-faced cards in the pool are painted
	// by two different artists.
	FaceImages []string `json:"face_images,omitempty"`
	// Layout is Scryfall's own word for how the card is printed — `adventure`,
	// `split`, `flip` — and it is carried only alongside `Faces`, because the
	// only thing that reads it is the room deciding *where on the picture* the
	// half being cast actually is. A card with `FaceImages` has no such
	// whereabouts: its halves are not on one picture, and the room draws the
	// whole of the right one instead of a glass over part of the wrong one.
	Layout string `json:"layout,omitempty"`
	// Mana is whether this card makes mana, from Scryfall's own
	// `produced_mana` rather than from reading rules text here.
	//
	// It exists for one layout rule and is a *card fact* rather than that
	// rule: a player keeps mana rocks back with the lands, because what those
	// rows answer is "what can this deck pay for" (Aaron, 2026-08-25: "mana
	// producing artifacts could really stay back with the lands"). Which row
	// that becomes is the browser's business; whether Sol Ring makes mana is
	// the pool's.
	Mana bool `json:"mana,omitempty"`
	// Makes is which mana this printing taps for -- Scryfall's
	// `produced_mana`, gated on the same [pool.CardRecord.MakesMana] test
	// `Mana` is, so a Food engine that has never made a mana in its life does
	// not report five colours here either.
	//
	// **It is what the card taps for, never what this game's pool received.**
	// Forge's `GameEventManaPool` carries a player, a mode and a colour set,
	// and no source of any kind; the tap event carries a card and no mana. The
	// two facts arrive on separate lines with no key between them, so nothing
	// on this pipe can say *this permanent* filled the pool (ADR 44, ADR 45).
	// A board drawing this beside a tapped permanent is stating a fact about
	// the printing, which is true whatever the permanent was tapped for.
	Makes []string `json:"makes,omitempty"`
	// Keywords is Scryfall's list for the card, sent whole rather than
	// filtered.
	//
	// **Unfiltered on purpose.** The board draws a small mark for the
	// keywords it has a mark for and ignores the rest, and which ones those
	// are is a drawing decision that belongs where the drawing is — adding a
	// glyph should be one file's change, not two and a deploy. The cost is a
	// few hundred short strings on a payload already measured in tens of
	// kilobytes.
	Keywords []string `json:"keywords,omitempty"`
	// CopiedBy is the board id of the card whose ability made this one a copy,
	// and zero for every card that is not one. Populate is what it is for —
	// see [tier3.BoardCard.CopiedBy], which records why it is **not** the card
	// this one was copied *from*.
	CopiedBy int `json:"copied_by,omitempty"`
}

// forgeBoard is the battlefield as the room receives it.
type forgeBoard struct {
	Seats []forgeBoardSeat  `json:"seats"`
	Cards []forgeBoardCard  `json:"cards"`
	Steps []tier3.BoardStep `json:"steps"`
}

// boardArt is one card's painting, resolved once per match.
type boardArt struct {
	Image      string
	Art        string
	Artist     string
	Mana       bool
	Makes      []string
	Keywords   []string
	Faces      []string
	FaceTypes  []string
	FaceImages []string
	Layout     string
}

// faceTypesOf is each face's own type line, index-aligned with `faces`, or nil
// when the card's type line does not split into the same number of halves.
//
// **Nil rather than a best effort**, and the reason is what the caller does
// with it: the room reads `faceTypes[half]` to say what kind of spell was cast,
// and an index into a list that does not line up is a confident sentence about
// the wrong card. Falling back to the card's own type line is right — it is
// what every single-faced card gets — and it is what a nil produces.
func faceTypesOf(rec *pool.CardRecord, faces []string) []string {
	kinds := strings.Split(rec.TypeLine, " // ")
	if len(kinds) != len(faces) {
		return nil
	}
	return kinds
}

// layoutOf is the word the room should use for how this card is printed —
// Scryfall's own, except where Scryfall stopped making a distinction the room
// still needs.
//
// **Aftermath is that exception, and it is the whole reason this function
// exists.** Amonkhet's split-with-a-turn cards used to carry
// `layout: "aftermath"`; Scryfall now files them under `split` and marks them
// with the `Aftermath` keyword instead. The room does not care what the family
// is called — it cares *where the two halves are printed*, and the two are not
// in the same places at all. An ordinary split card's gutter is 47% down its
// picture and an Aftermath card's is 54%, and, worse, they print their halves
// in **opposite order**: a split card is read sideways so its first face is
// the lower one, and an Aftermath card is read upright so its first face is
// the upper one. A room told only "split" would draw its glass over Fire while
// saying Ice, on exactly half the cards, and be right the other half.
//
// The keyword rather than the frame effect, because that is where Scryfall put
// it: a real record for Cut // Ribbons carries `keywords: ["Aftermath"]` and no
// `frame_effects` at all. Matched without regard to case for the reason every
// string from outside this program is — the spelling is somebody else's.
func layoutOf(rec *pool.CardRecord) string {
	if rec.Layout == "split" {
		for _, word := range rec.Keywords {
			if strings.EqualFold(word, "Aftermath") {
				return "aftermath"
			}
		}
	}
	return rec.Layout
}

// facesOf is the names this card answers to, or nil for a card with one name.
//
// Taken from the record's own combined name rather than from `card_faces`,
// because that is the same string `pool.Conn.GetCards` splits to *find* the
// card by a face name — so the set of names that can reach this record and the
// set of names it admits to are one list, read one way. The record's parsed
// faces are asked only to *corroborate* it: a name that splits in two on a
// record that does not carry two faces is a record this cannot describe, and
// the honest answer to that is nothing at all.
//
// **This used to be gated on a list of layouts, and the list was answering the
// wrong question.** It named `adventure`, `split`, `flip` and `aftermath` —
// the layouts that print two names on *one picture* — because at the time the
// record carried one picture, so "does this card answer to two names" and "are
// both names on the picture we are holding up" had the same answer and one
// gate could serve both. They are not the same question, and the moment
// [pool.CardRecord.Faces] arrived they stopped having the same answer: a modal
// double-faced card played as its land back really is on the board under the
// back's name, and the room can now hold up the back's own painting to it.
//
// So the two questions are separated. This one is the names; [facePicturesOf]
// is whether each of them has a picture of its own. A card that answers to a
// name the room cannot paint is still better than a card that answers to
// nothing — the beat's plate says *Agadeem, the Undercrypt* and *Land* rather
// than a name the arena could not place at all.
func facesOf(rec *pool.CardRecord) []string {
	faces := strings.Split(rec.Name, " // ")
	if len(faces) < 2 || len(rec.Faces) != len(faces) {
		return nil
	}
	return faces
}

// facePicturesOf is each face's own painting, index-aligned with `faces`, and
// nil for a card whose faces are all printed on the one picture.
//
// **This is the question the old layout list was really asking**, and asking
// it of the record rather than of a list of names is the point: Scryfall put
// `image_uris` on the *card* when both names are on one piece of cardboard and
// on the *faces* when each is a painting of its own, so the encoding states the
// distinction outright. Six layouts carry an `A // B` name and exactly two of
// them — `transform` and `modal_dfc` — paint each face; a `prepare` card, the
// newest of the one-picture family, is handled by this without anybody having
// heard of it.
//
// **All or nothing.** A half-filled list is an index that answers confidently
// for the wrong face, which is the exact fault the old gate existed to prevent:
// showing the front of a card whose back was played looks right, and is worse
// than showing no picture. So one face without a painting stands the whole card
// down to the card's own, which for a one-picture layout is the truth and for a
// pool too old to know is the plate.
func facePicturesOf(rec *pool.CardRecord, faces []string) []string {
	if len(rec.Faces) != len(faces) {
		return nil
	}
	out := make([]string, len(faces))
	for i, face := range rec.Faces {
		if face.ImageNormal == nil {
			return nil
		}
		out[i] = *face.ImageNormal
	}
	return out
}

// resolveBoardArt fills `known` with the paintings for any card in `cards` it
// does not already hold.
//
// **Two lookups, because a token is not a card.** A real card is
// [pool.Conn.GetCards], which answers out of `oracle_cards`; a token is
// [pool.Conn.TokenArtFor], which answers out of `printings` with the card's
// *earliest* printing rather than its newest. That distinction is the one this
// project keeps relearning: `GetCards` answering with the newest is what put
// Teenage Mutant Ninja Turtles art on the Grand Coliseum, and the newest Food
// printing is a Secret Lair about Bilbo's second breakfast.
//
// `known` is the caller's, and it spans the whole match: two games of the same
// pairing name almost the same hundred cards, so the second game costs one
// round trip for its tokens and nothing else.
//
// **No pool is not an error here.** A match is worth watching without
// paintings, and refusing to shape a board because the card pool has not been
// refreshed would cost somebody a match they are already watching.
func (a *API) resolveBoardArt(ctx context.Context, cards []tier3.BoardCard,
	known map[string]boardArt) {
	var wantCards, wantTokens []string
	for _, card := range cards {
		if _, seen := known[card.Name]; seen || card.Name == "" {
			continue
		}
		// Marked so a name the pool cannot answer is asked about once per
		// match rather than once per game.
		known[card.Name] = boardArt{}
		if card.Token {
			wantTokens = append(wantTokens, pool.TokenName(card.Name))
			continue
		}
		wantCards = append(wantCards, card.Name)
	}
	if len(wantCards) == 0 && len(wantTokens) == 0 {
		return
	}
	_ = a.usePool(ctx, func(c *pool.Conn) error {
		if len(wantCards) > 0 {
			found, err := c.GetCards(ctx, wantCards)
			if err != nil {
				return err
			}
			for name, rec := range found {
				art := boardArt{Mana: rec.MakesMana(), Keywords: rec.Keywords}
				// Only when the card taps for it itself. `produced_mana`
				// alone would hand Smothering Tithe five colours off its
				// Treasure's reminder text, and a board is not the place to
				// repeat that.
				//
				// **A land is asked differently, and it has to be.**
				// `MakesMana` reads the card's rules text with reminder text
				// stripped — and a basic Forest's *entire* text is reminder
				// text, `({T}: Add {G}.)`, so it strips to nothing and the
				// commonest tapped mana source in Magic answers "no". So do
				// the original duals and every land whose only ability is
				// printed in parentheses. That has never shown, because the
				// one thing `Mana` decides is whether an *artifact* stands
				// with the lands. A land's mana is a land's own, so a land
				// with a `produced_mana` is taken at its word.
				if art.Mana || rec.IsLand() {
					art.Makes = rec.ProducedMana
				}
				if rec.ImageNormal != nil {
					art.Image = *rec.ImageNormal
				}
				if rec.ImageArtCrop != nil {
					art.Art = *rec.ImageArtCrop
				}
				if faces := facesOf(rec); faces != nil {
					art.Faces, art.Layout = faces, layoutOf(rec)
					art.FaceTypes = faceTypesOf(rec, faces)
					art.FaceImages = facePicturesOf(rec, faces)
				}
				known[name] = art
			}
		}
		if len(wantTokens) > 0 {
			found, err := c.TokenArtFor(ctx, wantTokens)
			if err != nil {
				return err
			}
			// Back under Forge's spelling, which is the key every card in the
			// dictionary is named by.
			for _, card := range cards {
				if !card.Token {
					continue
				}
				if art, ok := found[strings.ToLower(pool.TokenName(card.Name))]; ok {
					known[card.Name] = boardArt{Image: art.Image,
						Art: art.Image, Artist: art.Artist}
				}
			}
		}
		return nil
	})
}

// grantKeywords fills [tier3.BoardChange.Granted] on every change that reported
// a live keyword set: the keywords that card instance has which its **printing
// does not carry**.
//
// **This is the half of the question Go can answer and `internal/sim/tier3`
// cannot.** The scribe sends the live set off Forge's view, which is honest and
// complete and says nothing about where any of it came from; Scryfall's list
// for the printing lives here, because this is the layer that already resolves
// every card to paint it. Kaheera standing beside a Beast is the case: the
// Beast's live set holds Vigilance, its printing does not, so the vigilance is
// something else's doing and the board can finally show it (Aaron, 2026-08-26).
//
// **It says that a keyword was granted, never by what.** Forge erases
// attribution at its view boundary — `KeywordView` carries the word, the enum,
// a title and reminder text, and not one field about a source or even the
// `isIntrinsic` flag its own model has. The card to blame is unavailable at any
// price on this pipe, so the copy that renders this must not name one. ADR 44.
//
// The steps are rebuilt rather than written through: the reel belongs to the
// stored job and a room re-reading a match must not find it edited underneath.
func grantKeywords(steps []tier3.BoardStep,
	printed map[int][]string) []tier3.BoardStep {
	touched := false
	for _, step := range steps {
		for _, change := range step.Changes {
			if change.Live != nil {
				touched = true
			}
		}
	}
	if !touched {
		return steps
	}
	out := make([]tier3.BoardStep, len(steps))
	copy(out, steps)
	for i := range out {
		if out[i].Changes == nil {
			continue
		}
		changes := make([]tier3.BoardChange, len(out[i].Changes))
		copy(changes, out[i].Changes)
		for j := range changes {
			if changes[j].Live == nil {
				continue
			}
			granted := beyondPrinting(*changes[j].Live, printed[changes[j].ID])
			changes[j].Granted = &granted
		}
		out[i].Changes = changes
	}
	return out
}

// beyondPrinting is the keywords in `live` that `oracle` does not account for.
//
// **The two lists speak different dialects of the same language**, which is the
// only hard part. Forge writes a keyword as its card scripts do — `Ward:2`,
// `Protection from red` — and Scryfall writes the bare keyword — `Ward`,
// `Protection`. Compared as plain strings, a card that *prints* Ward 2 would
// have it reported as granted, which is precisely the small confident lie ADR 44
// refuses. So a live keyword counts as printed when the oracle name is a prefix
// of it **at a word boundary**: `ward` accounts for `ward:2`, `protection` for
// `protection from red`, and `ward` does not account for a hypothetical
// `warden`. Whole-string equality is the ordinary case and falls out of the same
// test.
//
// An empty result is a real answer and is returned as an empty slice rather than
// nil, for the same reason [tier3.BoardChange.Counters] is a pointer: a creature
// that has just lost the last keyword something gave it must be able to say so.
func beyondPrinting(live, oracle []string) []string {
	granted := []string{}
	for _, word := range live {
		if word == "" || accountedFor(word, oracle) {
			continue
		}
		granted = append(granted, word)
	}
	return granted
}

// accountedFor is whether one live keyword is explained by the printing.
func accountedFor(word string, oracle []string) bool {
	live := strings.ToLower(strings.TrimSpace(word))
	if cut, _, ok := strings.Cut(live, ":"); ok {
		live = strings.TrimSpace(cut)
	}
	for _, printed := range oracle {
		name := strings.ToLower(strings.TrimSpace(printed))
		if name == "" {
			continue
		}
		if live == name {
			return true
		}
		// A word boundary, so that `ward` explains `ward 2` and not `warden`.
		if strings.HasPrefix(live, name+" ") {
			return true
		}
	}
	return false
}

// newForgeBoard shapes one game's board for the room.
//
// `steps` is cut to `kept` — the number of beats that survived
// [ForgeBeatsMax] — because a step and a beat are the same moment seen twice
// and a room paces them together. Cutting them apart would drift the picture
// away from the account by however many beats were dropped.
func newForgeBoard(reel *tier3.BoardReel, seats map[int]string,
	command map[int]forgeCommandZone, art map[string]boardArt,
	kept int) *forgeBoard {
	if reel == nil {
		return nil
	}
	// One index per seat, so a name is resolved against that player's own
	// cards. Two decks in one match can run the same commander, and a board id
	// belongs to exactly one of them.
	held := map[int]map[string]int{}
	for _, card := range reel.Cards {
		by, ok := held[card.Seat]
		if !ok {
			by = map[string]int{}
			held[card.Seat] = by
		}
		// First seen wins: a clone of somebody's commander is a second card
		// with the same name, and the throne belongs to the original. Forge
		// numbers cards as it makes them and the dictionary is in first-seen
		// order, so the original is the one already here.
		if _, seen := by[card.Name]; !seen {
			by[card.Name] = card.ID
		}
	}
	out := &forgeBoard{
		Seats: make([]forgeBoardSeat, 0, len(reel.Seats)),
		Cards: make([]forgeBoardCard, 0, len(reel.Cards)),
		Steps: reel.Steps,
	}
	if kept >= 0 && kept < len(out.Steps) {
		out.Steps = out.Steps[:kept]
	}
	if out.Steps == nil {
		out.Steps = []tier3.BoardStep{}
	}
	for _, seat := range reel.Seats {
		row := forgeBoardSeat{Seat: seat.Seat, Name: seat.Name, Life: seat.Life}
		if slug, ok := seats[seat.Seat]; ok {
			row.Slug = &slug
		}
		zone := command[seat.Seat]
		for _, name := range zone.Commanders {
			if id := boardIDOf(name, held[seat.Seat]); id != 0 {
				row.Commanders = append(row.Commanders, id)
			}
		}
		switch len(zone.Commanders) {
		case 0:
			// No deck behind this seat. The zone goes back to being a pile,
			// which is what every board drew before it was named at all.
		case 1:
			row.Shape = commandSolo
		default:
			row.Shape = commandPartners
		}
		row.Companion = boardIDOf(zone.Companion, held[seat.Seat])
		out.Seats = append(out.Seats, row)
	}
	printed := map[int][]string{}
	for _, card := range reel.Cards {
		painted := art[card.Name]
		printed[card.ID] = painted.Keywords
		out.Cards = append(out.Cards, forgeBoardCard{
			ID: card.ID, Name: card.Name, Token: card.Token,
			Types: card.Types, Seat: card.Seat, CopiedBy: card.CopiedBy,
			Image: painted.Image, Art: painted.Art, Artist: painted.Artist,
			Mana: painted.Mana, Makes: painted.Makes,
			Keywords:   painted.Keywords,
			Faces:      painted.Faces,
			FaceTypes:  painted.FaceTypes,
			FaceImages: painted.FaceImages,
			Layout:     painted.Layout})
	}
	out.Steps = grantKeywords(out.Steps, printed)
	return out
}

// boardIDOf is the board id of a card this seat holds, by its `deck.yaml`
// name, or zero when the game has no such card.
//
// **Zero is a real answer, not a failure.** The pre-flight drops a card Forge
// does not implement, so a commander can be absent from a match that ran
// anyway; a throne that is simply not drawn says that more honestly than a
// throne holding nothing would.
//
// The two spellings are reconciled through [tier3.Resolve], which is the same
// function that decided what to write into the `.dck` in the first place —
// Scryfall's combined `A // B` never appears in Forge's index, so a
// double-faced commander is on the board under one of its faces. Reusing the
// exporter's own answer is what keeps the two from drifting apart.
func boardIDOf(name string, held map[string]int) int {
	if name == "" || len(held) == 0 {
		return 0
	}
	if id, ok := held[name]; ok {
		return id
	}
	index := make(map[string]bool, len(held))
	for card := range held {
		index[card] = true
	}
	return held[tier3.Resolve(name, index)]
}

// newForgeBeats shapes one game's log for the browser.
//
// `seats` is the seat-to-slug map the plan built from the deck order, which is
// the same map [tier3.SimRun.Seats] holds — built by hand here because
// narration happens *during* the run, when there is no run yet, exactly as the
// CLI's `seatsOf` does for the same reason.
func newForgeBeats(log tier3.EventLog, seats map[int]string,
	command map[int]forgeCommandZone, art map[string]boardArt) forgeBeats {
	slug := func(seat int) *string {
		if s, ok := seats[seat]; ok && seat != 0 {
			return &s
		}
		return nil
	}
	events := log.Events
	truncated := log.Truncated
	if len(events) > ForgeBeatsMax {
		events = events[:ForgeBeatsMax]
		truncated = true
	}
	out := forgeBeats{Game: log.Game, Truncated: truncated,
		Beats: make([]forgeBeat, 0, len(events))}
	out.Board = newForgeBoard(log.Board, seats, command, art, len(events))
	for _, e := range events {
		out.Beats = append(out.Beats, forgeBeat{
			Kind: string(e.Kind), Turn: e.Turn, Who: slug(e.Seat),
			Card: e.Card, ID: e.ID, Zone: e.Zone, Trigger: e.Trigger,
			Target: e.Target, Against: slug(e.TargetSeat),
			Amount: e.Amount, Life: e.Life, Note: e.Note,
			Entered: e.Entered})
	}
	return out
}

// forgeSeat is one deck's line in the result.
type forgeSeat struct {
	Slug    string `json:"slug"`
	Name    string `json:"name"`
	Address string `json:"address"`
	Wins    int    `json:"wins"`
}

// forgeResult is the payload a match becomes. Medians and tails, never a mean
// alone.
type forgeResult struct {
	Decks          []forgeSeat   `json:"decks"`
	Games          int           `json:"games"`
	Played         int           `json:"played"`
	Draws          int           `json:"draws"`
	TimedOut       int           `json:"timed_out"`
	MedianSeconds  *floats.Float `json:"median_seconds"`
	MaxSeconds     *floats.Float `json:"max_seconds"`
	StartupSeconds floats.Float  `json:"startup_seconds"`
	WallSeconds    floats.Float  `json:"wall_seconds"`
	Clock          int           `json:"clock"`
	Seed           *big.Int      `json:"seed"`
	Rows           []forgeRow    `json:"rows"`
	Caveat         string        `json:"caveat"`
	// Beats is **every** game's narration and board, in the order they were
	// played, so a finished match can be watched back a game at a time.
	//
	// Two problems, one field. The job's `partial` carries one game — the
	// newest — and is cleared the moment the job finishes: a short match can
	// finish inside a single poll interval, so the room would draw an empty
	// field for a match played in full, and a long one would show whichever
	// game happened to be current when somebody last polled and no way back to
	// the others. A match is worth watching *after* it is over, which is when
	// somebody actually has the time.
	//
	// **The whole match, and the size is stated rather than discovered.** A
	// game costs about 26KB shaped, so a five-game match is ~130KB and the
	// twenty-game ceiling is ~520KB — sent once, at the end, on a request
	// somebody made deliberately, against a partial that was re-sending 26KB
	// two or three times a second while the match ran. [ForgeReplayGames]
	// bounds it.
	//
	// `omitempty` on a slice, deliberately: a match nobody asked to narrate —
	// which is every row in the frozen corpus — comes out byte-identical to
	// what was recorded.
	Beats []forgeBeats `json:"beats,omitempty"`
}

// ForgeReplayGames is how many games of a finished match can be watched back.
//
// The ceiling exists because the whole replay crosses in one response and a
// twenty-game match would be half a megabyte of it. Ten is past the point
// anybody watches — a match that long is a measurement, and the tale of the
// tape below the board is what it is read from. The games kept are the
// **first** ones, matching every other cut in this file: a match reads
// forwards, and the game somebody stopped watching at is the one they want
// back.
const ForgeReplayGames = 10

// forgePartial is what the job's `partial` carries while the match plays.
//
// `Beats` is **the most recent game's, and only that game's** — null until the
// first game closes, and null for the whole match when nobody asked to
// narrate. The reason it is one game rather than all of them: the partial is
// re-sent on every poll, so carrying the match's whole narration would mean
// re-sending a growing transcript two or three times a second to show beats a
// person watched a minute ago. The room accumulates what it has been handed;
// the job holds the newest thing it has to hand over.
//
// The consequence, stated because it is a real one: a game that finishes
// inside one poll interval can have its beats replaced before anybody read
// them. Games take seconds and polls take a fraction of one, so this is rare —
// and the beats are the colour, never the record. `Rows` is the record, it is
// cumulative, and it never drops a game.
type forgePartial struct {
	Rows  []forgeRow  `json:"rows"`
	Beats *forgeBeats `json:"beats"`
}

// shapeForge shapes the finished match.
//
// `wins` is counted per seat and reported per deck; real draws and clock-outs
// are separate columns because they are separate facts (the parser keeps them
// apart and this must not fold them back).
//
// **A deck played against itself reports the combined total on both lines**,
// a recorded wart, kept rather than fixed: `wins` is keyed on the
// slug, so `a_slug == b_slug` collapses two seats into one counter. Unreachable
// only by convention — nothing refuses the request — and pinned by
// `TestADeckPlayedAgainstItselfShowsTheCombinedWins`, because the guard beats
// the fix for a wart nobody has hit.
func shapeForge(decks []*deck.Deck, addresses []string, games int,
	seed *big.Int, run *tier3.SimRun, beats []forgeBeats) forgeResult {
	wins := map[string]int{}
	for _, d := range decks {
		wins[d.Slug] = 0
	}
	rows := make([]forgeRow, 0, len(run.Games()))
	for _, game := range run.Games() {
		slug := run.WinnerSlug(game)
		// A clocked-out game counts for nobody even when Forge printed a
		// winner line after the slow-match warning — the parser attaches the
		// pending timeout to whatever result line follows, and a "win" awarded
		// because the other AI ran out of thinking time is the measurement
		// giving up wearing a trophy.
		if slug != "" && !game.TimedOut {
			wins[slug]++
		}
		var winner *string
		if slug != "" {
			s := slug
			winner = &s
		}
		rows = append(rows, newForgeRow(game, winner))
	}

	seconds := make([]float64, 0, len(rows))
	for _, r := range rows {
		seconds = append(seconds, float64(r.Seconds))
	}
	sort.Float64s(seconds)

	out := forgeResult{
		Decks:          make([]forgeSeat, 0, len(decks)),
		Games:          games,
		Played:         len(rows),
		StartupSeconds: floats.Float(floats.RoundTo(run.StartupSeconds(), 1)),
		WallSeconds:    floats.Float(floats.RoundTo(run.WallSeconds, 1)),
		Clock:          ForgeClock,
		Seed:           seed,
		Rows:           rows,
		Caveat:         ForgeCaveat,
		Beats:          beats,
	}
	if len(out.Beats) > ForgeReplayGames {
		out.Beats = out.Beats[:ForgeReplayGames]
	}
	for i, d := range decks {
		out.Decks = append(out.Decks, forgeSeat{Slug: d.Slug, Name: d.Name,
			Address: addresses[i], Wins: wins[d.Slug]})
	}
	for _, r := range rows {
		if r.Draw {
			out.Draws++
		}
		if r.TimedOut {
			out.TimedOut++
		}
	}
	if len(seconds) > 0 {
		median := floats.Float(floats.RoundTo(medianOf(seconds), 1))
		out.MedianSeconds = &median
		max := floats.Float(seconds[len(seconds)-1])
		out.MaxSeconds = &max
	}
	if out.Rows == nil {
		out.Rows = []forgeRow{}
	}
	return out
}

// medianOf is the median over an already-sorted slice: the middle
// value, or the mean of the two middle values.
//
// The two-term mean is a single correctly-rounded addition, so it is the same
// number under `Fsum` and under `+` — the one float sum in this file that
// needs no argument about how it accumulates.
func medianOf(sorted []float64) float64 {
	n := len(sorted)
	i := n / 2
	if n%2 == 1 {
		return sorted[i]
	}
	return (sorted[i-1] + sorted[i]) / 2
}

// simForge is `POST /api/sim/forge` — one heads-up Forge match (ADR 35).
// Returns a **job**.
//
// Everything refusable is refused here, not in the job (the `themeruns`
// division): decks that do not resolve are 404, an uninstalled Forge is 503,
// and a deck with cards Forge does not implement is a 422 that names them —
// because a Forge game *plays on* without them and reports a winner, which is
// the one failure this surface exists to never serve.
//
// Heads-up only, and that is ADR 35 rather than a limitation to lift casually:
// measured on this hardware, 40% of four-player games hit the clock, and a
// mode whose results are mostly clock is not honest enough to ship. The CLI
// still plays pods for whoever wants to watch one.
func (a *API) simForge(w http.ResponseWriter, r *http.Request) {
	body, ok := readBody(w, r)
	if !ok {
		return
	}
	lib, err := a.library(r.Context())
	if a.refuse(w, "library", err) {
		return
	}

	type pair struct{ owner, slug string }
	var pairs []pair
	for _, side := range []string{"a", "b"} {
		raw := body[side+"_slug"]
		// The raw value's truthiness, before the stringification. A `0`
		// or an empty list is falsy and refused here; a non-empty list is
		// truthy and becomes a slug that no deck has, which is a 404.
		if !truthy(raw) {
			wire.Detail(w, http.StatusUnprocessableEntity, side+"_slug is required")
			return
		}
		owner := str(body, side+"_owner")
		if owner == "" {
			owner = lib.MyOwner()
		}
		pairs = append(pairs, pair{owner: owner, slug: str(body, side+"_slug")})
	}

	// The gate before the decks -- the recorded order: an instance
	// with no Forge answers 503 without ever asking the library who these
	// people are.
	if available, why := a.forgeStatus(); !available {
		// Said in the room's words, for `forgeTrouble`'s reason — `why` is
		// the installation's own account of itself and names paths and
		// environment variables. The log keeps it.
		if why != nil {
			a.log.Error("no arena to play in", "why", *why)
		}
		wire.Detail(w, http.StatusServiceUnavailable,
			forgeTrouble(tier3.ErrForgeNotInstalled))
		return
	}

	decks := make([]*deck.Deck, 0, len(pairs))
	addresses := make([]string, 0, len(pairs))
	// For the match ledger: the same ownership key the activity log uses (an
	// owner id, NULL for the file tier — never the URL's owner segment, which
	// is not stable across configurations).
	ownerIDs := make([]*int64, 0, len(pairs))
	for _, p := range pairs {
		src, err := lib.SourceFor(r.Context(), p.owner)
		if a.refuse(w, "source", err) {
			return
		}
		d, err := src.Get(r.Context(), p.slug)
		if a.refuse(w, "forge", err) {
			return
		}
		decks = append(decks, d)
		addresses = append(addresses, p.owner+"/"+p.slug)
		ownerIDs = append(ownerIDs, src.OwnerID())
	}

	// **A deck with nothing in it is refused here, and here is the only place
	// that can.** Coverage is a ratio, and a deck of no cards passes it
	// perfectly: nought of nought names are missing, so the pre-flight waves
	// through the one deck Forge cannot make a game out of. Forge then deals
	// it an empty library, it loses on turn one's draw, and the match reports
	// a clean sweep to the other seat -- a *wrong answer wearing a result's
	// clothes*, which is worse than an error because nothing about it looks
	// like one.
	//
	// Tier 1 learned this against this exact deck and refuses it in
	// `compile.Deck` (see `compile.NothingToSimulate`, which names it). The
	// Forge path never compiles -- Forge resolves names itself, and there is
	// no pool open here -- so the lesson never reached it and the guard had
	// to be written a second time, against the declared count.
	//
	// Before the pre-flight rather than after, because the pre-flight wakes
	// the worker machine: an empty deck should not cost a boot to refuse.
	for i, d := range decks {
		if d.TotalCards() > 0 {
			continue
		}
		wire.Detail(w, http.StatusUnprocessableEntity, addresses[i]+
			" has no cards in it yet, so no result would mean anything "+
			"— add its cards and send them in again")
		return
	}

	// **The cap is said, not applied behind somebody's back.** This used to
	// clamp: an ask for twenty-five games became twenty, nothing said so, and
	// the person who typed twenty-five watched a bar that was measuring a
	// different match than the one they asked for. Worse than either honest
	// answer, and the shape the arena's own clocks were being sized against.
	//
	// Before the pre-flight for the empty deck's reason: an ask this surface
	// will not honour should not cost a machine boot to refuse.
	if asked, err := forgeGamesAsked(body); err == nil && asked > ForgeGamesMax {
		wire.Detail(w, http.StatusUnprocessableEntity, fmt.Sprintf(
			"a bout runs to %d games at the most — these are whole games of "+
				"Commander played end to end, and the arena seats one bout at "+
				"a time. Ask for %d or fewer and send them in",
			ForgeGamesMax, ForgeGamesMax))
		return
	}

	// The pre-flight runs where the card scripts live: against the local zip,
	// or on the worker machine (which this wakes — the one request-thread cost
	// the hosted shape adds, bounded by the worker's boot budget so a machine
	// that will not come up is a 503 rather than a hang).
	hosted := a.forge.Configured()
	if err := a.preflight(r.Context(), hosted, decks); err != nil {
		switch {
		case errors.Is(err, tier3.ErrCoverageFailed):
			wire.Detail(w, http.StatusUnprocessableEntity, err.Error())
		case errors.Is(err, tier3.ErrForgeNotInstalled):
			// **The reason goes to the log and the room gets a sentence.**
			// This branch is where the Machines API's own words used to reach
			// a person: a URL, a status and raw JSON, in a room about Magic.
			// `forgeTrouble` carries the whole argument; logging first is the
			// half that makes it a redirection rather than a deletion.
			a.log.Error("the Forge worker would not answer", "error", err)
			wire.Detail(w, http.StatusServiceUnavailable, forgeTrouble(err))
		default:
			a.log.Error("the Forge pre-flight failed", "error", err)
			wire.Detail(w, http.StatusInternalServerError, forgeTrouble(err))
		}
		return
	}

	plan, err := a.planForge(decks, addresses, ownerIDs, body, hosted)
	if err != nil {
		// The integer grammar refused the games count or the seed. **The
		// recorded uncaught 500**, kept exactly: the plan runs in the
		// request with nothing catching the refusal, so the answer is the
		// plain-text three words -- not a JSON detail, which is what this
		// wrote until the real bytes were measured. See [forgeGames].
		uncaught500(w, a.log, "forge", err)
		return
	}
	a.submit(w, r, plan)
}

// preflight is coverage, computed on whichever machine holds the card scripts.
func (a *API) preflight(ctx context.Context, hosted bool, decks []*deck.Deck) error {
	if hosted {
		_, err := a.forgeWorker().CheckCoverage(ctx, decks)
		return err
	}
	_, err := a.forge.CheckCoverage(decks)
	return err
}

// forgeWorker is the hosted client, a method so a test can give the API one
// pointed at a stub shim.
func (a *API) forgeWorker() *tier3.Worker {
	if a.forgeClient != nil {
		return a.forgeClient
	}
	return &tier3.Worker{Settings: a.forge}
}

// planForge is `forgeruns.plan_forge`: one heads-up match, planned. Refusals
// happened at the route already.
//
// `decks` arrive resolved because resolving them is the route's job — it holds
// the library and the 404-versus-422 vocabulary. What this decides is the
// work: coverage has passed, so the closure is exactly one match. `addresses`
// are the `owner/slug` pairs the client asked with, echoed back so the result
// can say whose decks played without the job inventing a second naming scheme.
func (a *API) planForge(decks []*deck.Deck, addresses []string,
	ownerIDs []*int64, body map[string]any, hosted bool) (jobs.Plan, error) {
	games, err := forgeGames(body)
	if err != nil {
		return jobs.Plan{}, err
	}
	seed, err := forgeSeed(body)
	if err != nil {
		return jobs.Plan{}, err
	}
	// Asked for, never assumed. Narrating is free in time and about a hundred
	// beats a game in volume (`events.go` measured both), so the room that
	// watches a match asks for it and the surfaces that only want the tally do
	// not. Through `truthy` rather than a Go bool cast, because that is this
	// package's recorded reading of a JSON value.
	narrate := truthy(body["narrate"])

	slugs := make([]string, 0, len(decks))
	for _, d := range decks {
		slugs = append(slugs, d.Slug)
	}
	plural := "s"
	if games == 1 {
		plural = ""
	}
	label := fmt.Sprintf("Forge: %s, %d game%s", strings.Join(slugs, " vs "), games, plural)
	// The dedupe key carries narration, because a silent match and a narrated
	// one are not interchangeable answers: joining a running quiet job would
	// hand the watching room a match with no beats in it, and the room would
	// be mute for a reason nothing on screen could explain. Same decks, same
	// seed, same count and same narration is still one match, which is the
	// case dedupe was built for.
	//
	// **Appended only when narration was asked for**, and that is the frozen
	// corpus rather than an aesthetic: `forge.json` records this key's exact
	// text for silent asks, and a `|false` on the end would rewrite a golden.
	// A narrated ask is a case the recording never held, so it is free to wear
	// a suffix; a silent one comes out byte-identical to what was recorded,
	// which `TestTheLabelAndKeyAreTheRecordedText` still proves.
	key := "forge|" + strings.Join(addresses, "|") +
		fmt.Sprintf("|%d|%s", games, seed)
	if narrate {
		key += "|narrated"
	}

	worker := a.forgeWorker()
	recorder := a.matchLedger()
	return jobs.Plan{
		Kind: ForgeKind, Label: label, Lane: jobs.FORGE, Key: key,
		Run: func(rep jobs.Progress) (any, error) {
			// **Its own context.** The request is over by the time this runs,
			// and `r.Context()` is cancelled by net/http the moment the
			// handler returns — a job that took one would be a match killed
			// mid-JVM. The recorded lesson, and the one only a real server
			// test can see.
			ctx := context.Background()
			rep.Report(0, games)

			// Forge's output is streamed, so the job ticks once per finished
			// game — and each tick carries the game it just watched end,
			// shaped by the same builder the final tally uses and exposed on
			// the job's `partial` for the client to seat live. Clamped because
			// a tick is a progress report, not a result. Seat order is the
			// deck order (both run paths promise it), which is what lets a
			// slug be named before the run exists. A pre-theater shim streams
			// counts without rows; the bar still moves and `partial` simply
			// stays sparse.
			seats := map[int]string{}
			// What each seat's command zone holds before a card has moved.
			// Built here beside the slug map and for the same reason: seat
			// order is the deck order on both run paths, so the deck at a
			// seat is known before the run that fills it exists.
			command := map[int]forgeCommandZone{}
			for i, d := range decks {
				seats[i+1] = d.Slug
				zone := forgeCommandZone{Commanders: d.Commander}
				if d.Companion != nil {
					zone.Companion = *d.Companion
				}
				command[i+1] = zone
			}
			var rowsSoFar []forgeRow
			// The beats of the game that just closed, waiting for the row
			// that closes it. Both run paths hand over the beats *before* the
			// row (one pass over Forge's output, the event parser fed first),
			// so this is never stale by more than the few lines between them —
			// and publishing them together is what stops the room seeing a
			// game arrive with its narration one poll behind.
			// `pending` is the newest game, waiting for the row that closes
			// it — that is what the live `partial` carries. `played` is every
			// game, which is what the finished result carries so the match can
			// be watched back.
			var pending *forgeBeats
			var played []forgeBeats
			// The paintings, resolved once for the whole match: two games of
			// one pairing name almost the same hundred cards, so the second
			// game costs a round trip for its tokens and nothing else.
			painted := map[string]boardArt{}
			hear := func(log tier3.EventLog) {
				if log.Board != nil {
					a.resolveBoardArt(ctx, log.Board.Cards, painted)
				}
				shaped := newForgeBeats(log, seats, command, painted)
				pending = &shaped
				if len(played) < ForgeReplayGames {
					played = append(played, shaped)
				}
			}
			tick := func(finished int, game *tier3.GameResult) {
				if game != nil {
					var slug *string
					if game.WinnerSeat != nil {
						if s, ok := seats[*game.WinnerSeat]; ok {
							slug = &s
						}
					}
					rowsSoFar = append(rowsSoFar, newForgeRow(*game, slug))
				}
				rep.ReportPartial(min(finished, games), games,
					forgePartial{Rows: append([]forgeRow{}, rowsSoFar...),
						Beats: pending})
			}

			// Same match, two places it can run (ADR 35): the worker when the
			// environment names one, a local subprocess otherwise. The worker
			// hands back a run rebuilt from the wire and relays the same
			// per-game ticks, so the shaping and the bar cannot tell the
			// difference — that is the wire's whole promise.
			var run *tier3.SimRun
			var runErr error
			if hosted {
				run, runErr = worker.RunMatch(ctx, decks, tier3.MatchAsk{
					Games: games, Clock: ForgeClock, Seed: seed,
					Narrate: narrate, OnGame: tick, OnEvents: hear,
				})
			} else {
				run, runErr = a.forge.RunGames(decks, tier3.RunOptions{
					Games: games, Clock: ForgeClock, Seed: seed,
					Narrate: narrate, OnEvents: hear,
					OnGame: func(finished int, game tier3.GameResult) {
						tick(finished, &game)
					},
				})
			}
			if runErr != nil {
				// **The one that reached the live site.** A job's error is
				// rendered by the room verbatim — "The match failed: ..." —
				// so this is the last place a machine id and a status code
				// can be stopped. See `forgeTrouble`.
				a.log.Error("the Forge match failed", "error", runErr,
					"decks", strings.Join(addresses, " vs "))
				return nil, errors.New(forgeTrouble(runErr))
			}
			rep.Report(games, games)

			// The match ledger (ADR 36). After the run and before the shaping,
			// because the shape is for this response and the ledger is for
			// every question after it — and Record never fails, so a ledger
			// problem cannot cost anybody a match they just watched finish.
			recorder.Record(ctx, ledger.Match{Run: run, Decks: decks,
				Seed: seed, Clock: ForgeClock, GamesRequested: games,
				Hosted: hosted, OwnerIDs: ownerIDs})
			return shapeForge(decks, addresses, games, seed, run, played), nil
		},
	}, nil
}

// matchLedger is the recorder, or a nil one — which records nothing and warns
// about nothing, the no-`app.db` case rather than an error.
func (a *API) matchLedger() *ledger.Recorder { return a.matchLedgerOf }
