package claude

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// stanceCorpus is the recorded corpus testdata/stance.json: the dial,
// exhaustively.
type stanceCorpus struct {
	Axes        []string            `json:"axes"`
	Levels      map[string][]string `json:"levels"`
	PresetNames []string            `json:"preset_names"`
	Presets     map[string]axisMap  `json:"presets"`
	Blurbs      map[string]string   `json:"preset_blurbs"`
	Stances     []struct {
		Stance      axisMap `json:"stance"`
		AllowsCalls bool    `json:"allows_calls"`
		MayWrite    bool    `json:"may_write"`
		Describe    string  `json:"describe"`
	} `json:"stances"`
	Clamps []struct {
		Requested axisMap `json:"requested"`
		Limit     axisMap `json:"limit"`
		Clamped   axisMap `json:"clamped"`
	} `json:"clamps"`
	Parses []struct {
		Input  json.RawMessage `json:"input"`
		Stance *axisMap        `json:"stance"`
		Error  string          `json:"error"`
	} `json:"parses"`
	Ceilings []struct {
		Env     *string `json:"env"`
		Ceiling axisMap `json:"ceiling"`
	} `json:"ceilings"`
	Defaults []struct {
		Status *string `json:"status"`
		Stance axisMap `json:"stance"`
	} `json:"defaults"`
	// The whole of `GET /api/claude`, per case. See dial_test.go.
	Dial []dialRow `json:"dial"`
}

type axisMap struct {
	Initiative string `json:"initiative"`
	Scope      string `json:"scope"`
	Write      string `json:"write"`
}

func (a axisMap) stance() Stance { return Stance(a) }

func loadStanceCorpus(t *testing.T) stanceCorpus {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "stance.json"))
	if err != nil {
		t.Fatalf("reading the stance corpus: %v", err)
	}
	var c stanceCorpus
	if err := json.Unmarshal(raw, &c); err != nil {
		t.Fatalf("decoding the stance corpus: %v", err)
	}
	if len(c.Stances) != 36 || len(c.Clamps) != 36*36 {
		t.Fatalf("the corpus is not the exhaustive one: %d stances, %d clamps",
			len(c.Stances), len(c.Clamps))
	}
	return c
}

// TestTheAxesAndLevelsAreTheRecordedOnes pins the tables themselves.
// Everything below indexes into them, so a reordered axis would make every
// other test in this file agree with itself and disagree with the corpus.
func TestTheAxesAndLevelsAreTheRecordedOnes(t *testing.T) {
	t.Parallel()
	c := loadStanceCorpus(t)
	if got := Axes; !equalStrings(got, c.Axes) {
		t.Errorf("axes: got %v, corpus %v", got, c.Axes)
	}
	for axis, want := range c.Levels {
		if got := levels[axis]; !equalStrings(got, want) {
			t.Errorf("%s levels: got %v, corpus %v", axis, got, want)
		}
	}
	if !equalStrings(PresetNames, c.PresetNames) {
		t.Errorf("preset order: got %v, corpus %v", PresetNames, c.PresetNames)
	}
	for name, want := range c.Presets {
		got, err := Preset(name)
		if err != nil {
			t.Fatalf("preset %q: %v", name, err)
		}
		if got != want.stance() {
			t.Errorf("preset %q: got %+v, corpus %+v", name, got, want.stance())
		}
	}
	for name, want := range c.Blurbs {
		if got := PresetBlurbs[name]; got != want {
			t.Errorf("blurb %q:\n got    %q\n corpus %q", name, got, want)
		}
	}
}

// TestEveryStanceAnswersAsRecorded walks all 36 and compares the two
// properties every caller asks — plus the full Describe payload, as BYTES.
//
// Bytes rather than fields, and that is the point of the test. `tier1.Number`
// was bit-exact by its decimal spelling and by Float64bits and still went
// onto the wire as {"IsFloat":false,...} because nothing compared what
// encoding/json actually produced. A readout with the right values in the
// wrong field order is that failure again, and only this comparison sees it.
func TestEveryStanceAnswersAsRecorded(t *testing.T) {
	t.Parallel()
	c := loadStanceCorpus(t)
	for _, row := range c.Stances {
		s := row.Stance.stance()
		if err := s.Validate(); err != nil {
			t.Errorf("%+v: rejected by Validate: %v", s, err)
			continue
		}
		if got := s.AllowsCalls(); got != row.AllowsCalls {
			t.Errorf("%+v allows_calls: got %v, corpus %v", s, got, row.AllowsCalls)
		}
		if got := s.MayWrite(); got != row.MayWrite {
			t.Errorf("%+v may_write: got %v, corpus %v", s, got, row.MayWrite)
		}
		marshalled, err := json.Marshal(Describe(s))
		if err != nil {
			t.Fatalf("%+v: marshalling the readout: %v", s, err)
		}
		if !bytes.Equal(marshalled, []byte(row.Describe)) {
			t.Errorf("%+v describe:\n got    %s\n corpus %s", s, marshalled, row.Describe)
		}
	}
}

// TestEveryClampPairAgreesWithTheCorpus walks all 1,296. Per-axis minimum is
// four lines of code and it is the line an operator's cap runs through.
func TestEveryClampPairAgreesWithTheCorpus(t *testing.T) {
	t.Parallel()
	c := loadStanceCorpus(t)
	for _, row := range c.Clamps {
		got := Clamp(row.Requested.stance(), row.Limit.stance())
		if want := row.Clamped.stance(); got != want {
			t.Errorf("clamp(%+v, %+v): got %+v, corpus %+v",
				row.Requested.stance(), row.Limit.stance(), got, want)
		}
	}
}

// TestParsingAStanceMatchesTheCorpusIncludingItsRefusals drives StanceFromObj
// over every shape the wire can carry.
//
// The refusal TEXT is compared, not merely the fact of a refusal: these
// strings reach a 422 body that a person reads, and two of them differ from
// the obvious Go spelling in ways no structural test would notice — the
// recorded refusals quote with single quotes, and they tell `7` from `7.5`
// by the literal rather than by the value.
func TestParsingAStanceMatchesTheCorpusIncludingItsRefusals(t *testing.T) {
	t.Parallel()
	c := loadStanceCorpus(t)
	for _, row := range c.Parses {
		got, err := StanceFromObj(row.Input)
		if row.Error != "" {
			if err == nil {
				t.Errorf("input %s: accepted as %+v, the corpus refuses: %s",
					row.Input, got, row.Error)
				continue
			}
			if err.Error() != row.Error {
				t.Errorf("input %s refusal:\n got    %s\n corpus %s",
					row.Input, err.Error(), row.Error)
			}
			continue
		}
		if err != nil {
			t.Errorf("input %s: refused (%v), the corpus gives %+v",
				row.Input, err, row.Stance.stance())
			continue
		}
		if want := row.Stance.stance(); got != want {
			t.Errorf("input %s: got %+v, corpus %+v", row.Input, got, want)
		}
	}
}

// TestTheCeilingReadsItsSettingAsRecorded covers the two rows that carry the
// design: unset means uncapped, and unreadable means OFF. Failing closed is
// the decision — a typo in a deployment variable costs a feature, never opens
// one — and it is one `return` away from failing open.
//
// The corpus's `env` column is a *string, where nil is the variable being
// absent. `os.Getenv` cannot tell absent from blank, so both arrive at
// [CeilingFrom] as the empty string and the two rows are the same question
// asked twice — which is exactly why the reading is worth having as a pure
// function: the deployment is now a value this test holds, not a slot it
// takes away from every other test in the binary.
func TestTheCeilingReadsItsSettingAsRecorded(t *testing.T) {
	t.Parallel()
	c := loadStanceCorpus(t)
	for _, row := range c.Ceilings {
		raw, env := "", "<unset>"
		if row.Env != nil {
			raw, env = *row.Env, *row.Env
		}
		if got, want := CeilingFrom(raw), row.Ceiling.stance(); got != want {
			t.Errorf("ceiling with %q: got %+v, corpus %+v", env, got, want)
		}
	}
}

// TestAMissingLimitFallsToTheSameCeilingSettingsDo is the seam the twelve
// serial tests in this package were paying for, asserted as itself.
//
// Two nil-fallbacks used to disagree about where they read from: [Settings]
// answered Collaborator out of its own code, and every `limit *Stance`
// parameter answered whatever the process happened to export. They are one
// reading now, and a drift between them would be invisible — a deployment
// that capped one surface and not another.
func TestAMissingLimitFallsToTheSameCeilingSettingsDo(t *testing.T) {
	t.Parallel()
	if got := ceilingOr(nil); got != Collaborator {
		t.Errorf("no limit at all resolved to %+v, want collaborator", got)
	}
	if got := (Settings{}).ceiling(); got != ceilingOr(nil) {
		t.Errorf("Settings falls to %+v where a nil limit falls to %+v", got, ceilingOr(nil))
	}
	for _, name := range PresetNames {
		want, err := Preset(name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got := ceilingOr(&want); got != want {
			t.Errorf("a %s limit resolved to %+v", name, got)
		}
		if got := (Settings{Ceiling: &want}).ceiling(); got != want {
			t.Errorf("Settings with a %s ceiling resolved to %+v", name, got)
		}
	}
}

// TestASettingsBuiltFromTheEnvironmentAlwaysCarriesACeiling holds the one
// promise [ceilingOr] rests on.
//
// Nil is a legitimate ceiling — it means the built-in default, and it is what
// a test that says nothing about a cap gets. What would be a fault is a
// *serving* process reaching that branch, because then a deployment that set
// MTGLAB_CLAUDE_STANCE_CEILING would go uncapped with nothing failing. So the
// composition root's own reader fills it on every row, including the one
// where the variable is absent and the filled value is the default anyway.
func TestASettingsBuiltFromTheEnvironmentAlwaysCarriesACeiling(t *testing.T) {
	t.Parallel()
	deployments := []map[string]string{
		{},
		{CeilingEnv: ""},
		{CeilingEnv: "off"},
		{CeilingEnv: "consultant"},
		{CeilingEnv: "not-a-preset"},
		{CeilingEnv: "collaborator", modelEnv: "some-model", apiKeyEnv: "k"},
	}
	for _, env := range deployments {
		set := SettingsFromLookup(lookup(env))
		if set.Ceiling == nil {
			t.Fatalf("a Settings built from %v carries no ceiling", env)
		}
		if got, want := *set.Ceiling, CeilingFrom(env[CeilingEnv]); got != want {
			t.Errorf("%v: ceiling %+v, want %+v", env, got, want)
		}
		if got, want := set.ceiling(), *set.Ceiling; got != want {
			t.Errorf("%v: the threaded ceiling %+v is not the stored one %+v", env, got, want)
		}
	}
}

// lookup is a deployment as a value: the `getenv` every `…FromLookup` takes,
// backed by a map instead of by the process.
func lookup(env map[string]string) func(string) string {
	return func(name string) string { return env[name] }
}

// statused is the one field DefaultFor reads. A nil pointer stands for a deck
// object with no status at all, which is the row a real Deck cannot produce.
type statused struct{ status string }

func (s statused) DeckStatus() string { return s.status }

func TestTheDeckDefaultAgreesWithTheCorpus(t *testing.T) {
	t.Parallel()
	c := loadStanceCorpus(t)
	for _, row := range c.Defaults {
		var deck DeckStatused = statused{}
		if row.Status != nil {
			deck = statused{*row.Status}
		}
		if got, want := DefaultFor(deck), row.Stance.stance(); got != want {
			t.Errorf("default for status %v: got %+v, corpus %+v", row.Status, got, want)
		}
	}
	// The separate case the corpus cannot carry: no deck at all. Resolve with
	// a nil request and a nil deck is OFF, not the built-deck default — the
	// create flow has no deck, and answering `consultant` there would be the
	// bug ADR 15's surface argument was written about.
	got, err := Resolve(nil, nil, &Collaborator)
	if err != nil {
		t.Fatalf("resolving with no deck: %v", err)
	}
	if got != Off {
		t.Errorf("no deck, no request: got %+v, want %+v", got, Off)
	}
}

// TestOffIsTheFloorOfEveryFallback is the invariant behind three separate
// behaviours, asserted once as itself: a request that cannot be read, a
// mapping that names only some axes, and a ceiling that cannot be parsed all
// land at or below OFF's level on every axis. It is what makes "a malformed
// request can only ever be quieter" a property rather than three coincidences.
// The three fallbacks are described rather than installed: the unreadable
// ceiling is a string handed to [CeilingFrom], and the deployment it produces
// is then handed back down as the `limit` every caller already takes.
func TestOffIsTheFloorOfEveryFallback(t *testing.T) {
	t.Parallel()
	partial, err := StanceFromObj(json.RawMessage(`{"write":"applies"}`))
	if err != nil {
		t.Fatalf("partial mapping: %v", err)
	}
	if partial.Initiative != Off.Initiative || partial.Scope != Off.Scope {
		t.Errorf("unnamed axes did not fall to OFF: %+v", partial)
	}
	capped := CeilingFrom("not-a-preset")
	if capped != Off {
		t.Errorf("an unreadable ceiling must fail closed, got %+v", capped)
	}
	// And the clamp that follows from it: at an OFF ceiling nothing may call.
	resolved, err := Resolve("collaborator", nil, &capped)
	if err != nil {
		t.Fatalf("resolving under an off ceiling: %v", err)
	}
	if resolved.AllowsCalls() {
		t.Errorf("an off ceiling still allowed calls: %+v", resolved)
	}
	// And every deckless surface's own default is under the same floor -- the
	// four that fall back through `ceilingOr` rather than through Resolve.
	for name, stanceFor := range map[string]func(any, *Stance) (Stance, error){
		"research": ResearchStanceFor,
		"scan":     ScanStanceFor,
		"intake":   IntakeStanceFor,
		"theme":    ThemeStanceFor,
	} {
		got, err := stanceFor(nil, &capped)
		if err != nil {
			t.Fatalf("%s under an off ceiling: %v", name, err)
		}
		if got.AllowsCalls() {
			t.Errorf("%s still called under an off ceiling: %+v", name, got)
		}
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
