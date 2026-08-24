package compile

import "testing"

// Benchmarks for the oracle-text readers.
//
// These three run once per card per compile, and a compile happens on every
// uncached Tier 1 request -- so a hundred cards times however many decks a
// sweep touches. They are string scans over text nobody controls, which is
// exactly the shape that quietly gets slower when a case is added to it.
//
// The corpus below is real oracle text in the shapes each reader branches on,
// including the ones that do the most work: a long reminder-text card, and a
// fetchland whose sentence names two land types.

var (
	sinkBool bool
	sinkInt  int
)

var oracleCorpus = []struct{ name, text string }{
	{"basic", "({T}: Add {G}.)"},
	{"tapped", "Blossoming Sands enters the battlefield tapped.\nWhen Blossoming Sands enters the battlefield, you gain 2 life."},
	{"conditional", "As Glacial Fortress enters the battlefield, you may reveal a Plains or Island card from your hand. If you don't, Glacial Fortress enters the battlefield tapped."},
	{"fetch", "{T}, Pay 1 life, Sacrifice Windswept Heath: Search your library for a Forest or Plains card, put it onto the battlefield, then shuffle."},
	{"filter", "{T}: Add {C}.\n{1}, {T}: Add {G}{W}."},
	{"rock", "{T}: Add one mana of any color."},
	{"long", "Whenever a creature you control dies, put a +1/+1 counter on Ravos, Soultender. " +
		"At the beginning of your upkeep, return target creature card from your graveyard to your hand. " +
		"Partner (You can have two commanders if both have partner.) " +
		"Creatures you control get +1/+1. Flying, vigilance, lifelink, trample, haste."},
}

func BenchmarkEntersTapped(b *testing.B) {
	for _, c := range oracleCorpus {
		b.Run(c.name, func(b *testing.B) {
			for b.Loop() {
				sinkBool = EntersTapped(c.text)
			}
		})
	}
}

func BenchmarkManaProduced(b *testing.B) {
	for _, c := range oracleCorpus {
		b.Run(c.name, func(b *testing.B) {
			for b.Loop() {
				sinkInt = ManaProduced(c.text)
			}
		})
	}
}

func BenchmarkFetchesLands(b *testing.B) {
	for _, c := range oracleCorpus {
		b.Run(c.name, func(b *testing.B) {
			for b.Loop() {
				sinkInt = FetchesLands(c.text)
			}
		})
	}
}

// BenchmarkTheWholeCorpus is the per-compile shape: every reader over every
// card, which is what the cost actually looks like from a request's point of
// view.
func BenchmarkTheWholeCorpus(b *testing.B) {
	for b.Loop() {
		for _, c := range oracleCorpus {
			sinkBool = EntersTapped(c.text)
			sinkInt = ManaProduced(c.text)
			sinkInt = FetchesLands(c.text)
		}
	}
}
