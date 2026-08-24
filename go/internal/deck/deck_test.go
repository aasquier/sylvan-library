package deck

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// The rich fixture recorded to trip a careless parser: every shape the
// dumper writes, in one committed deck.
func richDeck(t *testing.T) *Deck {
	t.Helper()
	_, here, _, _ := runtime.Caller(0)
	path := filepath.Join(filepath.Dir(here), "..", "deckyaml", "testdata", "rich-deck.yaml")
	text, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	d, err := FromText(string(text), "from-the-directory")
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestFromTextReadsTheRichFixtureAsRecorded(t *testing.T) {
	t.Parallel()
	d := richDeck(t)
	if d.Slug != "rich-fixture" || d.Name != "Rich Fixture: Every Shape the Dumper Writes" {
		t.Fatalf("%q %q", d.Slug, d.Name)
	}
	if d.Status != "built" || d.Stage != "curated" || d.Shared || d.Pilot != "Mark's wife" {
		t.Fatalf("%+v", d)
	}
	if len(d.Commander) != 1 || d.Commander[0] != "Syr Gwyn, Hero of Ashvale" || d.CommanderArt == "" {
		t.Fatalf("commander %v", d.Commander)
	}
	if d.Companion == nil || *d.Companion != "Kaheera, the Orphanguard" || d.Bracket == nil || *d.Bracket != 3 {
		t.Fatalf("companion/bracket")
	}
	if d.LegacyArchetype != "midrange" || len(d.Themes) != 3 || d.Themes[2] != "voltron" {
		t.Fatalf("labels %q %v", d.LegacyArchetype, d.Themes)
	}
	if d.Archetype() != "midrange" { // no class word among the themes: the legacy key answers
		t.Fatalf("archetype %q", d.Archetype())
	}
	if len(d.Notes) != 3 || d.NoteText("weird") != "colon: and # hash, plus braces {G}{W} and a trailing space " {
		t.Fatalf("notes %v", d.Notes)
	}
	// The keys arrive in the file's order. This fixture cannot *prove* that --
	// mulligan, politics, weird is alphabetical, so a map would pass it too --
	// which is why the proof lives in `TestNotesKeepTheFilesOrder`, on a deck
	// whose notes are deliberately out of alphabetical order.
	if keys := []string{d.Notes[0].Key, d.Notes[1].Key, d.Notes[2].Key}; keys[0] != "mulligan" ||
		keys[1] != "politics" || keys[2] != "weird" {
		t.Fatalf("notes out of the file's order: %v", keys)
	}
	if len(d.Cards) != 10 || len(d.SwapBoard) != 1 || len(d.Graveyard) != 1 {
		t.Fatalf("%d %d %d", len(d.Cards), len(d.SwapBoard), len(d.Graveyard))
	}
	forest := d.Cards[3]
	if forest.Name != "Forest" || forest.Qty != 12 || forest.Category != "land" || forest.Why != "* starts with a star, which must be quoted" {
		t.Fatalf("forest %+v", forest)
	}
	if d.Cards[6].Why != "null" || d.Cards[5].Why != "yes" || d.Cards[7].Why != "12" {
		t.Fatal("the look-alikes were not kept as strings")
	}
	if d.Cards[2].Name != "Æther Vial" || d.Cards[2].ManaCost == nil || *d.Cards[2].ManaCost != "{1}" || d.Cards[2].Art == "" { //nolint:misspell // the card is spelled Æther
		t.Fatalf("aether %+v", d.Cards[2])
	}
	if d.TotalCards() != 8+12+13 || d.LandCount() != 25 || len(d.Unjustified()) != 0 {
		t.Fatalf("counts %d %d", d.TotalCards(), d.LandCount())
	}
	names := d.CardNames(true)
	if names[0] != "Syr Gwyn, Hero of Ashvale" || len(names) != 34 {
		t.Fatalf("names %d", len(names))
	}
}

func TestTheDefaultsAreTheModels(t *testing.T) {
	t.Parallel()
	d, err := FromText("name: Bare\ncards:\n  - Sol Ring\n  - name: Forest\n    qty: 2\n", "bare-slug")
	if err != nil {
		t.Fatal(err)
	}
	if d.Slug != "bare-slug" || d.Name != "Bare" || d.Status != "theoretical" || d.Stage != "curated" || !d.Shared {
		t.Fatalf("%+v", d)
	}
	if d.Cards[0].Category != "utility" || d.Cards[0].Qty != 1 || d.Cards[1].Qty != 2 {
		t.Fatalf("cards %+v", d.Cards)
	}
	if d.Companion != nil || d.Bracket != nil || d.Archetype() != "" || len(d.Themes) != 0 || len(d.Notes) != 0 {
		t.Fatalf("%+v", d)
	}
	if d.Strategy != "" {
		t.Fatalf("strategy %v", d.Strategy)
	}
	// An empty document is an empty deck named after its location.
	e, err := FromText("", "empty")
	if err != nil || e.Name != "empty" || len(e.Cards) != 0 {
		t.Fatalf("%v %+v", err, e)
	}
	// A single commander as a string; themes lowered; a null shared is true.
	c, _ := FromText("commander: Gyome, Master Chef\nthemes: [Food, ' COMBO ']\nshared:\nstatus: Built\n", "x")
	if len(c.Commander) != 1 || c.Themes[0] != "food" || c.Themes[1] != "combo" || !c.Shared || c.Status != "built" {
		t.Fatalf("%+v", c)
	}
	if c.Archetype() != "combo" {
		t.Fatalf("archetype %q", c.Archetype())
	}
	// What has no honest parse, this refuses.
	if _, err := FromText("cards:\n  - category: ramp\n", "x"); err == nil {
		t.Fatal("a card with no name was accepted")
	}
	if _, err := FromText("- just\n- a list\n", "x"); err == nil {
		t.Fatal("a document that is not a mapping was accepted")
	}
}

func TestPayloadIsTheDumpsProjection(t *testing.T) {
	t.Parallel()
	d := richDeck(t)
	p := d.Payload()
	if p["shared"] != false || p["pilot"] != "Mark's wife" || p["archetype"] != "midrange" || p["bracket"] != 3 {
		t.Fatalf("payload %v", p)
	}
	if _, has := p["graveyard"]; !has {
		t.Fatal("an occupied graveyard was not written")
	}
	// The projection is the dump's: a curated deck writes no blank why; a
	// draft writes one for every card that lacks it.
	curated, _ := FromText("stage: curated\ncards:\n  - Sol Ring\n", "c")
	draft, _ := FromText("stage: draft\ncards:\n  - Sol Ring\n", "c")
	if _, has := curated.Payload()["cards"].([]map[string]any)[0]["why"]; has {
		t.Fatal("a curated deck wrote a blank why")
	}
	if why, has := draft.Payload()["cards"].([]map[string]any)[0]["why"]; !has || why != "" {
		t.Fatal("a draft did not write its to-do")
	}
	// Shared true is absent; the legacy key disappears once shadowed.
	shadowed, _ := FromText("archetype: midrange\nthemes: [combo]\n", "s")
	if _, has := shadowed.Payload()["archetype"]; has {
		t.Fatal("a shadowed legacy key was written")
	}
	if _, has := shadowed.Payload()["shared"]; has {
		t.Fatal("shared: true was written")
	}
	// Same text twice is the same payload; one changed word is not.
	again := richDeck(t)
	if !d.SameAs(again) {
		t.Fatal("a deck is not the same as itself")
	}
	again.Cards[0].Why = "changed"
	if d.SameAs(again) {
		t.Fatal("a changed rationale read as current")
	}
}
