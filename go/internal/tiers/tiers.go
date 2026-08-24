// Package tiers is which Claude answers for whom.
//
// It landed in Phase 4, ahead of the Claude surfaces and for a reason that
// was not them: `model_tier` is a column on `users`, and every account the
// admin routes serialise carries the *resolved* key -- so the table had to be
// readable from the side that writes the account list. `auth.User.AsDict` is
// that caller, and the Admin page's tier picker is served from `Roster`.
//
// **This paragraph said the Claude surface "has not moved and may never move
// as a whole" until 2026-08-22**, which Phase 6 then contradicted -- the dial,
// the voices and the tarot table all arrived that day. It is corrected here rather
// than quietly deleted because the shape of the mistake is the one this
// repository keeps finding: a sentence about what the rest of the system has
// not done yet is a claim with an expiry date, and it expires without telling
// anybody. `LabelFor` below is the part Phase 6 came back for.
//
// Three rules, stated here because each is a decision rather than an
// implementation detail:
//
// **A tier is a name, never a model id.** Accounts carry `opus`, not a model
// id; a model id is a thing that gets superseded and a column full of them is
// a migration every time. Commandment 10 says the same thing from the user's
// side: no technology a person may see is ever named. Nothing in this package
// puts a model id on the wire -- `Roster` returns key, label and blurb,
// and the `Model` field is read only by the Claude client on its way to
// the pipe.
//
// **An unknown tier resolves to the default rather than erroring.** The
// column is data on a volume that outlives any deploy, so a key this build no
// longer knows about will happen: a renamed key, a rolled-back deploy, a row
// somebody edited by hand. The answer a person wants there is "you got the
// ordinary model", not a 500 on every screen that lists them.
//
// **The write path does not inherit that tolerance.** `Known` is what a
// route owes the caller before storing a key, or the next typo silently
// grants a tier that does not exist and reads on the Admin page as the
// ordinary one.
package tiers

import "strings"

// Tier is one seat's model, and why a maintainer would grant it.
type Tier struct {
	Key string
	// Model is the model id Key resolves to -- the one place a model id
	// appears in this package, and never serialised.
	Model string
	Label string
	// Blurb is one line, addressed to whoever is deciding. Rendered on the
	// Admin page.
	Blurb string
}

// DefaultKey is the tier every account has unless a maintainer says
// otherwise, and what an unknown key falls back to. It must never leave All.
const DefaultKey = "sonnet"

// All is the roster, in the order the Admin page offers it.
var All = []Tier{
	{
		Key:   "sonnet",
		Model: "claude-sonnet-5",
		Label: "Sonnet",
		Blurb: "The house answer. Fast, and enough for conversation over facts " +
			"the pool already worked out.",
	},
	{
		Key:   "opus",
		Model: "claude-opus-5",
		Label: "Opus",
		Blurb: "Deeper reasoning on the questions the pool cannot settle — the " +
			"meta, a commander's place in history, a theme read from a " +
			"person rather than a card.",
	},
	{
		Key:   "fable",
		Model: "claude-fable-5",
		Label: "Fable",
		Blurb: "The most capable there is, and the most expensive. Worth a seat " +
			"only where the answer is the whole point.",
	},
}

var byKey = func() map[string]Tier {
	m := make(map[string]Tier, len(All))
	for _, t := range All {
		m[t.Key] = t
	}
	// Stated rather than trusted: Get falling through to a key that is not
	// in the table would be a very quiet bug, and this is the one moment it
	// can be caught for free.
	if _, ok := m[DefaultKey]; !ok {
		panic("tiers: the default tier is not in the roster")
	}
	return m
}()

// Get is the tier key names, or the default for anything else -- including
// the empty string, which is how a NULL column arrives here.
func Get(key string) Tier {
	if t, ok := byKey[key]; ok {
		return t
	}
	return byKey[DefaultKey]
}

// Resolve is the model id an account on key should be answered by.
func Resolve(key string) string { return Get(key).Model }

// Known reports whether key names a tier. The check a write path owes its
// caller; reading deliberately tolerates what this refuses.
func Known(key string) bool {
	_, ok := byKey[key]
	return ok
}

// Entry is one roster row as the Admin page receives it. Keys and prose,
// never a model id.
type Entry struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Blurb string `json:"blurb"`
}

// Roster is `tiers.roster()`: the list the Admin page offers, so the page
// offers exactly what the server will accept. One list serialised, rather
// than a second one written in TypeScript that would drift the day a tier is
// added and present as a control that 422s.
func Roster() []Entry {
	out := make([]Entry, 0, len(All))
	for _, t := range All {
		out = append(out, Entry{Key: t.Key, Label: t.Label, Blurb: t.Blurb})
	}
	return out
}

// How a model id is spoken about on a screen. Prefix-matched on the family,
// because the family is the name Anthropic uses in prose and the digits after
// it are the technology commandment 10 keeps off every surface.
//
// Not derived from All, deliberately, even though the three labels coincide
// today. A ledger row can hold a model that is no tier at all — a model served
// after a fallback, an older id, an A/B via MTGLAB_CLAUDE_MODEL — and
// resolving those through Get would label them with the DEFAULT TIER, which is
// how a screen ends up naming the wrong Claude with total confidence. A test
// pins that every tier's model still lands on that tier's label.
var families = []struct{ Prefix, Label string }{
	{"claude-fable-", "Fable"},
	{"claude-mythos-", "Mythos"},
	{"claude-opus-", "Opus"},
	{"claude-sonnet-", "Sonnet"},
	{"claude-haiku-", "Haiku"},
}

// Various is what a roll-up says in the column it aggregated away. The ledger
// writes the marker; this is the word for it.
const Various = "(various)"

// LabelFor is how to name the Claude that answered, on a screen.
//
// Commandment 10 in the one place it was still being broken: the Admin ledger
// rendered `claude-sonnet-5` in its "Answered by" column, which is a model id,
// which is technology. The tier picker two tabs over already said "Sonnet";
// this is the two agreeing.
//
// An id from no known family is "Another Claude" rather than a guess. That is
// deliberately uninformative on the page and deliberately paired with the
// unpriced counter beside it: both mean "this build does not know that model",
// and the maintainer who needs the id itself has the CLI, which prints it — a
// terminal is the same carve-out `mtglab users list` gets for printing
// addresses.
func LabelFor(model string) string {
	if model == Various {
		return "Several"
	}
	for _, family := range families {
		if strings.HasPrefix(model, family.Prefix) {
			return family.Label
		}
	}
	return "Another Claude"
}
