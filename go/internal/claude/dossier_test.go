package claude

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/auth/authtest"
	"github.com/aasquier/sylvan-library/go/internal/deck"
	"github.com/aasquier/sylvan-library/go/internal/pool"
)

// The commander dossier, held to the recorded corpus `testdata/dossier.json`.
//
// Four things the corpus pins, in the order they matter. **The cache key**,
// because dossiers already stored on the instance are served under it
// -- a wrong fingerprint costs a four-minute paid search per commander and
// looks like a cache that simply missed. **The brief's opening message**, as
// bytes, which is the whole of `DossierBrief` and `dumpJSON` checked at once
// against the tiny pool. **The free GET's three shapes**, two of them
// different key sets. And **every outcome of a run**, each report compared as
// marshalled bytes -- key order included, because the report is the wire and
// `tier1.Number` taught that a value can be right and still go out wrong.

type dossierCorpus struct {
	Keys []struct {
		OracleID string `json:"oracle_id"`
		Tier     string `json:"tier"`
		Key      string `json:"key"`
	} `json:"keys"`
	KeysWithModelOverride []struct {
		OracleID string `json:"oracle_id"`
		Tier     string `json:"tier"`
		Key      string `json:"key"`
	} `json:"keys_with_model_override"`
	Fingerprint struct {
		Version            int    `json:"version"`
		InstructionsSHA256 string `json:"instructions_sha256"`
		SchemaDumps        string `json:"schema_dumps"`
		Model              string `json:"model"`
		Fingerprint        string `json:"fingerprint"`
	} `json:"fingerprint"`
	Brief struct {
		Slug            string          `json:"slug"`
		Commander       string          `json:"commander"`
		OracleID        string          `json:"oracle_id"`
		Facts           json.RawMessage `json:"facts"`
		Opening         string          `json:"opening"`
		Label           string          `json:"label"`
		HeadlessRefusal string          `json:"headless_refusal"`
	} `json:"brief"`
	CachedGet []struct {
		Note    string          `json:"note"`
		Payload json.RawMessage `json:"payload"`
	} `json:"cached_get"`
	Stored struct {
		Key       string          `json:"key"`
		Result    json.RawMessage `json:"result"`
		CreatedAt string          `json:"created_at"`
	} `json:"stored"`
	Reports []reportCase `json:"reports"`
}

// reportCase is one outcome of a run, for either searching mode.
type reportCase struct {
	Note      string          `json:"note"`
	Question  string          `json:"question"`
	Requested any             `json:"requested"`
	Refresh   bool            `json:"refresh"`
	Turn      *turnRecord     `json:"turn"`
	Report    json.RawMessage `json:"report"`
}

// turnRecord is the corpus's record of the Turn a run's conversation
// returned.
type turnRecord struct {
	Model           string   `json:"model"`
	StopReason      string   `json:"stop_reason"`
	Text            string   `json:"text"`
	Refused         bool     `json:"refused"`
	InputTokens     int      `json:"input_tokens"`
	OutputTokens    int      `json:"output_tokens"`
	CacheReadTokens int      `json:"cache_read_tokens"`
	Searched        []Page   `json:"searched"`
	SearchErrors    []string `json:"search_errors"`
}

func (r turnRecord) turn(mode string) Turn {
	return Turn{Mode: mode, Model: r.Model, StopReason: r.StopReason, Text: r.Text,
		ToolCalls: []ToolCall{}, InputTokens: r.InputTokens, OutputTokens: r.OutputTokens,
		Searched: r.Searched, SearchErrors: r.SearchErrors, Refused: r.Refused,
		CacheReadTokens: r.CacheReadTokens}
}

// frozenNow is the corpus's clock: every recorded stamp carries it, so a
// report compares as bytes only while the test clock reads the same.
const frozenNow = "2026-08-23T04:05:06.789012+00:00"

func freezeClock(t *testing.T) {
	t.Helper()
	was := now
	now = func() string { return frozenNow }
	t.Cleanup(func() { now = was })
}

// miniDeck is the corpus's deck: Gyome in front of ninety-nine Swamps.
const miniDeck = `slug: mini
name: Mini Deck
status: theoretical
stage: curated
commander:
  - Gyome, Master Chef
cards:
  - name: Swamp
    category: land
    qty: 99
    why: Black mana.
`

func miniDecks(t *testing.T) (*deck.Deck, *deck.Deck) {
	t.Helper()
	mini, err := deck.FromText(miniDeck, "mini")
	if err != nil {
		t.Fatal(err)
	}
	headless, err := deck.FromText(
		"slug: mini\nname: Mini Deck\nstatus: theoretical\nstage: curated\n"+
			"commander: []\ncards:\n  - name: Swamp\n    category: land\n    qty: 99\n"+
			"    why: Black mana.\n", "mini")
	if err != nil {
		t.Fatal(err)
	}
	return mini, headless
}

func loadDossierCorpus(t *testing.T) dossierCorpus {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "dossier.json"))
	if err != nil {
		t.Fatalf("reading the corpus: %v", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var corpus dossierCorpus
	if err := decoder.Decode(&corpus); err != nil {
		t.Fatalf("decoding the corpus: %v", err)
	}
	if len(corpus.Keys) == 0 || len(corpus.Reports) == 0 {
		t.Fatal("the corpus is empty; testdata/dossier.json is a frozen golden and always carries both")
	}
	return corpus
}

// scratchStore is a dossier store over a throwaway app.db built by the real
// schema ladder.
func scratchStore(t *testing.T) *DossierStore {
	t.Helper()
	rec, _ := scratchLedger(t)
	return NewDossierStore(rec.DB(), nil)
}

// noEnvOverrides is the environment the corpus was written in: no model
// override and no stance ceiling.
func noEnvOverrides(t *testing.T) {
	t.Helper()
	t.Setenv(modelEnv, "")
	t.Setenv(CeilingEnv, "")
}

// ------------------------------------------------------------------ the key

func TestTheDossierCacheKeyIsTheRecordedOneByteForByte(t *testing.T) {
	noEnvOverrides(t)
	corpus := loadDossierCorpus(t)
	for _, row := range corpus.Keys {
		got, err := CacheKey(row.OracleID, row.Tier)
		if err != nil {
			t.Fatal(err)
		}
		if got != row.Key {
			t.Errorf("CacheKey(%q, %q) = %q, corpus %q", row.OracleID, row.Tier, got, row.Key)
		}
	}
	// The override wins over every tier, so two seats get one key.
	t.Setenv(modelEnv, "claude-test-1")
	for _, row := range corpus.KeysWithModelOverride {
		got, err := CacheKey(row.OracleID, row.Tier)
		if err != nil {
			t.Fatal(err)
		}
		if got != row.Key {
			t.Errorf("with the model override, CacheKey(%q, %q) = %q, corpus %q",
				row.OracleID, row.Tier, got, row.Key)
		}
	}
}

// The fingerprint's parts, apart, so a failure says which half is wrong: the
// prompt bytes (data in modes.json), the schema's sorted-key canonical
// rendering (canonjson.go), and the model id.
func TestTheFingerprintsPartsAreEachTheRecordedOnes(t *testing.T) {
	noEnvOverrides(t)
	corpus := loadDossierCorpus(t)
	mode, err := GetMode(ModeCommanderDossier)
	if err != nil {
		t.Fatal(err)
	}
	if DossierVersion != corpus.Fingerprint.Version {
		t.Errorf("DossierVersion is %d, corpus %d", DossierVersion, corpus.Fingerprint.Version)
	}
	sum := sha256.Sum256([]byte(mode.Instructions))
	if got := hex.EncodeToString(sum[:]); got != corpus.Fingerprint.InstructionsSHA256 {
		t.Errorf("the instructions hash %s, corpus %s -- modes.json has drifted from the recorded prompt",
			got, corpus.Fingerprint.InstructionsSHA256)
	}
	if got := dumpJSON(mode.ResponseSchema, dumpOptions{SortKeys: true}); got != corpus.Fingerprint.SchemaDumps {
		t.Errorf("the canonical schema rendering differs:\n got    %s\n corpus %s",
			got, corpus.Fingerprint.SchemaDumps)
	}
	if got := ModelFor(""); got != corpus.Fingerprint.Model {
		t.Errorf("the default model is %q, corpus %q", got, corpus.Fingerprint.Model)
	}
	if got, _ := Fingerprint(""); got != corpus.Fingerprint.Fingerprint {
		t.Errorf("the fingerprint is %q, corpus %q", got, corpus.Fingerprint.Fingerprint)
	}
}

// ---------------------------------------------------------------- the brief

func TestTheBriefsOpeningMessageMatchesTheGoldenBytes(t *testing.T) {
	t.Parallel()
	corpus := loadDossierCorpus(t)
	mini, headless := miniDecks(t)
	withPool(t, func(c *pool.Conn) {
		facts, err := DossierBrief(context.Background(), c, "mini", mini)
		if err != nil {
			t.Fatal(err)
		}
		if got := dossierOpening(facts); got != corpus.Brief.Opening {
			t.Errorf("the opening message differs from the corpus:\n--- got\n%s\n--- corpus\n%s",
				got, corpus.Brief.Opening)
		}
		_, err = DossierBrief(context.Background(), c, "mini", headless)
		var refusal *ErrNoCommander
		if !errors.As(err, &refusal) {
			t.Fatalf("a headless deck answered %v, want ErrNoCommander", err)
		}
		if refusal.Error() != corpus.Brief.HeadlessRefusal {
			t.Errorf("the refusal reads\n  %q\nthe corpus says\n  %q", refusal.Error(), corpus.Brief.HeadlessRefusal)
		}
	})
	// No pool is the same refusal: `card` is null either way.
	_, err := DossierBrief(context.Background(), nil, "mini", mini)
	var refusal *ErrNoCommander
	if !errors.As(err, &refusal) {
		t.Errorf("with no pool the brief answered %v, want ErrNoCommander", err)
	}
}

// --------------------------------------------------------------- the GET

func TestTheCachedGetShapesAreTheRecordedOnes(t *testing.T) {
	noEnvOverrides(t)
	freezeClock(t)
	corpus := loadDossierCorpus(t)
	mini, headless := miniDecks(t)
	store := scratchStore(t)
	byNote := map[string]json.RawMessage{}
	for _, row := range corpus.CachedGet {
		byNote[row.Note] = row.Payload
	}
	withPool(t, func(c *pool.Conn) {
		ctx := context.Background()
		got, err := ReadCachedDossier(ctx, c, "mini", mini, store)
		if err != nil {
			t.Fatal(err)
		}
		assertSameJSONValue(t, "no row yet", got, byNote["no row yet"])

		got, err = ReadCachedDossier(ctx, c, "mini", headless, store)
		if err != nil {
			t.Fatal(err)
		}
		assertSameJSONValue(t, "no commander the pool knows", got, byNote["no commander the pool knows"])

		// The stored row is the corpus's own bytes, served raw -- under the
		// default tier's key, which is the GET's wart.
		key, _ := CacheKey(corpus.Brief.OracleID, "")
		if key != corpus.Stored.Key {
			t.Fatalf("the default key is %q, the corpus stored under %q", key, corpus.Stored.Key)
		}
		store.Put(ctx, key, corpus.Brief.OracleID, corpus.Brief.Commander, corpus.Stored.Result)
		got, err = ReadCachedDossier(ctx, c, "mini", mini, store)
		if err != nil {
			t.Fatal(err)
		}
		assertSameJSONValue(t, "a stored row", got, byNote["a stored row"])
	})
}

// The two GET shapes are two types because the difference is which keys
// exist: no commander means no `answered_by`.
func TestTheHeadlessGetHasFiveKeysAndNoAnsweredBy(t *testing.T) {
	t.Parallel()
	raw, err := json.Marshal(HeadlessDossier{Slug: "x", Dossier: emptyObject})
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"slug":"x","commander":"","dossier":{},"cached":false,"generated_at":null}`
	if string(raw) != want {
		t.Errorf("the headless shape is\n  %s\nwant\n  %s", raw, want)
	}
	stamp := "t"
	raw, err = json.Marshal(CachedDossier{AnsweredBy: "claude", Slug: "x", Commander: "c",
		Dossier: json.RawMessage(`{"who":1}`), Cached: true, GeneratedAt: &stamp})
	if err != nil {
		t.Fatal(err)
	}
	const wantHit = `{"answered_by":"claude","slug":"x","commander":"c","dossier":{"who":1},"cached":true,"generated_at":"t"}`
	if string(raw) != wantHit {
		t.Errorf("the hit shape is\n  %s\nwant\n  %s", raw, wantHit)
	}
}

// --------------------------------------------------------------- the runs

// Every outcome of a run, driven with the corpus's recorded Turn and
// compared as bytes. The order is the corpus's, because "served
// from the store" can only follow "a whole dossier" having stored one.
func TestEveryDossierOutcomeAgreesWithTheCorpus(t *testing.T) {
	noEnvOverrides(t)
	freezeClock(t)
	corpus := loadDossierCorpus(t)
	mini, _ := miniDecks(t)
	store := scratchStore(t)
	sawStore, sawHit := false, false
	withPool(t, func(c *pool.Conn) {
		ctx := context.Background()
		for _, row := range corpus.Reports {
			plan, err := CheckDossier(ctx, c, "mini", mini, DossierRequest{
				Requested: row.Requested, Refresh: row.Refresh, Store: store})
			if err != nil {
				t.Fatalf("%s: check: %v", row.Note, err)
			}
			var report DossierReport
			if row.Turn == nil {
				// A row with no recorded Turn means nothing may be asked. So
				// the plan must already carry the answer.
				if plan.Answer == nil {
					t.Errorf("%s: the plan wants a call and the corpus made none", row.Note)
					continue
				}
				report = *plan.Answer
				if report.Cached {
					sawHit = true
				}
			} else {
				if plan.Answer != nil {
					t.Errorf("%s: the plan answered without a call and the corpus called", row.Note)
					continue
				}
				report, err = readDossier(ctx, c, plan, row.Turn.turn(ModeCommanderDossier))
				if err != nil {
					t.Fatalf("%s: %v", row.Note, err)
				}
			}
			assertSameJSONValue(t, row.Note, report, row.Report)
			if row.Note == "a whole dossier" {
				hit := store.Get(ctx, plan.Key)
				if hit == nil {
					t.Fatal("a whole dossier was not stored")
				}
				assertSameJSONValue(t, "the stored row", hit.Result, corpus.Stored.Result)
				if hit.CreatedAt != corpus.Stored.CreatedAt {
					t.Errorf("stored at %q, corpus %q", hit.CreatedAt, corpus.Stored.CreatedAt)
				}
				sawStore = true
			}
		}
	})
	if !sawStore || !sawHit {
		t.Fatalf("the corpus must store a dossier and then serve it (store %v, hit %v)", sawStore, sawHit)
	}
}

// A refusal, an unparseable answer and a no-source answer store NOTHING --
// and the corpus covers each -- which is asserted here on its own because
// the store is the one side effect the report bytes cannot show.
func TestOnlyAWholeDossierIsStored(t *testing.T) {
	noEnvOverrides(t)
	freezeClock(t)
	corpus := loadDossierCorpus(t)
	mini, _ := miniDecks(t)
	withPool(t, func(c *pool.Conn) {
		ctx := context.Background()
		for _, row := range corpus.Reports {
			if row.Turn == nil || row.Note == "a whole dossier" {
				continue
			}
			store := scratchStore(t)
			plan, err := CheckDossier(ctx, c, "mini", mini, DossierRequest{
				Requested: row.Requested, Refresh: true, Store: store})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := readDossier(ctx, c, plan, row.Turn.turn(ModeCommanderDossier)); err != nil {
				t.Fatal(err)
			}
			if store.Get(ctx, plan.Key) != nil {
				t.Errorf("%s: a dossier was stored where nothing may be stored", row.Note)
			}
		}
	})
}

// ---------------------------------------------------------------- the store

func TestTheStoreNeverFailsTheFeature(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	// A nil store and a store over a nil handle both miss and swallow writes.
	var none *DossierStore
	none.Put(ctx, "k", "o", "c", map[string]any{})
	if none.Get(ctx, "k") != nil {
		t.Error("a nil store answered a hit")
	}
	empty := NewDossierStore(nil, nil)
	empty.Put(ctx, "k", "o", "c", map[string]any{})
	if empty.Get(ctx, "k") != nil {
		t.Error("a store with no database answered a hit")
	}
	// An empty key is a miss without touching the database: caching off.
	store := scratchStore(t)
	store.Put(ctx, "", "o", "c", map[string]any{"x": 1})
	if store.Get(ctx, "") != nil {
		t.Error("an empty key stored something")
	}
	// A row round-trips, raw, and a second put replaces it.
	store.Put(ctx, "k", "o", "c", json.RawMessage(`{"who":{"prose":"first"}}`))
	store.Put(ctx, "k", "o", "c", json.RawMessage(`{"who":{"prose":"second"}}`))
	hit := store.Get(ctx, "k")
	if hit == nil || string(hit.Result) != `{"who":{"prose":"second"}}` {
		t.Errorf("the round trip gave %+v", hit)
	}
	// A broken handle is a miss, never a panic or an error out.
	rec, _ := scratchLedger(t)
	broken := NewDossierStore(rec.DB(), nil)
	_ = rec.DB().Close()
	broken.Put(ctx, "k", "o", "c", map[string]any{})
	if broken.Get(ctx, "k") != nil {
		t.Error("a closed database answered a hit")
	}
}

// A scratch app.db built by the real schema holds the table the store
// writes; this is what makes the round trip above meaningful rather than a
// mock.
func TestTheScratchSchemaCarriesTheDossierTable(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "app.db")
	if err := authtest.NewScratchDB(path); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains([]byte(authtest.Schema()), []byte("CREATE TABLE dossier_cache")) {
		t.Fatal("app_schema.sql has no dossier_cache table")
	}
}

// ------------------------------------------------------------- the helpers

// assertSameJSONValue compares a Go value with the corpus's JSON twice: once
// as decoded values, so a failure says WHICH field, and once as compact bytes,
// because the key order is the wire and no value comparison carries it.
func assertSameJSONValue(t *testing.T, what string, got any, want json.RawMessage) {
	t.Helper()
	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("%s: %v", what, err)
	}
	var a, b any
	if err := json.Unmarshal(raw, &a); err != nil {
		t.Fatalf("%s: %v", what, err)
	}
	if err := json.Unmarshal(want, &b); err != nil {
		t.Fatalf("%s: %v", what, err)
	}
	if !reflect.DeepEqual(a, b) {
		t.Errorf("%s:\n go     %s\n python %s", what, raw, compactJSON(t, want))
		return
	}
	if string(raw) != compactJSON(t, want) {
		t.Errorf("%s: key order differs\n go     %s\n python %s", what, raw, compactJSON(t, want))
	}
}
