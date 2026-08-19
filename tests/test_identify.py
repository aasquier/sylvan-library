"""The camera's reader: what resolves, what only gets offered, and why.

Every assertion here runs against `tiny_pool`'s real cards and real
printings, so the corner tier is checked against collector numbers Wizards
actually printed rather than against numbers convenient to the test.

The single most important test in this file is
`test_a_title_never_resolves_however_good_the_score`. `identify.py`'s
docstring carries the measurement it comes from: the score distributions for
right and wrong answers overlap, so a title that scores a perfect 1.000 is
still only a suggestion. Anything that "fixes" that by thresholding is the
regression this file exists to catch.
"""

import sys
from pathlib import Path

import pytest

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "src"))
sys.path.insert(0, str(Path(__file__).resolve().parent))

import tiny_pool
from mtglab import config
from mtglab.cards import db, identify
from mtglab.cards.identify import Sighting


@pytest.fixture
def pool(tmp_path):
    """A real DuckDB pool with 22 real cards and 12 real printings."""
    with config.use_paths(data_dir=tmp_path / "data"):
        yield tiny_pool.build(config.DB_PATH)


@pytest.fixture
def con(pool):
    connection = db.connect(pool)
    try:
        yield connection
    finally:
        connection.close()


# ------------------------------------------------------- the corner tier

def test_a_set_and_number_resolve_outright(con):
    """The line `decklist.py`'s own docstring uses as its example."""
    assert identify.by_printing(con, "ltc", "284") == "Sol Ring"


def test_the_set_code_folds_case(con):
    """The pool stores `ltc`; a card prints `LTC` and OCR reads it that way."""
    assert identify.by_printing(con, "LTC", "284") == "Sol Ring"
    assert identify.by_printing(con, "Ltc", "284") == "Sol Ring"


def test_the_printed_denominator_is_dropped(con):
    """A card face says `284/281`. The pool stores the numerator alone."""
    assert identify.by_printing(con, "ltc", "284/281") == "Sol Ring"


def test_zero_padding_is_tolerated_in_either_direction(con):
    """Faces pad and the pool does not, so both sides get stripped."""
    assert identify.by_printing(con, "ltc", "0284") == "Sol Ring"
    assert identify.by_printing(con, "ltc", "000284/281") == "Sol Ring"


def test_one_card_resolves_from_either_of_its_printings(con):
    """Two sets, thirty years apart, one name -- the normal situation."""
    assert identify.by_printing(con, "ltc", "284") == "Sol Ring"
    assert identify.by_printing(con, "lea", "269") == "Sol Ring"


def test_a_letter_suffixed_number_is_its_own_printing(con):
    """`mul` 157 and 157z are both real, and both are Goreclaw.

    Two rows are not an ambiguity when they carry one name, which is what
    `DISTINCT name` is for -- refusing here would fail a card for being
    printed twice in its own set.
    """
    assert identify.by_printing(con, "mul", "157") == "Goreclaw, Terror of Qal Sisma"
    assert identify.by_printing(con, "mul", "157z") == "Goreclaw, Terror of Qal Sisma"


def test_a_banned_card_still_has_a_name(con):
    """The reader says what is on the table; the gate is what refuses it."""
    assert identify.by_printing(con, "m12", "188") == "Primeval Titan"
    assert identify.by_printing(con, "roe", "4") == "Emrakul, the Aeons Torn"


@pytest.mark.parametrize("set_code, number", [
    ("ltc", "99999"),          # right set, no such card
    ("zzz", "284"),            # no such set
    ("ltc", None),             # a frame with no collector number
    (None, "284"),             # a number with nothing to scope it
    ("ltc", ""),               # OCR found the field and read nothing
    ("", "284"),
    ("toolongforaset", "284"),  # not a set code, so never asked
    ("../../etc", "284"),      # nor this, and it never reaches the database
])
def test_what_the_corner_tier_refuses(con, set_code, number):
    assert identify.by_printing(con, set_code, number) is None


# -------------------------------------------------------- the title tier

def test_an_exact_title_scores_one(con):
    top = identify.by_title(con, "Craterhoof Behemoth")[0]
    assert top.name == "Craterhoof Behemoth"
    assert top.score == pytest.approx(1.0)


def test_a_front_face_matches_its_whole_card(con):
    """The camera only ever sees one side of a double-faced card."""
    for face, whole in [
        ("Etali, Primal Conqueror",
         "Etali, Primal Conqueror // Etali, Primal Sickness"),
        ("Ajani, Nacatl Pariah",
         "Ajani, Nacatl Pariah // Ajani, Nacatl Avenger"),
    ]:
        top = identify.by_title(con, face)[0]
        assert top.name == whole
        assert top.score == pytest.approx(1.0)


def test_a_misread_title_still_finds_its_card(con):
    """`rn`->`m` and `l`->`1`, the confusions a real engine makes."""
    names = [c.name for c in identify.by_title(con, "Craterhoof Behernoth")]
    assert "Craterhoof Behemoth" in names


def test_an_emblem_is_not_a_card(con):
    """Asked for the emblem by its exact name, the reader offers the Ajani.

    `Ajani Steadfast Emblem` is in the pool and is in the fixture; what it is
    not is something anybody can put in a deck. This fails the moment the
    legality filter narrows to `= 'legal'` or disappears.
    """
    names = [c.name for c in identify.by_title(con, "Ajani Steadfast Emblem")]
    assert "Ajani Steadfast Emblem" not in names
    assert "Ajani, Nacatl Pariah // Ajani, Nacatl Avenger" in names


def test_the_shortlist_is_capped(con):
    assert len(identify.by_title(con, "Forest")) == identify.CANDIDATES
    assert len(identify.by_title(con, "Forest", limit=2)) == 2


@pytest.mark.parametrize("title", [None, "", "   ", "\n\t "])
def test_an_empty_title_offers_nothing(con, title):
    assert identify.by_title(con, title) == []


def test_an_absurd_title_is_bounded_not_fatal(con):
    """`parse` never raises and neither does this; the input is clipped."""
    assert identify.by_title(con, "Forest" * 5000) != []


# ------------------------------------------------------------- reading

def test_a_title_never_resolves_however_good_the_score(con):
    """The load-bearing rule, and the reason this module has two tiers.

    A perfectly-read title scores 1.000 and still resolves nothing, because
    the measurement in `identify.py`'s docstring says a wrong card can score
    0.933 while a right one scores 0.780. There is no threshold; there is
    only a person tapping a name.
    """
    reading = identify.read(con, [Sighting(title="Craterhoof Behemoth")])[0]
    assert reading.resolved is None
    assert reading.via == "title"
    assert reading.candidates[0].name == "Craterhoof Behemoth"
    assert reading.candidates[0].score == pytest.approx(1.0)


def test_the_corner_is_preferred_to_the_title(con):
    """Both legible: the lookup wins, and the shortlist is not even built."""
    reading = identify.read(con, [
        Sighting(set_code="ltc", collector_number="284", title="Sol Rlng"),
    ])[0]
    assert reading.resolved == "Sol Ring"
    assert reading.via == "printing"
    assert reading.candidates == []


def test_a_misread_corner_falls_through_to_the_title(con):
    """The pre-2015 case, and the glare case: no corner, so offer names."""
    reading = identify.read(con, [
        Sighting(set_code="lea", collector_number="99999",
                 title="Black Lotus"),
    ])[0]
    assert reading.resolved is None
    assert reading.via == "title"
    assert reading.candidates[0].name == "Black Lotus"


def test_a_sighting_of_nothing_is_still_a_reading(con):
    """One reading per sighting, so the two lists stay side by side."""
    readings = identify.read(con, [
        Sighting(title="Sol Ring"),
        Sighting(),
        Sighting(set_code="ltc", collector_number="284"),
    ])
    assert [r.via for r in readings] == ["title", "nothing", "printing"]
    assert readings[1].candidates == []


def test_the_batch_is_bounded(con):
    readings = identify.read(con, [Sighting(title="Forest")] * 200)
    assert len(readings) == identify.MAX_SIGHTINGS


def test_reading_nothing_at_all(con):
    assert identify.read(con, []) == []

# ------------------------------------------- the corner as a reader sees it

def test_the_corner_a_real_reader_actually_returned(con):
    """The verbatim output of Tesseract on a real Lord of the Rings Sol Ring.

    Captured in a browser, against the crop this code ships, from Scryfall's
    488px image -- a *pessimistic* source, since a phone photograph carries
    four times the detail. It is here rather than a tidy invented string
    because every tidy string I would have written was wrong: the set code
    does not arrive as a token, it arrives welded to the language tag and the
    artist's initials.
    """
    sighting = identify.from_corner(con, "U0284\nLTCENLIK")
    assert sighting.set_code == "LTC"
    assert sighting.collector_number == "0284"
    assert identify.read(con, [sighting])[0].resolved == "Sol Ring"


def test_the_number_line_yields_no_set(con):
    """`U0284` is a rarity welded to a number. A token that starts with a
    digit is never a set code, and this one must not become `U0`."""
    assert identify.from_corner(con, "U0284").set_code is None
    assert identify.from_corner(con, "0284/0281 U").set_code is None


def test_only_the_first_token_of_a_line_may_be_a_set(con):
    """The set code is the leftmost thing on its line, so the artist never
    gets a vote -- and `CHRISRAHN` has `CHR` as a prefix."""
    assert identify.from_corner(con, "MOM 137 CHRISRAHN").set_code == "MOM"
    # The artist alone, with no set code in front of them, matches nothing.
    assert identify.from_corner(con, "CHRISRAHN").set_code is None


def test_the_longest_real_prefix_wins(con):
    """`MOM` and `M12` are both in the fixture; a run-together token has to
    resolve to the code that is actually there, not the shortest guess."""
    assert identify.from_corner(con, "MOMENEXT").set_code == "MOM"
    assert identify.from_corner(con, "M12ENAB").set_code == "M12"


def test_a_set_code_that_does_not_exist_is_not_one(con):
    """The whole reason this moved out of the browser. `TTBS` is what the
    reader returned when the crop was wrong, and no client-side rule can
    know it is not a set -- only the pool's own 986 can say."""
    assert identify.from_corner(con, "TTBS\nMAS").set_code is None


def test_the_copyright_line_is_not_a_collector_number(con):
    """It sits directly below the crop and bleeds in constantly."""
    assert identify.from_corner(con, "TM AND C 2021 WIZARDS").collector_number is None


def test_an_explicit_set_code_beats_a_raw_corner(con):
    """A caller that already has the fields is not second-guessed -- the
    text path exists for readers that only have pixels."""
    reading = identify.read(con, [Sighting(
        set_code="ltc", collector_number="284", corner="COMPLETE NONSENSE")])[0]
    assert reading.resolved == "Sol Ring"


def test_a_corner_of_nothing_is_harmless(con):
    for junk in [None, "", "   ", "|||", "\n\n"]:
        sighting = identify.from_corner(con, junk)
        assert sighting.set_code is None
        assert sighting.collector_number is None


def test_the_set_code_table_is_read_once_per_batch(con, monkeypatch):
    """Forty sightings must not be forty scans of 107,338 rows."""
    calls = []
    real = identify.set_codes
    monkeypatch.setattr(identify, "set_codes",
                        lambda c: (calls.append(1), real(c))[1])
    identify.read(con, [Sighting(corner="LTCENLIK\nU0284")] * 20)
    assert len(calls) == 1


def test_no_corner_text_means_no_scan_at_all(con, monkeypatch):
    monkeypatch.setattr(identify, "set_codes", lambda c: pytest.fail(
        "the set-code table was read for a batch with no corner text"))
    assert len(identify.read(con, [Sighting(title="Sol Ring")])) == 1
