package deck

import (
	"reflect"
	"strings"
	"testing"
)

// The combos block through the model: parsed, projected, dumped, and read back.
//
// `testdata/dumps.json` is a frozen golden and no combos entry is going into
// it, so the block's bytes are held here instead -- against a round trip and
// against the file this is meant to write, rather than against a recorded
// corpus nobody may regenerate.
//
// The fixture is written in the shape `Dump` produces, blank lines and all,
// which is what lets the round trip be a byte comparison. `Dump` writes the
// blocks back to back; the blank line before a block is the *surgical* editor's
// doing, and `deckedit`'s own tests hold that half.

const catalogue = `slug: walls
name: Walls
status: theoretical
stage: curated
commander:
  - Arcades, the Strategist
cards:
  - name: Axebane Guardian
    category: ramp
    why: Taps for one per defender.
  - name: High Alert
    category: engine
    why: The walls swing.
combos:
  - cards:
      - Axebane Guardian
      - High Alert
    produces: infinite colored mana; infinite untaps of your creatures
    how: 1) Tap Axebane Guardian for X. 2) Pay {2}{W}{U} to untap it. 3) Repeat.
    setup: six mana across two quiet turns, then the fifth defender
  - cards:
      - Axebane Guardian
    needs: Umbral Mantle
    produces: infinite colored mana
    how: 1) Equip. 2) Untap for {3}. 3) Repeat.
    setup: four mana and the Guardian free of summoning sickness
    cut: Suspicious Bookcase
    by: claude
`

func read(t *testing.T, text string) *Deck {
	t.Helper()
	d, err := FromText(text, "walls")
	if err != nil {
		t.Fatalf("reading the deck: %v", err)
	}
	return d
}

// Parse, dump, parse: the bytes come back and so does every field.
func TestACatalogueSurvivesTheRoundTrip(t *testing.T) {
	t.Parallel()
	d := read(t, catalogue)
	if len(d.Combos) != 2 {
		t.Fatalf("read %d combos, expected 2", len(d.Combos))
	}

	text, err := d.Dump()
	if err != nil {
		t.Fatalf("dumping: %v", err)
	}
	if text != catalogue {
		t.Errorf("the dump is not the file it read:\n--- was ---\n%s\n--- now ---\n%s",
			catalogue, text)
	}
	if again := read(t, text); !reflect.DeepEqual(again.Combos, d.Combos) {
		t.Errorf("a second parse disagrees:\n%+v\n%+v", again.Combos, d.Combos)
	}
}

// The heading is the cards, joined, and a near-miss knows it is one. Both are
// derived rather than stored, which is what stops either disagreeing with the
// entry it describes.
func TestAComboIsNamedAfterItsPieces(t *testing.T) {
	t.Parallel()
	d := read(t, catalogue)
	if got := d.Combos[0].Heading(); got != "Axebane Guardian + High Alert" {
		t.Errorf("the heading is %q", got)
	}
	if d.Combos[0].NearMiss() {
		t.Error("a complete machine reads as a near-miss")
	}
	if !d.Combos[1].NearMiss() || d.Combos[1].Cut != "Suspicious Bookcase" {
		t.Errorf("the near-miss lost its trade: %+v", d.Combos[1])
	}
	// The provenance mark rides through the parse; nothing in this phase
	// writes one, and a file that carries one keeps it.
	if d.Combos[0].By != "" || d.Combos[1].By != "claude" {
		t.Errorf("the mark did not land where the file put it: %+v", d.Combos)
	}
}

// **`Payload` is what `swaps.md` diffs against.** A block missing from the
// projection is a block whose changes report the deck as unchanged -- catalogue
// three machines, run a build, and the swap record says nothing happened.
func TestACataloguedDeckIsNotTheSameAsAnUncataloguedOne(t *testing.T) {
	t.Parallel()
	full := read(t, catalogue)
	bare := read(t, catalogue[:strings.Index(catalogue, "\ncombos:")]+"\n")

	if bare.SameAs(full) {
		t.Error("adding a combos block left the deck reading as unchanged")
	}
	if _, present := bare.Payload()["combos"]; present {
		t.Error("a deck with no combos asserts an empty block in its payload")
	}

	// And every field of an entry moves the projection, one at a time --
	// otherwise a build after somebody rewrote one setup line would report
	// nothing.
	for _, tc := range []struct {
		name  string
		apply func(*Combo)
	}{
		{"a piece", func(c *Combo) { c.Cards = append(c.Cards, "Wall of Omens") }},
		{"what it produces", func(c *Combo) { c.Produces += " and a draw" }},
		{"the instructions", func(c *Combo) { c.How += " 4) Win." }},
		{"the setup", func(c *Combo) { c.Setup = "five mana" }},
		{"the card it needs", func(c *Combo) { c.Needs = "Freed from the Real" }},
		{"the cut", func(c *Combo) { c.Cut = "Wall of Omens" }},
		{"the mark", func(c *Combo) { c.By = "claude" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			moved := read(t, catalogue)
			tc.apply(&moved.Combos[0])
			if moved.SameAs(full) {
				t.Errorf("changing %s left the deck reading as unchanged", tc.name)
			}
		})
	}
}

// A deck that catalogues nothing writes no block, like the swap board and the
// graveyard beside it. Six curated files should not each grow a `combos: []`
// line asserting the shelf they do not keep.
func TestAnEmptyCatalogueWritesNothing(t *testing.T) {
	t.Parallel()
	d := read(t, catalogue)
	d.Combos = nil
	text, err := d.Dump()
	if err != nil {
		t.Fatalf("dumping: %v", err)
	}
	if strings.Contains(text, "combos") {
		t.Errorf("an empty catalogue was asserted:\n%s", text)
	}
	// And it is last in the file when there is one, after everything the deck
	// is made of -- so cataloguing the first machine appends rather than
	// pushing the 99 down.
	full := read(t, catalogue)
	written, err := full.Dump()
	if err != nil {
		t.Fatalf("dumping: %v", err)
	}
	if at := strings.Index(written, "\ncombos:"); at < strings.Index(written, "\ncards:") {
		t.Errorf("the block is not below the cards:\n%s", written)
	}
}

// `ComboNames` is the one list behind three questions -- does the pool know
// this name, is it in the 99, what picture does it get -- and a name is a name
// wherever it stands in the entry.
func TestEveryNameACatalogueMentionsIsCollectedOnce(t *testing.T) {
	t.Parallel()
	d := read(t, catalogue)
	got := d.ComboNames()
	want := []string{"Axebane Guardian", "High Alert", "Umbral Mantle",
		"Suspicious Bookcase"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("collected %v, want %v", got, want)
	}

	// Deduplicated case-insensitively: a piece shared by three machines is one
	// spelling to look up, not three.
	d.Combos = append(d.Combos, Combo{Cards: []string{"axebane guardian"},
		Produces: "the same mana again"})
	if again := d.ComboNames(); !reflect.DeepEqual(again, want) {
		t.Errorf("a second mention was collected twice: %v", again)
	}
}

// A hand-written file is what the parser has to survive, and these are the
// shapes a person types rather than the ones the app writes.
func TestAHandWrittenBlockIsReadForgivingly(t *testing.T) {
	t.Parallel()
	loose := `slug: walls
name: Walls
commander:
  - Arcades, the Strategist
cards:
  - name: Axebane Guardian
    category: ramp
    why: Taps.

combos:
  # one card is still a list of pieces
  - cards: Axebane Guardian
    produces:   infinite mana
  - cards:
      - High Alert
      -
      - '  Wall of Omens  '
    produces: walls that swing
`
	d := read(t, loose)
	if len(d.Combos) != 2 {
		t.Fatalf("read %d combos, expected 2", len(d.Combos))
	}
	if got := d.Combos[0].Heading(); got != "Axebane Guardian" {
		t.Errorf("a bare string was not read as one piece: %q", got)
	}
	if d.Combos[0].Produces != "infinite mana" {
		t.Errorf("the surrounding space survived: %q", d.Combos[0].Produces)
	}
	// A blank entry in the list is dropped rather than becoming a card named
	// "" that the gate then warns about, and a padded one is trimmed.
	if got := d.Combos[1].Heading(); got != "High Alert + Wall of Omens" {
		t.Errorf("the loose list read as %q", got)
	}
}

// A block that is not a list of mappings is an error rather than something
// plausible: guessing what a bare sentence meant would file somebody's prose
// as a card name.
func TestAComboThatIsNotAMappingIsRefused(t *testing.T) {
	t.Parallel()
	_, err := FromText("slug: x\ncombos:\n  - Axebane Guardian + High Alert\n", "x")
	if err == nil {
		t.Fatal("a bare string was read as a combo")
	}
	if !strings.Contains(err.Error(), "combos[0]") {
		t.Errorf("the error does not say which entry: %v", err)
	}
}
