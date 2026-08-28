package reference

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestTheProseHoldsItsRecordedShape(t *testing.T) {
	t.Parallel()
	// The counts the embedded files have always held, so a truncated file
	// cannot embed quietly.
	if n := len(Colors().Combinations); n != 32 {
		t.Fatalf("%d combinations, want 32 (one colourless, five mono, ten pairs, ten triples, five quads, one five)", n)
	}
	if n := len(Colors().Colors); n != 5 {
		t.Fatalf("%d colours", n)
	}
	if n := len(Colors().Tiers); n != 7 {
		t.Fatalf("%d tiers", n)
	}
	if n := len(Colors().Eras); n != 3 {
		t.Fatalf("%d eras", n)
	}
	if n := len(Words().Sections); n != 3 {
		t.Fatalf("%d glossary sections", n)
	}
	if len(Words().Terms) < 50 || len(Lore().Facts) < 30 || len(Tarot().Facts) < 300 {
		t.Fatalf("thin: %d terms, %d facts, %d tarot facts", len(Words().Terms), len(Lore().Facts), len(Tarot().Facts))
	}
	if n := len(Lore().Volumes); n != 5 {
		t.Fatalf("%d volumes", n)
	}
	if got := Themes().Archetypes; strings.Join(got, ",") != "aggro,midrange,control,combo" {
		t.Fatalf("archetypes %v are not in piloted order", got)
	}
	for _, a := range Themes().Archetypes {
		if !IsTheme(a) {
			t.Fatalf("class word %q is not in the vocabulary (ADR 37: the class words are themes)", a)
		}
	}
}

func TestEveryCombinationIsAddressable(t *testing.T) {
	t.Parallel()
	for _, c := range Colors().Combinations {
		if KeyFor(c.Colors) != c.Key {
			t.Errorf("%s: KeyFor(%v) = %q", c.Name, c.Colors, KeyFor(c.Colors))
		}
		if c.Size != len(c.Colors) {
			t.Errorf("%s: size %d, %d colours", c.Name, c.Size, len(c.Colors))
		}
		if got, ok := CombinationByKey(c.Key); !ok || got.Name != c.Name {
			t.Errorf("%s not found by key %q", c.Name, c.Key)
		}
		// Never nil: the served payload writes [] for an empty list, and a
		// typed reader must see the same absence of champions as a list.
		if c.Champions == nil || c.Signature == nil || c.Aliases == nil || c.Colors == nil {
			t.Errorf("%s: a list decoded as nil", c.Name)
		}
	}
	if KeyFor(nil) != "C" || KeyFor([]string{"G", "W"}) != "WG" || KeyFor([]string{"W", "U", "B", "R", "G"}) != "WUBRG" {
		t.Fatal("KeyFor is not canonical")
	}
	if KeyFor([]string{"X", "G"}) != "G" {
		t.Fatal("KeyFor let a non-colour through")
	}
	if _, ok := CombinationByKey("nope"); ok {
		t.Fatal("an unknown key resolved")
	}
}

// The creeds are checked-in prose about real cards, so what a test can hold
// is the shape of the claim rather than its truth: ten guilds, ten creeds,
// nobody else, and every one of them citing something. The words themselves
// were copied out of the card pool (`printings.flavor_text`) and the citation
// beside each is how a later session re-checks one without trusting this file.
func TestEveryGuildSaysItsPieceAndNobodyElseDoes(t *testing.T) {
	t.Parallel()
	guilds := 0
	for _, c := range Colors().Combinations {
		if c.Tier != "guild" {
			if c.Creed != nil {
				t.Errorf("%s is a %s and has a creed; the field is the ten guilds'", c.Name, c.Tier)
			}
			continue
		}
		guilds++
		if c.Creed == nil {
			t.Errorf("%s has no creed", c.Name)
			continue
		}
		// All four or none. A creed with no citation is exactly the thing
		// this field exists to refuse -- a line somebody remembered.
		if c.Creed.Words == "" || c.Creed.Speaker == "" || c.Creed.Card == "" || c.Creed.Printing == "" {
			t.Errorf("%s: creed %+v is missing a part", c.Name, *c.Creed)
		}
		// The renderer supplies the quotation marks, so the stored line is
		// the sentence and never the flavour text's own punctuation around
		// it -- and it is one line, because the attribution is a field.
		if strings.ContainsAny(c.Creed.Words, "\"\n") {
			t.Errorf("%s: creed carries the card's quoting rather than the sentence: %q", c.Name, c.Creed.Words)
		}
		if strings.HasPrefix(c.Creed.Speaker, "—") {
			t.Errorf("%s: speaker keeps the card's dash: %q", c.Name, c.Creed.Speaker)
		}
	}
	if guilds != 10 {
		t.Fatalf("%d guilds, want 10", guilds)
	}
	// And the key is on the wire for all 32, not only the ten that fill it.
	// `/api/colors` serves this file's own bytes while `/api/colors/{key}`
	// marshals the struct, so a key present on only ten entries makes the two
	// routes disagree about whether a combination even has the field -- which
	// a typed reader of both cannot describe. The file already writes
	// `"aliases":[]` and `"lore":""` everywhere for the same reason.
	if n := strings.Count(string(ColorsJSON()), `"creed":`); n != 32 {
		t.Fatalf(`%d combinations carry "creed" on the wire, want all 32`, n)
	}
}

func TestTheServedBytesAreTheCompactedFiles(t *testing.T) {
	t.Parallel()
	// The raw payload is the embedded document with insignificant
	// whitespace removed and nothing else -- so it parses back to the same
	// value, and the bytes are the recorded wire shape.
	for name, raw := range map[string][]byte{"colors": ColorsJSON(), "glossary": GlossaryJSON(), "themes": ThemesJSON()} {
		if !json.Valid(raw) {
			t.Fatalf("%s: not JSON", name)
		}
		// Compact JSON has no raw newline (one inside a string is `\n`),
		// and compacting it again changes nothing.
		var again bytes.Buffer
		if err := json.Compact(&again, raw); err != nil || !bytes.Equal(again.Bytes(), raw) ||
			strings.Contains(string(raw), "\n") {
			t.Fatalf("%s: not compact", name)
		}
		if strings.Contains(string(raw), `<`) {
			t.Fatalf("%s: HTML-escaped; the wire never escapes", name)
		}
	}
	// The key order is the route's: `colors` first in the taxonomy, as the
	// recorded payload opens.
	if !strings.HasPrefix(string(ColorsJSON()), `{"colors":[{"code":"W"`) {
		t.Fatalf("colors.json does not open as the route does: %.60s", ColorsJSON())
	}
	if !strings.HasPrefix(string(ThemesJSON()), `{"themes":["aggro"`) {
		t.Fatalf("themes.json: %.60s", ThemesJSON())
	}
	if !strings.HasPrefix(string(GlossaryJSON()), `{"sections":[`) {
		t.Fatalf("glossary.json: %.60s", GlossaryJSON())
	}
}

func TestTheModelVocabularyIsWhatTheGateSpeaks(t *testing.T) {
	t.Parallel()
	m := Deck()
	if len(m.Categories) != 13 || m.Categories[0] != "land" || m.Categories[12] != "utility" {
		t.Fatalf("categories %v", m.Categories)
	}
	if strings.Join(m.DeckStatuses, ",") != "built,theoretical" || strings.Join(m.DeckStages, ",") != "draft,curated" {
		t.Fatalf("%v %v", m.DeckStatuses, m.DeckStages)
	}
	if !IsSingletonExempt("forest") || !IsSingletonExempt("snow-covered wastes") == true || IsSingletonExempt("Forest") {
		// snow-covered wastes does not exist and is not exempt; the
		// lookup is on the lowered name, exactly as the vocabulary is
		// stored.
		if !IsSingletonExempt("forest") || IsSingletonExempt("Forest") {
			t.Fatal("singleton exemptions")
		}
	}
	if !IsCategory("ramp") || IsCategory("hate") {
		t.Fatal("categories")
	}
	if tgt := m.CategoryTargets["land"]; len(tgt) != 2 || tgt[0] != 33 || tgt[1] != 38 {
		t.Fatalf("land target %v", tgt)
	}
	if m.GameChangerLimits["3"] == nil || *m.GameChangerLimits["3"] != 3 || m.GameChangerLimits["4"] != nil {
		t.Fatalf("game changer limits %v", m.GameChangerLimits)
	}
}

func TestTheShelvesAreWhatTheModulesHold(t *testing.T) {
	t.Parallel()
	sh := Runtime()
	if sh.Symbols.CDN == "" || sh.Symbols.Code != "[0-9A-Z]{1,10}" || sh.Symbols.MaxBytes != 65536 {
		t.Fatalf("symbols %+v", sh.Symbols)
	}
	if len(sh.OCR.Assets) != 4 || sh.OCR.CacheStamp == "" || sh.OCR.Assets["worker.min.js"].Digest == "" {
		t.Fatalf("ocr %+v", sh.OCR)
	}
	// **A property, not a tally.** This read `len(Effects) != 3` and broke the
	// day a fourth was added, which is the only day a count like that ever
	// does anything — and what it does is say "something changed", which the
	// diff already said. What actually has to hold is that every effect is
	// keyed by a fingerprint the shelf can find it by: sixteen hex characters,
	// and no two the same, because two effects sharing one would serve each
	// other's derivatives.
	if len(sh.Cardmotion.Effects) == 0 || len(sh.Cardmotion.Servable) != 4 {
		t.Fatalf("cardmotion %+v", sh.Cardmotion)
	}
	seen := map[string]string{}
	for name, effect := range sh.Cardmotion.Effects {
		if len(effect.Fingerprint) != 16 {
			t.Errorf("effect %q has fingerprint %q, want sixteen characters",
				name, effect.Fingerprint)
		}
		if other, clash := seen[effect.Fingerprint]; clash {
			t.Errorf("effects %q and %q share fingerprint %q, so each would "+
				"find the other's derivatives", name, other, effect.Fingerprint)
		}
		seen[effect.Fingerprint] = name
	}
	if _, ok := sh.Cardmotion.Effects["depth-drift"]; !ok {
		t.Error("depth-drift is gone; ADR 32's own effect is the anti-vacuity " +
			"check that this is reading the real file")
	}
}

func TestArchetypeIndexFollowsThePilotedOrder(t *testing.T) {
	t.Parallel()
	if ArchetypeIndex("aggro") != 0 || ArchetypeIndex("combo") != 3 || ArchetypeIndex("cedh") != -1 {
		t.Fatal("ArchetypeIndex is not ARCHETYPES' order")
	}
}
