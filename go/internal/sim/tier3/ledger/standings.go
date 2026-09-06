package ledger

import (
	"context"
	"encoding/json"
	"math"
	"sort"
	"strconv"

	"github.com/aasquier/sylvan-library/go/internal/floats"
)

// The three numbers that decide how a tally is presented, and the most
// important design judgement in this file.
//
// A win rate is a ratio, and a ratio computed over a handful of games is not
// a measurement — it is a coin landing heads twice. A board that sorts by the
// raw ratio puts a deck that won its only two bouts above a deck that won
// forty of sixty-five, and a newcomer reading that board is being told
// something false with a straight face. Commandment 2 makes that unacceptable
// in a way that "well, the number is arithmetically correct" does not excuse.
//
// Three rules answer it, and all three are always in force:
//
//   - **The denominator is never absent.** Every record on the wire carries
//     `played`, and no surface may render a rate without it. A rate alone is
//     the lie; a rate beside "of 5 bouts" is a fact.
//   - **Below [RateFloor] there is no rate at all.** `Rate` is nil, and the
//     record says how many bouts it is short. Printing 100% over two games
//     and hoping the denominator beside it does the work has never once
//     worked on anybody, and the two-game deck is exactly the one a
//     leaderboard flatters.
//   - **Boards sort by [Record.Lower], never by the rate.** That is the whole
//     fix. The Wilson lower bound of 2-0 is about 0.34; of 40-25 it is about
//     0.50. Uncertainty is charged to the deck that has not proved anything
//     yet, which is what "honest uncertainty" in this package's own doc
//     comment was always pointing at.
//
// [Proven] is softer: it is the point at which the board stops calling a
// record provisional. It changes wording, never arithmetic.
const (
	// RateFloor is the fewest games that may be shown as a rate. Five is a
	// judgement, not a theorem — it is the point where the interval stops
	// spanning nearly the whole line, and it is one constant to move if
	// Aaron wants it moved.
	RateFloor = 5
	// Proven is where a record stops being called provisional. At twenty
	// games a 95% interval is roughly ±20 points, which is still wide and
	// is why the interval keeps rendering past this line.
	Proven = 20
	// z is the 95% two-sided normal quantile, the interval's only tuning.
	z = 1.959963984540054
)

// Record is one contestant's tally with the interval that says how much of it
// to believe.
//
// Draws are in the denominator: a draw is not a win, and folding it out would
// quietly inflate everybody who ever drew. Clock-outs are **not** — a game the
// measurement gave up on is evidence about the clock, not about a deck — and
// they ride along in [Record.TimedOut] so the surface can say so rather than
// silently losing games between the ledger and the board.
type Record struct {
	Played   int `json:"played"`
	Wins     int `json:"wins"`
	Losses   int `json:"losses"`
	Draws    int `json:"draws"`
	TimedOut int `json:"timed_out"`
	// Rate is nil below [RateFloor]: a number nobody should read is not a
	// number this package will print. A pointer rather than a sentinel
	// float, so a surface that forgets to check renders nothing rather than
	// rendering a zero as a losing record.
	Rate *floats.Float `json:"rate"`
	// Lower and Upper are the Wilson score interval, and Lower is every
	// board's sort key. Present even below the floor, because the interval
	// is exactly what justifies withholding the rate.
	Lower floats.Float `json:"lower"`
	Upper floats.Float `json:"upper"`
	// Settled is Played >= Proven. Wording only.
	Settled bool `json:"settled"`
}

// tally accumulates one contestant's games before [tally.record] shapes it.
type tally struct {
	played, wins, draws, timedOut int
}

func (t tally) record() Record {
	r := Record{
		Played: t.played, Wins: t.wins, Draws: t.draws,
		Losses: t.played - t.wins - t.draws, TimedOut: t.timedOut,
		Settled: t.played >= Proven,
	}
	lower, upper := wilson(t.wins, t.played)
	r.Lower, r.Upper = floats.Float(lower), floats.Float(upper)
	if t.played >= RateFloor {
		rate := floats.Float(float64(t.wins) / float64(t.played))
		r.Rate = &rate
	}
	return r
}

// wilson is the 95% Wilson score interval for wins out of n.
//
// The Wilson interval rather than the textbook normal one for the reason this
// file exists: at the sample sizes a young ledger has, the normal interval
// runs off both ends of the line — it will happily offer a deck a lower bound
// below zero, or an interval of zero width for a deck that has won everything
// — and a board sorted on a bound that is not a probability is not sorted on
// anything. Wilson stays inside [0, 1] at every n and never collapses to a
// point, which is the property the sort actually depends on.
//
// n == 0 is the whole line: nothing has been observed, so nothing is excluded.
// That is the right answer and it is also the safe one — a deck with no games
// sorts to the bottom on a lower bound of 0 rather than dividing by zero.
//
// Every product-then-sum here is fenced with [floats.Rounded]. The expressions
// are exactly the shape a fused multiply-add rewrites, and the bounds are
// stored, sorted on and served: two machines disagreeing in the last bit would
// be two machines disagreeing about the order of a leaderboard.
func wilson(wins, n int) (lower, upper float64) {
	if n <= 0 {
		return 0, 1
	}
	nf := float64(n)
	p := float64(wins) / nf
	z2 := z * z
	denom := floats.Rounded(1 + z2/nf)
	centre := floats.Rounded(p+z2/(2*nf)) / denom
	spread := floats.Rounded(floats.Rounded(p*(1-p)/nf) + z2/floats.Rounded(4*nf*nf))
	half := floats.Rounded(z*math.Sqrt(spread)) / denom
	lower = math.Max(0, floats.Rounded(centre-half))
	upper = math.Min(1, floats.Rounded(centre+half))
	return lower, upper
}

// DeckRecord is one deck's line on the board.
//
// Identity is (owner, slug) — the pair the ledger's own index is built on —
// and never the slug alone: two accounts may both keep a deck called `cats`,
// and merging their records would be inventing a deck neither of them has.
type DeckRecord struct {
	OwnerID   *int64   `json:"owner_id"`
	Slug      string   `json:"slug"`
	Commander []string `json:"commander"`
	Archetype string   `json:"archetype"`
	Themes    []string `json:"themes"`
	Matches   int      `json:"matches"`
	Record    Record   `json:"record"`
	// Duel and Pod are the same games split by the size of the table they
	// were played at (Aaron, 2026-09-06), and the split is not cosmetic: a
	// deck's win rate heads-up and its win rate in a four-player pod are
	// answers to different questions, and pooling them hides both.
	//
	// **A pod's baseline is 25%, a duel's is 50%**, so the same 30% win rate
	// is a bad duellist and a good pod deck. Nothing here does that division
	// for the reader — `Record` carries the interval and the floor as always
	// — but keeping the two apart is what makes the comparison possible at
	// all. `Record` above stays the pooled total, because a deck's whole
	// record is still a thing somebody asks for.
	Duel Record `json:"duel"`
	Pod  Record `json:"pod"`
}

// ClassRecord is one archetype's line: Aaron's "type win rates".
//
// Grouped by the archetype **as recorded**, which is the snapshot rule this
// package was built on — a deck relabelled today does not rewrite what it was
// when it played. A seat that declared nothing carries an empty archetype and
// is left off this board entirely: "" is not a class, and a row of unlabelled
// decks pooled together would be the biggest bar on the chart and would mean
// nothing at all.
type ClassRecord struct {
	Archetype string `json:"archetype"`
	Decks     int    `json:"decks"`
	Record    Record `json:"record"`
}

// Meeting is one head-to-head: Aaron's "heads up win rates".
//
// **Only two-seat matches count here**, and that is a deliberate reading
// rather than a shortcut. In a pod, deck A winning a game is not deck A
// beating deck B — three other decks were also in the room, and crediting the
// win against each of them separately would manufacture head-to-head evidence
// that nobody ever played. A pod's games still count on the deck and class
// boards, where they mean what they say.
//
// A pair is stored once, in a stable orientation: `A` is the side whose
// (owner, slug) sorts first, so `cats` against `dinos` is one row rather than
// two mirror images that can drift apart.
type Meeting struct {
	A       DeckRef `json:"a"`
	B       DeckRef `json:"b"`
	Played  int     `json:"played"`
	AWins   int     `json:"a_wins"`
	BWins   int     `json:"b_wins"`
	Draws   int     `json:"draws"`
	Matches int     `json:"matches"`
	// Record is the pair read from A's side, so `Rate` is A's share and the
	// floor and interval rules apply to a meeting exactly as to a deck.
	Record Record `json:"record"`
}

// DeckRef names one side of a meeting without repeating its whole board line.
type DeckRef struct {
	OwnerID   *int64   `json:"owner_id"`
	Slug      string   `json:"slug"`
	Commander []string `json:"commander"`
}

// Standings is the whole board, as one read.
type Standings struct {
	Matches int `json:"matches"`
	// Games is every game recorded, clock-outs included: the honest count of
	// what the Forge actually ran. The per-record `played` figures will sum
	// to less, and TimedOut is where the difference went.
	Games    int    `json:"games"`
	TimedOut int    `json:"timed_out"`
	Since    string `json:"since"`
	Until    string `json:"until"`
	// Duels and Pods are how many recorded matches were played at each size
	// of table. They sum to Matches.
	Duels      int           `json:"duels"`
	Pods       int           `json:"pods"`
	Decks      []DeckRecord  `json:"decks"`
	Archetypes []ClassRecord `json:"archetypes"`
	Meetings   []Meeting     `json:"meetings"`
	// Blows is the ten biggest killing blows on record, largest first;
	// Giants the ten biggest creatures, and Stacks the ten deepest piles of
	// one token. Three leaderboards, each largest-first and each read off its
	// own partial index.
	Blows  []KillRecord  `json:"blows"`
	Giants []GiantRecord `json:"giants"`
	Stacks []StackRecord `json:"stacks"`
	// Floor and Proven travel with the board so one surface cannot drift
	// from another about what "too few" means, and so the copy can say the
	// number rather than hard-coding it in two languages.
	Floor  int `json:"floor"`
	Proven int `json:"proven"`
}

// Scope is whose recorded matches a read may see.
//
// **You see a match you were in, and the house's own.** Precisely: a match is
// visible when one of its seats is a deck the viewer owns, or when no seat
// belongs to any account at all — the NULL owner the CLI and the file-backed
// curated library write, which is the house's own shelf and is on the card for
// everybody.
//
// The obvious cheaper rule — *any* seat being the house's makes the match
// public — is wrong, and wrong in the direction that matters. Almost every
// match anybody plays is against a house deck, so that rule publishes nearly
// the entire ledger: your record, your decks' names, and who beat you, to
// anyone who ever fought the same opponent. It reads like a scoping rule and
// behaves like no scoping at all.
//
// There is deliberately no cross-account visibility beyond a shared match.
// Aaron wants leaderboards, and a leaderboard is a sharing decision that
// belongs to him rather than to whichever read happened to be written first —
// so this struct is the narrow answer, and widening it is the visible place
// that decision will be taken. ADR 5's shape holds either way: a match outside
// the scope is **absent**, never forbidden.
type Scope struct {
	// Viewer is the account asking, or 0 for the deployment with the door
	// open — nobody to keep anything from, so nothing is kept.
	Viewer int64
	// Open is that deployment: auth off, one person, everything visible.
	Open bool
}

// visible is the WHERE fragment and arguments that keep a read inside a scope.
//
// Expressed as a match-level EXISTS rather than a seat-level filter, because
// the unit a viewer is entitled to is the **match**: seeing your own deck's
// record while its opponent's row vanished would leave a head-to-head with one
// side, and a deck record whose losses had no author.
func (s Scope) visible() (string, []any) {
	if s.Open {
		return "1 = 1", nil
	}
	return `(EXISTS (SELECT 1 FROM forge_seats v WHERE v.match_id = m.id` +
		` AND v.owner_id = ?)` +
		` OR NOT EXISTS (SELECT 1 FROM forge_seats h WHERE h.match_id = m.id` +
		` AND h.owner_id IS NOT NULL))`, []any{s.Viewer}
}

// Board reads the whole ledger inside a scope and computes every rate on it.
//
// Set-based rather than [Recent]'s match-at-a-time walk: this reads the
// ledger's whole history rather than a page of it, and the same shape done
// per-match would be a query per match forever.
//
// Deterministic arithmetic on recorded rows, and no network anywhere in it
// (ADR 14): the gate decides, and a win rate is a gate.
func (r *Recorder) Board(ctx context.Context, s Scope) (*Standings, error) {
	if r == nil || r.db == nil {
		return &Standings{Decks: []DeckRecord{}, Archetypes: []ClassRecord{},
			Meetings: []Meeting{}, Blows: []KillRecord{},
			Giants: []GiantRecord{}, Stacks: []StackRecord{},
			Floor: RateFloor, Proven: Proven}, nil
	}
	where, args := s.visible()

	out := &Standings{Floor: RateFloor, Proven: Proven}
	if err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*), COALESCE(MIN(created_at), ''),`+
			` COALESCE(MAX(created_at), '')`+
			` FROM forge_matches m WHERE `+where, args...,
	).Scan(&out.Matches, &out.Since, &out.Until); err != nil {
		return nil, err
	}

	if err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*), COALESCE(SUM(g.timed_out), 0)`+
			` FROM forge_games g JOIN forge_matches m ON m.id = g.match_id`+
			` WHERE `+where, args...,
	).Scan(&out.Games, &out.TimedOut); err != nil {
		return nil, err
	}

	seats, err := r.boardSeats(ctx, where, args)
	if err != nil {
		return nil, err
	}
	out.Decks = deckBoard(seats)
	out.Archetypes = classBoard(seats)
	out.Meetings = meetings(seats)
	out.Duels, out.Pods = tableSizes(seats)
	if out.Blows, err = r.topBlows(ctx, where, args); err != nil {
		return nil, err
	}
	if out.Giants, err = r.topGiants(ctx, where, args); err != nil {
		return nil, err
	}
	if out.Stacks, err = r.topStacks(ctx, where, args); err != nil {
		return nil, err
	}
	return out, nil
}

// GiantRecord is one line of the biggest-creatures board.
//
// Ranked on power — a player says "a 15/15" and means the first number — with
// toughness beside it so the row prints the pair.
type GiantRecord struct {
	MatchID   int64  `json:"match_id"`
	Game      int    `json:"game"`
	Card      string `json:"card"`
	Power     int    `json:"power"`
	Toughness int    `json:"toughness"`
	Turn      int    `json:"turn"`
	Seats     int    `json:"seats"`
	// Deck is whose battlefield it stood on, by slug.
	Deck     string `json:"deck"`
	PlayedAt string `json:"played_at"`
}

// StackRecord is one line of the deepest-token-pile board.
//
// **Held at once**, never a running total of tokens made: a deck that makes
// forty Food across a game and eats each one never had a stack.
type StackRecord struct {
	MatchID  int64  `json:"match_id"`
	Game     int    `json:"game"`
	Card     string `json:"card"`
	Count    int    `json:"count"`
	Turn     int    `json:"turn"`
	Seats    int    `json:"seats"`
	Deck     string `json:"deck"`
	PlayedAt string `json:"played_at"`
}

// topGiants reads the biggest creatures the viewer may see.
func (r *Recorder) topGiants(ctx context.Context, where string, args []any) (
	[]GiantRecord, error) {
	rows, err := r.db.QueryContext(ctx, featQuery(
		`g.big_card, g.big_power, COALESCE(g.big_toughness, 0),`+
			` COALESCE(g.big_turn, 0)`, "g.big_power", "g.big_seat", where),
		append(append([]any{}, args...), TopBlows)...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []GiantRecord{}
	for rows.Next() {
		var k GiantRecord
		if err := rows.Scan(&k.MatchID, &k.Game, &k.Card, &k.Power,
			&k.Toughness, &k.Turn, &k.Deck, &k.PlayedAt, &k.Seats); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

// topStacks reads the deepest token piles the viewer may see.
func (r *Recorder) topStacks(ctx context.Context, where string, args []any) (
	[]StackRecord, error) {
	rows, err := r.db.QueryContext(ctx, featQuery(
		`g.stack_card, g.stack_count, COALESCE(g.stack_turn, 0)`,
		"g.stack_count", "g.stack_seat", where),
		append(append([]any{}, args...), TopBlows)...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []StackRecord{}
	for rows.Next() {
		var k StackRecord
		if err := rows.Scan(&k.MatchID, &k.Game, &k.Card, &k.Count, &k.Turn,
			&k.Deck, &k.PlayedAt, &k.Seats); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

// featQuery builds one leaderboard's SQL: the same join skeleton every feat
// board needs, with the columns and the ranking column named per board.
//
// One builder rather than three hand-written queries, because the parts that
// have to be identical across the boards — the visibility `where`, the seat
// join that turns a seat number into a deck, the tie-break, the limit — are
// exactly the parts a copy would drift on. `cols`, `rank` and `seat` are
// package literals at every call site and never user input.
func featQuery(cols, rank, seat, where string) string {
	return `SELECT g.match_id, g.game_index, ` + cols +
		`, COALESCE(d.slug, ''), m.created_at,` +
		` (SELECT COUNT(*) FROM forge_seats s WHERE s.match_id = m.id)` +
		` FROM forge_games g` +
		` JOIN forge_matches m ON m.id = g.match_id` +
		` LEFT JOIN forge_seats d ON d.match_id = g.match_id AND d.seat = ` + seat +
		` WHERE ` + rank + ` IS NOT NULL AND ` + where +
		` ORDER BY ` + rank + ` DESC, g.match_id DESC, g.game_index` +
		` LIMIT ?`
}

// KillRecord is one line of the top ten: the blow, and enough of the game
// around it to know whose it was.
//
// **The victim is named and the dealer is not**, which reads oddly until you
// know why: Forge's player-damage event carries the source *card* and the
// player it hit, and nothing else. Who controlled that card is a question the
// board could answer and the night — which does not build a board — could not,
// so naming a dealer would mean one path guessing and the other going blank.
// The card is the honest attribution, and it is also the one a player says out
// loud: "Gyome hit for sixteen".
type KillRecord struct {
	MatchID int64 `json:"match_id"`
	Game    int   `json:"game"`
	// Amount is the whole lethal swing; Sources is how many cards were in it.
	Amount  int    `json:"amount"`
	Card    string `json:"card"`
	Sources int    `json:"sources"`
	Combat  bool   `json:"combat"`
	// Turn is player-turns, not the halved rounds `Record` counts elsewhere.
	Turn int `json:"turn"`
	// Seats is the size of the table, so a pod kill and a duel kill can be
	// told apart at a glance.
	Seats int `json:"seats"`
	// Victim is the deck that died, by slug, and Killer is the deck whose
	// seat is credited — empty when the seat cannot be resolved to a deck,
	// which a match recorded before rung 16 will be.
	Victim   string `json:"victim"`
	PlayedAt string `json:"played_at"`
}

// TopBlows is how many the leaderboard holds.
const TopBlows = 10

// topBlows reads the biggest killing blows the viewer may see, largest first.
//
// Joined to `forge_seats` for the victim's name rather than resolved in Go:
// the blow records a seat number, and a seat number without its match's roster
// beside it is not a deck.
func (r *Recorder) topBlows(ctx context.Context, where string, args []any) (
	[]KillRecord, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT g.match_id, g.game_index, g.kill_amount, g.kill_card,`+
			` COALESCE(g.kill_sources, 1), COALESCE(g.kill_combat, 0),`+
			` COALESCE(g.kill_turn, 0), COALESCE(v.slug, ''), m.created_at,`+
			` (SELECT COUNT(*) FROM forge_seats s WHERE s.match_id = m.id)`+
			` FROM forge_games g`+
			` JOIN forge_matches m ON m.id = g.match_id`+
			` LEFT JOIN forge_seats v ON v.match_id = g.match_id`+
			`   AND v.seat = g.kill_seat`+
			` WHERE g.kill_amount IS NOT NULL AND `+where+
			` ORDER BY g.kill_amount DESC, g.match_id DESC, g.game_index`+
			` LIMIT ?`, append(append([]any{}, args...), TopBlows)...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []KillRecord{}
	for rows.Next() {
		var k KillRecord
		var combat int
		if err := rows.Scan(&k.MatchID, &k.Game, &k.Amount, &k.Card,
			&k.Sources, &combat, &k.Turn, &k.Victim, &k.PlayedAt,
			&k.Seats); err != nil {
			return nil, err
		}
		k.Combat = combat != 0
		out = append(out, k)
	}
	return out, rows.Err()
}

// tableSizes counts the distinct matches at each size of table.
func tableSizes(seats []seatRow) (duels, pods int) {
	sizes := map[int64]int{}
	for _, s := range seats {
		sizes[s.matchID] = s.seats
	}
	for _, n := range sizes {
		if n > 2 {
			pods++
		} else {
			duels++
		}
	}
	return duels, pods
}

// seatRow is one seat of one match with that match's games already tallied
// against it — the single row shape every board is folded out of.
type seatRow struct {
	matchID   int64
	seat      int
	ownerID   *int64
	slug      string
	commander []string
	archetype string
	themes    []string
	seats     int
	createdAt string
	tally     tally
}

// key is a deck's identity: the (owner, slug) pair, with NULL spelled apart
// from any real account id so the house's library cannot collide with a user
// whose id happens to render the same.
func (s seatRow) key() string { return deckKey(s.ownerID, s.slug) }

func deckKey(owner *int64, slug string) string {
	if owner == nil {
		return "house/" + slug
	}
	return "u" + strconv.FormatInt(*owner, 10) + "/" + slug
}

// boardSeats reads every visible seat with its games tallied in the database.
//
// The CASE WHEN spelling rather than the tidier aggregate FILTER clause:
// FILTER wants a newer SQLite than the oldest one this has to run against, and
// a board that works everywhere is worth four extra words.
func (r *Recorder) boardSeats(ctx context.Context, where string, args []any) ([]seatRow, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT s.match_id, s.seat, s.owner_id, s.slug, s.commander,`+
			` s.archetype, s.themes, m.created_at,`+
			` (SELECT COUNT(*) FROM forge_seats n WHERE n.match_id = s.match_id),`+
			` COALESCE(SUM(CASE WHEN g.timed_out = 0 THEN 1 ELSE 0 END), 0),`+
			` COALESCE(SUM(CASE WHEN g.timed_out = 0 AND g.winner_seat = s.seat`+
			`   AND g.draw = 0 THEN 1 ELSE 0 END), 0),`+
			` COALESCE(SUM(CASE WHEN g.timed_out = 0 AND g.draw != 0`+
			`   THEN 1 ELSE 0 END), 0),`+
			` COALESCE(SUM(CASE WHEN g.timed_out != 0 THEN 1 ELSE 0 END), 0)`+
			` FROM forge_seats s`+
			` JOIN forge_matches m ON m.id = s.match_id`+
			` LEFT JOIN forge_games g ON g.match_id = s.match_id`+
			` WHERE `+where+
			` GROUP BY s.match_id, s.seat`+
			` ORDER BY s.match_id, s.seat`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []seatRow{}
	for rows.Next() {
		var s seatRow
		var commander, themes string
		if err := rows.Scan(&s.matchID, &s.seat, &s.ownerID, &s.slug,
			&commander, &s.archetype, &themes, &s.createdAt, &s.seats,
			&s.tally.played, &s.tally.wins, &s.tally.draws,
			&s.tally.timedOut); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(commander), &s.commander); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(themes), &s.themes); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// deckBoard folds the seats into one line per deck.
//
// The labels shown are the **latest** ones a deck wore, while the tallies
// span every match it ever played. Those are two different questions and both
// answers are right: the record is history and must not move, the name beside
// it is how you find the deck today.
func deckBoard(seats []seatRow) []DeckRecord {
	at := map[string]int{}
	out := []DeckRecord{}
	matches := map[string]map[int64]bool{}
	for _, s := range seats {
		k := s.key()
		i, seen := at[k]
		if !seen {
			i = len(out)
			at[k] = i
			out = append(out, DeckRecord{OwnerID: s.ownerID, Slug: s.slug,
				Commander: s.commander, Archetype: s.archetype,
				Themes: s.themes})
			matches[k] = map[int64]bool{}
		}
		out[i].Commander, out[i].Archetype = s.commander, s.archetype
		out[i].Themes = s.themes
		matches[k][s.matchID] = true
		add(&out[i].Record, s.tally)
		// `seats` is the size of the table this match was played at, so the
		// split is a property of the row rather than a second query.
		if s.seats > 2 {
			add(&out[i].Pod, s.tally)
		} else {
			add(&out[i].Duel, s.tally)
		}
	}
	for i := range out {
		out[i].Matches = len(matches[deckKey(out[i].OwnerID, out[i].Slug)])
		out[i].Record = closeOut(out[i].Record)
		out[i].Duel = closeOut(out[i].Duel)
		out[i].Pod = closeOut(out[i].Pod)
	}
	sortBoard(out, func(d DeckRecord) (Record, string) { return d.Record, d.Slug })
	return out
}

// classBoard folds the seats into one line per declared archetype.
func classBoard(seats []seatRow) []ClassRecord {
	at := map[string]int{}
	out := []ClassRecord{}
	decks := map[string]map[string]bool{}
	for _, s := range seats {
		if s.archetype == "" {
			continue
		}
		i, seen := at[s.archetype]
		if !seen {
			i = len(out)
			at[s.archetype] = i
			out = append(out, ClassRecord{Archetype: s.archetype})
			decks[s.archetype] = map[string]bool{}
		}
		decks[s.archetype][s.key()] = true
		add(&out[i].Record, s.tally)
	}
	for i := range out {
		out[i].Decks = len(decks[out[i].Archetype])
		out[i].Record = closeOut(out[i].Record)
	}
	sortBoard(out, func(c ClassRecord) (Record, string) { return c.Record, c.Archetype })
	return out
}

// meetings folds the two-seat matches into one line per pair.
func meetings(seats []seatRow) []Meeting {
	byMatch := map[int64][]seatRow{}
	order := []int64{}
	for _, s := range seats {
		if s.seats != 2 {
			continue
		}
		if _, seen := byMatch[s.matchID]; !seen {
			order = append(order, s.matchID)
		}
		byMatch[s.matchID] = append(byMatch[s.matchID], s)
	}

	at := map[string]int{}
	out := []Meeting{}
	for _, id := range order {
		pair := byMatch[id]
		if len(pair) != 2 {
			continue
		}
		a, b := pair[0], pair[1]
		if b.key() < a.key() {
			a, b = b, a
		}
		k := a.key() + "|" + b.key()
		i, seen := at[k]
		if !seen {
			i = len(out)
			at[k] = i
			out = append(out, Meeting{A: refOf(a), B: refOf(b)})
		}
		out[i].A, out[i].B = refOf(a), refOf(b)
		out[i].Matches++
		out[i].Played += a.tally.played
		out[i].AWins += a.tally.wins
		out[i].BWins += b.tally.wins
		out[i].Draws += a.tally.draws
		add(&out[i].Record, a.tally)
	}
	for i := range out {
		out[i].Record = closeOut(out[i].Record)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Played != out[j].Played {
			return out[i].Played > out[j].Played
		}
		return out[i].A.Slug+out[i].B.Slug < out[j].A.Slug+out[j].B.Slug
	})
	return out
}

func refOf(s seatRow) DeckRef {
	return DeckRef{OwnerID: s.ownerID, Slug: s.slug, Commander: s.commander}
}

// add accumulates a seat's games into a record's raw counters. The derived
// fields are left alone until [closeOut], because a rate of a partial sum is
// not a smaller rate — it is a wrong one.
func add(r *Record, t tally) {
	r.Played += t.played
	r.Wins += t.wins
	r.Draws += t.draws
	r.TimedOut += t.timedOut
}

func closeOut(r Record) Record {
	return tally{played: r.Played, wins: r.Wins, draws: r.Draws,
		timedOut: r.TimedOut}.record()
}

// sortBoard is every board's one ordering: the interval's lower bound first,
// then the plain count, then the name.
//
// Sorting on `Lower` rather than on the rate is the small-sample rule in its
// operative form — see this file's opening comment. The count breaks a tie
// because two decks with identical bounds are separated by which has shown
// more, and the name breaks the last one so the board does not shuffle
// between two reads of the same data.
func sortBoard[T any](rows []T, of func(T) (Record, string)) {
	sort.SliceStable(rows, func(i, j int) bool {
		a, an := of(rows[i])
		b, bn := of(rows[j])
		if a.Lower != b.Lower {
			return a.Lower > b.Lower
		}
		if a.Played != b.Played {
			return a.Played > b.Played
		}
		return an < bn
	})
}
