package gate

import (
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/pool"
)

func card(name, typeLine, text string) *pool.CardRecord {
	return &pool.CardRecord{Name: name, TypeLine: typeLine, OracleText: text}
}

func TestPairingOfReadsTheAbilityAsTheReportsRecord(t *testing.T) {
	t.Parallel()
	cases := []struct {
		text string
		want *Pairing
	}{
		{"Partner (You can have two commanders if both have partner.)", &Pairing{Kind: Partner}},
		{"Partner", &Pairing{Kind: Partner}},
		{"Partner with Ley Weaver (When this creature enters, target player may put Ley Weaver into their hand.)",
			&Pairing{Kind: PartnerWith, PartnerName: "Ley Weaver"}},
		{"Partner—Friends forever (You can have two commanders if both have Friends forever.)",
			&Pairing{Kind: Labeled, Label: "Friends forever"}},
		{"Partner — Survivors", &Pairing{Kind: Labeled, Label: "Survivors"}},
		{"Flying\nChoose a Background (You can have a Background as a second commander.)",
			&Pairing{Kind: BackgroundChooser}},
		{"Doctor's companion (You can have two commanders if the other is the Doctor.)",
			&Pairing{Kind: DoctorsCompanion}},
		{"Partnership is a word, not an ability", nil},
		{"", nil},
	}
	for _, c := range cases {
		got := PairingOf(card("X", "Legendary Creature — Human", c.text))
		if (got == nil) != (c.want == nil) || (got != nil && *got != *c.want) {
			t.Errorf("%q -> %+v, want %+v", c.text, got, c.want)
		}
	}
	if PairingOf(card("X", "", "Partner—Friends forever")).Describe() != "Partner—Friends forever" {
		t.Fatal("describe")
	}
}

func TestCanBeCommanderHoldsTheLegendaryLine(t *testing.T) {
	t.Parallel()
	legend := card("Gyome, Master Chef", "Legendary Creature — Troll Warlock", "")
	background := card("Raised by Giants", "Legendary Enchantment — Background", "Commander creatures you own have base power and toughness 10/10.")
	battlebond := card("Ley Weaver", "Creature — Elf Druid", "Partner with Lore Weaver\n{T}: Untap two target lands.")
	saysSo := card("Ashling", "Legendary Planeswalker — Ashling", "Ashling can be your commander.")
	dfc := card("Etali // Etali, Primal Sickness", "Legendary Creature — Elder Dinosaur // Legendary Creature — Elder Dinosaur", "")
	if !CanBeCommander(legend, false) || !CanBeCommander(saysSo, false) || !CanBeCommander(dfc, false) {
		t.Fatal("a legal commander was refused")
	}
	if CanBeCommander(background, false) || !CanBeCommander(background, true) {
		t.Fatal("a Background is legal only as one of a pair")
	}
	if CanBeCommander(battlebond, false) || CanBeCommander(battlebond, true) {
		t.Fatal("the Battlebond trap: a nonlegendary partner is never a commander")
	}
	if !NonlegendaryPartner(battlebond) || NonlegendaryPartner(legend) {
		t.Fatal("nonlegendary_partner")
	}
	if !IsBackground(background) || IsDoctor(background) {
		t.Fatal("type marks")
	}
}

func TestCheckPairSaysWhyNot(t *testing.T) {
	t.Parallel()
	plainA := card("Akiri, Line-Slinger", "Legendary Creature — Kor Soldier Ally", "Partner")
	plainB := card("Bruse Tarl, Boorish Herder", "Legendary Creature — Human Ally", "Partner")
	ffA := card("Ruby", "Legendary Creature — Human", "Partner—Friends forever")
	ffB := card("Lucas", "Legendary Creature — Human", "Partner—Friends forever")
	survivor := card("Ches", "Legendary Creature — Human", "Partner—Survivors")
	withA := card("Pir, Imaginative Rascal", "Legendary Creature — Human", "Partner with Toothy, Imaginary Friend")
	withB := card("Toothy, Imaginary Friend", "Legendary Creature — Illusion", "Partner with Pir, Imaginative Rascal")
	chooser := card("Jaheira, Friend of the Forest", "Legendary Creature — Human Druid", "Choose a Background")
	bg := card("Raised by Giants", "Legendary Enchantment — Background", "")
	doctor := card("The Tenth Doctor", "Legendary Creature — Time Lord Doctor", "")
	companion := card("Rose Tyler", "Legendary Creature — Human", "Doctor's companion")
	lone := card("Gyome, Master Chef", "Legendary Creature — Troll Warlock", "")

	for _, ok := range [][2]*pool.CardRecord{{plainA, plainB}, {ffA, ffB}, {withA, withB}, {withB, withA},
		{chooser, bg}, {bg, chooser}, {companion, doctor}, {doctor, companion}} {
		if why := CheckPair(ok[0], ok[1]); why != "" {
			t.Errorf("%s + %s refused: %s", ok[0].Name, ok[1].Name, why)
		}
	}
	want := map[[2]*pool.CardRecord]string{
		{lone, card("Other", "Legendary Creature", "")}: "neither card has a pairing ability, so they cannot be two commanders together",
		{ffA, survivor}:     "Ruby has Partner—Friends forever but Ches has Partner—Survivors; both must have the same one",
		{withA, plainA}:     "Pir, Imaginative Rascal has Partner with Toothy, Imaginary Friend, so it can only pair with that card, not Akiri, Line-Slinger",
		{chooser, plainA}:   "Jaheira, Friend of the Forest chooses a Background, but Akiri, Line-Slinger is not a Background",
		{companion, plainA}: "Rose Tyler is a Doctor's companion, but Akiri, Line-Slinger is not a Doctor",
		{plainA, lone}:      "only Akiri, Line-Slinger has a pairing ability; both commanders need one that matches",
		{lone, plainA}:      "only Akiri, Line-Slinger has a pairing ability; both commanders need one that matches",
	}
	for pair, sentence := range want {
		if got := CheckPair(pair[0], pair[1]); got != sentence {
			t.Errorf("%s + %s:\n got %q\nwant %q", pair[0].Name, pair[1].Name, got, sentence)
		}
	}
}
