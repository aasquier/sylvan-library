package decklist

// Three edges of the paste: the companion nobody exports twice, a line that
// is only a number, and a line long enough to be refused whose reported text
// still has to be readable.
//
// A pasted list is somebody's whole deck, and the parser never fails -- so
// every one of these is about what the *report* says rather than about
// whether an error came back. A line echoed back whole would put a stranger's
// input into a response and a log; a line echoed back empty would tell them
// nothing about which line it was.

import (
	"strings"
	"testing"
)

func TestACompanionIsReadFromItsOwnSectionAndIsOtherwiseEmpty(t *testing.T) {
	t.Parallel()
	// The companion is bought into the hand rather than played from the 99,
	// so it is its own section in every exporter that knows about it -- and
	// most pasted lists do not have one at all.
	with := Parse("Companion\n1 Fixture Sidekick\n\nDeck\n1 Fixture Forest\n")
	if got := with.Companion(); got != "Fixture Sidekick" {
		t.Fatalf("companion %q", got)
	}
	if cards := with.Section("deck"); len(cards) != 1 || cards[0].Name != "Fixture Forest" {
		t.Fatalf("the deck section is %+v", cards)
	}
	without := Parse("Deck\n1 Fixture Forest\n")
	if got := without.Companion(); got != "" {
		t.Fatalf("a list with no companion section named %q", got)
	}
}

func TestACountWithNothingAfterItIsAName(t *testing.T) {
	t.Parallel()
	// The recorded grammar wants a non-space after the count's whitespace
	// run. The end of the line is not one -- and neither is a run that used
	// to have an annotation after it, which is how the state is actually
	// reached: the annotation peels off the right first and leaves the
	// trailing space behind.
	//
	// This is the equivalence `leadingQty`'s comment argues, stated as the
	// three answers a backtracking engine would give.
	for _, body := range []string{"4 ", "12x\t", "4"} {
		qty, rest, ok := leadingQty(body)
		if ok {
			t.Fatalf("leadingQty(%q) read %d copies of %q", body, qty, rest)
		}
		if rest != body {
			t.Fatalf("leadingQty(%q) handed back %q", body, rest)
		}
	}
	// Through the parser: one card, named for the digits, a single copy.
	// Not four copies of nothing, and not a line thrown away.
	list := Parse("4 (2X2)\n")
	if len(list.Cards) != 1 {
		t.Fatalf("a bare count with a printing parsed as %+v / %+v", list.Cards, list.Unreadable)
	}
	if list.Cards[0].Name != "4" || list.Cards[0].Qty != 1 {
		t.Fatalf("parsed as %+v", list.Cards[0])
	}
	// And a count with a name after it is still a count, so the refusal
	// above is the lookahead and not the digits.
	four := Parse("4 Fixture Forest\n")
	if len(four.Cards) != 1 || four.Cards[0].Qty != 4 || four.Cards[0].Name != "Fixture Forest" {
		t.Fatalf("a real quantity parsed as %+v", four.Cards)
	}
}

func TestAnOverlongLineIsReportedShortEnoughToRead(t *testing.T) {
	t.Parallel()
	// Over the bound the line is unreadable rather than fatal -- one absurd
	// line must not cost somebody the other ninety-eight. What comes back is
	// the line trimmed and then cut to the bound, and the cut is by code
	// point: a line of accented letters must not be sliced through one.
	name := "Æ" + strings.Repeat("é", MaxLine+40)
	list := Parse(name + "\n")
	if len(list.Unreadable) != 1 {
		t.Fatalf("%d unreadable lines", len(list.Unreadable))
	}
	if got := len([]rune(list.Unreadable[0].Text)); got != MaxLine {
		t.Fatalf("the reported text is %d code points, want %d", got, MaxLine)
	}
	if !strings.HasPrefix(list.Unreadable[0].Text, "Æé") {
		t.Fatalf("the reported text starts %q", list.Unreadable[0].Text[:8])
	}

	// A line that is over the bound only because of the whitespace in front
	// of it comes back whole once it is trimmed: the cut is not taken when
	// there is nothing left to cut.
	padded := strings.Repeat(" ", 80) + strings.Repeat("é", MaxLine-40)
	list = Parse(padded + "\n")
	if len(list.Unreadable) != 1 {
		t.Fatalf("%d unreadable lines for the padded one", len(list.Unreadable))
	}
	if got := len([]rune(list.Unreadable[0].Text)); got != MaxLine-40 {
		t.Fatalf("the padded line reported %d code points, want %d", got, MaxLine-40)
	}
}
