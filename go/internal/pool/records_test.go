package pool_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/aasquier/sylvan-library/go/internal/pool"
	"github.com/aasquier/sylvan-library/go/internal/pool/pooltest"
)

// use runs fn against p, failing the test on any error.
func use(t *testing.T, p *pool.Pool, fn func(c *pool.Conn)) {
	t.Helper()
	if err := p.Use(context.Background(), func(c *pool.Conn) error { fn(c); return nil }); err != nil {
		t.Fatal(err)
	}
}

func TestGetCardsResolvesNamesAsRecorded(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	use(t, pooltest.Open(t), func(c *pool.Conn) {
		// Case-insensitive; a double-faced card by its front face; a banned
		// card still found; a name the pool lacks simply absent; and the
		// result keyed by the spelling asked, not the pool's.
		got, err := c.GetCards(ctx, []string{"sol ring", "Ajani, Nacatl Pariah",
			"Primeval Titan", "No Such Card", "FOREST"})
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 4 {
			t.Fatalf("got %d records: %v", len(got), keys(got))
		}
		if got["sol ring"] == nil || got["sol ring"].Name != "Sol Ring" {
			t.Fatalf("sol ring -> %+v", got["sol ring"])
		}
		ajani := got["Ajani, Nacatl Pariah"]
		if ajani == nil || ajani.Name != "Ajani, Nacatl Pariah // Ajani, Nacatl Avenger" {
			t.Fatalf("the front face did not resolve the whole card: %+v", ajani)
		}
		// Rule 2: the identity is the WHOLE card's -- white front, red back.
		if len(ajani.ColorIdentity) != 2 || ajani.ColorIdentity[0] != "R" || ajani.ColorIdentity[1] != "W" {
			t.Fatalf("Ajani's identity read as %v; the back face is red", ajani.ColorIdentity)
		}
		if ajani.Layout != "transform" || ajani.ManaCost == nil || *ajani.ManaCost != "{1}{W}" {
			t.Fatalf("ajani: layout %q cost %v", ajani.Layout, ajani.ManaCost)
		}
		if got["Primeval Titan"].LegalCommander {
			t.Fatal("the Titan read as legal")
		}
		if !got["FOREST"].IsLand() || got["FOREST"].LegalCommander != true {
			t.Fatalf("forest: %+v", got["FOREST"])
		}
		if _, there := got["No Such Card"]; there {
			t.Fatal("a missing name was not simply absent")
		}
		// Empty lists are lists.
		if got["sol ring"].ColorIdentity == nil || got["sol ring"].Keywords == nil {
			t.Fatal("a nil list slipped through")
		}
		if got["sol ring"].EdhrecRank == nil || *got["sol ring"].EdhrecRank != 1 {
			t.Fatalf("sol ring rank %v", got["sol ring"].EdhrecRank)
		}
		if got["FOREST"].EdhrecRank != nil {
			t.Fatal("a NULL rank read as a number")
		}
	})
}

func keys(m map[string]*pool.CardRecord) []string {
	out := []string{}
	for k := range m {
		out = append(out, k)
	}
	return out
}

// **A card's faces come back with their own paintings, and a card with one
// face comes back with none.**
//
// The whole point of reading `card_faces` is the distinction Scryfall encodes
// in it: `image_uris` sits on the *card* when both names are printed on one
// piece of cardboard and on the *faces* when each is a painting of its own. So
// this asserts the encoding rather than the parse — a transforming card's back
// has a URL of its own, and it is not the card's.
//
// Read through the driver on purpose. `card_faces` is a JSON column and the
// driver decodes it before this package sees it, so a unit test over a string
// would be testing a shape nothing produces (see `coercions_test.go`, which
// records what the driver's widths cost this pool once already).
func TestACardsFacesCarryTheirOwnPaintings(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	use(t, pooltest.Open(t), func(c *pool.Conn) {
		got, err := c.GetCards(ctx,
			[]string{"Ajani, Nacatl Pariah", "Sol Ring"})
		if err != nil {
			t.Fatal(err)
		}
		if faces := got["Sol Ring"].Faces; len(faces) != 0 {
			t.Fatalf("a one-faced card carries %v", faces)
		}
		faces := got["Ajani, Nacatl Pariah"].Faces
		if len(faces) != 2 {
			t.Fatalf("Ajani has %d faces", len(faces))
		}
		if faces[0].Name != "Ajani, Nacatl Pariah" ||
			faces[1].Name != "Ajani, Nacatl Avenger" {
			t.Fatalf("the faces are named %q and %q",
				faces[0].Name, faces[1].Name)
		}
		// Each half is a different kind of thing, which is the fact a board
		// reads to say what a permanent turned into.
		if faces[0].TypeLine != "Legendary Creature — Cat Warrior" ||
			faces[1].TypeLine != "Legendary Planeswalker — Ajani" {
			t.Fatalf("the faces are %q and %q",
				faces[0].TypeLine, faces[1].TypeLine)
		}
		if faces[0].ImageNormal == nil || faces[1].ImageNormal == nil {
			t.Fatal("a transforming card came back with a face unpainted")
		}
		if *faces[0].ImageNormal == *faces[1].ImageNormal {
			t.Fatal("both faces answered with one painting")
		}
		// And the card's own picture is the front's, which is why a back face
		// needs its own: `images` in rows.go falls back to the first face, so
		// `ImageNormal` is right for the card and wrong for the other half.
		if got["Ajani, Nacatl Pariah"].ImageNormal == nil ||
			*got["Ajani, Nacatl Pariah"].ImageNormal != *faces[0].ImageNormal {
			t.Fatal("the card's own picture is not its front face's")
		}
	})
}

func TestGetCardsIsMemoisedPerOpen(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	p := pooltest.Open(t)
	use(t, p, func(c *pool.Conn) {
		a, _ := c.GetCards(ctx, []string{"Sol Ring"})
		b, _ := c.GetCards(ctx, []string{"Sol Ring"})
		if a["Sol Ring"] != b["Sol Ring"] {
			t.Fatal("a second ask was not the memoised record")
		}
		// The caller's map is its own; popping from it cannot reach the memo.
		delete(a, "Sol Ring")
		again, _ := c.GetCards(ctx, []string{"Sol Ring"})
		if again["Sol Ring"] == nil {
			t.Fatal("a caller's delete reached the cache")
		}
		if p.CacheLen() != 1 {
			t.Fatalf("%d cache entries", p.CacheLen())
		}
		if n, _ := c.GetCards(ctx, nil); len(n) != 0 {
			t.Fatal("no names is not an empty answer")
		}
	})
}

func TestSearchOrdersAndLimits(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	use(t, pooltest.Open(t), func(c *pool.Conn) {
		recs, err := c.Search(ctx, "type_line LIKE ? AND json_extract_string(legalities, 'commander') = 'legal'",
			[]any{"%Creature%"}, 3, "edhrec_rank NULLS LAST", 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(recs) != 3 {
			t.Fatalf("%d records", len(recs))
		}
		for i := 1; i < len(recs); i++ {
			if recs[i-1].EdhrecRank != nil && recs[i].EdhrecRank != nil && *recs[i-1].EdhrecRank > *recs[i].EdhrecRank {
				t.Fatal("not ordered by rank")
			}
		}
		offset, _ := c.Search(ctx, "type_line LIKE ?", []any{"%Creature%"}, 1, "edhrec_rank NULLS LAST", 1)
		if len(offset) != 1 || offset[0].Name != recs[1].Name {
			t.Fatalf("offset 1 gave %v, want %s", offset, recs[1].Name)
		}
	})
}

func TestColumnsAreReadFromThePool(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	use(t, pooltest.Open(t), func(c *pool.Conn) {
		cols, err := c.Columns(ctx, "printings")
		if err != nil {
			t.Fatal(err)
		}
		if !cols["artist"] || !cols["price_usd"] || cols["nope"] {
			t.Fatalf("printings columns %v", cols)
		}
		again, _ := c.Columns(ctx, "printings")
		if len(again) != len(cols) {
			t.Fatal("the memo disagreed with the first read")
		}
	})
}

func TestArtCropFrom(t *testing.T) {
	t.Parallel()
	normal := "https://cards.scryfall.io/normal/front/9/1/91fdb56b.jpg?1"
	if got := pool.ArtCropFrom(&normal); got == nil || *got != "https://cards.scryfall.io/art_crop/front/9/1/91fdb56b.jpg?1" {
		t.Fatalf("%v", got)
	}
	odd := "https://example.com/x.jpg"
	if pool.ArtCropFrom(&odd) != nil || pool.ArtCropFrom(nil) != nil {
		t.Fatal("a URL of another shape was guessed at")
	}
}

func TestTheLeaseHandsThePoolBack(t *testing.T) {
	t.Parallel()
	p := pooltest.Open(t)
	p.SetIdle(50 * time.Millisecond)
	if p.Held() {
		t.Fatal("open before any use")
	}
	use(t, p, func(*pool.Conn) {})
	if !p.Held() {
		t.Fatal("not held after a use")
	}
	if p.ReapOnce() {
		t.Fatal("reaped inside the lease")
	}
	time.Sleep(70 * time.Millisecond)
	// Either this call or the reaper goroutine hands it back; what matters is
	// that it is no longer held.
	p.ReapOnce()
	if p.Held() {
		t.Fatal("the lease did not expire")
	}
	// And it opens again on the next ask, with an empty memo.
	use(t, p, func(c *pool.Conn) {
		if _, err := c.GetCards(context.Background(), []string{"Sol Ring"}); err != nil {
			t.Fatal(err)
		}
	})
	if !p.Held() {
		t.Fatal("did not reopen")
	}
	// A lease that is still out is never reaped from under it.
	p.ForceLease(1, time.Now().Add(-time.Minute))
	if p.ReapOnce() {
		t.Fatal("reaped under an outstanding lease")
	}
	p.ForceLease(0, time.Now())
}

func TestAMissingPoolIsErrNoPool(t *testing.T) {
	t.Parallel()
	p := pool.New("/nowhere/at/all.duckdb", nil)
	err := p.Use(context.Background(), func(*pool.Conn) error { t.Fatal("ran"); return nil })
	if !errors.Is(err, pool.ErrNoPool) {
		t.Fatalf("err = %v", err)
	}
}

func TestAMovedPoolIsReopened(t *testing.T) {
	t.Parallel()
	// A `data refresh` rewrites the file; the next use must read the new
	// one rather than the snapshot the old instance held.
	first := pooltest.Build(t)
	p := pool.New(first, nil)
	t.Cleanup(p.Close)
	ctx := context.Background()
	use(t, p, func(c *pool.Conn) {
		n, _ := pool.Count(ctx, c.DB(), "printings")
		if n == 0 {
			t.Fatal("fixture has no printings")
		}
	})
	// Make a different pool -- same cards, no printings -- and move it over.
	other := pooltest.Build(t)
	w, err := pooltest.Writer(other)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Exec("DELETE FROM printings"); err != nil {
		t.Fatal(err)
	}
	w.Close()
	if err := os.Rename(other, first); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(2 * time.Second)
	_ = os.Chtimes(first, future, future)
	use(t, p, func(c *pool.Conn) {
		n, _ := pool.Count(ctx, c.DB(), "printings")
		if n != 0 {
			t.Fatalf("the moved pool was not reopened: %d printings", n)
		}
	})
}
