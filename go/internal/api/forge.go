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
	if n.Cmp(big.NewInt(ForgeGamesMax)) > 0 {
		return ForgeGamesMax, nil
	}
	if n.Cmp(big.NewInt(1)) < 0 {
		return 1, nil
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
	// Target is the card on the other end: blocked, or damaged.
	Target string `json:"target,omitempty"`
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
}

// forgeBoard is the battlefield as the room receives it.
type forgeBoard struct {
	Seats []forgeBoardSeat  `json:"seats"`
	Cards []forgeBoardCard  `json:"cards"`
	Steps []tier3.BoardStep `json:"steps"`
}

// boardArt is one card's painting, resolved once per match.
type boardArt struct {
	Image  string
	Art    string
	Artist string
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
				art := boardArt{}
				if rec.ImageNormal != nil {
					art.Image = *rec.ImageNormal
				}
				if rec.ImageArtCrop != nil {
					art.Art = *rec.ImageArtCrop
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

// newForgeBoard shapes one game's board for the room.
//
// `steps` is cut to `kept` — the number of beats that survived
// [ForgeBeatsMax] — because a step and a beat are the same moment seen twice
// and a room paces them together. Cutting them apart would drift the picture
// away from the account by however many beats were dropped.
func newForgeBoard(reel *tier3.BoardReel, seats map[int]string,
	art map[string]boardArt, kept int) *forgeBoard {
	if reel == nil {
		return nil
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
		out.Seats = append(out.Seats, row)
	}
	for _, card := range reel.Cards {
		painted := art[card.Name]
		out.Cards = append(out.Cards, forgeBoardCard{
			ID: card.ID, Name: card.Name, Token: card.Token,
			Types: card.Types, Seat: card.Seat,
			Image: painted.Image, Art: painted.Art, Artist: painted.Artist})
	}
	return out
}

// newForgeBeats shapes one game's log for the browser.
//
// `seats` is the seat-to-slug map the plan built from the deck order, which is
// the same map [tier3.SimRun.Seats] holds — built by hand here because
// narration happens *during* the run, when there is no run yet, exactly as the
// CLI's `seatsOf` does for the same reason.
func newForgeBeats(log tier3.EventLog, seats map[int]string,
	art map[string]boardArt) forgeBeats {
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
	out.Board = newForgeBoard(log.Board, seats, art, len(events))
	for _, e := range events {
		out.Beats = append(out.Beats, forgeBeat{
			Kind: string(e.Kind), Turn: e.Turn, Who: slug(e.Seat),
			Card: e.Card, Target: e.Target, Against: slug(e.TargetSeat),
			Amount: e.Amount, Life: e.Life, Note: e.Note})
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
	// Beats is the **last** game's narration and board, carried on the result
	// so a match that is over still has a battlefield to show.
	//
	// The job's `partial` is the live carrier and it is cleared the moment the
	// job finishes, which is fine for a bar and fatal for a picture: a
	// one-game match can finish inside a single poll interval, so the room
	// polls once, sees a done job with no partial, and draws an **empty
	// field** for a match that was played in full. That is worse than the
	// account it replaced, and it is exactly the case somebody watching a
	// short match hits every time.
	//
	// `omitempty` on a pointer, deliberately: a shaped match with no beats —
	// which is every match nobody asked to narrate, and every row in the
	// frozen corpus — comes out byte-identical to what was recorded.
	Beats *forgeBeats `json:"beats,omitempty"`
}

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
	seed *big.Int, run *tier3.SimRun, beats *forgeBeats) forgeResult {
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
		detail := ""
		if why != nil {
			detail = *why
		}
		wire.Detail(w, http.StatusServiceUnavailable, detail)
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
			wire.Detail(w, http.StatusServiceUnavailable, err.Error())
		default:
			a.log.Error("the Forge pre-flight failed", "error", err)
			wire.Detail(w, http.StatusInternalServerError, err.Error())
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
			for i, d := range decks {
				seats[i+1] = d.Slug
			}
			var rowsSoFar []forgeRow
			// The beats of the game that just closed, waiting for the row
			// that closes it. Both run paths hand over the beats *before* the
			// row (one pass over Forge's output, the event parser fed first),
			// so this is never stale by more than the few lines between them —
			// and publishing them together is what stops the room seeing a
			// game arrive with its narration one poll behind.
			var pending *forgeBeats
			// The paintings, resolved once for the whole match: two games of
			// one pairing name almost the same hundred cards, so the second
			// game costs a round trip for its tokens and nothing else.
			painted := map[string]boardArt{}
			hear := func(log tier3.EventLog) {
				if log.Board != nil {
					a.resolveBoardArt(ctx, log.Board.Cards, painted)
				}
				shaped := newForgeBeats(log, seats, painted)
				pending = &shaped
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
				return nil, runErr
			}
			rep.Report(games, games)

			// The match ledger (ADR 36). After the run and before the shaping,
			// because the shape is for this response and the ledger is for
			// every question after it — and Record never fails, so a ledger
			// problem cannot cost anybody a match they just watched finish.
			recorder.Record(ctx, ledger.Match{Run: run, Decks: decks,
				Seed: seed, Clock: ForgeClock, GamesRequested: games,
				Hosted: hosted, OwnerIDs: ownerIDs})
			return shapeForge(decks, addresses, games, seed, run, pending), nil
		},
	}, nil
}

// matchLedger is the recorder, or a nil one — which records nothing and warns
// about nothing, the no-`app.db` case rather than an error.
func (a *API) matchLedger() *ledger.Recorder { return a.matchLedgerOf }
