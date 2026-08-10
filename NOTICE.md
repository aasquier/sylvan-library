# Notices and attribution

## Wizards of the Coast

mtg-lab is unofficial Fan Content permitted under the Wizards of the Coast Fan
Content Policy. Not approved or endorsed by Wizards. Portions of the materials
used are property of Wizards of the Coast. ©Wizards of the Coast LLC.

Magic: The Gathering, Wizards of the Coast, and their logos are trademarks of
Wizards of the Coast LLC in the United States and other countries.

**The Fan Content Policy permits noncommercial use only.** This project must
stay free. No paid tiers, no sponsorship of the tool, no donations tied to it,
no advertising against it. If you fork it, that constraint travels with you.

## Scryfall

Card data and images are provided by [Scryfall](https://scryfall.com), free of
charge, for the purpose of building Magic software and community content. Per
their guidelines:

- Do not imply Scryfall has endorsed this project.
- Do not put Scryfall data behind payment, subscription, or a survey.
- Do not redistribute their bulk data as your own dataset. **This repository
  contains no card data.** `mtglab data refresh` downloads it at runtime and
  `data/` is gitignored.
- Use the daily bulk files rather than hammering the API card by card. The
  ingest in `cards/db.py` makes one request per bulk file per day.

Card images are hotlinked or cached locally at runtime and are never committed.
Artwork remains the property of its artists and Wizards of the Coast.

## Forge

The optional Tier 3 bridge shells out to [Forge](https://github.com/Card-Forge/forge)
as a separate process. It is not bundled, linked, or redistributed here, and
its own license applies to it. You install it yourself.

## Prices

Price data comes from the Scryfall printings feed. TCGplayer's developer API is
closed to new applicants; this project does not scrape TCGplayer and does not
automate purchases.
