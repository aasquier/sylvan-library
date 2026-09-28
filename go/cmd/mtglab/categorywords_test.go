package main

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/reference"
)

// Every shelf of the 99 has a word a beginner can look up.
//
// The deck model names thirteen jobs a card can be doing — land, ramp,
// card-advantage, tutor, interaction, protection, threat, engine, sac-outlet,
// payoff, recursion, win-con, utility — and the deck page prints those words
// as section headers over somebody's first Commander deck. Aaron, 2026-09-27:
// *"it would be nice if there was help text for the genres, like
// interaction"*. Ten of the thirteen had no entry at all when that was asked.
//
// # Why this guard and not `glossarykeys_test.go`
//
// That file sweeps `web/src` for keys written as literals and holds each one
// to the served table. A category's key is *not* written as a literal: it
// arrives as the group's own key and goes through `categoryTerm` in
// `web/src/lib/mtg.ts`, so `components/term.tsx` carries the tree's one
// `<HelpTip name={…}>` that the sweep cannot follow — and that file is
// exempted there **on the strength of this test**, which covers the whole
// list the indirection can produce rather than the one call site.
//
// The failure being guarded is the silent one, and it is the reason the
// glossary needs guards at all: a mark whose key has no entry renders as
// **nothing**. A shelf added to `model.json` with no word to go with it would
// simply have no help, on every deck page, with every suite green.
//
// # The authority on both sides
//
// The categories are read from [reference.Deck], not from a list retyped
// here, and the entries from [reference.Words] — the served table, not the
// file behind it. The only thing read out of the frontend is the table of
// exceptions, because that is the one fact that lives there.

// categoryAliasTable is `CATEGORY_TERMS` as `lib/mtg.ts` declares it. Written
// against the declaration rather than the value so that a table moved to
// another shape fails loudly here instead of parsing as empty and passing
// everything.
var categoryAliasTable = regexp.MustCompile(
	`(?s)export const CATEGORY_TERMS: Record<string, string> = \{(.*?)\n\}`)

// categoryAliasPair is one line of it: `'win-con': 'wincon',`.
var categoryAliasPair = regexp.MustCompile(`'([a-z0-9-]+)'\s*:\s*'([a-z0-9._-]+)'`)

// categoryAliases reads the frontend's category-to-entry table.
func categoryAliases(t *testing.T) map[string]string {
	t.Helper()
	rel := filepath.Join("web", "src", "lib", "mtg.ts")
	body, err := os.ReadFile(filepath.Join(repoRoot(t), rel))
	if err != nil {
		t.Fatalf("reading %s: %v", rel, err)
	}
	block := categoryAliasTable.FindSubmatch(body)
	if block == nil {
		t.Fatalf("%s no longer declares `export const CATEGORY_TERMS: Record<string, "+
			"string> = {…}`. This guard reads that declaration to learn which "+
			"categories reach their entry under another key; unable to read it, it "+
			"would pass a tree whose mapping had quietly gone.", rel)
	}
	out := map[string]string{}
	for _, pair := range categoryAliasPair.FindAllSubmatch(block[1], -1) {
		out[string(pair[1])] = string(pair[2])
	}
	return out
}

// TestEveryCardCategoryHasAWordAPlayerCanLookUp is the rule.
func TestEveryCardCategoryHasAWordAPlayerCanLookUp(t *testing.T) {
	t.Parallel()

	served := map[string]bool{}
	for _, term := range reference.Words().Terms {
		served[term.Key] = true
	}
	if len(served) < 20 {
		t.Fatalf("the served glossary holds %d terms, which is too few to be the "+
			"real table -- this guard would pass everything", len(served))
	}

	categories := reference.Deck().Categories
	if len(categories) < 13 {
		t.Fatalf("the deck model names %d categories, which is fewer than the "+
			"thirteen it has always had -- either the model is not being read or "+
			"this guard is sweeping an empty list", len(categories))
	}

	alias := categoryAliases(t)
	var missing []string
	for _, category := range categories {
		key := category
		if under, ok := alias[category]; ok {
			key = under
		}
		if !served[key] {
			missing = append(missing, category+" (looked up as "+key+")")
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("these shelves of the 99 have no entry in %s, so their help mark "+
			"renders as nothing at all on every deck page:\n  %s\nWrite the entry, "+
			"or -- if the word genuinely belongs to an entry that already exists "+
			"under another key -- add the line to CATEGORY_TERMS in "+
			"web/src/lib/mtg.ts.",
			filepath.Join("go", "internal", "reference", "data", "glossary.json"),
			strings.Join(missing, "\n  "))
	}
}

// TestTheCategoryTermTableCarriesOnlyWhatItMustBeCarrying is the anti-drift
// half. A table of exceptions is only trustworthy while every line in it is
// still an exception: a stale entry sends a real category at a key nobody
// serves, and an identity line is a line doing nothing that the next reader
// has to work out is doing nothing.
func TestTheCategoryTermTableCarriesOnlyWhatItMustBeCarrying(t *testing.T) {
	t.Parallel()

	alias := categoryAliases(t)
	if len(alias) == 0 {
		t.Fatal("CATEGORY_TERMS parsed as empty. Either every category is keyed " +
			"after itself -- in which case delete the table and `categoryTerm` " +
			"with it rather than leaving a dead lookup -- or this guard has " +
			"stopped reading it and the test above is passing on nothing.")
	}

	categories := map[string]bool{}
	for _, category := range reference.Deck().Categories {
		categories[category] = true
	}
	served := map[string]bool{}
	for _, term := range reference.Words().Terms {
		served[term.Key] = true
	}

	from := make([]string, 0, len(alias))
	for category := range alias {
		from = append(from, category)
	}
	sort.Strings(from)
	for _, category := range from {
		to := alias[category]
		if !categories[category] {
			t.Errorf("CATEGORY_TERMS maps %q, which the deck model does not name as "+
				"a category. A line for a shelf that does not exist is a line "+
				"nobody will ever notice is wrong.", category)
		}
		if !served[to] {
			t.Errorf("CATEGORY_TERMS sends %q to %q, which the served glossary has "+
				"no entry for -- so that shelf's help mark is silently nothing.",
				category, to)
		}
		if category == to {
			t.Errorf("CATEGORY_TERMS maps %q to itself, which `categoryTerm`'s "+
				"fallback already does. Delete the line.", category)
		}
	}
}
