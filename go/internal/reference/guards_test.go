package reference

// The guards, not the paths.
//
// Everything this package serves is committed prose, so none of the refusals
// below can be reached by anything a person does — they stand between a
// *damaged embedded file* and a binary that boots anyway. That is exactly why
// they are worth holding: the failure they prevent is silent. A duplicate
// arena key loses an arena with nothing logged; a fact of an unknown kind
// renders as a blank slide; a zone with no painter credited is commandment 19
// broken on a live page. A panic at start is a build that must not ship, and
// these tests are what says the panic is still there.
//
// The shape is `canonjson_test.go`'s: the indexing and the checking are
// functions over a document, so a test can hand them the documents the
// committed file is not allowed to become.

import (
	"strings"
	"testing"
)

// mustPanic runs f and returns what it panicked with, failing when it did not.
func mustPanic(t *testing.T, what string, f func()) string {
	t.Helper()
	var got string
	func() {
		defer func() {
			if r := recover(); r != nil {
				got, _ = r.(string)
			}
		}()
		f()
	}()
	if got == "" {
		t.Fatalf("%s did not refuse", what)
	}
	return got
}

func TestADamagedColiseumRefusesToBootRatherThanServeHalfAnArena(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		doc  Coliseum
		says string
	}{
		{
			name: "the same arena twice",
			doc: Coliseum{Arenas: []Arena{
				{Key: "grand-coliseum"}, {Key: "grand-coliseum"},
			}},
			says: `names "grand-coliseum" twice`,
		},
		{
			name: "a fact of a kind nothing can render",
			doc: Coliseum{Arenas: []Arena{{
				Key:   "valors-reach",
				Facts: []ColiseumFact{{Kind: "roman"}, {Kind: "limerick"}},
			}}},
			says: `unknown kind "limerick"`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			doc := tc.doc
			got := mustPanic(t, tc.name, func() { indexArenas(&doc) })
			if !strings.Contains(got, tc.says) {
				t.Fatalf("panicked with %q, which does not say %q", got, tc.says)
			}
		})
	}
	// And the committed file still indexes, so the sweep above is not passing
	// because `indexArenas` refuses everything.
	live := indexArenas(&coliseum)
	if len(live) < 6 {
		t.Fatalf("the committed coliseum indexed %d arenas", len(live))
	}
}

func TestADressedZoneWithoutItsPainterIsABuildThatDoesNotShip(t *testing.T) {
	t.Parallel()
	good := ZoneArt{Key: "graveyard", Card: "Unearth",
		Art: ArenaArt{URL: "https://example.invalid/a.jpg", Artist: "A Painter"}}
	cases := []struct {
		name string
		zone ZoneArt
		says string
	}{
		{"a zone the board does not have", ZoneArt{Key: "sideboard"}, `unknown zone "sideboard"`},
		{"no card", ZoneArt{Key: "exile", Art: good.Art}, `zone "exile" needs a card`},
		{"no painting", ZoneArt{Key: "exile", Card: "Swords to Plowshares"}, `zone "exile" needs a card`},
		{"no painter", ZoneArt{Key: "exile", Card: "Swords to Plowshares",
			Art: ArenaArt{URL: good.Art.URL}}, `zone "exile" needs a card`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			doc := Coliseum{Zones: []ZoneArt{tc.zone}}
			got := mustPanic(t, tc.name, func() { checkZones(&doc) })
			if !strings.Contains(got, tc.says) {
				t.Fatalf("panicked with %q, which does not say %q", got, tc.says)
			}
		})
	}
	// The same zone dressed twice is its own refusal: two paintings for one
	// place is one of them silently never drawn.
	t.Run("the same zone twice", func(t *testing.T) {
		t.Parallel()
		doc := Coliseum{Zones: []ZoneArt{good, good}}
		got := mustPanic(t, "a zone dressed twice", func() { checkZones(&doc) })
		if !strings.Contains(got, `dresses zone "graveyard" twice`) {
			t.Fatalf("panicked with %q", got)
		}
	})
	// And the committed dressing passes, so none of the above is passing on a
	// check that rejects everything.
	checkZones(&coliseum)
}

func TestAColourNamedTwiceIsRefusedRatherThanQuietlyLosingOne(t *testing.T) {
	t.Parallel()
	doc := Taxonomy{Combinations: []Combination{{Key: "wu"}, {Key: "wu"}}}
	got := mustPanic(t, "a duplicate combination", func() { indexCombinations(&doc) })
	if !strings.Contains(got, `names "wu" twice`) {
		t.Fatalf("panicked with %q", got)
	}
	// The committed taxonomy is the thirty-two, and each entry in the index
	// points at the combination that carries its key rather than a copy.
	live := indexCombinations(&taxonomy)
	if len(live) != len(taxonomy.Combinations) {
		t.Fatalf("indexed %d of %d combinations", len(live), len(taxonomy.Combinations))
	}
	if len(live) < 32 {
		t.Fatalf("the committed taxonomy holds %d combinations", len(live))
	}
	for key, c := range live {
		if c.Key != key {
			t.Fatalf("%q is filed under %q", c.Key, key)
		}
	}
}

func TestAnEmbeddedFileThatDoesNotParseIsABuildThatDoesNotShip(t *testing.T) {
	t.Parallel()
	var into map[string]any
	got := mustPanic(t, "a damaged embed", func() {
		mustCompact("colors.json", []byte(`{"combinations": [`), &into)
	})
	if !strings.Contains(got, "colors.json does not parse") {
		t.Fatalf("panicked with %q", got)
	}
	// A file that parses comes back compacted to the recorded wire shape:
	// the same document, without the whitespace an editor left in it.
	raw := mustCompact("x.json", []byte("{\n  \"a\": [1, 2]\n}\n"), &into)
	if string(raw) != `{"a":[1,2]}` {
		t.Fatalf("compacted to %q", raw)
	}
}
