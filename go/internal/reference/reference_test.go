package reference

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestTheProseIsWhatThePythonModulesHold(t *testing.T) {
	// The counts a reader of colors.py, glossary.py and lore.py knows, so a
	// truncated render cannot embed quietly.
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

func TestTheServedBytesAreTheCompactedFiles(t *testing.T) {
	// The raw payload is the embedded document with insignificant whitespace
	// removed and nothing else -- so it parses back to the same value, and
	// it is what FastAPI's separators would have written.
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
			t.Fatalf("%s: HTML-escaped; FastAPI never escapes", name)
		}
	}
	// The key order is the route's: `colors` first in the taxonomy, as
	// `service.color_taxonomy` builds it.
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
	m := Deck()
	if len(m.Categories) != 13 || m.Categories[0] != "land" || m.Categories[12] != "utility" {
		t.Fatalf("categories %v", m.Categories)
	}
	if strings.Join(m.DeckStatuses, ",") != "built,theoretical" || strings.Join(m.DeckStages, ",") != "draft,curated" {
		t.Fatalf("%v %v", m.DeckStatuses, m.DeckStages)
	}
	if !IsSingletonExempt("forest") || !IsSingletonExempt("snow-covered wastes") == true || IsSingletonExempt("Forest") {
		// snow-covered wastes does not exist and is not exempt; the
		// lookup is on the lowered name exactly as Python's set is.
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
	sh := Runtime()
	if sh.Symbols.CDN == "" || sh.Symbols.Code != "[0-9A-Z]{1,10}" || sh.Symbols.MaxBytes != 65536 {
		t.Fatalf("symbols %+v", sh.Symbols)
	}
	if len(sh.OCR.Assets) != 4 || sh.OCR.CacheStamp == "" || sh.OCR.Assets["worker.min.js"].Digest == "" {
		t.Fatalf("ocr %+v", sh.OCR)
	}
	if len(sh.Cardmotion.Effects) != 3 || len(sh.Cardmotion.Effects["depth-drift"].Fingerprint) != 16 ||
		len(sh.Cardmotion.Servable) != 4 {
		t.Fatalf("cardmotion %+v", sh.Cardmotion)
	}
}

func TestArchetypeIndexFollowsThePilotedOrder(t *testing.T) {
	if ArchetypeIndex("aggro") != 0 || ArchetypeIndex("combo") != 3 || ArchetypeIndex("cedh") != -1 {
		t.Fatal("ArchetypeIndex is not ARCHETYPES' order")
	}
}
