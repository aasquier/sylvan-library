package claude

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/pool"
)

// The slot argument's two Python-owned halves, held to Python by a corpus.
//
// `OnlyCharges` and `ResolveAlternatives` are where ADR 25 stops being a prompt
// and becomes code: the first decides what counts as an argument, the second is
// rule 2 made executable. Both are judged against `testdata/argue.json`, which
// `tests/go_fixtures.py` renders from the real functions against the real
// 21-card pool.

type argueCorpus struct {
	Charges []struct {
		Note    string   `json:"note"`
		Items   []any    `json:"items"`
		Kept    []Charge `json:"kept"`
		Dropped int      `json:"dropped"`
	} `json:"charges"`
	Alternatives []struct {
		Note     string              `json:"note"`
		Names    []any               `json:"names"`
		Identity []string            `json:"identity"`
		InDeck   []string            `json:"in_deck"`
		Kept     []string            `json:"kept"`
		Dropped  map[string][]string `json:"dropped"`
	} `json:"alternatives"`
}

func loadArgueCorpus(t *testing.T) argueCorpus {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "argue.json"))
	if err != nil {
		t.Fatalf("reading the corpus: %v", err)
	}
	var corpus argueCorpus
	if err := json.Unmarshal(raw, &corpus); err != nil {
		t.Fatalf("decoding the corpus: %v", err)
	}
	if len(corpus.Charges) == 0 || len(corpus.Alternatives) == 0 {
		t.Fatal("the corpus is empty; run `python tests/go_fixtures.py`")
	}
	return corpus
}

func TestOnlyChargesAgreesWithPython(t *testing.T) {
	corpus := loadArgueCorpus(t)
	for _, row := range corpus.Charges {
		kept, dropped := OnlyCharges(row.Items)
		if dropped != row.Dropped {
			t.Errorf("%s: dropped %d, python %d", row.Note, dropped, row.Dropped)
		}
		if len(kept) != len(row.Kept) {
			t.Errorf("%s: kept %d, python %d", row.Note, len(kept), len(row.Kept))
			continue
		}
		for i := range kept {
			if kept[i] != row.Kept[i] {
				t.Errorf("%s: charge %d is %+v, python %+v", row.Note, i, kept[i], row.Kept[i])
			}
		}
	}
}

// A charge is rendered into a payload the client reads, so the bytes are the
// contract and only bytes carry field order.
func TestAChargeMarshalsInPythonsFieldOrder(t *testing.T) {
	raw, err := json.Marshal(Charge{Claim: "c", Ground: "cost", Fact: "f", Strength: "minor"})
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"claim":"c","ground":"cost","fact":"f","strength":"minor"}`
	if string(raw) != want {
		t.Errorf("a charge marshals as\n  %s\nwant\n  %s", raw, want)
	}
}

func TestResolveAlternativesAgreesWithPython(t *testing.T) {
	corpus := loadArgueCorpus(t)
	withPool(t, func(c *pool.Conn) {
		for _, row := range corpus.Alternatives {
			// "no pool at all" is the one case that cannot be produced beside a
			// pool: every name comes back unresolved and must be reported as
			// `no_pool` rather than as six invented cards.
			conn := c
			if row.Note == "no pool at all" {
				conn = nil
			}
			inDeck := map[string]bool{}
			for _, n := range row.InDeck {
				inDeck[n] = true
			}
			kept, dropped, err := ResolveAlternatives(context.Background(), conn,
				row.Names, row.Identity, inDeck)
			if err != nil {
				t.Errorf("%s: %v", row.Note, err)
				continue
			}
			names := []string{}
			for _, k := range kept {
				names = append(names, k.Name)
			}
			if !reflect.DeepEqual(names, row.Kept) {
				t.Errorf("%s:\n kept   %v\n python %v", row.Note, names, row.Kept)
			}
			got := map[string][]string{
				"not_in_pool": dropped.NotInPool, "banned": dropped.Banned,
				"off_colour": dropped.OffColour, "no_pool": dropped.NoPool,
				"already_in_deck": dropped.AlreadyInDeck,
			}
			for bucket, want := range row.Dropped {
				if !reflect.DeepEqual(got[bucket], want) {
					t.Errorf("%s: %s is %v, python %v", row.Note, bucket, got[bucket], want)
				}
			}
		}
	})
}

// The empty-run shape has FOUR keys where a real run has five, and that is
// Python's rather than anybody's preference: `argue._report`'s default for
// `alternatives_dropped` omits `already_in_deck` while `resolve_alternatives`
// always returns it. A single Go struct would put a fifth key on the wire in
// exactly the two cases -- stance off, and a refusal -- where Python leaves it
// off, which is why there are two types.
func TestTheEmptyDroppedShapeIsPythonsFourKeys(t *testing.T) {
	raw, err := json.Marshal(noneDropped())
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"not_in_pool":[],"banned":[],"off_colour":[],"no_pool":[]}`
	if string(raw) != want {
		t.Errorf("the empty shape is\n  %s\nwant\n  %s", raw, want)
	}
	full, err := json.Marshal(DroppedAlternatives{
		NotInPool: []string{}, Banned: []string{}, OffColour: []string{},
		AlreadyInDeck: []string{}, NoPool: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	const wantFull = `{"not_in_pool":[],"banned":[],"off_colour":[],"already_in_deck":[],"no_pool":[]}`
	if string(full) != wantFull {
		t.Errorf("the full shape is\n  %s\nwant\n  %s", full, wantFull)
	}
}

// ADR 25's absence, asserted at EVERY level of the schema.
//
// `TestTheSlotArgumentHasNowhereToPutADefence` in modes_test.go already checks
// the top-level properties and `additionalProperties`. This one walks the whole
// tree, because the top level is not where such a field would plausibly be
// added: a `defence` or `strength_in_favour` belongs beside `claim` inside a
// charge, one level down, where the existing check cannot see it. The forbidden
// list is wider here for the same reason -- `keep`, `in_favour` and `why` are
// what a well-meaning change would actually name it.
func TestNoLevelOfTheSlotArgumentSchemaCanHoldADefence(t *testing.T) {
	mode, err := GetMode(ModeSlotArgument)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(mode.ResponseSchema)
	if err != nil {
		t.Fatal(err)
	}
	var walk func(any) []string
	walk = func(node any) []string {
		out := []string{}
		switch v := node.(type) {
		case map[string]any:
			if props, ok := v["properties"].(map[string]any); ok {
				for key := range props {
					out = append(out, key)
				}
			}
			for _, child := range v {
				out = append(out, walk(child)...)
			}
		case []any:
			for _, child := range v {
				out = append(out, walk(child)...)
			}
		}
		return out
	}
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	keys := walk(decoded)
	if len(keys) == 0 {
		t.Fatal("no properties found; this test would pass against an empty schema")
	}
	for _, forbidden := range []string{"defence", "defense", "verdict", "summary",
		"keep", "in_favour", "recommendation", "why"} {
		for _, key := range keys {
			if key == forbidden {
				t.Errorf("the schema has a %q field; ADR 25 is that it cannot", forbidden)
			}
		}
	}
}
