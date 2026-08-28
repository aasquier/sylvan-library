// Package reference is the checked-in prose as the app serves it: the 32
// colour combinations, the glossary, the lore shelves, the tarot deck's
// facts, and the labelling vocabulary. Nothing here is authored at runtime.
// The JSON under `data/` is the authoritative text, embedded and edited
// directly -- which is what checked-in prose is for: bland prose is fixed
// by editing, never by generating (finite, editable, free).
//
// Two things a reader should know. The raw payloads (`ColorsJSON`,
// `GlossaryJSON`, `ThemesJSON`) are served as they are, compacted once at
// start to the recorded wire shape, so `/api/glossary` is the same bytes
// every boot. And the typed views are for code that has to
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

//go:embed data/coliseum.json
var coliseumFile []byte

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

// Palette is an arena's two colours: the ink its words are set in and the
// glow its motion is lit by. Two rather than a full scheme, because the
// backdrop is a painting somebody else made and the page's job is to agree
// with it, not to repaint it.
type Palette struct {
	Ink  string `json:"ink"`
	Glow string `json:"glow"`
}

// ColiseumFact is one slide of the arena's rotation.
//
// **One struct, five kinds**, because the variety is the point: a wait of
// several minutes shown one shape of card over and over is a wait that reads
// as a loop. `Kind` says which shape a slide is, and the two text fields are
// filled to match:
//
//	roman      Rome's practice and custom      -> Rome
//	gladiator  the fighters themselves         -> Rome
//	coliseum   the building                    -> Rome
//	magic      Magic's own lore and mechanics  -> Magic
//	paired     a Roman fact and its Magic echo -> both
//
// A single struct rather than five, for the reason `GameEvent` is one struct:
// they cross the same wire into the same list, and the alternative is a
// discriminated union on the far side to buy nothing.
//
// All of it is checked-in prose. `Card` is optional and names the card the
// slide is about; like every other name here it resolves through the pool and
// is dropped when it does not.
type ColiseumFact struct {
	Kind  string `json:"kind"`
	Rome  string `json:"rome,omitempty"`
	Magic string `json:"magic,omitempty"`
	Card  string `json:"card,omitempty"`
}

// FactKinds is every kind a [ColiseumFact] may be, which the data is checked
// against at start so a typo in the JSON is a boot failure rather than a slide
// the frontend silently cannot render.
var FactKinds = map[string]bool{
	"roman": true, "gladiator": true, "coliseum": true,
	"magic": true, "paired": true,
}

// ArenaArt is the painting an arena is shown as, chosen rather than looked up.
//
// **A named printing, deliberately.** The pool answers a card name with its
// *newest* printing, and for these six that meant Teenage Mutant Ninja Turtles
// art on the Grand Coliseum and Marvel art on Valor's Reach — crossover
// paintings on a room that is an homage to Magic's own arenas. Naming the
// printing here is the same rule `PageMasthead` follows for every other page's
// art, and it is what lets the painter be credited: this is somebody's
// painting, and it is hotlinked rather than committed (rule 5, ADR 6).
type ArenaArt struct {
	URL      string `json:"url"`
	Artist   string `json:"artist"`
	Printing string `json:"printing"`
}

// Arena is one of the coliseum's six houses: a card whose art is the backdrop,
// the champions who fight there, and the facts it rotates while a match warms
// up.
//
// `Motion` names the animation the backdrop wears (`sand`, `banners`, `stone`,
// `wind`, `oil`, `water`) and is a class name the frontend owns -- the prose
// says which arena moves how, and the stylesheet says what that means.
type Arena struct {
	Key       string         `json:"key"`
	Name      string         `json:"name"`
	Card      string         `json:"card"`
	Plane     string         `json:"plane"`
	Art       ArenaArt       `json:"art"`
	Motion    string         `json:"motion"`
	Palette   Palette        `json:"palette"`
	Champions []Champion     `json:"champions"`
	Facts     []ColiseumFact `json:"facts"`
}

// ZoneArt is the painting one of the board's own zones is dressed in.
//
// **Not an arena and not a card on the table**: the graveyard, exile and the
// command zone are *furniture*, and they were three-letter labels on a 26px
// tile — which is what a scoreboard does and not what a table does. A player
// knows those three places by sight, and the board should say them the way
// Magic says them.
//
// `Card` is the card the painting belongs to and `Art.Printing` pins *which*
// printing, because the pool answers a bare name with the **newest** one and
// the newest is increasingly a crossover: Path to Exile's current default art
// is a Marvel Secret Lair of the Thing, and this project has already put
// Ninja Turtles on the Grand Coliseum once. The URL is checked in beside the
// name for the same reason [ArenaArt]'s is — it is prose about a painting, and
// a painting that changes under a room is a room that changed without anybody
// deciding to.
//
// The painter is named because rule 9 says so, and because somebody painted
// it.
type ZoneArt struct {
	// Key is the zone this dresses: `command`, `graveyard` or `exile`, or
	// `ghost` for the mark that rises when a card reaches a graveyard.
	Key  string   `json:"key"`
	Card string   `json:"card"`
	Art  ArenaArt `json:"art"`
	// Why is the argument for this painting over another, kept beside it so a
	// later session inherits the reasoning rather than the conclusion.
	Why string `json:"why"`
}

// ZoneKeys is every zone the board can dress, checked at boot so a typo in the
// JSON is a boot failure rather than a picture that silently never draws.
var ZoneKeys = map[string]bool{
	"command": true, "graveyard": true, "exile": true, "ghost": true,
}

// Coliseum is `data/coliseum.json`: the six arenas and everything they hold.
type Coliseum struct {
	Note   string    `json:"note"`
	Arenas []Arena   `json:"arenas"`
	Zones  []ZoneArt `json:"zones"`
}

// Creed is a guild speaking for itself: one line of flavour text, printed on
// a real card, with the speaker and the printing it was read off.
//
// **Every word of it was copied out of the card pool, never recalled.**
// Nothing in the pool's flavour text is labelled as one of the ten guilds'
// motto: the one Ravnican line carrying the word belongs to the Azorius
// *Arresters* rather than to the Senate. So what this field can honestly hold
// is the best creed-shaped thing the guild's own leadership actually says,
// and `Card` and `Printing` are here so that claim is checkable rather than
// trusted. Six of the ten come off the guild Charm cycle; Izzet's, Orzhov's
// and Selesnya's Charms were printed with no flavour text at all in any
// printing, and Boros's has a line but Aurelia says a better one elsewhere.
//
// `Card` is **not** resolved through the pool the way `Champions` and
// `Signature` are, and that is deliberate: it is an attribution, not a
// picture to draw, so a fresh clone with no pool still reads the whole
// citation. Nothing about it should ever be rendered as a card.
type Creed struct {
	// Words is the sentence inside the card's quotation marks. The marks
	// themselves are the flavour-text convention rather than part of the
	// line, so the renderer supplies them and the data stays the sentence.
	Words    string `json:"words"`
	Speaker  string `json:"speaker"`
	Card     string `json:"card"`
	Printing string `json:"printing"`
}

// Combination is one of the 32. `Champions` and `Signature` are names; the
// route that serves a combination resolves them through the pool and drops
// and counts what does not resolve, exactly as `service.combination_detail`
// does.
//
// `Creed` is a pointer because most of the 32 do not have one: the ten guilds
// carry it and the other twenty-two serialise `null`, which is a shape a
// renderer can branch on without having to decide whether an empty string
// means "no creed" or "someone left it blank".
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
	Creed      *Creed     `json:"creed"`
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

// Model is the deck model's spoken vocabulary: the categories a card may
// be filed under, the two statuses and two stages, the basics the singleton
// rule exempts, the conventional category targets and the Game Changer
// limits per bracket -- rendered into data so the gate's sentences quote
// one vocabulary rather than carrying a second copy of it.
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
// fingerprint the build wrote into `attribution.json`, computed by the
// cardmotion toolbox when the art is made and not re-derived here.
type Effect struct {
	Fingerprint string `json:"fingerprint"`
	NeedsDepth  bool   `json:"needs_depth"`
}

// RuntimeShelves is the three runtime shelves' configuration: where the
// mana symbols come from and the shape a code may take, the reading
// engine's pinned files and its versioned cache stamp, and the effects
// table with the toolbox's fingerprints.
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
	coliseum   Coliseum
	byKey      = map[string]*Combination{}
	byArena    = map[string]*Arena{}
)

func init() {
	colorsJSON = mustCompact("colors.json", colorsFile, &taxonomy)
	glossaryJSON = mustCompact("glossary.json", glossaryFile, &glossary)
	themesJSON = mustCompact("themes.json", themesFile, &vocabulary)
	mustCompact("lore.json", loreFile, &shelves)
	mustCompact("tarotlore.json", tarotloreFile, &tarot)
	mustCompact("model.json", modelFile, &model)
	mustCompact("shelves.json", shelvesFile, &shelf)
	mustCompact("coliseum.json", coliseumFile, &coliseum)
	for i := range coliseum.Arenas {
		a := &coliseum.Arenas[i]
		if _, dup := byArena[a.Key]; dup {
			panic(fmt.Sprintf("reference: coliseum.json names %q twice", a.Key))
		}
		for _, f := range a.Facts {
			if !FactKinds[f.Kind] {
				panic(fmt.Sprintf("reference: coliseum.json arena %q has a fact of "+
					"unknown kind %q", a.Key, f.Kind))
			}
		}
		byArena[a.Key] = a
	}
	seenZone := map[string]bool{}
	for _, z := range coliseum.Zones {
		if !ZoneKeys[z.Key] {
			panic(fmt.Sprintf("reference: coliseum.json dresses unknown zone %q",
				z.Key))
		}
		if seenZone[z.Key] {
			panic(fmt.Sprintf("reference: coliseum.json dresses zone %q twice",
				z.Key))
		}
		if z.Card == "" || z.Art.URL == "" || z.Art.Artist == "" {
			panic(fmt.Sprintf("reference: coliseum.json zone %q needs a card, a "+
				"painting and a painter", z.Key))
		}
		seenZone[z.Key] = true
	}
	for i := range taxonomy.Combinations {
		c := &taxonomy.Combinations[i]
		if _, dup := byKey[c.Key]; dup {
			panic(fmt.Sprintf("reference: colors.json names %q twice", c.Key))
		}
		byKey[c.Key] = c
	}
}

// mustCompact parses one embedded file into its typed view and returns the
// same document compacted to the recorded wire shape. A file that does not
// parse is a build that must not ship: the data is committed and tested, so
// a panic here means the embedded file itself is damaged, not that a user
// did anything.
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

// Arenas is the coliseum's six, in the order the file lists them.
func Arenas() []Arena { return coliseum.Arenas }

// Zones is the paintings the board's own zones are dressed in.
func Zones() []ZoneArt { return coliseum.Zones }

// ArenaByKey is one arena by its key.
func ArenaByKey(key string) (*Arena, bool) {
	a, ok := byArena[key]
	return a, ok
}

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

// KeyFor is the canonical key for a set of colour codes -- `{"G","W"}` is
// "WG" -- and "C" for none. Anything that is not one of the five letters is
// ignored, never an error: the recorded behaviour, kept.
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
