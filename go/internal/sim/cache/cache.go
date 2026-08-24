// Package cache is memoised Tier 1 results, keyed on the
// simulation's own input ([ADR 18]).
//
// The whole design turns on one requirement: **a cached number must never be a
// stale number.** `Run` takes exactly `(library, commander, games, turns,
// keepRule, seed)` and reads nothing else, and `library` comes out of
// `sim/compile`, which reads a card's cost, type line, oracle text and
// produced mana **from the pool**. So the obvious key -- a hash of `deck.yaml`
// -- is not sufficient: a `data refresh` can change what a card does while the
// deck file is byte identical. That is not hypothetical; Scryfall retemplated
// "enters the battlefield tapped" to "enters tapped", and a deck-hash cache
// would have served the pre-refresh numbers forever.
//
// So the key is a hash of the **compiled** deck: the `sim.Card` list the
// engine is actually handed, plus the clamped parameters, the seed, a
// fingerprint of the engine's own source, and `SimVersion`.
//
// # The fingerprint, and rows that age apart
//
// ADR 18's second consequence is that the engine's source is part of the
// key so that *no engine change can serve a pre-change number, including a
// change nobody remembered to declare*. When the engine's code moves -- a
// package rename, a one-character fix -- the fingerprint moves with it, old
// rows stop matching any new request, and nothing serves a stale number
// under a fresh engine's name. An answer one ulp adrift served under a
// colliding key would be indistinguishable from correct on the screen,
// which is the precise failure ADR 18 was written against.
//
// The cost is one recomputation per deck, per parameter set, per engine
// change. Against `MaxRows` = 2,000 and rows of one or two kilobytes,
// orphaned rows simply age out through the LRU once nothing asks for
// them.
//
// # What it deliberately does not do
//
// **It never raises.** A cache is an optimisation, and an optimisation that
// can turn a working simulation into a failed one is a bad trade. Every entry
// point here swallows its own errors, logs, and behaves as a miss.
//
// **It does not deduplicate concurrent work.** Two identical requests arriving
// together both miss and the second recomputes; `jobs.Submit` with a key is
// where that is solved, one layer up.
//
// **It does not cache an unseeded run.** Callers guard on that rather than
// this module doing it silently.
//
// **It writes no schema.** `app.db` is opened `mode=rw`, never `rwc`: the
// ladder runs once at boot (`auth.Migrate`), and a file this created would be
// a database at version zero.
//
// [ADR 18]: docs/adr/0018-a-cached-simulation-is-keyed-on-its-compiled-input.md
package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/aasquier/sylvan-library/go/internal/floats"
	"github.com/aasquier/sylvan-library/go/internal/mana"
	"github.com/aasquier/sylvan-library/go/internal/mt19937"
	"github.com/aasquier/sylvan-library/go/internal/sim"
	"github.com/aasquier/sylvan-library/go/internal/sim/tier1"
)

// SimVersion is bumped when something changes what a stored result *means*
// while neither the engine nor the mana solver moved -- the serialisation
// below, or a `sim/compile` fix that must invalidate cards compiled before
// it.
//
// It is not an engine marker -- the fingerprint already does that job, and
// doing it twice would make it impossible to tell a deliberate semantic
// bump from an accidental one.
const SimVersion = 2

// MaxRows is how many rows to keep. A mana result is ~1.5 kB and a land-sweep
// row a few hundred bytes, so this is a couple of megabytes at worst --
// bounded because an unbounded cache on a 3 GB volume is a slow-motion outage,
// not because the rows are expensive.
const MaxRows = 2000

// engineSources are the packages whose *code* decides the answer, in the order
// they are hashed.
//
// `sim/compile` is deliberately absent: its output is the `sim.Card`s,
// which are hashed directly, so
// hashing its source as well would invalidate on changes that provably cannot
// matter. `sim/mulligan` is absent for the same reason and a second one -- the
// grid rides in `Input.Extra`, because a per-kind input belongs in a per-kind
// key rather than in a global fingerprint that would throw away every stored
// Tier 1 result each time a grid constant moved.
//
// `internal/sim`, `internal/floats` and `internal/mt19937` are present
// because their code decides the numbers; each package's `source.go` argues
// its own case.
//
// **This list is the only guard against a file leaving a package.** Each
// package's embed is held complete against its own directory by a test, and
// that test is satisfied on *both* sides when a file moves out: it is gone
// from the directory and gone from the list, so nothing is missing anywhere
// and the fingerprint quietly stops covering it. `floats.go` did exactly that,
// moving out of `internal/sim` into `internal/floats`, and
// what noticed was the build refusing an embed pattern that matched no file --
// a move into an *existing* package would not even have done that. So adding
// a package under the engine is a decision to take here, deliberately.
//
// **A comment counts.** The fingerprint hashes each package's embedded source
// *bytes*, not its behaviour, so reflowing a doc comment inside any package
// listed below changes the key, and every stored Tier 1 row on the volume
// stops matching. Nothing fails and no test speaks -- the instance simply
// recomputes what it had already paid for. So a prose-only edit under one of
// these packages is a deliberate cost, not a free tidy; the list is the
// authority on which they are, and it is ten lines down rather than repeated
// here for exactly that reason.
type engineSource struct {
	name string
	fs   fs.FS
}

var engineSources = []engineSource{
	{"internal/sim/tier1", tier1.SourceFS},
	{"internal/mana", mana.SourceFS},
	{"internal/sim", sim.SourceFS},
	{"internal/floats", floats.SourceFS},
	{"internal/mt19937", mt19937.SourceFS},
}

var (
	fingerprintOnce sync.Once
	fingerprintVal  string
)

// Fingerprint is `cache.fingerprint`: a hash of the code that turns
// `sim.Card`s into numbers.
//
// An empty string means the sources could not be read, and an empty
// fingerprint **disables caching entirely** -- deliberately, because the
// fallback for "I cannot tell which engine this is" must be to compute rather
// than to guess.
//
// A binary has no source on disk at runtime, so each fingerprinted package
// embeds its own (see `mana.SourceFS`), and a missing file cannot arise --
// the bytes are in the binary. The empty-string branch is kept anyway,
// since a caller that treats "" as a miss is one line and a caller that
// assumes a fingerprint always exists is a silent wrong answer waiting for
// the first build that surprises it.
//
// Computed once per process. The files cannot change under a running binary.
func Fingerprint() string {
	fingerprintOnce.Do(func() {
		digest := sha256.New()
		for _, pkg := range engineSources {
			names, err := fs.Glob(pkg.fs, "*")
			if err != nil {
				fingerprintVal = ""
				return
			}
			sort.Strings(names)
			for _, name := range names {
				body, err := fs.ReadFile(pkg.fs, name)
				if err != nil {
					fingerprintVal = ""
					return
				}
				// The path is hashed as well as the body, so renaming a file
				// is a change and two files swapping contents is a change.
				digest.Write([]byte(pkg.name + "/" + name + "\x00"))
				digest.Write(body)
			}
		}
		fingerprintVal = hex.EncodeToString(digest.Sum(nil))
	})
	return fingerprintVal
}

// Input is every argument of `tier1.Run` that changes its output, plus the
// per-kind extras.
//
// `Extra` carries inputs that belong to one *kind* of run rather than to `Run`
// itself -- the mulligan sweep's grid is the first, since which rules were
// tried decides the answer and no argument of `Run` mentions them. Absent
// rather than null when unused, so every key computed before that parameter
// existed still hashes to what it hashed to.
//
// **Its types are part of the key.** An integer must arrive as an `int`,
// never as a `float64` that happens to be whole: the serialisation writes
// `8` and `8.0` differently, the key is the bytes, and every stored row's
// key was computed over the `8` form -- the `8.0` spelling would orphan it.
// That is the trap in building one out
// of decoded JSON -- `encoding/json` puts every number into an `any` as a
// `float64` -- so build an `Extra` from the values themselves.
type Input struct {
	Library   []*sim.Card
	Commander *sim.Card
	Games     int
	Turns     int
	KeepRule  tier1.KeepRule
	Seed      int
	Extra     map[string]any
}

// Key is the cache key for one `Run`, or "" if caching is
// unavailable.
//
// Every argument here is an argument of `Run` -- the property the `Input`
// struct exists to keep true -- and `KeepRule` goes in whole rather than
// field by field, so a new mulligan lever is in the key the day it is added
// instead of the day somebody remembers it.
func Key(kind string, in Input) string {
	engine := Fingerprint()
	if engine == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(Payload(engine, kind, in)))
	return hex.EncodeToString(sum[:])
}

// Payload is the exact bytes `Key` hashes: one canonical JSON rendering --
// keys sorted, no spaces, ASCII only.
//
// The layout is a contract with the deployed database, not a style. Every
// stored row's key was computed over exactly these bytes, so any drift -- a
// reordered field, a stray space, a differently-spelled number -- silently
// orphans every row while the cache reads as merely cold. Exported because
// the recorded corpus compares these bytes directly, which is what turns
// "the key moved" into a claim about
// one field rather than an unexamined difference between two serialisations.
func Payload(engine, kind string, in Input) string {
	var b strings.Builder
	b.WriteByte('{')
	writeKey(&b, "commander")
	if in.Commander == nil {
		b.WriteString("null")
	} else {
		writeCardForm(&b, in.Commander)
	}
	b.WriteByte(',')
	writeKey(&b, "engine")
	writeString(&b, engine)
	if len(in.Extra) > 0 {
		b.WriteByte(',')
		writeKey(&b, "extra")
		writeAny(&b, in.Extra)
	}
	b.WriteByte(',')
	writeKey(&b, "games")
	b.WriteString(strconv.Itoa(in.Games))
	b.WriteByte(',')
	writeKey(&b, "keep_rule")
	writeKeepRule(&b, in.KeepRule)
	b.WriteByte(',')
	writeKey(&b, "kind")
	writeString(&b, kind)
	b.WriteByte(',')
	writeKey(&b, "library")
	b.WriteByte('[')
	for i, card := range in.Library {
		if i > 0 {
			b.WriteByte(',')
		}
		writeCardForm(&b, card)
	}
	b.WriteByte(']')
	b.WriteByte(',')
	writeKey(&b, "seed")
	b.WriteString(strconv.Itoa(in.Seed))
	b.WriteByte(',')
	writeKey(&b, "turns")
	b.WriteString(strconv.Itoa(in.Turns))
	b.WriteByte(',')
	writeKey(&b, "version")
	b.WriteString(strconv.Itoa(SimVersion))
	b.WriteByte('}')
	return b.String()
}

func writeKey(b *strings.Builder, name string) {
	writeString(b, name)
	b.WriteByte(':')
}

// writeCardForm is one `sim.Card`, serialised the same way every time.
//
// Colour sets are sorted on the way out. A colour set is a set -- the order
// its slice happens to hold is incidental, and every stored key was computed
// over the sorted form -- so serialising the incidental order would produce
// a key that changes between otherwise-identical inputs: the cache would
// miss constantly and nobody would notice it was broken rather than merely
// cold.
//
// Sequence order is *preserved*, in `Pips` and in `Produces`, because it is
// real
// input: the engine matches pips in order and the leftovers it returns depend
// on that order.
//
// `Category` is carried even though the engine never reads it. Excluding a
// field is a claim about engine behaviour that can go stale; including one
// costs a handful of bytes. It is also the field easiest to serialise wrong
// for free, since the recorded default is the word "utility" while Go's zero
// value is "" -- see `compile.Category`.
func writeCardForm(b *strings.Builder, c *sim.Card) {
	b.WriteByte('[')
	writeString(b, c.Name)
	b.WriteByte(',')
	writeString(b, c.Category)
	b.WriteByte(',')
	b.WriteString(strconv.Itoa(c.Cost.Generic))
	b.WriteByte(',')
	writeSortedSets(b, c.Cost.Pips)
	b.WriteByte(',')
	writeSortedSets(b, c.Cost.Phyrexian)
	b.WriteByte(',')
	writeBool(b, c.Cost.HasX)
	b.WriteByte(',')
	writeBool(b, c.IsLand)
	b.WriteByte(',')
	writeBool(b, c.EntersTapped)
	b.WriteByte(',')
	b.WriteByte('[')
	for i, s := range c.Produces {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteByte('[')
		writeStrings(b, sortedCopy(s.Colors))
		b.WriteByte(',')
		b.WriteString(strconv.Itoa(s.Amount))
		b.WriteByte(']')
	}
	b.WriteByte(']')
	b.WriteByte(',')
	b.WriteString(strconv.Itoa(c.ProduceDelay))
	b.WriteByte(',')
	b.WriteString(strconv.Itoa(c.FetchesLands))
	b.WriteByte(']')
}

func writeKeepRule(b *strings.Builder, k tier1.KeepRule) {
	// The rule's fields in alphabetical order -- the recorded key order --
	// not the declaration order.
	b.WriteByte('{')
	for i, pair := range []struct {
		name  string
		value int
	}{
		{"cheap_ramp_mv", k.CheapRampMV},
		{"max_lands", k.MaxLands},
		{"max_mulligans", k.MaxMulligans},
		{"min_lands", k.MinLands},
		{"min_mana_pieces", k.MinManaPieces},
	} {
		if i > 0 {
			b.WriteByte(',')
		}
		writeKey(b, pair.name)
		b.WriteString(strconv.Itoa(pair.value))
	}
	b.WriteByte('}')
}

func writeSortedSets(b *strings.Builder, sets [][]string) {
	b.WriteByte('[')
	for i, s := range sets {
		if i > 0 {
			b.WriteByte(',')
		}
		writeStrings(b, sortedCopy(s))
	}
	b.WriteByte(']')
}

func writeStrings(b *strings.Builder, xs []string) {
	b.WriteByte('[')
	for i, x := range xs {
		if i > 0 {
			b.WriteByte(',')
		}
		writeString(b, x)
	}
	b.WriteByte(']')
}

func writeBool(b *strings.Builder, v bool) {
	if v {
		b.WriteString("true")
		return
	}
	b.WriteString("false")
}

func sortedCopy(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}
