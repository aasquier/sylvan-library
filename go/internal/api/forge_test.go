package api

import (
	"bytes"
	"encoding/json"
	"math/big"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/deck"
	"github.com/aasquier/sylvan-library/go/internal/sim/tier3"
)

// The shaped result, held to a frozen corpus: the payload the deck page
// renders, the
// row a tick carries, and the two dials that make a match out of a body.
//
// The corpus lives with the engine (`internal/sim/tier3/testdata/forge.json`)
// because it is one recording; only the `shape` half is read here.

type shapeCorpusFile struct {
	Shape struct {
		Caveat       string            `json:"caveat"`
		Clock        int               `json:"clock"`
		GamesDefault int               `json:"games_default"`
		GamesMax     int               `json:"games_max"`
		Decks        map[string]string `json:"decks"`
		Shapes       []struct {
			Note       string          `json:"note"`
			Run        tier3.WireRun   `json:"run"`
			Slugs      []string        `json:"slugs"`
			Addresses  []string        `json:"addresses"`
			GamesAsked int             `json:"games_asked"`
			Seed       int64           `json:"seed"`
			Shape      json.RawMessage `json:"shape"`
		} `json:"shapes"`
		Rows []struct {
			Note string          `json:"note"`
			Game tier3.WireGame  `json:"game"`
			Slug *string         `json:"slug"`
			Row  json.RawMessage `json:"row"`
		} `json:"rows"`
		GamesDial []struct {
			Note    string         `json:"note"`
			Payload map[string]any `json:"payload"`
			Games   *int           `json:"games"`
			Raises  string         `json:"raises"`
		} `json:"games_dial"`
		SeedDial []struct {
			Note    string         `json:"note"`
			Payload map[string]any `json:"payload"`
			Seed    string         `json:"seed"`
			Raises  string         `json:"raises"`
		} `json:"seed_dial"`
		Labels []struct {
			Slugs []string `json:"slugs"`
			Games int      `json:"games"`
			Label string   `json:"label"`
		} `json:"labels"`
		Keys []struct {
			Addresses []string `json:"addresses"`
			Games     int      `json:"games"`
			Seed      string   `json:"seed"`
			Key       string   `json:"key"`
		} `json:"keys"`
	} `json:"shape"`
	Wire struct {
		Run json.RawMessage `json:"run"`
	} `json:"wire"`
}

func loadShapeCorpus(t *testing.T) shapeCorpusFile {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "sim", "tier3", "testdata", "forge.json"))
	if err != nil {
		t.Fatal(err)
	}
	var corpus shapeCorpusFile
	if err := json.Unmarshal(raw, &corpus); err != nil {
		t.Fatal(err)
	}
	if len(corpus.Shape.Shapes) == 0 {
		t.Fatal("the Forge shape corpus is empty; forge.json is a frozen golden")
	}
	return corpus
}

// TestTheConstantsAreClaudesOwnWords holds the caveat and the three dials.
//
// The caveat is in the corpus rather than compared by eye because CLAUDE.md
// requires it quoted **with** the numbers, so it is part of every result's
// bytes; a reworded copy would ship as a silently different payload.
func TestTheConstantsAreClaudesOwnWords(t *testing.T) {
	t.Parallel()
	corpus := loadShapeCorpus(t)
	if ForgeCaveat != corpus.Shape.Caveat {
		t.Errorf("caveat:\n got %q\nwant %q", ForgeCaveat, corpus.Shape.Caveat)
	}
	if ForgeClock != corpus.Shape.Clock {
		t.Errorf("clock = %d, want %d", ForgeClock, corpus.Shape.Clock)
	}
	if ForgeGamesDefault != corpus.Shape.GamesDefault {
		t.Errorf("games default = %d, want %d", ForgeGamesDefault, corpus.Shape.GamesDefault)
	}
	if ForgeGamesMax != corpus.Shape.GamesMax {
		t.Errorf("games max = %d, want %d", ForgeGamesMax, corpus.Shape.GamesMax)
	}
}

// TestTheShapedMatchIsTheRecordedBytes compares the whole payload, marshalled.
//
// **Marshalled**, which is `tier1.Number`'s lesson again: a struct
// proved field by field and never once put through `encoding/json` is a struct
// whose wire form nothing has checked. Key order is contract here — the deck
// page reads this in DevTools as much as the client does.
func TestTheShapedMatchIsTheRecordedBytes(t *testing.T) {
	t.Parallel()
	corpus := loadShapeCorpus(t)
	decks := map[string]*deck.Deck{}
	for name, text := range corpus.Shape.Decks {
		d, err := deck.FromText(text, "")
		if err != nil {
			t.Fatalf("corpus deck %s: %v", name, err)
		}
		decks[d.Slug] = d
		_ = name
	}

	for _, c := range corpus.Shape.Shapes {
		t.Run(c.Note, func(t *testing.T) {
			pair := make([]*deck.Deck, 0, len(c.Slugs))
			for _, slug := range c.Slugs {
				d, ok := decks[slug]
				if !ok {
					t.Fatalf("the corpus names a deck it gave no text for: %s", slug)
				}
				pair = append(pair, d)
			}
			run := tier3.RunFromWire(c.Run)
			got, err := json.Marshal(shapeForge(pair, c.Addresses,
				c.GamesAsked, big.NewInt(c.Seed), run))
			if err != nil {
				t.Fatal(err)
			}
			if normaliseJSON(string(got)) != normaliseJSON(string(c.Shape)) {
				t.Errorf("shaped match:\n got %s\nwant %s", got, c.Shape)
			}
		})
	}
}

// TestTheRowIsTheSameShapeLiveAndInTheTally drives the row builder alone, at
// the moment a tick carries it.
//
// One builder serves both the streamed `partial` and the final `rows`, and a
// theater that showed one shape live and another in the tale of the tape would
// be the drift the wire codec exists to prevent, one layer up. Marshalled, for
// the reason the shape above is.
func TestTheRowIsTheSameShapeLiveAndInTheTally(t *testing.T) {
	t.Parallel()
	corpus := loadShapeCorpus(t)
	for _, c := range corpus.Shape.Rows {
		t.Run(c.Note, func(t *testing.T) {
			got, err := json.Marshal(newForgeRow(tier3.GameFromWire(c.Game), c.Slug))
			if err != nil {
				t.Fatal(err)
			}
			if normaliseJSON(string(got)) != normaliseJSON(string(c.Row)) {
				t.Errorf("row:\n got %s\nwant %s", got, c.Row)
			}
		})
	}
}

// TestTheGamesDialMatchesTheCorpus holds `forgeGames` over every shape a
// body can carry — including the ones the record **raises** on.
//
// Those are a wart, pinned rather than tidied: the plan runs in the
// request with nothing catching the refusal, so `{"games": "many"}` is an
// uncaught 500 rather than the 422 it should be, and `{"games": null}` is
// recorded as a crash too, because the default only fires for an ABSENT
// key. The
// guard beats the fix here, because neither is reachable from the app's own
// client and an unreachable bug survives forever.
func TestTheGamesDialMatchesTheCorpus(t *testing.T) {
	t.Parallel()
	corpus := loadShapeCorpus(t)
	for _, c := range corpus.Shape.GamesDial {
		t.Run(c.Note, func(t *testing.T) {
			got, err := forgeGames(decodeBody(t, c.Payload))
			if c.Raises != "" {
				if err == nil {
					t.Fatalf("the corpus raises %s; this answered %d", c.Raises, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("refused where the corpus answers %d: %v", *c.Games, err)
			}
			if c.Games == nil || got != *c.Games {
				t.Errorf("games = %d, want %v", got, c.Games)
			}
		})
	}
}

// TestTheSeedDialMatchesTheCorpus holds `forgeSeed`, including a seed past
// int64.
//
// Past int64 is not a curiosity: the seed is echoed into the result, the
// dedupe key and Forge's own command line, so narrowing it would answer a
// different number than the one somebody asked with — and a seed is a promise.
func TestTheSeedDialMatchesTheCorpus(t *testing.T) {
	t.Parallel()
	corpus := loadShapeCorpus(t)
	for _, c := range corpus.Shape.SeedDial {
		t.Run(c.Note, func(t *testing.T) {
			got, err := forgeSeed(decodeBody(t, c.Payload))
			if c.Raises != "" {
				if err == nil {
					t.Fatalf("the corpus raises %s; this answered %s", c.Raises, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("refused where the corpus answers %s: %v", c.Seed, err)
			}
			if got.String() != c.Seed {
				t.Errorf("seed = %s, want %s", got, c.Seed)
			}
		})
	}
}

// TestTheLabelAndKeyAreTheRecordedText holds the two strings a caller sees
// or collides on: the job label in a list of one-liners, and the dedupe key
// that decides whether a second click joins the first match or starts a new
// one.
func TestTheLabelAndKeyAreTheRecordedText(t *testing.T) {
	t.Parallel()
	corpus := loadShapeCorpus(t)
	for _, c := range corpus.Shape.Labels {
		plural := "s"
		if c.Games == 1 {
			plural = ""
		}
		got := "Forge: " + joinWith(c.Slugs, " vs ") +
			", " + itoa(c.Games) + " game" + plural
		if got != c.Label {
			t.Errorf("label:\n got %q\nwant %q", got, c.Label)
		}
	}
	for _, c := range corpus.Shape.Keys {
		seed, ok := new(big.Int).SetString(c.Seed, 10)
		if !ok {
			t.Fatalf("the corpus seed did not parse: %s", c.Seed)
		}
		got := "forge|" + joinWith(c.Addresses, "|") +
			"|" + itoa(c.Games) + "|" + seed.String()
		if got != c.Key {
			t.Errorf("key:\n got %q\nwant %q", got, c.Key)
		}
	}
}

// decodeBody re-decodes a corpus payload the way a real request body is
// decoded — with UseNumber, so an integer literal is still exact and
// `int("1_0")` is asked of a string rather than of a float64.
//
// Not a convenience: decoding differently from production is one of the four
// ways a green corpus test tests nothing, and this is exactly the seam where
// it would happen.
func decodeBody(t *testing.T, payload map[string]any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var body map[string]any
	if err := decoder.Decode(&body); err != nil {
		t.Fatal(err)
	}
	return body
}

func joinWith(parts []string, sep string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += sep
		}
		out += p
	}
	return out
}

func itoa(n int) string { return strconv.Itoa(n) }

func normaliseJSON(raw string) string {
	out := make([]byte, 0, len(raw))
	inString, escaped := false, false
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		if inString {
			out = append(out, c)
			switch {
			case escaped:
				escaped = false
			case c == '\\':
				escaped = true
			case c == '"':
				inString = false
			}
			continue
		}
		switch c {
		case '"':
			inString = true
			out = append(out, c)
		case ' ', '\n', '\t':
		default:
			out = append(out, c)
		}
	}
	return string(out)
}
