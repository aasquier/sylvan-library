"""What the fortune-teller knows about the cards in her hands.

Reference prose, the fourth module of its kind. `colors.py` teaches the 32
combinations, `glossary.py` the words, `lore.py` the shelves -- and this
teaches the deck the tarot table actually deals: the 1909 Rider printing, the
woman who painted all 78 of it, and the man who told her what to paint.

**Checked in rather than asked for, on the same four counts as its siblings**
(`colors.py`'s docstring makes the argument in full). The set is finite and
written once. It answers with no card pool, no network and no key, which is
what lets a reading stay warm while the model is thinking. Bland prose is
fixed by editing, which only checked-in text allows. And this is the half of
a tarot reading that *has* a right answer -- Pamela Colman Smith was paid a
flat fee or she was not -- which is exactly the half `tarot.py` already
reserves for Python. That module knows every card and no card's meaning; this
one knows every card's history and still no card's meaning.

**The subject is the deck, not divination** (Aaron's call, 2026-08-18, from a
menu of four wells). Not what The Tower portends -- the reader does that, and
it has no right answer -- but who drew the tower, what she was paid, whose
photographs of a fifteenth-century deck she had seen two years before, and
which of these plates carries her monogram in the corner. It is history and
people, the same register `lore.py` works in, and it is chosen because it is
*true*: a fact a querent can check is a different gift from a fortune.

**Claude picks; Python supplies the words.** A fact rides into the reader's
frame with its id, and the schema asks for that id back -- `tarot:pixie-fee`
-- rather than for the sentence. `theme.keep_fact` resolves the id and renders
**this file's text, verbatim**; an id that does not resolve is dropped and
counted. That is deliberately stricter than the `'taxonomy'` source beside it,
which trusts the model's own sentence because the colour data is small and
sits in the prompt. Here the model's job is *which fact, and why it belongs
now* -- selection and connection, which is judgement -- and never authorship,
because a fun fact invented at a fortune-teller's table is the one thing at
that table that would actually be a lie. ADR 14 in miniature.

**Two tiers, and the deck tier is why no reading is ever empty.** A fact is
either about one card or about the deck and its makers. `for_reading` hands
over both, so a spread of three minors -- which the sampler can absolutely
deal -- still arrives with Pixie Smith's whole life available. The per-card
tier begins with the 22 trumps; the 56 minors are owed their own, and
ROADMAP item 15 records the debt rather than pretending it is paid.

Every fact carries a `source` a person can go and check. Nothing here is
generated, and the two standing references are Stuart R. Kaplan and others,
Pamela Colman Smith: The Untold Story (U.S. Games, 2018) -- the biography
that assembled most of what is known about her -- and A. E. Waite's own *The
Pictorial Key to the Tarot* (1911), which is the primary source for what he
thought he had commissioned.
"""

from __future__ import annotations

from dataclasses import dataclass

#: The two references cited often enough to be worth a name.
UNTOLD_STORY = ("Kaplan, Greer, O'Connor & Parsons, "
                "Pamela Colman Smith: The Untold Story (U.S. Games, 2018)")
PICTORIAL_KEY = "A. E. Waite, The Pictorial Key to the Tarot (1911)"


@dataclass(frozen=True)
class Fact:
    """One thing that is true about this deck.

    `text` is what renders, verbatim, because the reader cites the `id` and
    never writes the sentence. One or two sentences: `theme.MAX_FACT_CHARS`
    is 600 and a fun fact longer than that was not a fun fact.

    `card` is a `tarot.Card.key` or empty for the deck tier. `source` is where
    a person goes to check it, and no fact ships without one -- the same rule
    every other Claude surface here obeys, kept at the file level because the
    file is the thing under review.
    """

    id: str
    text: str
    source: str
    card: str = ""


#: About the deck, its makers, and the year -- true whatever lands on the
#: table, which is what keeps a spread of three minors from arriving empty.
DECK_FACTS: tuple[Fact, ...] = (
    Fact(
        "pixie-fee",
        "Pamela Colman Smith drew all 78 cards of this deck and was paid a "
        "single flat fee for the lot — no royalties, ever. It went on to "
        "become the most reproduced tarot deck in the world, and she died in "
        "1951 owing money.",
        UNTOLD_STORY),
    Fact(
        "pixie-name",
        "For most of a century the deck was called Rider-Waite: Rider was the "
        "publisher and Waite wrote the book. The artist who made every image "
        "anyone actually pictures was left out of its name, and 'Rider-Waite-"
        "Smith' is a correction that only caught on recently.",
        UNTOLD_STORY),
    Fact(
        "pixie-monogram",
        "Her signature is on every card, if you know where to look: a tiny "
        "monogram of her initials worked into a corner of each plate, usually "
        "low and to one side. Once you have seen it you cannot stop seeing it.",
        UNTOLD_STORY),
    Fact(
        "pixie-speed",
        "She produced all 78 designs in roughly six months of 1909 — 22 "
        "trumps and 56 minors, every one of them a composed scene. It is an "
        "extraordinary rate for work that has stayed in print for over a "
        "century.",
        UNTOLD_STORY),
    Fact(
        "sola-busca",
        "The idea of giving the numbered cards little scenes instead of rows "
        "of cups and swords was not new — the fifteenth-century Sola Busca "
        "deck did it first. Photographs of it were exhibited at the British "
        "Museum in 1907, two years before Smith sat down to draw, and several "
        "of her cards echo it closely.",
        "British Museum Sola-Busca holdings; comparative studies of the two "
        "decks"),
    Fact(
        "waite-brief",
        "Waite gave Smith detailed instructions for the 22 trumps and then, "
        "by most accounts, left her largely to herself on the other 56. The "
        "half he specified is the half people argue about; the half he did "
        "not is the half that made the deck famous.",
        f"{PICTORIAL_KEY}; {UNTOLD_STORY}"),
    Fact(
        "pixie-jamaica",
        "Smith spent much of her childhood in Jamaica and later performed "
        "Jamaican folk tales on stage in London, in costume, telling Anansy "
        "stories to English audiences. She published them as a book before "
        "she ever drew a tarot card.",
        UNTOLD_STORY),
    Fact(
        "pixie-synaesthesia",
        "She saw pictures when she heard music — she would sit at concerts "
        "and paint what the sound looked like. Some of those paintings "
        "survive, and the habit shows in the cards: they are composed like "
        "stage pictures rather than diagrams.",
        UNTOLD_STORY),
    Fact(
        "pixie-stieglitz",
        "Alfred Stieglitz gave Smith a show at his New York gallery 291 in "
        "1907 — she was the first non-photographer he ever exhibited there, "
        "which put her in the room where American modernism was being "
        "invented.",
        UNTOLD_STORY),
    Fact(
        "pixie-theatre",
        "Before the tarot she worked in the theatre with Ellen Terry and "
        "Henry Irving's company, designing and making miniature stage sets. "
        "Bram Stoker, who managed that theatre and wrote Dracula, was a "
        "friend and encouraged her.",
        UNTOLD_STORY),
    Fact(
        "pixie-nickname",
        "Everyone called her Pixie. Ellen Terry gave her the name, and she "
        "signed letters with it for the rest of her life.",
        UNTOLD_STORY),
    Fact(
        "golden-dawn",
        "Both Smith and Waite belonged to the Hermetic Order of the Golden "
        "Dawn, the occult society whose members also included W. B. Yeats. "
        "The deck came out of a London club of poets, painters and "
        "ceremonial magicians who took each other extremely seriously.",
        UNTOLD_STORY),
    Fact(
        "roses-lilies",
        "This deck is the original 1909 Rider printing, the one collectors "
        "call 'Roses and Lilies' after the pattern on its backs. The bright "
        "flat colours most people picture are a 1971 recolouring — a "
        "different object, and still in copyright.",
        "Rider (London), 1909; U.S. Games Systems reissue history"),
    Fact(
        "hand-colour",
        "The 1909 colours are muted and a little muddy compared to the deck "
        "you have probably seen, because they were printed by a process with "
        "a limited palette. What looks like an artistic choice was partly "
        "what the press could do.",
        "Rider (London), 1909; printing analyses of the first edition"),
    Fact(
        "waite-vague",
        "Waite wrote a whole book explaining the deck and still withheld "
        "things, hinting that the real meanings were reserved for initiates. "
        "Readers have been arguing with The Pictorial Key ever since, "
        "usually while holding Smith's pictures, which explain themselves.",
        PICTORIAL_KEY),
    Fact(
        "pixie-grave",
        "Smith died in Cornwall in 1951. Her possessions were sold to cover "
        "debts, no museum took her work, and her grave is unmarked — the "
        "artist of the most recognisable deck on earth has no headstone.",
        UNTOLD_STORY),
    Fact(
        "pixie-suffrage",
        "She drew for the women's suffrage movement, producing posters and "
        "illustrations for the cause while she was making her living from "
        "book work and theatre design.",
        UNTOLD_STORY),
    Fact(
        "seventy-eight",
        "A tarot deck is 78 cards: 22 trumps and four suits of 14, which is "
        "an ordinary playing deck with an extra court card per suit and a "
        "storyline bolted on. The trumps came first as a trick-taking game in "
        "fifteenth-century Italy — fortune-telling arrived centuries later.",
        "Michael Dummett, The Game of Tarot (1980)"),
)


#: One trump at a time. Keys are `tarot.Card.key`; a test pins every one of
#: them against the deck, because a fact filed under a card that does not
#: exist is a fact nobody will ever be told.
CARD_FACTS: tuple[Fact, ...] = (

    # ------------------------------------------------------------- 0-4
    Fact("fool-cliff",
         "The Fool is the only card in the deck about to have an accident. He "
         "is stepping off a cliff edge in full sunshine, and Smith drew him "
         "mid-stride so you cannot tell whether the next moment is a fall or "
         "a first step.",
         PICTORIAL_KEY, "00-fool"),
    Fact("fool-zero",
         "The Fool is numbered zero, which is why he can go anywhere in the "
         "sequence — before the first card, after the last, or loose in the "
         "middle. Older decks left him unnumbered entirely.",
         "Michael Dummett, The Game of Tarot (1980)", "00-fool"),
    Fact("fool-rose",
         "The white rose in his hand is the one detail Waite was specific "
         "about. It is meant to be innocence rather than ignorance — the "
         "difference between not knowing yet and not caring to.",
         PICTORIAL_KEY, "00-fool"),
    Fact("fool-dog",
         "The little dog at his heels has been read as a warning, a "
         "companion, and instinct itself. Smith gave it no leash and no "
         "collar, which is the sort of decision the book does not cover.",
         PICTORIAL_KEY, "00-fool"),
    Fact("fool-joker",
         "The Fool is the ancestor of the joker in an ordinary pack of "
         "cards — the odd card that belongs to no suit and beats things it "
         "has no business beating.",
         "Michael Dummett, The Game of Tarot (1980)", "00-fool"),
    Fact("fool-bundle",
         "Everything he owns is in the small bundle on the stick over his "
         "shoulder. Smith painted it closed, so nobody has ever been able to "
         "say what is in it.",
         UNTOLD_STORY, "00-fool"),

    Fact("magician-infinity",
         "The sideways figure eight over the Magician's head is a lemniscate, "
         "the mathematician's symbol for infinity. It turns up again over the "
         "woman in Strength, and on no other card.",
         PICTORIAL_KEY, "01-magician"),
    Fact("magician-tools",
         "On his table are a cup, a coin, a sword and a wand — one of each "
         "suit in the deck. The Magician is the only figure shown holding the "
         "whole pack's worth of tools at once.",
         PICTORIAL_KEY, "01-magician"),
    Fact("magician-posture",
         "One hand points up and the other down: 'as above, so below', the "
         "old Hermetic tag. Waite specified the gesture, and Smith made it "
         "look like a man conducting rather than a man reciting.",
         PICTORIAL_KEY, "01-magician"),
    Fact("magician-garden",
         "The roses and lilies growing around him are the same two flowers "
         "printed on the backs of the 1909 deck — the pattern collectors "
         "named that edition after.",
         "Rider (London), 1909", "01-magician"),
    Fact("magician-juggler",
         "In older Italian and French decks this card is not a magician at "
         "all but a juggler or a market conjurer — a street performer with a "
         "table of cups. Waite promoted him.",
         "Michael Dummett, The Game of Tarot (1980)", "01-magician"),

    Fact("priestess-pillars",
         "The two pillars behind her are marked B and J, for Boaz and Jachin, "
         "the pillars of Solomon's Temple. She is sitting exactly between "
         "them, which is the whole card.",
         PICTORIAL_KEY, "02-high-priestess"),
    Fact("priestess-veil",
         "The veil strung between the pillars is patterned with pomegranates. "
         "It hangs behind her so that whatever is beyond it stays behind it, "
         "and the card is often read as the thing you are not being told.",
         PICTORIAL_KEY, "02-high-priestess"),
    Fact("priestess-scroll",
         "The scroll in her lap reads TORA, half hidden by her robe. Waite "
         "was careful about this and vague about it in the same breath, which "
         "is very much his register.",
         PICTORIAL_KEY, "02-high-priestess"),
    Fact("priestess-popess",
         "In the oldest decks this card is the Popess — a female pope, which "
         "was scandalous enough that some printers replaced her with Juno "
         "rather than print it.",
         "Michael Dummett, The Game of Tarot (1980)", "02-high-priestess"),
    Fact("priestess-moon",
         "There is a crescent moon at her feet, and the moon turns up again "
         "on her crown. Smith kept the water behind the veil the same blue as "
         "the moon, which is the kind of joining-up the book never asked for.",
         PICTORIAL_KEY, "02-high-priestess"),

    Fact("empress-venus",
         "The heart-shaped shield beside the Empress carries the symbol for "
         "Venus. It is the only planetary sign drawn plainly on a trump.",
         PICTORIAL_KEY, "03-empress"),
    Fact("empress-wheat",
         "She is sitting in a field of ripe wheat with a forest and a "
         "waterfall behind her — the only trump whose setting is simply "
         "abundance, with nothing symbolic to decode.",
         PICTORIAL_KEY, "03-empress"),
    Fact("empress-cushions",
         "Smith seated her on cushions out of doors, which no one had asked "
         "for. It is a small joke about comfort that the earlier decks, with "
         "their stiff thrones, do not make.",
         UNTOLD_STORY, "03-empress"),
    Fact("empress-stars",
         "The twelve stars in her crown are usually read as the zodiac, and "
         "the same twelve appear on the Virgin in a great deal of Christian "
         "painting. Waite borrowed freely and rarely said so.",
         PICTORIAL_KEY, "03-empress"),

    Fact("emperor-rams",
         "Four rams' heads are carved into the Emperor's stone throne, for "
         "Aries — the sign of war, and about as subtle as this card gets.",
         PICTORIAL_KEY, "04-emperor"),
    Fact("emperor-mountains",
         "The mountains behind him are bare rock and the river at their foot "
         "is thin. Set it beside the Empress's wheat field and the pair reads "
         "as a single argument in two pictures.",
         PICTORIAL_KEY, "04-emperor"),
    Fact("emperor-armour",
         "He is wearing full armour under his robe. Smith let it show at the "
         "wrists and ankles, so the card is a man dressed for a fight sitting "
         "very still.",
         PICTORIAL_KEY, "04-emperor"),
    Fact("emperor-ankh",
         "The sceptre in his right hand is an ankh, the Egyptian sign for "
         "life — a piece of the Golden Dawn's Egypt-flavoured borrowing, "
         "which ran through the whole society.",
         UNTOLD_STORY, "04-emperor"),

    # ------------------------------------------------------------- 5-10
    Fact("hierophant-keys",
         "Two crossed keys lie at the Hierophant's feet, and he is the only "
         "figure in the deck with somebody else in the frame receiving "
         "instruction. The card is about being taught rather than knowing.",
         PICTORIAL_KEY, "05-hierophant"),
    Fact("hierophant-pope",
         "This card was simply the Pope for four hundred years. Waite renamed "
         "him with a Greek word for a priest of the mysteries, which let an "
         "occult society print him without printing a pope.",
         "Michael Dummett, The Game of Tarot (1980)", "05-hierophant"),
    Fact("hierophant-triple",
         "His crown and staff are both in three tiers, echoing the papal "
         "tiara. Smith drew the tiers slightly uneven, which no reproduction "
         "has ever tidied up.",
         PICTORIAL_KEY, "05-hierophant"),

    Fact("lovers-angel",
         "The angel over the Lovers is Raphael, and the couple below are Adam "
         "and Eve — she stands by a tree with a serpent in it, he by a tree "
         "of flames. Neither is looking at the other.",
         PICTORIAL_KEY, "06-lovers"),
    Fact("lovers-choice",
         "Older decks show a young man choosing between two women, with cupid "
         "aiming overhead: the card was about a decision before it was about "
         "a couple. Waite's version moved it to the garden.",
         "Michael Dummett, The Game of Tarot (1980)", "06-lovers"),
    Fact("lovers-flames",
         "The tree behind the man has twelve flames on it, one for each sign "
         "of the zodiac. It is the sort of countable detail Smith put in "
         "knowing almost nobody would count.",
         PICTORIAL_KEY, "06-lovers"),

    Fact("chariot-sphinxes",
         "The Chariot is pulled by two sphinxes, one black and one white, and "
         "they are lying down. Smith drew a vehicle in motion whose engines "
         "are at rest, and readers have argued about it ever since.",
         PICTORIAL_KEY, "07-chariot"),
    Fact("chariot-noreins",
         "There are no reins. The driver holds only a wand, so whatever is "
         "steering the thing is not his hands — which is either the point of "
         "the card or an oversight, depending on who you ask.",
         PICTORIAL_KEY, "07-chariot"),
    Fact("chariot-canopy",
         "The canopy over him is painted with stars, turning the chariot into "
         "a small portable night sky. The same star-field turns up on the "
         "High Priestess's veil.",
         PICTORIAL_KEY, "07-chariot"),

    Fact("strength-eight",
         "Strength and Justice are swapped in this deck. Traditionally "
         "Justice is 8 and Strength 11; Waite exchanged them to fit the "
         "Golden Dawn's astrological ordering, and nearly every modern deck "
         "has copied him without saying why.",
         PICTORIAL_KEY, "08-strength"),
    Fact("strength-lemniscate",
         "The infinity sign floats over her head, the same one the Magician "
         "has. They are the only two cards that carry it, which quietly pairs "
         "them across ten cards of the sequence.",
         PICTORIAL_KEY, "08-strength"),
    Fact("strength-gentle",
         "She is closing the lion's mouth with her bare hands and her "
         "expression is mild. Smith drew no struggle at all — the lion looks "
         "as though it is being persuaded.",
         PICTORIAL_KEY, "08-strength"),

    Fact("hermit-lantern",
         "There is a six-pointed star inside the Hermit's lantern. It is a "
         "hexagram, the Seal of Solomon, and it is the only light in the "
         "picture.",
         PICTORIAL_KEY, "09-hermit"),
    Fact("hermit-summit",
         "He is standing on snow at the top of a mountain, facing down. Every "
         "other figure in the deck is on level ground or seated; he is the "
         "only one who has climbed.",
         PICTORIAL_KEY, "09-hermit"),
    Fact("hermit-time",
         "In older decks this card is Time, an old man with an hourglass "
         "rather than a lamp. Swapping the hourglass for a light changed the "
         "card from something running out to something being carried.",
         "Michael Dummett, The Game of Tarot (1980)", "09-hermit"),

    Fact("wheel-letters",
         "The letters around the wheel spell TARO — or ROTA, Latin for wheel, "
         "depending where you start reading. They are interleaved with the "
         "Hebrew letters of the divine name, so the rim says two things at "
         "once.",
         PICTORIAL_KEY, "10-wheel-of-fortune"),
    Fact("wheel-corners",
         "The four winged creatures in the corners — man, eagle, bull, lion — "
         "are the evangelists' symbols, and they are all reading books. They "
         "appear again on The World, which is the only other card that has "
         "them.",
         PICTORIAL_KEY, "10-wheel-of-fortune"),
    Fact("wheel-sphinx",
         "A sphinx sits on top of the wheel holding a sword, a snake slides "
         "down one side, and a jackal-headed figure rises up the other. It is "
         "the busiest card in the deck.",
         PICTORIAL_KEY, "10-wheel-of-fortune"),

    # ------------------------------------------------------------ 11-15
    Fact("justice-eleven",
         "Justice is numbered 11 here and 8 almost everywhere older. Waite "
         "swapped her with Strength; if you ever see a deck where Justice is "
         "8, it is following the older order rather than making a mistake.",
         PICTORIAL_KEY, "11-justice"),
    Fact("justice-eyes",
         "She is not blindfolded. Every courthouse statue of Justice is, and "
         "Smith drew her looking straight out of the card at whoever is "
         "reading it.",
         PICTORIAL_KEY, "11-justice"),
    Fact("justice-sword",
         "The sword is held upright and the scales hang level, and she has "
         "one of each in one hand apiece. Neither is doing anything yet.",
         PICTORIAL_KEY, "11-justice"),

    Fact("hanged-man-four",
         "His crossed legs make a figure four, and he hangs by one ankle from "
         "a living tree rather than a gallows. The pose is deliberate on both "
         "counts.",
         PICTORIAL_KEY, "12-hanged-man"),
    Fact("hanged-man-halo",
         "There is a halo of light around his head. Whatever is happening to "
         "him, the card is clear that he is not being punished.",
         PICTORIAL_KEY, "12-hanged-man"),
    Fact("hanged-man-traitor",
         "In fifteenth-century Italy, hanging a man upside down by one foot "
         "in a painting was a real public punishment for traitors and "
         "debtors, called a pittura infamante. The card started as that "
         "picture.",
         "Michael Dummett, The Game of Tarot (1980)", "12-hanged-man"),
    Fact("hanged-man-calm",
         "Turn the card upside down and he is a man standing comfortably with "
         "one knee bent. Smith composed it so that reversing it does not "
         "distress him.",
         PICTORIAL_KEY, "12-hanged-man"),

    Fact("death-unnamed",
         "In many old decks the thirteenth trump is the only one printed with "
         "no name on it at all — a superstition about writing the word down. "
         "Waite gave it a title.",
         "Michael Dummett, The Game of Tarot (1980)", "13-death"),
    Fact("death-sunrise",
         "There is a sunrise between two towers on the horizon behind the "
         "rider, and it is the same pair of towers that stand on The Moon. "
         "The card with the skeleton on it contains the deck's clearest "
         "dawn.",
         PICTORIAL_KEY, "13-death"),
    Fact("death-bishop",
         "A bishop in full mitre stands in the rider's path with his hands "
         "together, and a child and a young woman kneel beside him. The king "
         "is already down. Rank is the joke.",
         PICTORIAL_KEY, "13-death"),
    Fact("death-rose",
         "The black banner the rider carries is charged with a white five-"
         "petalled rose — the same rose the Fool is holding on card zero.",
         PICTORIAL_KEY, "13-death"),

    Fact("temperance-foot",
         "The angel has one foot on land and one in the water. It is the only "
         "figure in the deck standing in two elements at once.",
         PICTORIAL_KEY, "14-temperance"),
    Fact("temperance-cups",
         "The water pours between the two cups at an angle no liquid would "
         "actually take. Smith drew the impossible pour on purpose; it is the "
         "one openly magical act in the deck.",
         PICTORIAL_KEY, "14-temperance"),
    Fact("temperance-irises",
         "Yellow irises grow at the water's edge, and there is a crown of "
         "light on the road up to the mountains behind. The path and the "
         "flowers are the same yellow.",
         PICTORIAL_KEY, "14-temperance"),

    Fact("devil-copy",
         "The Devil is posed exactly as the Lovers are — a man and a woman "
         "under a winged figure — with the tree and the flames turned into "
         "tails. The two cards are the same composition, and that is the "
         "argument.",
         PICTORIAL_KEY, "15-devil"),
    Fact("devil-chains",
         "The chains around the couple's necks are loose enough to lift off "
         "over their heads. Smith made the escape visible and left them "
         "standing there.",
         PICTORIAL_KEY, "15-devil"),
    Fact("devil-inverted",
         "The torch in his left hand is held pointing down, and the pentagram "
         "over his head is upside down. Both are ordinary symbols turned over "
         "rather than different symbols.",
         PICTORIAL_KEY, "15-devil"),

    # ------------------------------------------------------------ 16-21
    Fact("tower-crown",
         "The thing falling off the top of the Tower is a crown, knocked off "
         "by the lightning. Whatever the building is, the card begins by "
         "removing who was in charge of it.",
         PICTORIAL_KEY, "16-tower"),
    Fact("tower-babel",
         "The card is usually traced to the Tower of Babel, and in some old "
         "decks it is called the House of God or simply The Lightning. It has "
         "had more names than any other trump.",
         "Michael Dummett, The Game of Tarot (1980)", "16-tower"),
    Fact("tower-flames",
         "Twenty-two flames fall through the air around the tower — one for "
         "each trump in the deck, on the card about everything coming apart.",
         PICTORIAL_KEY, "16-tower"),
    Fact("tower-two",
         "Two figures are falling, one crowned and one not, and they are "
         "drawn in the same posture. It is the only card where rank makes no "
         "difference to what is happening.",
         PICTORIAL_KEY, "16-tower"),

    Fact("star-eight",
         "One large eight-pointed star with seven smaller ones around it, and "
         "the woman below is pouring water with both hands — one jug into the "
         "pool, one onto the land. Nothing in the picture is being kept.",
         PICTORIAL_KEY, "17-star"),
    Fact("star-bird",
         "There is a bird in the tree on the right, usually called an ibis — "
         "the bird of Thoth, and a piece of the Golden Dawn's Egypt showing "
         "through.",
         PICTORIAL_KEY, "17-star"),
    Fact("star-naked",
         "She is the only entirely unclothed figure on a trump other than the "
         "dancer on The World, and she is kneeling with one knee on the "
         "ground and one foot on the water.",
         PICTORIAL_KEY, "17-star"),

    Fact("moon-crayfish",
         "A crayfish is crawling out of the pool at the bottom of The Moon. "
         "Smith put it there facing the viewer, at the start of a road that "
         "runs between two towers and away into the hills.",
         PICTORIAL_KEY, "18-moon"),
    Fact("moon-dogs",
         "A dog and a wolf howl at the moon on either side of the path — the "
         "tame and the wild version of the same animal, doing the same "
         "thing.",
         PICTORIAL_KEY, "18-moon"),
    Fact("moon-face",
         "The moon has a face in profile inside a full disc, so the card "
         "shows a crescent and a full moon simultaneously. Fifteen drops of "
         "light fall from it.",
         PICTORIAL_KEY, "18-moon"),

    Fact("sun-child",
         "A naked child on a white horse rides out from behind a wall, arms "
         "open, holding a red banner. It is the only card in the deck with a "
         "child as its whole subject.",
         PICTORIAL_KEY, "19-sun"),
    Fact("sun-wall",
         "There is a garden wall behind the horse with four sunflowers "
         "growing above it, all of them turned toward the child rather than "
         "toward the sun.",
         PICTORIAL_KEY, "19-sun"),
    Fact("sun-rays",
         "The sun's rays alternate straight and wavy, twenty-one in all, and "
         "the face in the disc is looking directly down at the rider.",
         PICTORIAL_KEY, "19-sun"),

    Fact("judgement-flag",
         "The angel's trumpet carries a square banner with a cross on it, and "
         "the figures rising below stand in open coffins on a grey sea. Smith "
         "drew a family — man, woman, child — rather than a crowd.",
         PICTORIAL_KEY, "20-judgement"),
    Fact("judgement-gabriel",
         "The angel is Gabriel, and this is the card Waite was least "
         "inventive about: it is the Last Judgement more or less as any "
         "church would paint it.",
         PICTORIAL_KEY, "20-judgement"),
    Fact("judgement-arms",
         "Every risen figure has their arms up and none of them are looking "
         "at each other. They are the only crowd in the deck facing the same "
         "way.",
         PICTORIAL_KEY, "20-judgement"),

    Fact("world-corners",
         "The same four winged creatures from the Wheel of Fortune are in the "
         "corners of The World — man, eagle, bull, lion. First and last "
         "appearance bracket the deck, ten cards apart.",
         PICTORIAL_KEY, "21-world"),
    Fact("world-wreath",
         "The dancer is inside a laurel wreath bound top and bottom with red "
         "ribbon tied in a figure eight — the infinity sign again, this time "
         "as knotwork rather than a floating symbol.",
         PICTORIAL_KEY, "21-world"),
    Fact("world-last",
         "The World is the last trump and the highest card in the deck, which "
         "in the original Italian game meant simply that it beat everything "
         "else. The meaning arrived later; the ranking came first.",
         "Michael Dummett, The Game of Tarot (1980)", "21-world"),
    Fact("world-wands",
         "She holds a wand in each hand, and they are the only pair in the "
         "deck. The Magician on card one holds a single wand in the same "
         "grip.",
         PICTORIAL_KEY, "21-world"),
)


ALL: tuple[Fact, ...] = DECK_FACTS + CARD_FACTS

_BY_ID: dict[str, Fact] = {f.id.casefold(): f for f in ALL}


def by_id(fact_id: str) -> Fact | None:
    """The fact a reader cited, or None if it invented the id.

    None rather than a raise: an unresolvable id is dropped and counted the
    way an unresolvable citation is everywhere else here, because one bad
    reference must cost the fact and not the turn.

    Case- and space-insensitive, which is not politeness. `theme.keep_fact`
    matches the `tarot:` prefix case-insensitively, so a reader that shouts
    `TAROT:PIXIE-FEE` gets past the prefix check and would then miss here on
    the one difference nobody would ever debug from a dropped-fact counter.
    """
    return _BY_ID.get(fact_id.strip().casefold())


def for_card(key: str) -> tuple[Fact, ...]:
    """Everything known about one card. Empty for the 56 minors, so far."""
    return tuple(f for f in CARD_FACTS if f.card == key)


def for_reading(keys: tuple[str, ...] | list[str]) -> tuple[Fact, ...]:
    """What may be told at a table where these cards are face up.

    The deck tier first and always: it is true of every spread, and it is why
    a reading of three minors is not a reading with nothing to say. Then the
    cards, in the order they were dealt, so a reader skimming the list meets
    the table's own cards in the table's own order.

    Duplicates are impossible (a card cannot be dealt twice) but the dict
    build keeps it that way if the sampler ever changes.
    """
    out: dict[str, Fact] = {f.id: f for f in DECK_FACTS}
    for key in keys:
        for fact in for_card(key):
            out[fact.id] = fact
    return tuple(out.values())


def offer(keys: tuple[str, ...] | list[str], told: tuple[str, ...] = ()) -> str:
    """The facts, as the reader's frame message carries them.

    Prose with ids rather than JSON, for the reason `Reading.describe` gives:
    this is read by a model being asked to sound like a person, and a data
    structure invites a data-structure answer.

    `told` drops what this querent has already heard — the same list
    `theme.repeats` checks against, applied one step earlier so the reader is
    not tempted by a fact it cannot use. Belt and braces on purpose: the
    prompt asks, this narrows what is asked about, and `keep_fact` still
    checks. Returns an empty string when there is nothing left to offer, and
    the caller omits the whole section rather than printing a heading over
    nothing.
    """
    seen = {t.strip() for t in told}
    lines = [f"- {f.id}: {f.text}" for f in for_reading(keys)
             if f.text.strip() not in seen]
    if not lines:
        return ""
    return ("\n\nTrue things you know about this deck and these cards. To "
            "tell one, put its id in the fact's `source` field as "
            "`tarot:<id>` — the exact words below are what they will read, "
            "so choose the one that belongs and let your question carry the "
            "connection. Never retell one, and never write your own.\n"
            + "\n".join(lines))
