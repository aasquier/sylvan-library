// Package tiers is `mtglab/claude/tiers.py`: which Claude answers for whom.
//
// It lives in the Go module for one reason and it is not the Claude surface,
// which has not moved and may never move as a whole. It is that `model_tier`
// is a column on `users`, and every account the admin routes serialise
// carries the *resolved* key -- so the table has to be readable from the side
// that writes the account list. `auth.User.AsDict` is the caller, and the
// Admin page's tier picker is served from `Roster`.
//
// The three rules Python states, kept here because each is a decision rather
// than an implementation detail:
//
// **A tier is a name, never a model id.** Accounts carry `opus`, not a model
// id; a model id is a thing that gets superseded and a column full of them is
// a migration every time. Commandment 10 says the same thing from the user's
// side: no technology a person may see is ever named. Nothing in this package
// puts a model id on the wire -- `Roster` returns key, label and blurb, and
// `Model` exists for the Claude surfaces that have not crossed yet.
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
	// Stated rather than trusted, as Python states it with an assert: Get
	// falling through to a key that is not in the table would be a very quiet
	// bug, and this is the one moment it can be caught for free.
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
