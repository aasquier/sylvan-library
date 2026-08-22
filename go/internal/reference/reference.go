// Package reference is the checked-in prose as the Go module serves it: the
// 32 colour combinations, the glossary, the lore shelves, the tarot deck's
// facts, and the labelling vocabulary. Nothing here is authored in Go.
// `src/mtglab/reference.py` renders each payload -- exactly what the Python
// route serves today -- `tests/go_fixtures.py` writes them into `data/`, and
// `tests/test_go_fixtures.py` holds the committed files equal to a fresh
// render; this package embeds them, so the two runtimes answer the same words
// or the suite says so. It is the `web_dist` pattern applied to prose
// (docs/go-migration/PLAN.md, Phase 3), and at retirement the JSON here
// becomes the authoritative text, edited directly -- which is what checked-in
// prose is for (`colors.py`: bland prose is fixed by editing).
//
// Two things a reader should know. The raw payloads (`ColorsJSON`,
// `GlossaryJSON`, `ThemesJSON`) are served as they are, compacted once at
// start to the separators FastAPI writes, so `/api/glossary` is the same
// bytes from either door. And the typed views are for Go code that has to
// *read* the prose -- `Combination` for `/api/colors/{key}`'s resolution
// through the pool, the vocabulary for the gate's theme check -- never for
// re-rendering it: a field this package forgot would be missing from the
// served payload only if the served payload were built from the struct, and
// it is not.
package reference

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
)

//go:embed data/colors.json
var colorsFile []byte

//go:embed data/glossary.json
var glossaryFile []byte

//go:embed data/themes.json
var themesFile []byte

//go:embed data/lore.json
var loreFile []byte

//go:embed data/tarotlore.json
var tarotloreFile []byte

//go:embed data/model.json
var modelFile []byte

//go:embed data/shelves.json
var shelvesFile []byte

// Color is one of the five, and what it wants.
type Color struct {
	Code  string `json:"code"`
	Name  string `json:"name"`
	Wants string `json:"wants"`
	Fears string `json:"fears"`
}

// Tier is colourless, mono, guild, shard, wedge, quad or five.
type Tier struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Blurb string `json:"blurb"`
}

// Era is a block that named a set of combinations.
type Era struct {
	Name    string `json:"name"`
	Setting string `json:"setting"`
	Named   string `json:"named"`
	Story   string `json:"story"`
}

// Champion is the face of a faction: a card name the pool resolves, and the
// one editorial sentence attached to it.
type Champion struct {
	Card string `json:"card"`
	Role string `json:"role"`
}

// Combination is one of the 32. `Champions` and `Signature` are names; the
// route that serves a combination resolves them through the pool and drops
// and counts what does not resolve, exactly as `service.combination_detail`
// does.
type Combination struct {
	Key        string     `json:"key"`
	Name       string     `json:"name"`
	Tier       string     `json:"tier"`
	Colors     []string   `json:"colors"`
	Size       int        `json:"size"`
	Tagline    string     `json:"tagline"`
	History    string     `json:"history"`
	Aliases    []string   `json:"aliases"`
	VerifiedBy string     `json:"verified_by"`
	Lore       string     `json:"lore"`
	Champions  []Champion `json:"champions"`
	Signature  []string   `json:"signature"`
}

// Taxonomy is `/api/colors`: the five colours, the seven tiers, the three
// eras and the 32.
type Taxonomy struct {
	Colors       []Color       `json:"colors"`
	Tiers        []Tier        `json:"tiers"`
	Eras         []Era         `json:"eras"`
	Combinations []Combination `json:"combinations"`
}

// Section is one of the glossary's three.
type Section struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Blurb string `json:"blurb"`
}

// Term is one word, defined twice at two lengths.
type Term struct {
	Key     string   `json:"key"`
	Term    string   `json:"term"`
	Short   string   `json:"short"`
	Long    string   `json:"long"`
	Section string   `json:"section"`
	SeeAlso []string `json:"see_also"`
}

// Glossary is `/api/glossary`.
type Glossary struct {
	Sections []Section `json:"sections"`
	Terms    []Term    `json:"terms"`
}

// Vocabulary is `/api/themes`: the open labelling list and the four class
// words in piloted order (ADR 37).
type Vocabulary struct {
	Themes     []string `json:"themes"`
	Archetypes []string `json:"archetypes"`
}

// Volume is one shelf.
type Volume struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Blurb string `json:"blurb"`
}

// Learn is a Learn-page anchor a fact points at.
type Learn struct {
	Tab string `json:"tab"`
	Key string `json:"key"`
}

// Fact is one thing from the shelves. `Cards` are names for the route to
// resolve; the prose reads whole if every one of them drops.
type Fact struct {
	Key    string   `json:"key"`
	Volume string   `json:"volume"`
	Fact   string   `json:"fact"`
	More   string   `json:"more"`
	Cards  []string `json:"cards"`
	Learn  *Learn   `json:"learn"`
}

// Shelves is the lore, minus the resolved cards `/api/lore` adds.
type Shelves struct {
	Volumes []Volume `json:"volumes"`
	Facts   []Fact   `json:"facts"`
}

// TarotFact is one true thing about the 1909 deck, cited by id by the
// fortune-teller (ADR 21). `Card` is empty for the deck tier.
type TarotFact struct {
	ID     string `json:"id"`
	Text   string `json:"text"`
	Source string `json:"source"`
	Card   string `json:"card"`
}

// TarotLore is every fact, deck tier first.
type TarotLore struct {
	Facts []TarotFact `json:"facts"`
}

// Model is the deck model's spoken vocabulary (`decks/model.py`,
// `decks/validate.py`, `decks/analyze.py`): the categories a card may be
// filed under, the two statuses and two stages, the basics the singleton
// rule exempts, the conventional category targets and the Game Changer
// limits per bracket -- rendered so the Go gate says exactly what the Python
// gate says.
type Model struct {
	Categories        []string         `json:"categories"`
	DeckStatuses      []string         `json:"deck_statuses"`
	DeckStages        []string         `json:"deck_stages"`
	SingletonExempt   []string         `json:"singleton_exempt"`
	CategoryTargets   map[string][]int `json:"category_targets"`
	GameChangerLimits map[string]*int  `json:"game_changer_limits"`
}

// OCRAsset is one file of the reading engine, pinned by digest (`ocr.Asset`).
type OCRAsset struct {
	URL       string `json:"url"`
	Digest    string `json:"digest"`
	Size      int64  `json:"size"`
	MediaType string `json:"media_type"`
}

// Effect is one card-art effect as the serving tier needs it: the
// fingerprint the build wrote into `attribution.json`, computed by Python
// (`cardmotion/effects.py:Effect.fingerprint`) and not re-derived here.
type Effect struct {
	Fingerprint string `json:"fingerprint"`
	NeedsDepth  bool   `json:"needs_depth"`
}

// RuntimeShelves is the three runtime shelves' configuration (`symbols.py`, `ocr.py`,
// `cardmotion/cache.py` + `effects.py`): where the mana symbols come from
// and the shape a code may take, the reading engine's pinned files and its
// versioned cache stamp, and the effects table with Python's fingerprints.
type RuntimeShelves struct {
	Symbols struct {
		CDN      string `json:"cdn"`
		Code     string `json:"code"`
		MaxBytes int64  `json:"max_bytes"`
	} `json:"symbols"`
	OCR struct {
		CacheStamp string              `json:"cache_stamp"`
		MaxBytes   int64               `json:"max_bytes"`
		Assets     map[string]OCRAsset `json:"assets"`
	} `json:"ocr"`
	Cardmotion struct {
		Servable []string          `json:"servable"`
		Effects  map[string]Effect `json:"effects"`
	} `json:"cardmotion"`
}

var (
	colorsJSON, glossaryJSON, themesJSON []byte

	taxonomy   Taxonomy
	glossary   Glossary
	vocabulary Vocabulary
	shelves    Shelves
	tarot      TarotLore
	model      Model
	shelf      RuntimeShelves
	byKey      = map[string]*Combination{}
)

func init() {
	colorsJSON = mustCompact("colors.json", colorsFile, &taxonomy)
	glossaryJSON = mustCompact("glossary.json", glossaryFile, &glossary)
	themesJSON = mustCompact("themes.json", themesFile, &vocabulary)
	mustCompact("lore.json", loreFile, &shelves)
	mustCompact("tarotlore.json", tarotloreFile, &tarot)
	mustCompact("model.json", modelFile, &model)
	mustCompact("shelves.json", shelvesFile, &shelf)
	for i := range taxonomy.Combinations {
		c := &taxonomy.Combinations[i]
		if _, dup := byKey[c.Key]; dup {
			panic(fmt.Sprintf("reference: colors.json names %q twice", c.Key))
		}
		byKey[c.Key] = c
	}
}

// mustCompact parses one embedded file into its typed view and returns the
// same document compacted to FastAPI's separators. A file that does not
// parse is a build that must not ship: the data is generated and tested on
// the Python side, so a panic here means the embed and the generator have
// come apart, not that a user did anything.
func mustCompact(name string, raw []byte, into any) []byte {
	if err := json.Unmarshal(raw, into); err != nil {
		panic(fmt.Sprintf("reference: %s does not parse: %v", name, err))
	}
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err != nil {
		panic(fmt.Sprintf("reference: %s will not compact: %v", name, err))
	}
	return buf.Bytes()
}

// ColorsJSON is `/api/colors`' body, byte for byte.
func ColorsJSON() []byte { return colorsJSON }

// GlossaryJSON is `/api/glossary`' body, byte for byte.
func GlossaryJSON() []byte { return glossaryJSON }

// ThemesJSON is `/api/themes`' body, byte for byte.
func ThemesJSON() []byte { return themesJSON }

// Colors is the taxonomy, typed. Read-only: callers share one copy.
func Colors() *Taxonomy { return &taxonomy }

// Words is the glossary, typed. Read-only.
func Words() *Glossary { return &glossary }

// Themes is the labelling vocabulary. Read-only.
func Themes() *Vocabulary { return &vocabulary }

// Lore is the shelves, typed. Read-only.
func Lore() *Shelves { return &shelves }

// Tarot is the fortune-teller's corpus, typed. Read-only.
func Tarot() *TarotLore { return &tarot }

// Deck is the deck model's vocabulary, typed. Read-only.
func Deck() *Model { return &model }

// Runtime is the runtime shelves' configuration. Read-only.
func Runtime() *RuntimeShelves { return &shelf }

// IsCategory is `category in model.CATEGORIES`.
func IsCategory(word string) bool {
	for _, c := range model.Categories {
		if c == word {
			return true
		}
	}
	return false
}

// IsSingletonExempt is `name.lower() in validate.SINGLETON_EXEMPT`.
func IsSingletonExempt(lowered string) bool {
	for _, n := range model.SingletonExempt {
		if n == lowered {
			return true
		}
	}
	return false
}

// CombinationByKey is `colors.BY_KEY`: the slot for a canonical key ("WG",
// "C"), or false.
func CombinationByKey(key string) (*Combination, bool) {
	c, ok := byKey[key]
	return c, ok
}

// WUBRG is the canonical order every key is written in.
const WUBRG = "WUBRG"

// KeyFor is `colors.key_for`: the canonical key for a set of colour codes --
// `{"G","W"}` is "WG" -- and "C" for none. Anything that is not one of the
// five letters is ignored, as the Python set comprehension ignores it.
func KeyFor(codes []string) string {
	have := map[string]bool{}
	for _, c := range codes {
		have[c] = true
	}
	var b strings.Builder
	for _, c := range WUBRG {
		if have[string(c)] {
			b.WriteRune(c)
		}
	}
	if b.Len() == 0 {
		return "C"
	}
	return b.String()
}

// IsTheme is `theme in model.THEMES`.
func IsTheme(word string) bool {
	for _, t := range vocabulary.Themes {
		if t == word {
			return true
		}
	}
	return false
}

// ArchetypeIndex is the position of a class word in `ARCHETYPES`' piloted
// order, or -1 for a word that is not one. ADR 37's reading picks the
// largest index among the declared themes -- worst-piloted wins.
func ArchetypeIndex(word string) int {
	for i, a := range vocabulary.Archetypes {
		if a == word {
			return i
		}
	}
	return -1
}
