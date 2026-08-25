package pool_test

import (
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/pool"
)

// What "makes mana" means, and why Scryfall's own answer is the wrong one.
//
// The board keeps mana rocks back with the lands, because those two rows
// together answer one question -- what can this deck pay for. Deciding which
// artifacts belong there looked like a solved problem: Scryfall publishes
// `produced_mana` and the pool already carries it.
//
// It is not the same question. `produced_mana` counts mana produced by *tokens
// the card creates*, read out of the token's reminder text -- so a card that
// makes Treasures reports every colour a Treasure can make, having never
// produced a mana itself. Found on a real board, with Nuka-Cola Vending
// Machine standing in the land row next to the Swamps: a claim about a deck's
// mana that the card does not make.
//
// The oracle text below is each card's real text, taken from the pool.
func TestMakesManaReadsTheCardAndNotItsTokens(t *testing.T) {
	t.Parallel()

	five := []string{"B", "G", "R", "U", "W"}
	for _, tc := range []struct {
		name     string
		produced []string
		oracle   string
		want     bool
		why      string
	}{{
		name:     "Sol Ring",
		produced: []string{"C"},
		oracle:   "{T}: Add {C}{C}.",
		want:     true,
		why:      "the plainest mana rock there is",
	}, {
		name:     "Arcane Signet",
		produced: five,
		oracle:   "{T}: Add one mana of any color in your commander's color identity.",
		want:     true,
		why:      "a mana ability written in words rather than symbols",
	}, {
		name:     "Gaea's Cradle",
		produced: []string{"G"},
		oracle:   "{T}: Add {G} for each creature you control.",
		want:     true,
		why:      "an amount that is not a fixed number is still an amount",
	}, {
		name:     "Nuka-Cola Vending Machine",
		produced: five,
		oracle: "{1}, {T}: Create a Food token. (It's an artifact with " +
			"\"{2}, {T}, Sacrifice this token: You gain 3 life.\")\n" +
			"Whenever you sacrifice a Food, create a tapped Treasure token. " +
			"(It's an artifact with \"{T}, Sacrifice this token: Add one " +
			"mana of any color.\")",
		want: false,
		why: "the one that started this: a Food engine two steps removed " +
			"from a mana, and the only 'Add' on it is inside a token's " +
			"reminder text",
	}, {
		name:     "Smothering Tithe",
		produced: five,
		oracle: "Whenever an opponent draws a card, that player may pay {2}. " +
			"If the player doesn't, you create a Treasure token. (It's an " +
			"artifact with \"{T}, Sacrifice this token: Add one mana of any " +
			"color.\")",
		want: false,
		why:  "the same fault on an enchantment, where the row hid it",
	}, {
		name:     "Lightning Greaves",
		produced: nil,
		oracle:   "Equipped creature has haste and shroud.\nEquip {0}",
		want:     false,
		why:      "no mana by any reading, and the cheap first pass says so",
	}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rec := &pool.CardRecord{
				Name: tc.name, OracleText: tc.oracle, ProducedMana: tc.produced,
			}
			if got := rec.MakesMana(); got != tc.want {
				t.Errorf("MakesMana() = %v, want %v -- %s", got, tc.want, tc.why)
			}
		})
	}
}

// Reminder text is stripped, not searched, and that is the whole mechanism --
// so a card whose *own* ability adds mana keeps it even when it also creates a
// token whose reminder text mentions mana.
func TestMakesManaKeepsARealAbilityBesideAToken(t *testing.T) {
	t.Parallel()

	rec := &pool.CardRecord{
		Name:         "A rock that also makes Treasure",
		ProducedMana: []string{"C", "W"},
		OracleText: "{T}: Add {C}.\n{2}, {T}: Create a Treasure token. " +
			"(It's an artifact with \"{T}, Sacrifice this token: Add one " +
			"mana of any color.\")",
	}
	if !rec.MakesMana() {
		t.Error("stripping the reminder text took the real ability with it")
	}
}
