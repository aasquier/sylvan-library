package main

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/spf13/cobra"

	"github.com/aasquier/sylvan-library/go/internal/auth"
	"github.com/aasquier/sylvan-library/go/internal/config"
	"github.com/aasquier/sylvan-library/go/internal/deck"
	"github.com/aasquier/sylvan-library/go/internal/floats"
	"github.com/aasquier/sylvan-library/go/internal/pool"
	"github.com/aasquier/sylvan-library/go/internal/sim"
	simcache "github.com/aasquier/sylvan-library/go/internal/sim/cache"
	"github.com/aasquier/sylvan-library/go/internal/sim/compile"
	"github.com/aasquier/sylvan-library/go/internal/sim/karsten"
	"github.com/aasquier/sylvan-library/go/internal/sim/mulligan"
	"github.com/aasquier/sylvan-library/go/internal/sim/tier1"
	"github.com/aasquier/sylvan-library/go/internal/sim/tier3"
	"github.com/aasquier/sylvan-library/go/internal/sim/tier3/ledger"
)

// simCommand is the `mtglab sim` family: cli.py's seven simulator commands,
// ported line for line over the engines Phases 5 and 7 already crossed. The
// compute is all `internal/sim/*`; what this file owns is the orchestration
// and the TEXT -- the fixed-width tables and teaching sheets Python prints,
// reproduced to the space, because a diff of the two CLIs' output is the
// cheapest referee this surface will ever have.
//
// Three Python behaviours are ported deliberately rather than improved:
//
//   - `sim mana` compiles through `compile.Deck` (compile_deck), NOT
//     `compile.Compile` (compile_report) -- so a card the pool cannot resolve
//     still shrinks the deck silently here, exactly as in cli.py, and
//     `NothingToSimulate` is unreachable from this surface. The deck page is
//     where the report renders; the CLI never asked for it.
//   - `sim lands` with an empty range (low > high) prints the header and the
//     footer and exits 0. Python's `range(low, high + 1)` simply runs no
//     iterations, and a refusal here would be an invention.
//   - a deck where not one name resolves is refused as PoolRequired, not
//     NothingToSimulate -- the wart `internal/sim/compile` pins in both
//     runtimes.
//
// One departure, Go's rather than the design's: Python failures that were
// tracebacks (a zero games count, a one-deck Forge match) come back as plain
// errors here, printed by the root as `mtglab: <message>` -- same words,
// no stack.
func simCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sim",
		Short: "The simulator: goldfish runs, the closed form, keep rules, Forge",
	}
	cmd.AddCommand(
		simManaCommand(),
		simLandsCommand(),
		simShelfCommand(),
		simMulliganCommand(),
		simCacheCommand(),
		simForgeCommand(),
		simMatchesCommand(),
	)
	return cmd
}

// ------------------------------------------------------------ deck + pool

// loadSimDeck is cli.py's `_load`: the file tier, one slug, or Python's exact
// refusal.
func loadSimDeck(slug string) (*deck.Deck, error) {
	path := filepath.Join(config.DecksDir(), slug, "deck.yaml")
	text, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("no deck at %s", path)
		}
		return nil, err
	}
	return deck.FromText(string(text), slug)
}

// deckPool is cli.py's `_pool`: look up every card in the deck. Returns nil
// when the pool is absent, so compiling degrades to `PoolRequired` with its
// own message -- Python returns None for the same reason.
//
// The name list is _pool's, not service.py's: commander, the 99, the swap
// board, and the companion. The graveyard is deliberately not queried,
// because Python's CLI does not query it.
func deckPool(ctx context.Context, d *deck.Deck) (map[string]*pool.CardRecord, error) {
	names := append([]string{}, d.Commander...)
	for _, c := range d.Cards {
		names = append(names, c.Name)
	}
	for _, c := range d.SwapBoard {
		names = append(names, c.Name)
	}
	if d.Companion != nil {
		names = append(names, *d.Companion)
	}
	p := pool.New(config.DBPath(), nil)
	defer p.Close()
	var cards map[string]*pool.CardRecord
	err := p.Use(ctx, func(c *pool.Conn) error {
		var err error
		cards, err = c.GetCards(ctx, names)
		return err
	})
	if errors.Is(err, pool.ErrNoPool) {
		return nil, nil
	}
	return cards, err
}

// simCards is `_sim_cards`: compile, or fail with the compiler's own words.
// The only error `compile.Deck` returns is PoolRequired, whose Error() is
// exactly what Python's sys.exit printed.
func simCards(ctx context.Context, slug string) (*deck.Deck, []*sim.Card, *sim.Card, error) {
	d, err := loadSimDeck(slug)
	if err != nil {
		return nil, nil, nil, err
	}
	cards, err := deckPool(ctx, d)
	if err != nil {
		return nil, nil, nil, err
	}
	library, commander, err := compile.Deck(d, cards)
	if err != nil {
		return nil, nil, nil, err
	}
	return d, library, commander, nil
}

// ------------------------------------------------------------------- mana

func simManaCommand() *cobra.Command {
	var (
		games, turns, minLands, maxLands, minPieces int
		seed                                        int64
	)
	cmd := &cobra.Command{
		Use:   "mana <slug>",
		Short: "Tier 1 -- baseline mana consistency, goldfished",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if games < 1 {
				return errors.New("a run needs at least one game")
			}
			_, library, commander, err := simCards(context.Background(), args[0])
			if err != nil {
				return err
			}
			rule := tier1.DefaultKeepRule()
			rule.MinLands, rule.MaxLands, rule.MinManaPieces = minLands, maxLands, minPieces
			opts := tier1.Options{Games: games, Turns: turns, KeepRule: &rule}
			if cmd.Flags().Changed("seed") {
				s := seed
				opts.Seed = &s
			}
			fmt.Println(summaryReport(tier1.Run(library, commander, opts)))
			return nil
		},
	}
	f := cmd.Flags()
	f.IntVar(&games, "games", 20000, "games to goldfish")
	f.IntVar(&turns, "turns", 12, "turns per game")
	f.IntVar(&minLands, "min-lands", 2, "keep rule: fewest lands in the opener")
	f.IntVar(&maxLands, "max-lands", 5, "keep rule: most lands in the opener")
	f.IntVar(&minPieces, "min-pieces", 3, "keep rule: lands plus cheap ramp, at least")
	f.Int64Var(&seed, "seed", 0, "seed the shuffles; unset is an unreproducible run")
	return cmd
}

// summaryReport is `SimSummary.report()` -- the fixed-width text table cli.py
// prints for a Tier 1 run. `internal/sim/tier1`'s package comment has promised
// since Phase 5 that it "arrives with the CLI at Phase 8"; this is that
// arrival, and it lives here rather than in the engine because the CLI is
// still its only reader.
func summaryReport(s tier1.SimSummary) string {
	lines := []string{
		fmt.Sprintf("games=%d  turns=%d", s.Games, s.Turns),
		"mulligan policy: " + s.KeepRule,
		fmt.Sprintf("mulligan rate: %s  (avg %s)",
			pyPercent(s.MulliganRate, 1), pyFixed(s.AvgMulligans, 2)),
		"median commander turn: " + numberOrNone(s.MedianCommanderTurn),
		fmt.Sprintf("commander never cast by T%d: %s",
			s.Turns, pyPercent(s.NeverCastCommander, 1)),
		fmt.Sprintf("turns with a color-only block: %s per game",
			pyFixed(s.ColorScrewRate, 2)),
		"median first spell: T" + floatOrNone(s.MedianFirstSpellTurn),
		fmt.Sprintf("stalled turns (castless with a spell in hand): %s per game",
			pyFixed(s.AvgStalledTurns, 2)),
		"",
		"  turn   lands   mana   unused   spells   P(commander down)",
	}
	for t := 1; t <= s.Turns; t++ {
		lines = append(lines, fmt.Sprintf("  %s   %s  %s  %s  %s   %s",
			padLeft(strconv.Itoa(t), 4),
			padLeft(pyFixed(s.AvgLandsByTurn[t-1], 2), 5),
			padLeft(pyFixed(s.AvgManaByTurn[t-1], 2), 5),
			padLeft(pyFixed(s.AvgUnusedByTurn[t-1], 2), 6),
			padLeft(pyFixed(s.AvgSpellsByTurn[t-1], 2), 6),
			padLeft(pyPercent(s.CommanderByTurn[t], 1), 6)))
	}
	return strings.Join(lines, "\n")
}

// ------------------------------------------------------------------ lands

func simLandsCommand() *cobra.Command {
	var (
		games int
		seed  int64
	)
	cmd := &cobra.Command{
		Use:   "lands <slug> <low> <high>",
		Short: "Sweep the land count; read where deployment plateaus",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			low, err := strconv.Atoi(args[1])
			if err != nil {
				return fmt.Errorf("argument low: invalid int value: '%s'", args[1])
			}
			high, err := strconv.Atoi(args[2])
			if err != nil {
				return fmt.Errorf("argument high: invalid int value: '%s'", args[2])
			}
			if games < 1 {
				return errors.New("a run needs at least one game")
			}
			_, library, commander, err := simCards(context.Background(), args[0])
			if err != nil {
				return err
			}
			var lands, spells []*sim.Card
			for _, c := range library {
				if c.IsLand {
					lands = append(lands, c)
				} else {
					spells = append(spells, c)
				}
			}
			if len(lands) == 0 {
				return errors.New("deck has no lands to sweep")
			}

			fmt.Println(" lands  P(cmdr T5)  spells thru T8  wasted thru T8  mull%")
			for n := low; n <= high; n++ {
				// Resize by cycling the existing land pool, preserving its
				// colour mix.
				resized := []*sim.Card{}
				for i := 0; i < n; i++ {
					resized = append(resized, lands[i%len(lands)])
				}
				lib := append(append([]*sim.Card{}, resized...),
					pyHead(spells, len(library)-n)...)
				s := seed
				summary := tier1.Run(lib, commander,
					tier1.Options{Games: games, Turns: 10, Seed: &s})
				fmt.Printf(" %s     %s          %s           %s          %s\n",
					padLeft(strconv.Itoa(n), 5),
					padLeft(pyPercent(summary.CommanderByTurn[5], 1), 6),
					padLeft(pyFixed(summary.SpellsThrough(8), 2), 5),
					padLeft(pyFixed(summary.WastedThrough(8), 2), 5),
					padLeft(pyPercent(summary.MulliganRate, 1), 4))
			}
			fmt.Println("\nPick the land count where 'spells thru T8' plateaus -- past that " +
				"you are buying commander speed with flood.")
			return nil
		},
	}
	f := cmd.Flags()
	f.IntVar(&games, "games", 5000, "games per land count")
	f.Int64Var(&seed, "seed", 7, "one seed across the sweep, so the counts compare")
	return cmd
}

// ------------------------------------------------------------------ shelf

func simShelfCommand() *cobra.Command {
	var (
		target    float64
		onTheDraw bool
		top       int
	)
	cmd := &cobra.Command{
		Use:   "shelf <slug>",
		Short: "Tier 1.5 -- the closed form: coloured source requirements, a land count and per-card lag",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			d, library, commander, err := simCards(context.Background(), args[0])
			if err != nil {
				return err
			}
			shelf := karsten.Read(cardValues(library), commander, target, !onTheDraw)

			seat := "on the play"
			if !shelf.OnThePlay {
				seat = "on the draw"
			}
			fmt.Println(d.Name)
			fmt.Printf("%d cards, %d lands, judged at %s consistency, %s.\n\n",
				shelf.DeckSize, shelf.Lands, pyPercent(shelf.Target, 0), seat)

			fmt.Println("COLOURED SOURCES -- what your own cards demand")
			fmt.Println("  A rung per pip count, because a deck short on triple-pip cards is")
			fmt.Print("  not a deck short on colour.\n\n")
			for _, req := range shelf.Colors {
				fmt.Printf("  %s: you have %d sources (%d lands, %d other)\n",
					req.Color, req.Have, req.HaveLands, req.Have-req.HaveLands)
				for _, tier := range req.Tiers {
					verdict := "ok"
					if !tier.Met() {
						verdict = fmt.Sprintf("short %d", tier.Shortfall())
					}
					more := ""
					if len(tier.Cards) > 1 {
						more = fmt.Sprintf(" (+%d more)", len(tier.Cards)-1)
					}
					fmt.Printf("      %d pip on T%d: wants %s  -- %s you make it %s of the time  [%s%s]\n",
						tier.Pips, tier.Turn, padLeft(strconv.Itoa(tier.Need), 2),
						padRight(verdict, 8), pyPercent(tier.OddsNow, 0),
						tier.Cards[0], more)
				}
				fmt.Println()
			}

			est := shelf.LandEstimate
			fmt.Println("LAND COUNT -- a regression, not a simulation")
			fmt.Printf("  You run %d. The fit says %d (%+d), from an average mana value of %s and %d cheap accelerants.\n",
				est.LandsNow, est.Recommended, est.Delta(),
				floats.Repr(est.AverageManaValue), est.CheapAccelerants)
			for _, caveat := range est.Caveats {
				fmt.Printf("    - %s\n", caveat)
			}
			fmt.Println("  Read `mtglab sim lands` beside this: it simulates *this* deck and prices flood,")
			fmt.Print("  which the fit cannot.\n\n")

			fmt.Println("LATEST CARDS -- cost against when the mana is actually there")
			fmt.Println("  'lag' is turns between what a card costs and when you can rely on")
			fmt.Println("  casting it. This assumes the card is in your hand; it is a question")
			fmt.Print("  about the mana base, not about drawing.\n\n")
			var inHorizon []karsten.CardOdds
			for _, o := range shelf.Odds {
				if o.MV <= karsten.Horizon {
					inHorizon = append(inHorizon, o)
				}
			}
			shown := pyHead(inHorizon, top)
			fmt.Printf("  %s %s %s %s %s\n", padRight("card", 38),
				padLeft("cost", 4), padLeft("on curve", 9),
				padLeft("reliable", 9), padLeft("lag", 6))
			for _, odds := range shown {
				curve := "n/a"
				if v := odds.OnCurve(); v != nil {
					curve = pyPercent(*v, 0)
				}
				reliable := "never"
				if v := odds.ReliableTurn(); v != nil {
					reliable = "T" + strconv.Itoa(*v)
				}
				lag := ">=" + strconv.Itoa(odds.Lateness())
				if v := odds.Lag(); v != nil {
					lag = "+" + strconv.Itoa(*v)
				}
				fmt.Printf("  %s %s %s %s %s\n",
					padRight(headRunes(odds.Name, 38), 38),
					padLeft(strconv.Itoa(odds.MV), 4), padLeft(curve, 9),
					padLeft(reliable, 9), padLeft(lag, 6))
			}
			if len(shelf.Approximated) > 0 {
				fmt.Printf("\n  %d card(s) demand two or more colours, where this method\n",
					len(shelf.Approximated))
				tail := ""
				if len(shelf.Approximated) > 4 {
					tail = ", ..."
				}
				fmt.Println("  approximates and reads slightly low: " +
					strings.Join(pyHead(shelf.Approximated, 4), ", ") + tail)
			}
			return nil
		},
	}
	f := cmd.Flags()
	f.Float64Var(&target, "target", 0.90, "consistency to judge against (default 0.90)")
	f.BoolVar(&onTheDraw, "on-the-draw", false, "judge on the draw; the default is the harder case")
	f.IntVar(&top, "top", 15, "how many of the latest cards to list")
	return cmd
}

// cardValues is the []*sim.Card -> []sim.Card conversion the closed forms
// take -- `karsten` counts and multiplies, and a copy is simpler to reason
// about than a slice of aliases (the same helper `internal/api` keeps).
func cardValues(library []*sim.Card) []sim.Card {
	out := make([]sim.Card, len(library))
	for i, c := range library {
		out[i] = *c
	}
	return out
}

// --------------------------------------------------------------- mulligan

func simMulliganCommand() *cobra.Command {
	var games, turns, seed, top int
	cmd := &cobra.Command{
		Use:   "mulligan <slug>",
		Short: "search keep rules and report the best policy",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if games < 1 {
				return errors.New("a run needs at least one game")
			}
			d, library, commander, err := simCards(context.Background(), args[0])
			if err != nil {
				return err
			}
			grid := mulligan.Candidates()
			fmt.Printf("%s: %d keep rules x %s games (seed %d) ...\n\n",
				d.Name, len(grid), groupThousands(games), seed)
			sweep, err := mulligan.Search(library, commander,
				mulligan.Options{Games: games, Turns: turns, Seed: seed})
			if err != nil {
				return err
			}

			fmt.Printf("  %s %s %s  rule\n", padLeft("spells T8", 9),
				padLeft("mull%", 7), padLeft("cmdr", 5))
			for _, row := range pyHead(sweep.Rows, top) {
				marks := " "
				if sameRule(row, sweep.Best) {
					marks = "*"
				}
				if sameRule(row, sweep.Baseline) {
					marks += "="
				} else {
					marks += " "
				}
				cmdr := "--"
				if row.MedianCommanderTurn != nil {
					cmdr = "T" + pyG(row.MedianCommanderTurn.Value())
				}
				fmt.Printf("%s%s %s %s  %s\n", marks,
					padLeft(pyFixed(row.SpellsThroughT8, 2), 9),
					padLeft(pyPercent(row.MulliganRate, 1), 7),
					padLeft(cmdr, 5), row.Describe)
			}
			fmt.Print("\n  * best   = the default this simulator uses when you choose nothing\n\n")

			if sweep.IsFlat() {
				fmt.Printf("NO CHANGE WORTH MAKING. The best rule beats your default by %s spells\n",
					pySigned(sweep.Gain()))
				fmt.Printf("through turn 8, under the %s threshold this calls noise. The grid\n",
					floats.Repr(mulligan.Flat))
				fmt.Printf("spans %s spells overall, but most of that range is rules nobody\n",
					pyFixed(sweep.Spread, 2))
				fmt.Println("would play -- flatness is measured against your default, not against the grid.")
				gentle := sweep.Gentlest()
				if gentle.MulliganRate < sweep.Baseline.MulliganRate-0.05 {
					fmt.Printf("\nStill worth knowing: '%s' deploys the same (%s)\n",
						gentle.Describe, pyFixed(gentle.SpellsThroughT8, 2))
					fmt.Printf("while mulliganing %s of hands instead of %s. Same result, fewer hands thrown away.\n",
						pyPercent(gentle.MulliganRate, 0),
						pyPercent(sweep.Baseline.MulliganRate, 0))
				}
			} else {
				fmt.Printf("BEST: %s\n", sweep.Best.Describe)
				fmt.Printf("  %s spells through turn 8, %s against your default's %s,\n",
					pyFixed(sweep.Best.SpellsThroughT8, 2), pySigned(sweep.Gain()),
					pyFixed(sweep.Baseline.SpellsThroughT8, 2))
				fmt.Printf("  mulliganing %s of hands against %s.\n",
					pyPercent(sweep.Best.MulliganRate, 0),
					pyPercent(sweep.Baseline.MulliganRate, 0))
			}
			fmt.Println()
			fmt.Println("Judged on spells deployed through turn 8: mulligan rate alone recommends keeping")
			fmt.Println("everything, and hand quality alone recommends mulliganing forever.")
			return nil
		},
	}
	f := cmd.Flags()
	f.IntVar(&games, "games", 2000, "games per rule; the grid is ~33 rules")
	f.IntVar(&turns, "turns", 10, "turns per game")
	f.IntVar(&seed, "seed", 7, "one seed across the grid, so the rules compare")
	f.IntVar(&top, "top", 10, "how many rules to list")
	return cmd
}

// sameRule is Python's `row is sweep.best` and its baseline-triple test in one:
// the grid's (min_lands, max_lands, min_pieces) triples are unique, so equality
// on the triple picks exactly the row identity picked.
func sameRule(a, b mulligan.Row) bool {
	return a.MinLands == b.MinLands && a.MaxLands == b.MaxLands &&
		a.MinPieces == b.MinPieces
}

// ------------------------------------------------------------------ cache

func simCacheCommand() *cobra.Command {
	var clear bool
	cmd := &cobra.Command{
		Use:   "cache",
		Short: "what Tier 1 results are memoised",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			store := openSimStore()
			defer func() { _ = store.Close() }()
			if clear {
				fmt.Printf("cleared %d cached result(s) from %s\n",
					store.Clear(ctx), config.AppDBPath())
				return nil
			}
			info := store.Stats(ctx)
			fmt.Printf("store:   %s\n", config.AppDBPath())
			enabled := "no -- results are not cached"
			if info.Enabled {
				enabled = "yes"
			}
			fmt.Printf("enabled: %s\n", enabled)
			fmt.Printf("rows:    %d (%s kB)\n", info.Rows,
				pyFixed(float64(info.Bytes)/1024, 1))
			kinds := make([]string, 0, len(info.ByKind))
			for kind := range info.ByKind {
				kinds = append(kinds, kind)
			}
			sort.Strings(kinds)
			for _, kind := range kinds {
				fmt.Printf("  %s %d\n", padRight(kind, 18), info.ByKind[kind])
			}
			if info.Oldest != nil {
				fmt.Printf("computed between %s and %s UTC\n",
					headRunes(*info.Oldest, 19), headRunes(*info.Newest, 19))
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&clear, "clear", false,
		"drop every cached result; they recompute on demand")
	return cmd
}

// openSimStore attaches to app.db for `sim cache`, or answers with a nil
// store -- which reports exactly what Python reports over a fresh file: zero
// rows, caching enabled.
//
// Python's `db.connection` would CREATE app.db here; the door's rule is that
// a reader never acquires a database, so an absent file is read as an empty
// one instead. An existing file is migrated first, which is what Python's
// connect-and-migrate helper did on every call.
func openSimStore() *simcache.Store {
	path := config.AppDBPath()
	if _, err := os.Stat(path); err != nil {
		return nil
	}
	if err := auth.Migrate(path); err != nil {
		fmt.Fprintf(os.Stderr, "simulation cache open failed (%v)\n", err)
		return nil
	}
	store, err := simcache.Open(path, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "simulation cache open failed (%v)\n", err)
		return nil
	}
	return store
}

// ------------------------------------------------------------------ forge

func simForgeCommand() *cobra.Command {
	var (
		games, clock int
		seed         int64
		checkOnly    bool
	)
	cmd := &cobra.Command{
		Use:   "forge <a> <b> [c] [d]",
		Short: "Tier 3 -- Forge plays real games",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			decks := make([]*deck.Deck, 0, len(args))
			for _, slug := range args {
				d, err := loadSimDeck(slug)
				if err != nil {
					return err
				}
				decks = append(decks, d)
			}
			if checkOnly {
				// The coverage pre-flight on its own: reads a zip, needs no
				// JVM, and is the only half of this command that works
				// without the distribution installed.
				reports, err := tier3.CheckCoverage(decks, "")
				if err != nil {
					return err
				}
				for i := range reports {
					fmt.Println(reports[i].Summary())
				}
				return nil
			}
			var seedPtr *big.Int
			if cmd.Flags().Changed("seed") {
				seedPtr = big.NewInt(seed)
			}
			result, err := tier3.RunGames(decks, tier3.RunOptions{
				Games: games, Clock: clock, Seed: seedPtr,
			})
			if err != nil {
				return err
			}

			// The match ledger (ADR 36): recorded here and in the API job --
			// the two places a match finishes. Never fails the run; an
			// overnight round-robin must not die on a ledger hiccup with the
			// JVM's work already done.
			recordForgeMatch(result, decks, seedPtr, clock, games)

			wins := map[string]int{}
			for _, game := range result.Games() {
				// The clock-out rule, same as the API's shaping and the
				// ledger's reference reading: a game that hit the clock
				// counts for nobody, even when Forge printed a winner line
				// after the slow-match warning.
				key := result.WinnerSlug(game)
				if key == "" || game.TimedOut {
					key = "draw"
				}
				wins[key]++
			}

			played := make([]float64, 0, len(result.Games()))
			for _, g := range result.Games() {
				played = append(played, float64(g.Milliseconds)/1000)
			}
			fmt.Printf("%d games in %ss (%ss of it JVM + card database)\n",
				len(result.Games()), pyFixed(result.WallSeconds, 1),
				pyFixed(result.StartupSeconds(), 1))
			// `Fsum` for the rule rather than for this number: these are
			// wall-clock readings off a JVM, so the mean is not reproducible
			// whatever sums it. Spelled like every other float sum so a bare
			// running total stays a defect anybody can grep for.
			minS, maxS := played[0], played[0]
			for _, v := range played[1:] {
				minS, maxS = min(minS, v), max(maxS, v)
			}
			fmt.Printf("per game: %ss min / %ss mean / %ss max\n",
				pyFixed(minS, 1),
				pyFixed(floats.Fsum(played)/float64(len(played)), 1),
				pyFixed(maxS, 1))
			for _, slug := range args {
				fmt.Printf("  %s %d\n", padRight(slug, 22), wins[slug])
			}
			if wins["draw"] > 0 {
				fmt.Printf("  %s %d\n", padRight("draw", 22), wins["draw"])
			}
			clocked := 0
			for _, g := range result.Games() {
				if g.TimedOut {
					clocked++
				}
			}
			if clocked > 0 {
				// Never folded into the draw count: a clock-out is the
				// measurement giving up, not the game ending.
				fmt.Printf("  (%d hit the %ds clock and were called draws)\n",
					clocked, clock)
			}
			fmt.Println("\nForge's AI is best at aggro and midrange, poor at control and bad " +
				"at most combo.\nRead these per archetype, not as one ranking.")
			return nil
		},
	}
	f := cmd.Flags()
	f.IntVar(&games, "games", 10, "games to play")
	// 300, not Forge's default of 120: a long game should be a long game, not
	// a draw the clock invented.
	f.IntVar(&clock, "clock", 300, "seconds before a game is called a draw")
	f.Int64Var(&seed, "seed", 0, "seed Forge's shuffles; unset is unseeded")
	f.BoolVar(&checkOnly, "check-only", false,
		"card-coverage pre-flight only; needs no JVM")
	return cmd
}

// recordForgeMatch writes the finished match into the ledger, never failing
// the caller -- Python's `ledger.record` warns and moves on, and so does
// this. Unlike `sim cache` it MAY mint app.db: the Python CLI's
// connect-and-migrate did, the ladder is Go's since Phase 8, and a match
// silently unrecorded on a fresh machine would be a regression.
func recordForgeMatch(result *tier3.SimRun, decks []*deck.Deck, seed *big.Int,
	clock, games int) {
	path := config.AppDBPath()
	if err := auth.Migrate(path); err != nil {
		fmt.Fprintf(os.Stderr, "match ledger record failed (%v)\n", err)
		return
	}
	rec, err := ledger.NewRecorder(path, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "match ledger record failed (%v)\n", err)
		return
	}
	defer func() { _ = rec.Close() }()
	rec.Record(context.Background(), ledger.Match{
		Run:            result,
		Decks:          decks,
		Seed:           seed,
		Clock:          clock,
		GamesRequested: games,
		Hosted:         false,
	})
}

// ---------------------------------------------------------------- matches

func simMatchesCommand() *cobra.Command {
	var limit int
	cmd := &cobra.Command{
		Use:   "matches",
		Short: "the match ledger -- every Forge match recorded",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			path := config.AppDBPath()
			if _, err := os.Stat(path); err != nil {
				fmt.Println("no matches recorded yet -- `mtglab sim forge` records as it plays")
				return nil
			}
			if err := auth.Migrate(path); err != nil {
				return err
			}
			rec, err := ledger.NewRecorder(path, nil)
			if err != nil {
				return err
			}
			defer func() { _ = rec.Close() }()
			matches, err := rec.Recent(context.Background(), limit)
			if err != nil {
				return err
			}
			if len(matches) == 0 {
				fmt.Println("no matches recorded yet -- `mtglab sim forge` records as it plays")
				return nil
			}
			fmt.Printf("ledger: %s\n", path)
			for _, m := range matches {
				when := strings.ReplaceAll(headRunes(m.CreatedAt, 19), "T", " ")
				where := "local"
				if m.Hosted {
					where = "worker"
				}
				version := ""
				if m.ForgeVersion != nil && *m.ForgeVersion != "" {
					version = ", Forge " + *m.ForgeVersion
				}
				seeded := "unseeded"
				if m.Seed != nil {
					seeded = fmt.Sprintf("seed %d", *m.Seed)
				}
				fmt.Printf("\n#%d  %s UTC  (%s%s, %s)\n", m.ID, when, where,
					version, seeded)
				for _, seat := range m.Seats {
					labels := seat.Archetype
					if labels == "" {
						labels = "unlabelled"
					}
					if len(seat.Themes) > 0 {
						labels += "; " + strings.Join(seat.Themes, ", ")
					}
					plural := "s"
					if seat.Wins == 1 {
						plural = ""
					}
					fmt.Printf("  %s %s win%s  (%s)\n", padRight(seat.Slug, 22),
						padLeft(strconv.Itoa(seat.Wins), 2), plural, labels)
				}
				extras := []string{}
				if m.Draws > 0 {
					plural := "s"
					if m.Draws == 1 {
						plural = ""
					}
					extras = append(extras,
						fmt.Sprintf("%d real draw%s", m.Draws, plural))
				}
				if m.TimedOut > 0 {
					extras = append(extras,
						fmt.Sprintf("%d hit the %ds clock", m.TimedOut, m.Clock))
				}
				tail := ""
				if len(extras) > 0 {
					tail = "  (" + strings.Join(extras, ", ") + ")"
				}
				fmt.Printf("  %d of %d games%s\n", m.Played, m.GamesRequested, tail)
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 20, "how many matches to show, newest first")
	return cmd
}

// -------------------------------------------------- Python's text, in Go
//
// The helpers below reproduce the f-string conversions the Python commands
// lean on, because the tables are compared by eye against Python's and a
// one-space drift reads as a bug. Widths count CODE POINTS, as `str.__format__`
// does -- the pool holds Bösium Strip and Déjà Vu, and byte-padding those
// would misalign the row.

// pyFixed is `format(v, ".<prec>f")`.
func pyFixed(v float64, prec int) string {
	return strconv.FormatFloat(v, 'f', prec, 64)
}

// pySigned is `format(v, "+.2f")` -- the one signed conversion the sim
// family prints (the mulligan verdict's gain).
func pySigned(v float64) string {
	s := pyFixed(v, 2)
	if !strings.HasPrefix(s, "-") {
		s = "+" + s
	}
	return s
}

// pyPercent is `format(v, ".<prec>%")`: multiply by 100 as a double, render
// fixed, append the sign -- the same arithmetic CPython's float.__format__
// does, so the two runtimes round the same readings the same way.
func pyPercent(v float64, prec int) string {
	return pyFixed(v*100, prec) + "%"
}

// pyG is `format(v, "g")`: Go's 'g' at precision 6, which is Python's default
// -- the theme lane measured that equivalence before this file leant on it.
func pyG(v float64) string {
	return strconv.FormatFloat(v, 'g', 6, 64)
}

// groupThousands is `format(n, ",")`.
func groupThousands(n int) string {
	s := strconv.Itoa(n)
	neg := strings.HasPrefix(s, "-")
	if neg {
		s = s[1:]
	}
	var parts []string
	for len(s) > 3 {
		parts = append([]string{s[len(s)-3:]}, parts...)
		s = s[:len(s)-3]
	}
	parts = append([]string{s}, parts...)
	out := strings.Join(parts, ",")
	if neg {
		out = "-" + out
	}
	return out
}

// padLeft is `format(s, ">N")` -- right-justify to N code points.
func padLeft(s string, width int) string {
	if n := utf8.RuneCountInString(s); n < width {
		return strings.Repeat(" ", width-n) + s
	}
	return s
}

// padRight is `format(s, "<N")` -- left-justify to N code points.
func padRight(s string, width int) string {
	if n := utf8.RuneCountInString(s); n < width {
		return s + strings.Repeat(" ", width-n)
	}
	return s
}

// headRunes is `s[:n]` -- by code points, which is what Python slices.
func headRunes(s string, n int) string {
	runes := []rune(s)
	if len(runes) > n {
		runes = runes[:n]
	}
	return string(runes)
}

// pyHead is Python's `xs[:k]`: a negative k counts from the end, and a k past
// the end is the whole slice -- and `cmd_sim_lands` leans on the negative
// case, since `spells[:len(library)-n]` goes negative the moment the swept
// count exceeds the deck.
func pyHead[T any](xs []T, k int) []T {
	if k < 0 {
		k += len(xs)
	}
	k = min(max(k, 0), len(xs))
	return xs[:k]
}

// numberOrNone renders `median_commander_turn` as the f-string does: an int
// as an int, a float as CPython reprs it, an absent value as `None`.
func numberOrNone(n *tier1.Number) string {
	if n == nil {
		return "None"
	}
	if n.IsFloat {
		return floats.Repr(n.Float)
	}
	return strconv.Itoa(n.Int)
}

// floatOrNone renders `median_first_spell_turn`: always a float in Python
// (`float(median(...))`), so `T4.0` rather than `T4` -- or `TNone`.
func floatOrNone(v *float64) string {
	if v == nil {
		return "None"
	}
	return floats.Repr(*v)
}
