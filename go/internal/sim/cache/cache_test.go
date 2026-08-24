package cache_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/auth/authtest"
	"github.com/aasquier/sylvan-library/go/internal/sim"
	"github.com/aasquier/sylvan-library/go/internal/sim/cache"
	"github.com/aasquier/sylvan-library/go/internal/sim/tier1"
)

// ADR 18's cache key, held to the frozen corpus `testdata/cache.json`.
//
// # The question this file exists to answer
//
// The deployed database holds rows keyed by earlier engines, and a row
// computed today and a row one of those engines computed for the
// same deck and seed **sit apart**: different keys, both in `sim_cache`,
// neither able to serve the other. That is the fingerprint doing its job -- see
// the package comment for the argument -- and it is a claim worth two tests
// rather than one, because there are two ways to arrive at "the keys differ"
// and only one of them is right.
//
//   - The **wrong** way is a serialisation that drifted somewhere
//     in the payload. Then the keys differ for a reason nobody chose, and the
//     day somebody "fixes" the fingerprint the rows would collide while still
//     meaning different things.
//   - The **right** way is one field. `TestThePayloadIsTheRecordedBytes`
//     proves
//     the other nine are identical byte for byte, and
//     `TestTheKeyDiffersFromTheCorpusOnlyByTheFingerprint` proves the tenth
//     is
//     the difference and the whole of it.
//
// The payload string is recorded, not just the digest, because a sha256
// mismatch says nothing about *where*: the three classic drifts -- HTML
// escaping, ASCII escaping, and float rendering --
// present as the same opaque sixty-four characters.

type cardJSON struct {
	Name string `json:"name"`
	Cost struct {
		Generic   int        `json:"generic"`
		Pips      [][]string `json:"pips"`
		Phyrexian [][]string `json:"phyrexian"`
		HasX      bool       `json:"has_x"`
	} `json:"cost"`
	Category     string       `json:"category"`
	IsLand       bool         `json:"is_land"`
	EntersTapped bool         `json:"enters_tapped"`
	Produces     []sim.Source `json:"produces"`
	ProduceDelay int          `json:"produce_delay"`
	FetchesLands int          `json:"fetches_lands"`
}

func (c cardJSON) card() *sim.Card {
	return &sim.Card{
		Name: c.Name,
		Cost: sim.Cost{
			Generic:   c.Cost.Generic,
			Pips:      c.Cost.Pips,
			Phyrexian: c.Cost.Phyrexian,
			HasX:      c.Cost.HasX,
		},
		Category:     c.Category,
		IsLand:       c.IsLand,
		EntersTapped: c.EntersTapped,
		Produces:     c.Produces,
		ProduceDelay: c.ProduceDelay,
		FetchesLands: c.FetchesLands,
	}
}

type keepRuleJSON struct {
	MinLands      int `json:"min_lands"`
	MaxLands      int `json:"max_lands"`
	MinManaPieces int `json:"min_mana_pieces"`
	CheapRampMV   int `json:"cheap_ramp_mv"`
	MaxMulligans  int `json:"max_mulligans"`
}

type caseJSON struct {
	Label     string         `json:"label"`
	Why       string         `json:"why"`
	Kind      string         `json:"kind"`
	Library   []cardJSON     `json:"library"`
	Commander *cardJSON      `json:"commander"`
	Games     int            `json:"games"`
	Turns     int            `json:"turns"`
	KeepRule  keepRuleJSON   `json:"keep_rule"`
	Seed      int            `json:"seed"`
	Extra     map[string]any `json:"extra"`
	Payload   string         `json:"payload"`
	Key       string         `json:"key"`
}

func (c caseJSON) input() cache.Input {
	in := cache.Input{
		Games: c.Games,
		Turns: c.Turns,
		KeepRule: tier1.KeepRule{
			MinLands: c.KeepRule.MinLands, MaxLands: c.KeepRule.MaxLands,
			MinManaPieces: c.KeepRule.MinManaPieces,
			CheapRampMV:   c.KeepRule.CheapRampMV,
			MaxMulligans:  c.KeepRule.MaxMulligans,
		},
		Seed:  c.Seed,
		Extra: c.Extra,
	}
	for _, card := range c.Library {
		in.Library = append(in.Library, card.card())
	}
	if c.Commander != nil {
		in.Commander = c.Commander.card()
	}
	return in
}

type corpus struct {
	Note          string   `json:"note"`
	SimVersion    int      `json:"sim_version"`
	MaxRows       int      `json:"max_rows"`
	RunInputs     []string `json:"run_inputs"`
	RunNonInputs  []string `json:"run_non_inputs"`
	EngineSources []string `json:"engine_sources"`
	// RecordedFingerprint is the fingerprint of the engine the corpus was
	// recorded under -- the field's stored name is historical and, like
	// everything else in the frozen golden, never changes.
	RecordedFingerprint string     `json:"python_fingerprint"`
	Cases               []caseJSON `json:"cases"`
}

func load(t *testing.T) corpus {
	t.Helper()
	body, err := os.ReadFile("testdata/cache.json")
	if err != nil {
		t.Fatalf("read corpus: %v", err)
	}
	var c corpus
	// **`UseNumber`, and it is not a nicety.** `encoding/json` decodes every
	// number into an `any` as `float64`, so `8` comes back as `8.0` and
	// `Payload` renders it `8.0` -- a different key for the same input, and
	// the corpus would have failed for a reason that is nothing to do with
	// the key. The recorded `extra` really does hold ints and floats as
	// different types, so the decode has to keep them apart. It is the same
	// trap a future caller hits if it builds an `Extra` out of decoded JSON
	// rather than out of its own values; see `Input.Extra`.
	dec := json.NewDecoder(strings.NewReader(string(body)))
	dec.UseNumber()
	if err := dec.Decode(&c); err != nil {
		t.Fatalf("decode corpus: %v", err)
	}
	for i := range c.Cases {
		c.Cases[i].Extra = corpusTypes(c.Cases[i].Extra).(map[string]any)
	}
	if len(c.Cases) == 0 {
		t.Fatal("the corpus is empty; testdata/cache.json is a frozen " +
			"golden -- restore it from version control")
	}
	return c
}

// corpusTypes turns `json.Number` back into the `int` or `float64` the
// recorded case holds.
func corpusTypes(v any) any {
	switch x := v.(type) {
	case nil:
		return nil
	case json.Number:
		if !strings.ContainsAny(x.String(), ".eE") {
			n, err := x.Int64()
			if err == nil {
				return int(n)
			}
		}
		f, err := x.Float64()
		if err != nil {
			panic("corpus holds an unreadable number: " + x.String())
		}
		return f
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, item := range x {
			out[k] = corpusTypes(item)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, item := range x {
			out[i] = corpusTypes(item)
		}
		return out
	default:
		return v
	}
}

func TestTheConstantsMatchTheCorpus(t *testing.T) {
	t.Parallel()
	c := load(t)
	if cache.SimVersion != c.SimVersion {
		t.Errorf("SimVersion = %d, the corpus says %d", cache.SimVersion, c.SimVersion)
	}
	if cache.MaxRows != c.MaxRows {
		t.Errorf("MaxRows = %d, the corpus says %d", cache.MaxRows, c.MaxRows)
	}
}

// TestThePayloadIsTheRecordedBytes is the real gate.
//
// Handed the corpus's recorded fingerprint, `Payload` must produce the
// identical string
// -- so the serialisation is provably the one the stored keys were computed
// over, and the
// only thing left that can move the key is the fingerprint itself.
func TestThePayloadIsTheRecordedBytes(t *testing.T) {
	t.Parallel()
	c := load(t)
	for _, tc := range c.Cases {
		t.Run(tc.Label, func(t *testing.T) {
			got := cache.Payload(c.RecordedFingerprint, tc.Kind, tc.input())
			if got != tc.Payload {
				t.Fatalf("payload differs\n got: %s\nrecorded: %s\n(%s)",
					got, tc.Payload, firstDifference(got, tc.Payload))
			}
			sum := sha256.Sum256([]byte(got))
			if hex.EncodeToString(sum[:]) != tc.Key {
				t.Errorf("key = %s, the corpus says %s",
					hex.EncodeToString(sum[:]), tc.Key)
			}
		})
	}
}

// firstDifference names the byte, because a 4 kB payload diffed by eye is how
// an ASCII-escaping bug survives a code review.
func firstDifference(a, b string) string {
	n := min(len(a), len(b))
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			lo := max(i-30, 0)
			return "first difference at byte " + itoa(i) +
				": got ..." + a[lo:min(i+30, len(a))] +
				"... vs recorded ..." + b[lo:min(i+30, len(b))] + "..."
		}
	}
	return "one is a prefix of the other: got " + itoa(len(a)) +
		" bytes, recorded " + itoa(len(b))
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var out []byte
	for n > 0 {
		out = append([]byte{byte('0' + n%10)}, out...)
		n /= 10
	}
	return string(out)
}

// TestTheKeyDiffersFromTheCorpusOnlyByTheFingerprint is the decision, pinned.
//
// A row computed by this engine and a row the corpus's engine computed for
// the same deck and seed do
// not collide -- and the difference is one field, not a serialisation
// drift. Both halves are asserted, because either alone would be
// satisfied by the wrong implementation.
func TestTheKeyDiffersFromTheCorpusOnlyByTheFingerprint(t *testing.T) {
	t.Parallel()
	c := load(t)
	engine := cache.Fingerprint()
	if engine == "" {
		t.Fatal("Fingerprint is empty, so caching would be off entirely")
	}
	if engine == c.RecordedFingerprint {
		t.Fatal("the fingerprint equals the corpus's recorded one, which " +
			"would mean it is not measuring this engine's own source -- see " +
			"the package comment")
	}
	for _, tc := range c.Cases {
		t.Run(tc.Label, func(t *testing.T) {
			in := tc.input()
			key := cache.Key(tc.Kind, in)
			if key == tc.Key {
				t.Fatal("the key equals the recorded one; the rows would collide")
			}
			// And the payloads differ in exactly the fingerprint: substitute
			// the recorded string for this engine's and the bytes must become
			// the recorded bytes.
			mine := cache.Payload(engine, tc.Kind, in)
			if strings.Count(mine, engine) != 1 {
				t.Fatalf("the fingerprint appears %d times in the payload; "+
					"this substitution is not the check it looks like",
					strings.Count(mine, engine))
			}
			if swapped := strings.Replace(mine, engine, c.RecordedFingerprint, 1); swapped != tc.Payload {
				t.Errorf("swapping the fingerprint does not give the recorded "+
					"payload, so the serialisation drifted somewhere else too:\n%s",
					firstDifference(swapped, tc.Payload))
			}
		})
	}
}

// TestColourSetsAreSortedIntoTheKey is the one claim the corpus structurally
// cannot make.
//
// A colour set is a set, and every recorded case carries its sets already in
// sorted order -- so
// no corpus case can show the sorting doing anything, and the sorting is
// exactly what stops a key from depending on the incidental order a slice
// arrives in.
// A cache that missed constantly would look cold rather than broken, which is
// why this is asserted rather than left to the corpus.
func TestColourSetsAreSortedIntoTheKey(t *testing.T) {
	t.Parallel()
	build := func(colors, pip []string) cache.Input {
		return cache.Input{
			Library: []*sim.Card{{
				Name:     "Rainbow",
				Cost:     sim.Cost{Pips: [][]string{pip}},
				Category: "utility",
				Produces: []sim.Source{{Colors: colors, Amount: 1}},
			}},
			Games: 1, Turns: 1, KeepRule: tier1.DefaultKeepRule(), Seed: 1,
		}
	}
	sorted := cache.Payload("engine", "sim.mana",
		build([]string{"B", "G", "R", "U", "W"}, []string{"G", "W"}))
	shuffled := cache.Payload("engine", "sim.mana",
		build([]string{"W", "U", "B", "R", "G"}, []string{"W", "G"}))
	if sorted != shuffled {
		t.Fatalf("the colour order reached the key:\n sorted: %s\nshuffled: %s",
			sorted, shuffled)
	}
	if !strings.Contains(sorted, `[["B","G","R","U","W"],1]`) {
		t.Errorf("the produced colours are not sorted into the key: %s", sorted)
	}
	if !strings.Contains(sorted, `[["G","W"]]`) {
		t.Errorf("the cost's pips are not sorted into the key: %s", sorted)
	}
}

func TestTheFingerprintIsAStableHexDigest(t *testing.T) {
	t.Parallel()
	first := cache.Fingerprint()
	if len(first) != 64 {
		t.Fatalf("fingerprint is %q, want 64 hex characters", first)
	}
	if _, err := hex.DecodeString(first); err != nil {
		t.Fatalf("fingerprint is not hex: %v", err)
	}
	if second := cache.Fingerprint(); second != first {
		t.Fatalf("fingerprint moved between calls: %s then %s", first, second)
	}
}

// ---------------------------------------------------------------- the store

func scratch(t *testing.T) *cache.Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "app.db")
	if err := authtest.NewScratchDB(path); err != nil {
		t.Fatalf("build the scratch app.db: %v", err)
	}
	store, err := cache.Open(path, nil)
	if err != nil {
		t.Fatalf("open the store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

type result struct {
	Games int     `json:"games"`
	Rate  float64 `json:"mulligan_rate"`
}

func TestARowGoesInAndComesBack(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := scratch(t)
	if hit := store.Get(ctx, "nothing-stored"); hit != nil {
		t.Fatal("a miss returned a hit")
	}
	store.Put(ctx, "k1", "sim.mana", result{Games: 20000, Rate: 0.25})
	hit := store.Get(ctx, "k1")
	if hit == nil {
		t.Fatal("the row did not come back")
	}
	var got result
	if err := json.Unmarshal(hit.Result, &got); err != nil {
		t.Fatalf("decode the stored result: %v", err)
	}
	if got.Games != 20000 || got.Rate != 0.25 {
		t.Fatalf("stored %+v", got)
	}
	// The recorded timestamp format: a `+00:00` offset, never `Z`.
	if !strings.HasSuffix(hit.CreatedAt, "+00:00") {
		t.Errorf("created_at is %q, which is not the recorded timestamp format",
			hit.CreatedAt)
	}
	// A struct keeps its field order where a map would be sorted -- the
	// constraint `internal/jobs` found and every stored result inherits.
	if !strings.HasPrefix(string(hit.Result), `{"games":`) {
		t.Errorf("stored blob is %s; the fields are not in the declared order",
			hit.Result)
	}
}

func TestAnEmptyKeyIsAMissAndAnEmptyStore(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := scratch(t)
	store.Put(ctx, "", "sim.mana", result{Games: 1})
	if hit := store.Get(ctx, ""); hit != nil {
		t.Fatal("an empty key returned a hit")
	}
	if store.Stats(ctx).Rows != 0 {
		t.Fatal("an empty key stored a row")
	}

	// A nil store is the Go spelling of "caching is off": every method works
	// and every one of them is a miss.
	var off *cache.Store
	off.Put(ctx, "k", "sim.mana", result{Games: 1})
	if off.Get(ctx, "k") != nil {
		t.Fatal("a nil store returned a hit")
	}
	if off.Clear(ctx) != 0 {
		t.Fatal("a nil store cleared rows")
	}
	if got := off.Stats(ctx); got.Rows != 0 || len(got.ByKind) != 0 {
		t.Fatalf("a nil store reports %+v", got)
	}
	if err := off.Close(); err != nil {
		t.Fatalf("closing a nil store: %v", err)
	}
}

func TestAReadTouchesTheRowSoEvictionIsLeastRecentlyUsed(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := scratch(t)
	store.Put(ctx, "old", "sim.mana", result{Games: 1})
	before := lastUsed(t, store, "old")
	// Distinguishable only at microsecond resolution, so read until it moves
	// rather than sleeping for a fixed span.
	var after string
	for i := 0; i < 200; i++ {
		store.Get(ctx, "old")
		if after = lastUsed(t, store, "old"); after != before {
			break
		}
	}
	if after == before {
		t.Fatal("reading a row never moved its last_used_at, so eviction is " +
			"least-recently-COMPUTED rather than least-recently-used")
	}
}

func lastUsed(t *testing.T, store *cache.Store, key string) string {
	t.Helper()
	var out string
	if err := store.DB().QueryRow(
		"SELECT last_used_at FROM sim_cache WHERE key = ?", key).Scan(&out); err != nil {
		t.Fatalf("read last_used_at: %v", err)
	}
	return out
}

func TestTheTableIsBoundedAtMaxRows(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := scratch(t)
	// Filled directly rather than through `Put`, which would run MaxRows
	// transactions to prove one branch. The last write is a real `Put`, so the
	// eviction that runs is the production one.
	tx, err := store.DB().Begin()
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	for i := 0; i < cache.MaxRows; i++ {
		// **`created_at` runs the OTHER WAY from `last_used_at`**, and that is
		// the whole point of this fixture. Stamping both the same way makes
		// `ORDER BY created_at` and `ORDER BY last_used_at` agree, so the
		// eviction policy ADR 18 chose -- least recently *used* -- would be
		// indistinguishable from least recently computed. It was written that
		// way first, and a mutation swapping the column passed.
		used := "2026-08-22T00:00:00." + pad6(i) + "+00:00"
		created := "2026-08-22T00:00:00." + pad6(cache.MaxRows-i) + "+00:00"
		if _, err := tx.Exec(
			"INSERT INTO sim_cache (key, kind, result_json, created_at, last_used_at)"+
				" VALUES (?, 'sim.mana', '{}', ?, ?)",
			"filler-"+itoa(i), created, used); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}

	store.Put(ctx, "the-newcomer", "sim.policy", result{Games: 7})
	stats := store.Stats(ctx)
	if stats.Rows != cache.MaxRows {
		t.Fatalf("%d rows after the bound, want %d", stats.Rows, cache.MaxRows)
	}
	if store.Get(ctx, "the-newcomer") == nil {
		t.Fatal("the row that triggered eviction evicted itself")
	}
	if store.Get(ctx, "filler-0") != nil {
		t.Fatal("the least recently used row survived")
	}
	if store.Get(ctx, "filler-"+itoa(cache.MaxRows-1)) == nil {
		t.Fatal("the most recently used filler was evicted")
	}
	if stats.ByKind["sim.policy"] != 1 {
		t.Errorf("by_kind is %v, which does not name the new row's kind",
			stats.ByKind)
	}
}

func pad6(n int) string {
	s := itoa(n)
	for len(s) < 6 {
		s = "0" + s
	}
	return s
}

func TestStatsAndClear(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := scratch(t)
	empty := store.Stats(ctx)
	if !empty.Enabled {
		t.Error("an empty table reports caching as disabled, which is the one " +
			"thing `enabled` exists to tell apart from it")
	}
	if empty.Rows != 0 || empty.Oldest != nil || empty.Newest != nil {
		t.Fatalf("an empty table reports %+v", empty)
	}
	store.Put(ctx, "a", "sim.mana", result{Games: 1})
	store.Put(ctx, "b", "sim.mana", result{Games: 2})
	store.Put(ctx, "c", "sim.lands.count", result{Games: 3})
	got := store.Stats(ctx)
	if got.Rows != 3 || got.ByKind["sim.mana"] != 2 || got.ByKind["sim.lands.count"] != 1 {
		t.Fatalf("stats are %+v", got)
	}
	if got.Bytes == 0 || got.Oldest == nil || got.Newest == nil {
		t.Fatalf("stats are %+v", got)
	}
	if *got.Oldest > *got.Newest {
		t.Errorf("oldest %q is after newest %q", *got.Oldest, *got.Newest)
	}
	if n := store.Clear(ctx); n != 3 {
		t.Fatalf("clear returned %d, want 3", n)
	}
	if store.Stats(ctx).Rows != 0 {
		t.Fatal("clear left rows behind")
	}
}

// TestOpenWillNotCreateTheDatabase is `mode=rw`, never `rwc`.
//
// The ladder (`auth.Migrate`) runs once at boot and is the only creator; a
// file this created would be a database at version zero, or worse one filled
// in from a second copy of the schema.
func TestOpenWillNotCreateTheDatabase(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "app.db")
	if _, err := cache.Open(path, nil); err == nil {
		t.Fatal("opening an absent app.db succeeded")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("opening an absent app.db created it")
	}
}

// TestAnUnstorableResultIsAMissNotAFailure: `Put` never fails the caller --
// the same trade `decklog.Record` and `claude/ledger`'s recorder make.
func TestAnUnstorableResultIsAMissNotAFailure(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := scratch(t)
	store.Put(ctx, "bad", "sim.mana", make(chan int)) // encoding/json refuses
	if store.Stats(ctx).Rows != 0 {
		t.Fatal("an unstorable result stored something")
	}
	if store.Get(ctx, "bad") != nil {
		t.Fatal("an unstorable result came back")
	}
}

// TestEveryKeepRuleFieldChangesTheKey guards the one place the serialisation
// names fields by hand.
//
// `Key`'s doc promises **a new mulligan lever is in the key
// the day it is added**, and for every other input that falls out of the
// struct going in whole. `writeKeepRule` cannot get it for free:
// `encoding/json` on a struct would sort nothing and a `map` would sort
// everything, so the five keys are written out by hand, in the recorded
// alphabetical order.
//
// A hand-written list is a list that can fall behind the struct, and the
// failure is the exact one ADR 18 exists to prevent rather than a cosmetic
// drift: a sixth field would be **absent from the key while genuinely
// changing the run**, so two runs under different keep rules would
// share one row and the second would be served the first one's numbers.
// Every figure well-formed, none of them about the rule that was asked for.
//
// The corpus cannot see it. It is a frozen recording of today's five fields,
// so it
// would keep passing while saying nothing about the sixth. Reflection is what
// makes this a question about the struct instead of about the fixture.
func TestEveryKeepRuleFieldChangesTheKey(t *testing.T) {
	t.Parallel()
	input := func(k tier1.KeepRule) cache.Input {
		return cache.Input{
			Library:  []*sim.Card{{Name: "Forest", Category: "land", IsLand: true}},
			Games:    100,
			Turns:    10,
			KeepRule: k,
			Seed:     7,
		}
	}
	base := cache.Key("sim.mana", input(tier1.DefaultKeepRule()))
	if base == "" {
		t.Fatal("Key is empty, so caching would be off entirely")
	}
	payload := cache.Payload("engine", "sim.mana", input(tier1.DefaultKeepRule()))

	rt := reflect.TypeOf(tier1.KeepRule{})
	for i := range rt.NumField() {
		field := rt.Field(i)
		t.Run(field.Name, func(t *testing.T) {
			// The field's wire name from its json tag, which is the name that
			// has to appear in the blob.
			name := strings.Split(field.Tag.Get("json"), ",")[0]
			if name == "" {
				t.Fatalf("%s carries no json tag, so its wire name is "+
					"unknowable from here", field.Name)
			}
			if !strings.Contains(payload, `"`+name+`":`) {
				t.Errorf("`%s` is a KeepRule field and does not appear in the "+
					"cache key. `writeKeepRule` names its fields by hand, so a "+
					"new one has to be added there -- in alphabetical order "+
					"-- or two different keep rules will share a row.", name)
			}

			mutated := tier1.DefaultKeepRule()
			f := reflect.ValueOf(&mutated).Elem().Field(i)
			if f.Kind() != reflect.Int {
				t.Fatalf("%s is a %s; this test only knows how to move an int, "+
					"and a field it cannot move is a field it cannot check",
					field.Name, f.Kind())
			}
			f.SetInt(f.Int() + 1)
			if got := cache.Key("sim.mana", input(mutated)); got == base {
				t.Errorf("moving `%s` did not change the key", name)
			}
		})
	}
}
