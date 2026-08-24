package claude

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/pool"
)

// The two hosted-search modes' shared instruments, held to the recorded
// corpus.
//
// What this file is really guarding is the pair of ASYMMETRIES: the same input
// handed to a dossier passage and a research finding must come back different,
// and the same card name handed to `Competitors` and `ResolveCards` must come
// back different. Both are ADR 26, and both are the kind of thing a rewrite
// "tidies" into consistency without noticing it has deleted a decision.

type sourcesCorpus struct {
	Canonical []struct {
		URL       string `json:"url"`
		Canonical string `json:"canonical"`
	} `json:"canonical"`
	Keep []struct {
		Note     string   `json:"note"`
		Claimed  []any    `json:"claimed"`
		Searched []Page   `json:"searched"`
		Kept     []Source `json:"kept"`
		Dropped  int      `json:"dropped"`
	} `json:"keep"`
	Grounded []struct {
		Note            string    `json:"note"`
		Item            any       `json:"item"`
		Allowed         []string  `json:"allowed"`
		Section         Passage   `json:"section"`
		Grounded        []Finding `json:"grounded"`
		GroundedDropped int       `json:"grounded_dropped"`
	} `json:"grounded"`
	Pool []struct {
		Note               string            `json:"note"`
		Name               any               `json:"name"`
		Allowed            []string          `json:"allowed"`
		Competitors        []json.RawMessage `json:"competitors"`
		CompetitorsDropped int               `json:"competitors_dropped"`
		ResearchCards      []json.RawMessage `json:"research_cards"`
		ResearchUnresolved int               `json:"research_unresolved"`
	} `json:"pool"`
}

func loadSourcesCorpus(t *testing.T) sourcesCorpus {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "sources.json"))
	if err != nil {
		t.Fatalf("reading the corpus: %v", err)
	}
	// **UseNumber, because production does.** `Turn.Parsed` decodes with it, so
	// a numeric field reaches these functions as a `json.Number` carrying its
	// literal. A test decoding with plain Unmarshal hands them a `float64`
	// instead and never exercises the branch that actually runs -- which a
	// mutation proved by blanking the json.Number case and surviving.
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var corpus sourcesCorpus
	if err := decoder.Decode(&corpus); err != nil {
		t.Fatalf("decoding the corpus: %v", err)
	}
	if len(corpus.Canonical) == 0 || len(corpus.Pool) == 0 {
		t.Fatal("the corpus is empty; testdata/sources.json is a frozen golden and always carries both")
	}
	return corpus
}

func allowedSet(ids []string) map[string]bool {
	out := map[string]bool{}
	for _, id := range ids {
		out[id] = true
	}
	return out
}

// The asymmetry a tidier rewrite flattens: with a scheme, only the scheme
// and host are lowercased and THE PATH KEEPS ITS CASE; with no scheme,
// everything goes down.
func TestCanonicalURLMatchesTheRecordedCorpus(t *testing.T) {
	t.Parallel()
	corpus := loadSourcesCorpus(t)
	for _, row := range corpus.Canonical {
		if got := CanonicalURL(row.URL); got != row.Canonical {
			t.Errorf("%q\n got    %q\n corpus %q", row.URL, got, row.Canonical)
		}
	}
}

func TestKeepSourcesAgreesWithTheCorpus(t *testing.T) {
	t.Parallel()
	corpus := loadSourcesCorpus(t)
	for _, row := range corpus.Keep {
		kept, dropped := KeepSources(row.Claimed, row.Searched)
		if dropped != row.Dropped {
			t.Errorf("%s: dropped %d, corpus %d", row.Note, dropped, row.Dropped)
		}
		if !reflect.DeepEqual(kept, row.Kept) {
			t.Errorf("%s:\n got    %+v\n corpus %+v", row.Note, kept, row.Kept)
		}
	}
}

// **The first ADR 26 asymmetry.** The corpus hands the SAME item to both, so
// this cannot pass by treating them alike: a dossier passage keeps its prose
// when every citation failed, and a research finding does not.
func TestAPassageAndAFindingAnswerDifferently(t *testing.T) {
	t.Parallel()
	corpus := loadSourcesCorpus(t)
	sawTheAsymmetry, sawAFindingSurvive := false, false
	for _, row := range corpus.Grounded {
		allowed := allowedSet(row.Allowed)
		if got := Section(row.Item, allowed); !reflect.DeepEqual(got, row.Section) {
			t.Errorf("%s: section\n got    %+v\n corpus %+v", row.Note, got, row.Section)
		}
		kept, dropped := OnlyGrounded([]any{row.Item}, allowed)
		if dropped != row.GroundedDropped {
			t.Errorf("%s: grounded dropped %d, corpus %d", row.Note, dropped, row.GroundedDropped)
		}
		if !reflect.DeepEqual(kept, row.Grounded) {
			t.Errorf("%s: grounded\n got    %+v\n corpus %+v", row.Note, kept, row.Grounded)
		}
		// The case that makes the two modes visibly different.
		if row.Section.Prose != "" && len(row.Section.SourceIDs) == 0 && len(row.Grounded) == 0 {
			sawTheAsymmetry = true
		}
		if len(row.Grounded) > 0 {
			sawAFindingSurvive = true
		}
	}
	if !sawTheAsymmetry {
		t.Fatal("no corpus case has prose the dossier keeps and research drops; " +
			"this test would pass against an implementation that treated them alike")
	}
	// **Both halves, or the guard above is satisfiable by a corpus that
	// exercises neither.** The first version of this corpus gave every item a
	// `prose` key and no `claim`, so `OnlyGrounded` dropped all nine for a
	// missing claim -- the asymmetry check passed, and a mutation deleting the
	// citation predicate survived, because the claim predicate was already
	// doing all the dropping. A test that only ever sees a function return
	// nothing has not tested that function.
	if !sawAFindingSurvive {
		t.Fatal("no corpus case has a finding that SURVIVES; OnlyGrounded is " +
			"returning nothing for every input and this file cannot tell why")
	}
}

// **The second ADR 26 asymmetry, and one accident.** An unresolved rival is a
// card the model invented, so the dossier drops it; an unresolved research card
// may be one spoiled since the last refresh, so research labels it. The
// accident is that a DFC named by its front face resolves in one and not the
// other -- see Competitors.
func TestCompetitorsAndResearchCardsAgreeWithTheCorpus(t *testing.T) {
	t.Parallel()
	corpus := loadSourcesCorpus(t)
	withPool(t, func(c *pool.Conn) {
		for _, row := range corpus.Pool {
			allowed := allowedSet(row.Allowed)
			comp, compDropped, err := Competitors(context.Background(), c,
				[]any{map[string]any{"card": row.Name, "prose": "p",
					"source_ids": []any{"s1"}}}, allowed)
			if err != nil {
				t.Fatalf("%s: %v", row.Note, err)
			}
			if compDropped != row.CompetitorsDropped {
				t.Errorf("%s: competitors dropped %d, corpus %d",
					row.Note, compDropped, row.CompetitorsDropped)
			}
			assertSameJSON(t, row.Note+" / competitors", comp, row.Competitors)

			cards, unresolved, err := ResolveCards(context.Background(), c,
				[]any{row.Name}, MaxResearchCards)
			if err != nil {
				t.Fatalf("%s: %v", row.Note, err)
			}
			if unresolved != row.ResearchUnresolved {
				t.Errorf("%s: research unresolved %d, corpus %d",
					row.Note, unresolved, row.ResearchUnresolved)
			}
			assertSameJSON(t, row.Note+" / research", cards, row.ResearchCards)
		}
	})
}

// assertSameJSON compares as MARSHALLED BYTES, because the key order is the
// contract here and no struct comparison carries it. `Competitor` and
// `ResearchCard` hold the same facts in a DIFFERENT order, and both orders
// are recorded.
func assertSameJSON(t *testing.T, what string, got any, want []json.RawMessage) {
	t.Helper()
	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	var gotRows []json.RawMessage
	if err := json.Unmarshal(raw, &gotRows); err != nil {
		t.Fatal(err)
	}
	if len(gotRows) != len(want) {
		t.Errorf("%s: %d rows, corpus %d\n got    %s", what, len(gotRows), len(want), raw)
		return
	}
	for i := range gotRows {
		var a, b any
		_ = json.Unmarshal(gotRows[i], &a)
		_ = json.Unmarshal(want[i], &b)
		if !reflect.DeepEqual(a, b) {
			t.Errorf("%s row %d:\n got    %s\n corpus %s", what, i, gotRows[i], want[i])
			continue
		}
		// Values agreeing is not enough: the ORDER of the keys is the wire.
		if string(gotRows[i]) != compactJSON(t, want[i]) {
			t.Errorf("%s row %d: key order differs\n got    %s\n corpus %s",
				what, i, gotRows[i], compactJSON(t, want[i]))
		}
	}
}

// compactJSON re-marshals the corpus's own bytes so the two sides are compared
// in one formatting, not two: the corpus is written indented and Go marshals
// compact, and this test is about KEY ORDER rather than whitespace.
func compactJSON(t *testing.T, raw json.RawMessage) string {
	t.Helper()
	out, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

// The three payload shapes, asserted as bytes. `oracle_text` is FIFTH in a
// research card and LAST in a competitor; an unresolved card has exactly TWO
// keys. All three are recorded, none is tidy, and a single shared type would
// break at least two of them.
func TestTheThreeCardShapesKeepTheirOwnKeyOrders(t *testing.T) {
	t.Parallel()
	cost, line, text := "{2}{G}", "Creature", "Trample"
	comp, err := json.Marshal(Competitor{
		Name: "X", Prose: "p", SourceIDs: []string{"s1"},
		ManaCost: &cost, TypeLine: &line, ColorIdentity: []string{"G"},
		LegalCommander: true, OracleText: &text})
	if err != nil {
		t.Fatal(err)
	}
	const wantComp = `{"name":"X","prose":"p","source_ids":["s1"],"mana_cost":"{2}{G}",` +
		`"type_line":"Creature","color_identity":["G"],"image":null,"art_crop":null,` +
		`"legal_commander":true,"oracle_text":"Trample"}`
	if string(comp) != wantComp {
		t.Errorf("competitor:\n go   %s\n want %s", comp, wantComp)
	}

	card, err := json.Marshal(ResearchCard{
		Name: "X", InPool: true, ManaCost: &cost, TypeLine: &line,
		OracleText: &text, ColorIdentity: []string{"G"}, LegalCommander: true})
	if err != nil {
		t.Fatal(err)
	}
	const wantCard = `{"name":"X","in_pool":true,"mana_cost":"{2}{G}","type_line":"Creature",` +
		`"oracle_text":"Trample","color_identity":["G"],"image":null,"art_crop":null,` +
		`"legal_commander":true}`
	if string(card) != wantCard {
		t.Errorf("research card:\n go   %s\n want %s", card, wantCard)
	}

	missing, err := json.Marshal(UnresolvedCard{Name: "Spoiled Card", InPool: false})
	if err != nil {
		t.Fatal(err)
	}
	const wantMissing = `{"name":"Spoiled Card","in_pool":false}`
	if string(missing) != wantMissing {
		t.Errorf("unresolved card:\n go   %s\n want %s -- two keys and no more",
			missing, wantMissing)
	}
}
