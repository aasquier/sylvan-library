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

#: For a fact about what is actually in the picture. The strongest citation
#: this file has, because the reader is holding the evidence: every one of
#: these was checked against the committed plate in `assets/tarot/` rather
#: than recalled, which is rule 1's habit applied to a deck instead of a pool.
PLATE = "The 1909 Rider plate itself — look at the card"


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


#: The minor arcana. Aaron asked for five apiece (2026-08-18); the well is the
#: same one the trumps draw from, and the richest seam in it turns out to be
#: what Smith actually drew, because Waite's instructions ran out at card 22
#: and these 56 are hers. Every picture fact here was checked against the
#: committed plate rather than recalled.
MINOR_FACTS: tuple[Fact, ...] = (

    # ------------------------------------------------------------- wands
    Fact("wands-01-hand",
         "A hand comes out of a cloud holding a living branch — you can see "
         "where the leaves are still sprouting off it, and several have "
         "shaken loose and hang in the air. All four Aces in this deck are "
         "that same disembodied hand offering you the suit.",
         PLATE, "wands-01"),
    Fact("wands-01-titled",
         "Look at the bottom of an Ace and there is a printed banner: ACE of "
         "WANDS. The numbered cards from two to ten have no title at all, "
         "just a Roman numeral at the top — so the deck names its Aces and "
         "its court cards and leaves the middle of each suit to the picture.",
         PLATE, "wands-01"),
    Fact("wands-01-castle",
         "There is a castle on a crag in the bottom left, small enough to "
         "miss, with a river running past it. Smith put scenery under almost "
         "every Ace rather than leaving them floating.",
         PLATE, "wands-01"),
    Fact("wands-01-suits",
         "Wands are the batons or staves of the old Italian packs, and they "
         "are the ancestor of clubs in an ordinary deck of cards. Cups became "
         "hearts, swords became spades, and coins became diamonds.",
         "Michael Dummett, The Game of Tarot (1980)", "wands-01"),
    Fact("wands-01-sprouting",
         "Every wand in this suit is drawn as a rough cut branch still "
         "putting out leaves, all 14 cards of it. Smith made the suit's "
         "material alive, which is a decision no earlier deck had to make "
         "because no earlier deck drew scenes on the pips.",
         PLATE, "wands-01"),

    Fact("wands-02-globe",
         "The man is holding a globe of the world in his right hand, small "
         "enough to sit in his palm, and looking out over a harbour. It is "
         "the only globe in the deck.",
         PLATE, "wands-02"),
    Fact("wands-02-fixed",
         "One of his two wands is strapped to the wall beside him and the "
         "other is in his hand. He owns one and holds one, and the picture "
         "makes you notice which is which.",
         PLATE, "wands-02"),
    Fact("wands-02-emblem",
         "Carved into the wall on the left is a saltire of white lilies and "
         "red roses — the same two flowers printed on the backs of this "
         "deck, which collectors named the edition after.",
         PLATE, "wands-02"),
    Fact("wands-02-back",
         "He is turned away from us, and so is the figure on the Three. Smith "
         "used the back of a person far more than earlier decks did, which is "
         "a theatre habit: it points your eye where the figure is looking.",
         PLATE, "wands-02"),
    Fact("wands-02-two-lands",
         "The view splits down the middle — grey sea and a rocky shore on the "
         "left, green fields and hills on the right. He is standing between "
         "two different futures and the picture puts one under each hand.",
         PLATE, "wands-02"),

    Fact("wands-03-back",
         "This is the clearest back-view in the deck: a figure on a headland "
         "in a red robe and a green cloak, one hand on a staff, watching the "
         "water. You never see his face, so you are made to look where he is "
         "looking.",
         PLATE, "wands-03"),
    Fact("wands-03-water",
         "Ships are out on the yellow water below him — the small marks in "
         "the bay. Wands is the fire suit and this is one of the few cards in "
         "it that is mostly about the sea.",
         PLATE, "wands-03"),
    Fact("wands-03-third",
         "Two staves stand planted and he holds the third. Smith kept that "
         "arrangement from the Two and added one, so the suit reads as a "
         "sequence rather than as ten separate pictures.",
         PLATE, "wands-03"),
    Fact("wands-03-scarf",
         "His cloak is patterned in a green and yellow check that appears "
         "nowhere else in the deck. Smith had been designing and making "
         "costumes for the London stage for years before this commission.",
         f"{PLATE}; {UNTOLD_STORY}", "wands-03"),
    Fact("wands-03-height",
         "He is standing on a cliff edge with the ground falling away, which "
         "is the same footing the Fool has on card zero. Smith liked putting "
         "people where the next step matters.",
         PLATE, "wands-03"),

    Fact("wands-04-garland",
         "A swag of flowers and fruit is slung between the tops of the four "
         "staves, with ribbons tied at the outer two. It is the only card in "
         "the deck where the suit's objects have been decorated.",
         PLATE, "wands-04"),
    Fact("wands-04-castle",
         "There is a walled castle behind, with a red-roofed turret and small "
         "figures gathered at the foot of the wall — a whole village party "
         "going on in the background of a four of anything.",
         PLATE, "wands-04"),
    Fact("wands-04-bouquets",
         "Two figures under the garland are holding bouquets up over their "
         "heads. Nobody in the picture is looking at the wands, which are "
         "simply the frame the celebration is happening inside.",
         PLATE, "wands-04"),
    Fact("wands-04-front",
         "The four staves stand in front of everything, cutting across the "
         "scene like the posts of a canopy you are standing under. Smith put "
         "the viewer inside the card rather than in front of it.",
         PLATE, "wands-04"),
    Fact("wands-04-yellow",
         "The sky is flat yellow, the same yellow as the Three and the "
         "Seven's ground. The 1909 printing had a limited palette, and its "
         "yellows carry an enormous amount of the deck's weather.",
         PLATE, "wands-04"),

    Fact("wands-05-brawl",
         "Five young men are swinging staves at each other on broken ground, "
         "and not one of them is looking at the same thing. It is the "
         "busiest fight in the deck and nobody appears to be winning.",
         PLATE, "wands-05"),
    Fact("wands-05-clothes",
         "Every one of the five is dressed differently — a green tunic, a red "
         "one, a checked shirt, a blue jerkin, striped hose. Smith kept them "
         "individual rather than drawing one boy five times.",
         PLATE, "wands-05"),
    Fact("wands-05-noharm",
         "Nobody is hurt, nobody is bleeding, and no staff has landed. Read "
         "beside the Nine, where a man stands bandaged, it is a picture of a "
         "scuffle rather than a battle.",
         PLATE, "wands-05"),
    Fact("wands-05-mine",
         "Waite's instructions ran out after the 22 trumps, so scenes like "
         "this one are Smith's invention. The 56 minors are the half of the "
         "deck nobody told her what to draw, and they are the half that made "
         "it famous.",
         f"{PICTORIAL_KEY}; {UNTOLD_STORY}", "wands-05"),
    Fact("wands-05-sky",
         "The sky behind them is empty pale blue with no horizon line at all "
         "— the ground simply stops. Smith left out the landscape on the one "
         "card where the people are too busy to see it.",
         PLATE, "wands-05"),

    Fact("wands-06-laurel",
         "The rider wears a laurel wreath and there is a second one tied to "
         "his staff with a red ribbon. Two crowns for one man, and the deck "
         "does not do that anywhere else.",
         PLATE, "wands-06"),
    Fact("wands-06-heads",
         "Look behind him and there are other heads and other staves at the "
         "edge of the frame, walking. The card is a procession, and he is the "
         "only one on a horse.",
         PLATE, "wands-06"),
    Fact("wands-06-cloth",
         "The horse is draped in a green and white cloth with a fringe, and "
         "you can see the harness is decorated. Smith dressed the animal as "
         "carefully as the man.",
         PLATE, "wands-06"),
    Fact("wands-06-horses",
         "White horses turn up on the Sun, on Death and here, and each rider "
         "means something different by one. It is the deck's most reused "
         "animal.",
         PLATE, "wands-06"),
    Fact("wands-06-faces",
         "His face is in profile and calm, and none of the followers' faces "
         "are finished. In a card about being cheered, only the man being "
         "cheered has features.",
         PLATE, "wands-06"),

    Fact("wands-07-boots",
         "He is wearing two different shoes. One foot has a boot and the "
         "other something lower and lighter, and it has been argued about for "
         "a century — a slip, a joke, or a man who dressed in a hurry to "
         "defend the hill.",
         PLATE, "wands-07"),
    Fact("wands-07-highground",
         "He is above the six staves coming at him from below the frame, so "
         "you never see who is holding them. Smith kept the attackers out of "
         "the picture entirely.",
         PLATE, "wands-07"),
    Fact("wands-07-six",
         "Count them: six staves rising, and the seventh is in his hands. The "
         "card's number includes his own weapon, which is not how the other "
         "cards in the suit count.",
         PLATE, "wands-07"),
    Fact("wands-07-edge",
         "There is water at the bottom left corner, so the ground he is "
         "standing on runs out behind him. He has nowhere to retreat to and "
         "the picture says so quietly.",
         PLATE, "wands-07"),
    Fact("wands-07-face",
         "His expression is the most directly worried face in the suit, and "
         "he is looking slightly past the viewer rather than at us. Smith "
         "almost never draws a figure looking straight out.",
         PLATE, "wands-07"),

    Fact("wands-08-empty",
         "There is no person on this card at all. Eight staves fly across the "
         "sky over a river and a hill, and that is the whole picture — one of "
         "the very few cards in the deck with nobody in it.",
         PLATE, "wands-08"),
    Fact("wands-08-angle",
         "All eight run parallel on the same diagonal, dropping left to "
         "right, with their leaves streaming behind them. Smith drew speed "
         "with nothing but repetition and a slope.",
         PLATE, "wands-08"),
    Fact("wands-08-landing",
         "They are angled down toward the bottom right, not up. Whatever was "
         "thrown is coming in to land rather than setting out.",
         PLATE, "wands-08"),
    Fact("wands-08-house",
         "There is a small hill with a building on it at the bottom left and "
         "a river across the foot of the card, drawn in about a dozen strokes "
         "— the only settled thing in a card about things in flight.",
         PLATE, "wands-08"),
    Fact("wands-08-plain",
         "This is the plainest design in the suit, and it is the one that "
         "reproduces best at a small size. Smith was a working illustrator "
         "who knew what survives shrinking.",
         f"{PLATE}; {UNTOLD_STORY}", "wands-08"),

    Fact("wands-09-bandage",
         "The man has a bandage wrapped round his head. He is the only "
         "visibly wounded person in the whole deck, on a card where nothing "
         "is currently happening to him.",
         PLATE, "wands-09"),
    Fact("wands-09-fence",
         "Eight staves stand behind him in a line like a palisade, and the "
         "ninth is in his hands. He has built a fence out of the suit and is "
         "standing in front of it.",
         PLATE, "wands-09"),
    Fact("wands-09-looking",
         "He is turned to the side and looking off past the edge of the card, "
         "not at the wands and not at us. Whatever he is watching for is "
         "outside the picture.",
         PLATE, "wands-09"),
    Fact("wands-09-hills",
         "Behind the palisade there is a low range of hills and nothing "
         "else — no army, no attacker, no smoke. The threat this card is "
         "about is not in it.",
         PLATE, "wands-09"),
    Fact("wands-09-grip",
         "His hands are wrapped round the staff at chest height, holding it "
         "upright rather than levelled. It is a guard, not an attack, and "
         "Smith drew the difference.",
         PLATE, "wands-09"),

    Fact("wands-10-face",
         "You cannot see his face. He is bent forward under all ten staves "
         "with the bundle in front of his head, and Smith hid the one thing "
         "the rest of the deck always shows you.",
         PLATE, "wands-10"),
    Fact("wands-10-town",
         "There is a house with a red roof and some trees ahead of him on the "
         "right. He is nearly there, which is either a comfort or the joke, "
         "depending on the reading.",
         PLATE, "wands-10"),
    Fact("wands-10-splay",
         "The ten staves splay out at the top like a fan, so the load is much "
         "wider than the man. Smith made the burden look awkward rather than "
         "just heavy.",
         PLATE, "wands-10"),
    Fact("wands-10-grip",
         "He is carrying all ten in his two arms with none over a shoulder "
         "and none dropped. Whatever else is true, he has not put any of it "
         "down.",
         PLATE, "wands-10"),
    Fact("wands-10-last",
         "The tens are the last of the numbered cards, and in this suit the "
         "sequence runs from one branch offered by a hand to ten of them "
         "carried by one man. Smith gave the suit an arc.",
         PLATE, "wands-10"),

    Fact("wands-11-salamander",
         "The Page's yellow tunic is covered in salamanders — the creature "
         "that was believed to live in fire. They run through all four Wands "
         "court cards, which is how the suit says it is the fire suit "
         "without a single flame.",
         PLATE, "wands-11"),
    Fact("wands-11-pyramids",
         "There are pyramids in the desert behind him, and behind the Knight "
         "as well. The Golden Dawn's Egypt shows up in the scenery of this "
         "suit more than anywhere else in the deck.",
         f"{PLATE}; {UNTOLD_STORY}", "wands-11"),
    Fact("wands-11-court",
         "Page, Knight, Queen, King — four court cards per suit where an "
         "ordinary pack has three. The Knight is the extra one, and it is "
         "what makes a tarot deck 78 cards instead of 56.",
         "Michael Dummett, The Game of Tarot (1980)", "wands-11"),
    Fact("wands-11-hat",
         "His hat has a red feather standing straight up out of it, drawn "
         "like a small flame. Smith put the suit's element on his head "
         "without drawing fire.",
         PLATE, "wands-11"),
    Fact("wands-11-looking-up",
         "He is holding the staff at arm's length and looking up its length "
         "at the leaves. Every court card in this suit is doing something "
         "with the wand; nobody is simply holding it.",
         PLATE, "wands-11"),

    Fact("wands-12-rearing",
         "The horse is up on its hind legs with both front hooves off the "
         "ground. It is the only horse in the deck that is not standing "
         "still or walking.",
         PLATE, "wands-12"),
    Fact("wands-12-plume",
         "A long orange plume streams off the back of his helmet, and more "
         "orange shows at his elbows. Smith gave the fire suit its colour in "
         "the trim rather than the ground.",
         PLATE, "wands-12"),
    Fact("wands-12-salamanders",
         "His surcoat carries the same salamanders as the Page's tunic — and "
         "if you look closely, some of them are drawn with their tails in "
         "their mouths and some are not, which is the old sign for a thing "
         "completed or not yet.",
         PLATE, "wands-12"),
    Fact("wands-12-armour",
         "He is in full plate under the surcoat, the only Wands court figure "
         "in armour. The other three are in cloth.",
         PLATE, "wands-12"),
    Fact("wands-12-pyramids",
         "The same three pyramids from the Page's card sit on the horizon "
         "behind him. The two of them are in the same place, at different "
         "speeds.",
         PLATE, "wands-12"),

    Fact("wands-13-cat",
         "There is a black cat sitting at the Queen's feet, facing straight "
         "out of the card. It is the only cat in all 78, and the only animal "
         "in the deck looking directly at the reader.",
         PLATE, "wands-13"),
    Fact("wands-13-sunflower",
         "She is holding a staff in one hand and a sunflower in the other, "
         "and there is another sunflower worked into her throne. She is the "
         "only figure in the deck holding a flower that is not a rose or a "
         "lily.",
         PLATE, "wands-13"),
    Fact("wands-13-lions",
         "Lions are carved on both arms of her throne and on the cloth "
         "hanging behind it, facing outward. The same lion turns up on "
         "Strength, being closed rather than displayed.",
         PLATE, "wands-13"),
    Fact("wands-13-open",
         "She is the only court figure in this suit sitting square to the "
         "viewer with her knees apart under the robe. Every other seated "
         "royal in the deck is angled.",
         PLATE, "wands-13"),
    Fact("wands-13-crown",
         "Her crown is worked with leaves rather than points, matching the "
         "sprouting staff beside her. Smith kept the suit's living wood on "
         "her head.",
         PLATE, "wands-13"),

    Fact("wands-14-salamander",
         "There is a live salamander on the ground beside the King's foot, "
         "small and easy to miss, drawn in a few strokes. His throne and "
         "cloak are covered in them; that one is real.",
         PLATE, "wands-14"),
    Fact("wands-14-profile",
         "He is seated in profile, turned away, mid-movement — the least "
         "settled king in the deck. The other three sit square and face you.",
         PLATE, "wands-14"),
    Fact("wands-14-lions",
         "The cloth behind his throne carries both lions and salamanders "
         "together, which no other court card does. The Queen has lions "
         "alone; he has both.",
         PLATE, "wands-14"),
    Fact("wands-14-staff",
         "His staff rests on the ground and leans away from him rather than "
         "standing upright, held loosely near the top. He is the only royal "
         "not gripping the suit.",
         PLATE, "wands-14"),
    Fact("wands-14-nothrone",
         "There is almost no background at all — a bare grey step, a hanging, "
         "and empty sky. Smith gave the fire suit's king less scenery than "
         "she gave its Page.",
         PLATE, "wands-14"),

    # -------------------------------------------------------------- cups
    Fact("cups-01-dove",
         "A dove is coming down into the cup with a disc in its beak marked "
         "with a cross — a communion wafer. It is the most openly Christian "
         "image in the deck, sitting in a pack an occult society published.",
         PLATE, "cups-01"),
    Fact("cups-01-letter",
         "There is a large letter on the front of the chalice. Read one way "
         "up it is a W, and turned over it is an M, and nobody has ever "
         "settled which Smith meant.",
         PLATE, "cups-01"),
    Fact("cups-01-five",
         "Five streams pour out of a cup with four spouts, and the water "
         "falls in separate drops all the way down. Smith drew about "
         "twenty-six of them, individually.",
         PLATE, "cups-01"),
    Fact("cups-01-pond",
         "Underneath is a pond crowded with lily pads and open water lilies. "
         "The suit's element is not just implied here — the card's whole "
         "bottom edge is water.",
         PLATE, "cups-01"),
    Fact("cups-01-suit",
         "Cups are the ancestor of hearts in an ordinary pack, and in the "
         "oldest Italian decks they were literally drinking vessels stacked "
         "in a row. Giving them scenes instead is the innovation this deck is "
         "famous for.",
         "Michael Dummett, The Game of Tarot (1980)", "cups-01"),

    Fact("cups-02-caduceus",
         "Above the couple floats a caduceus — two snakes twined round a rod "
         "— topped with a winged lion's head. It is the strangest object in "
         "the minor arcana and Waite specified it exactly.",
         f"{PLATE}; {PICTORIAL_KEY}", "cups-02"),
    Fact("cups-02-exchange",
         "They are each holding a cup and reaching toward the other's. The "
         "card is the moment before the exchange rather than after it.",
         PLATE, "cups-02"),
    Fact("cups-02-wreaths",
         "She wears a wreath of leaves and he a wreath of red flowers, and "
         "his tunic is scattered with the same flowers. Smith dressed them as "
         "a matched pair without making them look alike.",
         PLATE, "cups-02"),
    Fact("cups-02-house",
         "There is a small house with a red roof on a green rise behind them, "
         "drawn tiny. Almost every happy card in this suit has a house in the "
         "background somewhere.",
         PLATE, "cups-02"),
    Fact("cups-02-ground",
         "The ground under them is bare and level, with no path and no "
         "furniture. Smith cleared the stage so the only thing happening is "
         "the two of them.",
         PLATE, "cups-02"),

    Fact("cups-03-dance",
         "Three women are dancing in a ring with their cups raised until the "
         "rims nearly touch overhead. It is the only round dance in the deck.",
         PLATE, "cups-03"),
    Fact("cups-03-harvest",
         "At their feet are pumpkins, grapes and apples lying loose on the "
         "ground. This is the harvest card of the suit and Smith paid the "
         "fruit as much attention as the faces.",
         PLATE, "cups-03"),
    Fact("cups-03-three-robes",
         "One is in white, one in orange over deep red, one in cream. Their "
         "robes are three different weights of cloth, which is a costume "
         "designer's distinction rather than an illustrator's.",
         f"{PLATE}; {UNTOLD_STORY}", "cups-03"),
    Fact("cups-03-middle",
         "The woman in the middle has her back to us and the other two are "
         "turned in. You are standing outside a circle that has already "
         "closed.",
         PLATE, "cups-03"),
    Fact("cups-03-feet",
         "Look at the feet: bare, sandalled and shod, one of each. Smith kept "
         "the three of them individual right down to the ground.",
         PLATE, "cups-03"),

    Fact("cups-04-arms",
         "He is sitting under a tree with his arms folded and his legs "
         "crossed — closed at both ends. Smith made refusal a posture rather "
         "than an expression.",
         PLATE, "cups-04"),
    Fact("cups-04-fourth",
         "A hand comes out of a cloud offering a fourth cup, exactly like the "
         "hand on the Ace. Three already stand in the grass in front of him "
         "and he is not looking at any of them.",
         PLATE, "cups-04"),
    Fact("cups-04-tree",
         "The tree he is leaning against grows straight up the middle of the "
         "card and out of the top of the frame. It splits the picture into "
         "the offer and the man refusing it.",
         PLATE, "cups-04"),
    Fact("cups-04-eyes",
         "His eyes are open and aimed at the ground in front of him — he is "
         "not asleep, which is the reading the posture invites. Smith drew "
         "the pupils.",
         PLATE, "cups-04"),
    Fact("cups-04-cloud",
         "The cloud that hands him the cup is the only thing in the sky. In a "
         "suit full of scenery, this card has almost none.",
         PLATE, "cups-04"),

    Fact("cups-05-black",
         "The figure is a solid black column from shoulder to ankle — the "
         "largest single area of flat black in the deck. Smith turned "
         "mourning into a shape.",
         PLATE, "cups-05"),
    Fact("cups-05-three-two",
         "Three cups lie spilled in front of him and two still stand upright "
         "behind him. He is looking at the three, and the picture makes very "
         "sure you can see the two.",
         PLATE, "cups-05"),
    Fact("cups-05-bridge",
         "There is a river with a bridge over it leading to a castle on the "
         "far bank. The way out of the card is drawn in, behind his back.",
         PLATE, "cups-05"),
    Fact("cups-05-sky",
         "The sky is flat grey and takes up over half the card, with no "
         "horizon feature at all. It is the bleakest surface in the deck and "
         "it was made by leaving the paper nearly bare.",
         PLATE, "cups-05"),
    Fact("cups-05-spill",
         "The spilled wine is drawn as small red and blue pools around the "
         "fallen cups. Two different liquids came out of three cups, and "
         "nobody has explained it.",
         PLATE, "cups-05"),

    Fact("cups-06-flowers",
         "Every one of the six cups has a white five-petalled flower growing "
         "out of it. Nothing is being drunk on this card; the cups have "
         "become planters.",
         PLATE, "cups-06"),
    Fact("cups-06-children",
         "Two children stand among them, the taller in a red hood handing a "
         "cup down to the smaller. It is the only card in the deck where one "
         "person gives another something without ceremony.",
         PLATE, "cups-06"),
    Fact("cups-06-guard",
         "In the background a figure with a halberd is walking away up a "
         "path, out of the picture. Smith put an adult in the scene and sent "
         "him off.",
         PLATE, "cups-06"),
    Fact("cups-06-shield",
         "There is a stone plinth on the left carrying a shield with a saltire "
         "— a diagonal cross — on it. The same X shape turns up on the "
         "heraldry of the Two of Wands' wall.",
         PLATE, "cups-06"),
    Fact("cups-06-yellow",
         "The whole courtyard is washed in the deck's warm yellow, walls and "
         "ground alike, with the sky the only cool thing in it. It is the "
         "most golden card in the suit.",
         PLATE, "cups-06"),

    Fact("cups-07-seven",
         "Seven cups float in cloud, and each holds something different: a "
         "head, a veiled figure with light coming off it, a snake, a castle, "
         "a heap of jewels, a laurel wreath and a dragon. It is the most "
         "crowded minor card in the deck.",
         PLATE, "cups-07"),
    Fact("cups-07-skull",
         "The cup under the laurel wreath has a skull drawn on the bowl "
         "itself. The prize and the warning are on the same object.",
         PLATE, "cups-07"),
    Fact("cups-07-silhouette",
         "The man looking up at them is a flat black silhouette with no face "
         "and no detail — the only figure in the deck drawn that way. You "
         "cannot tell anything about who is choosing.",
         PLATE, "cups-07"),
    Fact("cups-07-veiled",
         "The shrouded shape in the middle cup is drawn glowing, with rays "
         "coming off the cloth, and its face is covered. Smith left the best "
         "thing on offer unidentifiable.",
         PLATE, "cups-07"),
    Fact("cups-07-reach",
         "His arm is out and his hand is open, but it is not near any of "
         "them. The card catches him before the choice.",
         PLATE, "cups-07"),

    Fact("cups-08-moon",
         "The moon in the sky is a crescent drawn inside a full disc with a "
         "face in profile — the exact device from The Moon trump. Smith reused "
         "her own moon rather than drawing a new one.",
         PLATE, "cups-08"),
    Fact("cups-08-gap",
         "The eight cups are stacked five along the bottom and three on top, "
         "with a deliberate gap in the upper row. Something has been taken "
         "out of the arrangement and the hole is the point.",
         PLATE, "cups-08"),
    Fact("cups-08-away",
         "He is walking away from us into the hills with a staff, and we see "
         "only his back and one turned cheek. He leaves by the top of the "
         "card, which is the least usual exit in the deck.",
         PLATE, "cups-08"),
    Fact("cups-08-red",
         "His cloak and boots are the only strong red on the card, against "
         "green hills and grey water. Smith made the one moving thing the one "
         "warm thing.",
         PLATE, "cups-08"),
    Fact("cups-08-water",
         "He is walking along a shoreline, so the whole journey runs beside "
         "water. In the suit of cups, leaving is drawn as going up out of it.",
         PLATE, "cups-08"),

    Fact("cups-09-counter",
         "The nine cups stand on a curved counter draped in blue cloth, "
         "arranged in an arc behind him. It reads like a shop display or a "
         "bar, and he is sitting in front of it.",
         PLATE, "cups-09"),
    Fact("cups-09-arms",
         "His arms are folded exactly like the man on the Four of Cups, and "
         "his face is the opposite. Same posture, completely different card.",
         PLATE, "cups-09"),
    Fact("cups-09-bench",
         "He is on a small low wooden bench, much too plain for the display "
         "behind him. Smith seated the deck's most satisfied man on almost "
         "nothing.",
         PLATE, "cups-09"),
    Fact("cups-09-yellow",
         "The background is flat unbroken yellow, top to bottom, with no "
         "scenery at all. It is the boldest colour field in the deck.",
         PLATE, "cups-09"),
    Fact("cups-09-hat",
         "He wears a red cap with a long tail hanging down the back, and the "
         "same hat shape turns up on the Two of Wands. Smith had a small "
         "wardrobe and reused it.",
         PLATE, "cups-09"),

    Fact("cups-10-rainbow",
         "The ten cups sit inside a rainbow arching right across the sky. It "
         "is the only rainbow in the deck.",
         PLATE, "cups-10"),
    Fact("cups-10-four",
         "There are four people: a couple with their arms round each other "
         "and their free arms flung up, and two children dancing hand in hand "
         "beside them. Nobody is looking at the cups.",
         PLATE, "cups-10"),
    Fact("cups-10-backs",
         "All four have their backs to us. In the deck's happiest card you "
         "cannot see a single face.",
         PLATE, "cups-10"),
    Fact("cups-10-house",
         "A house with a red roof stands among trees on the right, and a "
         "river runs across the field. It is the same red roof from the Two "
         "and the Ten of Wands.",
         PLATE, "cups-10"),
    Fact("cups-10-children",
         "The two children are drawn mid-step with their feet off the ground, "
         "which almost nothing else in the deck is. Smith could draw "
         "movement and mostly chose not to.",
         PLATE, "cups-10"),

    Fact("cups-11-fish",
         "A fish is rising out of the Page's cup and looking him in the face. "
         "It is the only card in the deck where an animal comes out of an "
         "object.",
         PLATE, "cups-11"),
    Fact("cups-11-hat",
         "His hat is a soft rolled cap with a long tail of cloth hanging off "
         "it, and it is the most elaborate headgear on any Page. Smith had "
         "made hats for the stage.",
         f"{PLATE}; {UNTOLD_STORY}", "cups-11"),
    Fact("cups-11-lotus",
         "His tunic is patterned all over with lotus flowers, and the sea is "
         "drawn behind him in flat wavy bands. Every Cups court card has "
         "water in it somewhere.",
         PLATE, "cups-11"),
    Fact("cups-11-calm",
         "He is holding the cup out at arm's length and looking at the fish "
         "quite calmly, with his other hand on his hip. Nobody in this deck "
         "is startled by anything.",
         PLATE, "cups-11"),
    Fact("cups-11-pink",
         "The pink and blue of his costume appear together nowhere else in "
         "the 78. The 1909 printing had a narrow palette and Smith spent an "
         "unusual amount of it here.",
         PLATE, "cups-11"),

    Fact("cups-12-wings",
         "There are wings on his helmet and wings on his heels. The Knight of "
         "Cups is dressed as Hermes, and he is the only winged human figure "
         "in the deck.",
         PLATE, "cups-12"),
    Fact("cups-12-walking",
         "His horse is walking, one hoof lifted, with its head down. Set it "
         "beside the Knight of Wands rearing on the other side of the deck "
         "and the two suits introduce themselves.",
         PLATE, "cups-12"),
    Fact("cups-12-level",
         "He carries the cup out in front of him, perfectly level, like "
         "something he has been asked not to spill. No other court figure "
         "holds the suit that carefully.",
         PLATE, "cups-12"),
    Fact("cups-12-fish",
         "His surcoat is covered in fish, the way the Wands courts are "
         "covered in salamanders. Each suit gave its royals a creature.",
         PLATE, "cups-12"),
    Fact("cups-12-stream",
         "A stream runs across the bottom right of the card and he is riding "
         "toward it. The Cups knight is always drawn approaching water rather "
         "than in it.",
         PLATE, "cups-12"),

    Fact("cups-13-closed",
         "Her cup has a lid on it, with handles shaped like angels and a "
         "little tower on top. It is the only closed cup in the suit, and she "
         "is the only person in the deck holding something she cannot drink "
         "from.",
         PLATE, "cups-13"),
    Fact("cups-13-throne",
         "Her stone throne is carved with cherubs at the top and a small "
         "child-like figure at the base, and it is standing at the water's "
         "edge. The sea comes right up to her feet.",
         PLATE, "cups-13"),
    Fact("cups-13-pebbles",
         "The shore under her is drawn as a spread of coloured pebbles, each "
         "one individually shaded. It is one of the most patiently drawn "
         "square inches in the deck.",
         PLATE, "cups-13"),
    Fact("cups-13-looking",
         "She is gazing straight into the closed cup and nowhere else. Every "
         "other royal in the deck is looking out at something.",
         PLATE, "cups-13"),
    Fact("cups-13-profile",
         "She is drawn in full profile with her feet together — the most "
         "still figure in the 78. Smith gave the water suit's queen no "
         "movement at all.",
         PLATE, "cups-13"),

    Fact("cups-14-sea",
         "His throne is a stone block sitting in open water, with waves "
         "breaking round the base. The King of Cups is the only figure in the "
         "deck enthroned at sea.",
         PLATE, "cups-14"),
    Fact("cups-14-fish",
         "A fish leaps out of the water on his left and a ship sails on his "
         "right, both small and easy to miss. He has the suit's creature and "
         "the suit's traffic on either side of him.",
         PLATE, "cups-14"),
    Fact("cups-14-two-hands",
         "He holds a cup in one hand and a short sceptre in the other, and "
         "neither is raised. He is the only king carrying two things.",
         PLATE, "cups-14"),
    Fact("cups-14-fish-pendant",
         "There is a fish worked into the pendant at his throat as well as "
         "swimming beside him. Smith put the emblem on the man and the animal "
         "in the water.",
         PLATE, "cups-14"),
    Fact("cups-14-dry",
         "His throne is wet to the base and his feet are dry on the slab. "
         "Whatever else the card says, the king of the water suit is not in "
         "it.",
         PLATE, "cups-14"),

    # ------------------------------------------------------------ swords
    Fact("swords-01-crown",
         "A crown sits on the point of the sword with an olive branch hanging "
         "off one side and a palm frond off the other — victory and peace, "
         "balanced on the tip of the blade.",
         f"{PLATE}; {PICTORIAL_KEY}", "swords-01"),
    Fact("swords-01-mountains",
         "The landscape under this Ace is a range of bare grey mountains and "
         "nothing else. Each Ace gets a different ground, and the sword suit "
         "gets the hardest one.",
         PLATE, "swords-01"),
    Fact("swords-01-grip",
         "The hand holds the sword upright by the grip with the blade "
         "straight up the middle of the card. It is the only Ace where the "
         "object is a weapon and the hand is holding it the way you would use "
         "it.",
         PLATE, "swords-01"),
    Fact("swords-01-suit",
         "Swords became spades in an ordinary pack, and the word is the "
         "giveaway: Italian spade means swords. The pip on a modern spade is a "
         "sword blade that lost its handle over four centuries of printing.",
         "Michael Dummett, The Game of Tarot (1980)", "swords-01"),
    Fact("swords-01-clouds",
         "Clouds are the sword suit's signature and they start here, boiling "
         "round the wrist. Air is the suit's element and Smith drew it as "
         "weather rather than as emptiness.",
         PLATE, "swords-01"),

    Fact("swords-02-blindfold",
         "She is blindfolded and holding two swords crossed over her chest, "
         "which means she tied it on before she picked them up, or somebody "
         "else did. Either way she cannot see and she is armed.",
         PLATE, "swords-02"),
    Fact("swords-02-justice",
         "Justice, eleven cards away in the trumps, is the one who is "
         "famously *not* blindfolded in this deck. Smith moved the blindfold "
         "off the figure everybody expects it on and put it here.",
         PLATE, "swords-02"),
    Fact("swords-02-moon",
         "There is a thin crescent moon in the top right, the only card in "
         "the suit with a moon. It is waxing.",
         PLATE, "swords-02"),
    Fact("swords-02-rocks",
         "Behind her is a flat sea with rocks scattered through it, drawn as "
         "small dark humps. It is the calmest water in the deck and the least "
         "safe to sail.",
         PLATE, "swords-02"),
    Fact("swords-02-bench",
         "She sits on a plain stone bench with her feet together, out of "
         "doors, on the shore. Nothing about the setting explains why she is "
         "there.",
         PLATE, "swords-02"),

    Fact("swords-03-heart",
         "A red heart pierced by three swords, in the rain. There is no "
         "person on this card at all — it is the deck's most famous image and "
         "one of only a handful with nobody in it.",
         PLATE, "swords-03"),
    Fact("swords-03-sola",
         "The pierced heart is usually traced back to the fifteenth-century "
         "Sola Busca deck, whose photographs were on show at the British "
         "Museum two years before Smith drew this one.",
         "British Museum Sola-Busca holdings; comparative studies of the two "
         "decks", "swords-03"),
    Fact("swords-03-rain",
         "The rain is drawn as long straight ruled lines right across the "
         "card, over the clouds and the heart alike. Smith let the weather "
         "cross in front of the subject, which almost nothing else in the "
         "deck does.",
         PLATE, "swords-03"),
    Fact("swords-03-flat",
         "The heart is a flat shape with no shading and no anatomy — the "
         "valentine heart, not the organ. It is the most graphic, least "
         "illustrative image in the 78.",
         PLATE, "swords-03"),
    Fact("swords-03-simple",
         "Three swords, one heart, some rain. In a deck where the Seven of "
         "Cups holds seven separate visions, this one says everything with "
         "four objects.",
         PLATE, "swords-03"),

    Fact("swords-04-tomb",
         "A knight lies full length on a tomb with his hands together in "
         "prayer, carved in effigy. He is not asleep and not dead in the "
         "usual sense — he is a monument.",
         PLATE, "swords-04"),
    Fact("swords-04-three-one",
         "Three swords hang point-down on the wall above him and the fourth "
         "is carved along the side of the tomb beneath him. Three in the air, "
         "one in the stone.",
         PLATE, "swords-04"),
    Fact("swords-04-window",
         "There is a stained-glass window in the top left corner, drawn in "
         "full colour, showing a standing figure and a smaller one kneeling. "
         "It is the only stained glass in the deck.",
         PLATE, "swords-04"),
    Fact("swords-04-church",
         "Everything else in the card is the inside of a church rendered in "
         "flat grey — so the one bright thing is a window somebody else is "
         "praying in. Smith put the colour where the living are.",
         PLATE, "swords-04"),
    Fact("swords-04-quiet",
         "This is the only card in the whole suit where nothing is happening "
         "and nobody is suffering. In a suit of thirteen difficult pictures, "
         "the rest is a tomb.",
         PLATE, "swords-04"),

    Fact("swords-05-smirk",
         "The young man in the foreground is smirking, and he is holding "
         "three swords while two more lie on the ground. It is the only "
         "openly unpleasant face in the deck.",
         PLATE, "swords-05"),
    Fact("swords-05-losers",
         "Two figures walk away with their heads down and their backs to him, "
         "one with a hand to their face. Smith drew the victory and the cost "
         "in one picture and put the cost in the distance.",
         PLATE, "swords-05"),
    Fact("swords-05-sky",
         "The clouds are torn into ragged strips right across the sky — the "
         "most violent weather in the deck, on a card where the fighting is "
         "already over.",
         PLATE, "swords-05"),
    Fact("swords-05-ground",
         "Two swords lie abandoned in the sand at his feet and he has not "
         "picked them up. He has taken three of five, which is exactly enough "
         "to have won.",
         PLATE, "swords-05"),
    Fact("swords-05-shore",
         "There is water behind them and a low shoreline. The defeated are "
         "walking toward it, and there is nothing on the far side.",
         PLATE, "swords-05"),

    Fact("swords-06-ferry",
         "A ferryman poles a flat punt across water carrying a cloaked figure "
         "and a child, with six swords stood upright in the bow. The swords "
         "are cargo, not weapons.",
         PLATE, "swords-06"),
    Fact("swords-06-water",
         "The water is choppy on the near side of the boat and glassy flat "
         "ahead of it. Smith put the whole meaning of the card in the "
         "surface of the water.",
         PLATE, "swords-06"),
    Fact("swords-06-faces",
         "Nobody's face is visible. The passenger is hooded and turned away, "
         "the child is a bundle, and the ferryman has his back to us.",
         PLATE, "swords-06"),
    Fact("swords-06-swords",
         "The six swords are stuck through the bottom of the boat and it is "
         "not sinking. It is the quietest impossible thing in the deck.",
         PLATE, "swords-06"),
    Fact("swords-06-trees",
         "There are two small trees on the far bank and nothing else. The "
         "destination is drawn, and it is almost empty.",
         PLATE, "swords-06"),

    Fact("swords-07-blades",
         "He is carrying five swords by the blades, gripped against his "
         "chest, which is how you carry swords you did not bring. Two are "
         "still standing in the ground behind him.",
         PLATE, "swords-07"),
    Fact("swords-07-look",
         "He is creeping away on his toes and looking back over his shoulder "
         "with a grin. It is the only figure in the deck caught mid-theft.",
         PLATE, "swords-07"),
    Fact("swords-07-camp",
         "The tents behind him are a military camp with a flag flying, and "
         "small figures moving about at the far left. He is robbing an army "
         "that has not noticed.",
         PLATE, "swords-07"),
    Fact("swords-07-two",
         "The two he left behind are planted upright in the earth, exactly "
         "like the ones the Five's victor did not pick up. Smith kept the "
         "abandoned sword as a motif across the suit.",
         PLATE, "swords-07"),
    Fact("swords-07-yellow",
         "The whole sky is flat yellow. In this suit, which is otherwise all "
         "grey cloud and black night, the theft happens in broad daylight.",
         PLATE, "swords-07"),

    Fact("swords-08-bound",
         "She is bound in cloth wound round her arms and body, and "
         "blindfolded, standing in shallow water. Her feet are not tied.",
         PLATE, "swords-08"),
    Fact("swords-08-gap",
         "Eight swords stand around her, but they are set in a broken line "
         "with a clear gap in front of her. The cage is not closed and the "
         "card makes sure you can see that.",
         PLATE, "swords-08"),
    Fact("swords-08-castle",
         "A castle sits on a crag behind her, high and small, with a red "
         "roof. Somebody up there put her here.",
         PLATE, "swords-08"),
    Fact("swords-08-marsh",
         "The ground is a marsh — you can see the standing water round her "
         "bare feet. It is the only card where a figure is standing in water "
         "without being at a shore.",
         PLATE, "swords-08"),
    Fact("swords-08-blindfold",
         "She is the second blindfolded woman in the suit, after the Two, and "
         "the only figure in the deck who is both blindfolded and bound. "
         "Smith did not repeat herself often.",
         PLATE, "swords-08"),

    Fact("swords-09-black",
         "The background is solid black, top to bottom, and the nine swords "
         "hang across it in a stack. It is the darkest card in the deck by a "
         "long way.",
         PLATE, "swords-09"),
    Fact("swords-09-quilt",
         "The quilt on the bed is a chequerboard of red roses and astrological "
         "signs, alternating square by square. Somebody sat and drew every "
         "one of them for a card about a nightmare.",
         PLATE, "swords-09"),
    Fact("swords-09-carving",
         "The side of the bed is carved with a scene of one figure standing "
         "over another with a blade. The furniture in the room is having the "
         "same dream.",
         PLATE, "swords-09"),
    Fact("swords-09-hands",
         "She is sitting up with her face in both hands, and the swords are "
         "behind her on the wall rather than over the bed. Nothing in the "
         "room is actually threatening her.",
         PLATE, "swords-09"),
    Fact("swords-09-roses",
         "The roses on the quilt are the same five-petalled rose the Fool "
         "carries and Death flies on a banner. It is the most repeated flower "
         "in the deck.",
         PLATE, "swords-09"),

    Fact("swords-10-dawn",
         "The sky is black, but there is a broad band of yellow dawn along "
         "the horizon under it. The most final card in the deck has a sunrise "
         "in it.",
         PLATE, "swords-10"),
    Fact("swords-10-hand",
         "His right hand is arranged in a blessing — two fingers up, the "
         "others folded. Smith gave the gesture to a man face down with ten "
         "swords in his back.",
         PLATE, "swords-10"),
    Fact("swords-10-water",
         "The water beyond him is completely flat and unbroken. In a suit "
         "whose Six made choppy water mean something, this is water with "
         "nothing left to say.",
         PLATE, "swords-10"),
    Fact("swords-10-red",
         "The red cloak spread under him is the strongest colour on the card "
         "and it is arranged like a pool. Smith let the cloth do what she did "
         "not draw.",
         PLATE, "swords-10"),
    Fact("swords-10-ten",
         "Ten is more swords than any injury needs, and they are placed "
         "evenly down his spine. The excess is the point.",
         PLATE, "swords-10"),

    Fact("swords-11-birds",
         "There is a flock of birds in the sky above the Page. Birds and "
         "butterflies run through all four Swords court cards the way "
         "salamanders run through Wands and fish through Cups — each suit "
         "gave its royals a creature.",
         PLATE, "swords-11"),
    Fact("swords-11-wind",
         "His hair is blown sideways, the clouds are piled and moving, and "
         "the grass on the hilltop is bent. The air suit is the only one "
         "where Smith drew the element itself.",
         PLATE, "swords-11"),
    Fact("swords-11-twohands",
         "He holds the sword up in both hands, off to one side, and looks the "
         "other way. He is the only court figure whose weapon and attention "
         "point in different directions.",
         PLATE, "swords-11"),
    Fact("swords-11-hilltop",
         "He is standing on a green rise above the clouds, with more cloud "
         "below him than above. The Swords court cards climb: the Page on a "
         "hill, the Queen above the cloud line entirely.",
         PLATE, "swords-11"),
    Fact("swords-11-braced",
         "His feet are planted wide apart, braced. Compare the Page of Cups "
         "standing easy with a hand on his hip — Smith gave each suit's Page "
         "a different way of standing.",
         PLATE, "swords-11"),

    Fact("swords-12-gallop",
         "The horse is at full gallop with all four legs off the ground, and "
         "it is the fastest thing in the deck. Everything else on horseback "
         "is walking or rearing.",
         PLATE, "swords-12"),
    Fact("swords-12-bent",
         "The trees at the bottom left are bent right over and the clouds are "
         "drawn in streaks. Smith bent the scenery rather than adding motion "
         "lines to the horse.",
         PLATE, "swords-12"),
    Fact("swords-12-birds",
         "The cloth on his horse is patterned with birds, and there is a red "
         "plume streaming off his helmet. Even at a gallop the suit's "
         "creature is on him.",
         PLATE, "swords-12"),
    Fact("swords-12-forward",
         "He is leaning forward past the horse's neck with the sword up and "
         "back, and his visor is open. He is the only knight whose face you "
         "can see clearly and he is not looking at us.",
         PLATE, "swords-12"),
    Fact("swords-12-alone",
         "There is nothing in front of him — no enemy, no army, no gate. The "
         "card is a charge with no target drawn.",
         PLATE, "swords-12"),

    Fact("swords-13-hand",
         "Her left hand is raised open, palm out — the only royal in the deck "
         "making a gesture rather than holding something. The sword is in the "
         "other hand, upright.",
         PLATE, "swords-13"),
    Fact("swords-13-cherub",
         "Her throne is carved with a winged cherub's head on the side and "
         "butterflies below it. Butterflies are the air suit's second "
         "creature and they turn up on the King's throne as well.",
         PLATE, "swords-13"),
    Fact("swords-13-clouds",
         "The clouds are level with the seat of her throne, so she is sitting "
         "at the top of the sky. She is the highest-placed figure in the 78.",
         PLATE, "swords-13"),
    Fact("swords-13-bird",
         "There is exactly one bird in her sky, small and dark and a long way "
         "off. The Page has a whole flock; she has one.",
         PLATE, "swords-13"),
    Fact("swords-13-tassel",
         "A long tassel hangs from her wrist and blows sideways in the same "
         "wind as the Page's hair. Smith kept one weather system running "
         "across four cards.",
         PLATE, "swords-13"),

    Fact("swords-14-front",
         "He faces straight out at you, square on, with the sword upright and "
         "tilted slightly across his body. He is the only king in the deck "
         "who looks directly at the reader.",
         PLATE, "swords-14"),
    Fact("swords-14-butterflies",
         "The back of his throne is carved with butterflies and crescent "
         "moons. His queen has butterflies and a cherub; the pair share a "
         "workshop.",
         PLATE, "swords-14"),
    Fact("swords-14-birds",
         "Two birds fly in the sky on his right, and there are trees on both "
         "sides of him. After a suit of black skies and marshes, the king "
         "sits in ordinary weather.",
         PLATE, "swords-14"),
    Fact("swords-14-tilt",
         "The sword is not vertical — it leans, and the tilt is the only "
         "thing in his posture that is not perfectly symmetrical. Smith "
         "unbalanced him by a few degrees on purpose.",
         PLATE, "swords-14"),
    Fact("swords-14-ground",
         "He is seated on bare earth and grass with no dais, in the open air. "
         "The King of Cups gets a slab in the sea and this one gets a field.",
         PLATE, "swords-14"),
)


#: Every per-card fact, trumps and minors together. `CARD_FACTS` and
#: `MINOR_FACTS` stay separate above only because they were written and
#: reviewed in two passes; nothing downstream cares which tier a card
#: sits in, and `for_card` must never have to.
ALL_CARD_FACTS: tuple[Fact, ...] = CARD_FACTS + MINOR_FACTS

ALL: tuple[Fact, ...] = DECK_FACTS + ALL_CARD_FACTS

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
    """Everything known about one card, whichever tier it belongs to."""
    return tuple(f for f in ALL_CARD_FACTS if f.card == key)


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

