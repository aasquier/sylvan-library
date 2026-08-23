package claude

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// stanceCorpus is tests/go_fixtures.py's stance.json: the dial, exhaustively.
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

// TestTheAxesAndLevelsAreThePythonOnes pins the tables themselves. Everything
// below indexes into them, so a reordered axis would make every other test in
// this file agree with itself and disagree with Python.
func TestTheAxesAndLevelsAreThePythonOnes(t *testing.T) {
	c := loadStanceCorpus(t)
	if got := Axes; !equalStrings(got, c.Axes) {
		t.Errorf("axes: go %v, python %v", got, c.Axes)
	}
	for axis, want := range c.Levels {
		if got := levels[axis]; !equalStrings(got, want) {
			t.Errorf("%s levels: go %v, python %v", axis, got, want)
		}
	}
	if !equalStrings(PresetNames, c.PresetNames) {
		t.Errorf("preset order: go %v, python %v", PresetNames, c.PresetNames)
	}
	for name, want := range c.Presets {
		got, err := Preset(name)
		if err != nil {
			t.Fatalf("preset %q: %v", name, err)
		}
		if got != want.stance() {
			t.Errorf("preset %q: go %+v, python %+v", name, got, want.stance())
		}
	}
	for name, want := range c.Blurbs {
		if got := PresetBlurbs[name]; got != want {
			t.Errorf("blurb %q:\n go     %q\n python %q", name, got, want)
		}
	}
}

// TestEveryStanceAnswersAsPythonDoes walks all 36 and compares the two
// properties every caller asks — plus the full describe() payload, as BYTES.
//
// Bytes rather than fields, and that is the point of the test. `tier1.Number`
// was bit-exact against CPython by repr and by Float64bits and still went onto
// the wire as {"IsFloat":false,...} because nothing compared what
// encoding/json actually produced. A readout with the right values in the
// wrong field order is that failure again, and only this comparison sees it.
func TestEveryStanceAnswersAsPythonDoes(t *testing.T) {
	c := loadStanceCorpus(t)
	for _, row := range c.Stances {
		s := row.Stance.stance()
		if err := s.Validate(); err != nil {
			t.Errorf("%+v: rejected by Validate: %v", s, err)
			continue
		}
		if got := s.AllowsCalls(); got != row.AllowsCalls {
			t.Errorf("%+v allows_calls: go %v, python %v", s, got, row.AllowsCalls)
		}
		if got := s.MayWrite(); got != row.MayWrite {
			t.Errorf("%+v may_write: go %v, python %v", s, got, row.MayWrite)
		}
		marshalled, err := json.Marshal(Describe(s))
		if err != nil {
			t.Fatalf("%+v: marshalling the readout: %v", s, err)
		}
		if !bytes.Equal(marshalled, []byte(row.Describe)) {
			t.Errorf("%+v describe:\n go     %s\n python %s", s, marshalled, row.Describe)
		}
	}
}

// TestEveryClampPairAgreesWithPython walks all 1,296. Per-axis minimum is four
// lines of code and it is the line an operator's cap runs through.
func TestEveryClampPairAgreesWithPython(t *testing.T) {
	c := loadStanceCorpus(t)
	for _, row := range c.Clamps {
		got := Clamp(row.Requested.stance(), row.Limit.stance())
		if want := row.Clamped.stance(); got != want {
			t.Errorf("clamp(%+v, %+v): go %+v, python %+v",
				row.Requested.stance(), row.Limit.stance(), got, want)
		}
	}
}

// TestParsingAStanceAgreesWithPythonIncludingItsRefusals drives from_obj over
// every shape the wire can carry.
//
// The refusal TEXT is compared, not merely the fact of a refusal: these
// strings reach a 422 body that a person reads, and two of them differ from
// the obvious Go spelling in ways no structural test would notice — Python
// repr-quotes with single quotes, and it tells `7` from `7.5` by the literal
// rather than by the value.
func TestParsingAStanceAgreesWithPythonIncludingItsRefusals(t *testing.T) {
	c := loadStanceCorpus(t)
	for _, row := range c.Parses {
		got, err := StanceFromObj(row.Input)
		if row.Error != "" {
			if err == nil {
				t.Errorf("input %s: go accepted it as %+v, python refused: %s",
					row.Input, got, row.Error)
				continue
			}
			if err.Error() != row.Error {
				t.Errorf("input %s refusal:\n go     %s\n python %s",
					row.Input, err.Error(), row.Error)
			}
			continue
		}
		if err != nil {
			t.Errorf("input %s: go refused it (%v), python gave %+v",
				row.Input, err, row.Stance.stance())
			continue
		}
		if want := row.Stance.stance(); got != want {
			t.Errorf("input %s: go %+v, python %+v", row.Input, got, want)
		}
	}
}

// TestTheCeilingReadsTheEnvironmentAsPythonDoes covers the two rows that carry
// the design: unset means uncapped, and unreadable means OFF. Failing closed is
// the decision — a typo in a deployment variable costs a feature, never opens
// one — and it is one `return` away from failing open.
func TestTheCeilingReadsTheEnvironmentAsPythonDoes(t *testing.T) {
	c := loadStanceCorpus(t)
	for _, row := range c.Ceilings {
		if row.Env == nil {
			os.Unsetenv(CeilingEnv)
		} else {
			t.Setenv(CeilingEnv, *row.Env)
		}
		if got, want := Ceiling(), row.Ceiling.stance(); got != want {
			env := "<unset>"
			if row.Env != nil {
				env = *row.Env
			}
			t.Errorf("ceiling with %q: go %+v, python %+v", env, got, want)
		}
	}
	os.Unsetenv(CeilingEnv)
}

// statused is the one field DefaultFor reads. A nil pointer stands for a deck
// object with no status at all, which is the row a real Deck cannot produce.
type statused struct{ status string }

func (s statused) DeckStatus() string { return s.status }

func TestTheDeckDefaultAgreesWithPython(t *testing.T) {
	c := loadStanceCorpus(t)
	for _, row := range c.Defaults {
		var deck DeckStatused = statused{}
		if row.Status != nil {
			deck = statused{*row.Status}
		}
		if got, want := DefaultFor(deck), row.Stance.stance(); got != want {
			t.Errorf("default for status %v: go %+v, python %+v", row.Status, got, want)
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
		t.Errorf("no deck, no request: go %+v, want %+v", got, Off)
	}
}

// TestOffIsTheFloorOfEveryFallback is the invariant behind three separate
// behaviours, asserted once as itself: a request that cannot be read, a
// mapping that names only some axes, and a ceiling that cannot be parsed all
// land at or below OFF's level on every axis. It is what makes "a malformed
// request can only ever be quieter" a property rather than three coincidences.
func TestOffIsTheFloorOfEveryFallback(t *testing.T) {
	partial, err := StanceFromObj(json.RawMessage(`{"write":"applies"}`))
	if err != nil {
		t.Fatalf("partial mapping: %v", err)
	}
	if partial.Initiative != Off.Initiative || partial.Scope != Off.Scope {
		t.Errorf("unnamed axes did not fall to OFF: %+v", partial)
	}
	t.Setenv(CeilingEnv, "not-a-preset")
	if got := Ceiling(); got != Off {
		t.Errorf("an unreadable ceiling must fail closed, got %+v", got)
	}
	// And the clamp that follows from it: at an OFF ceiling nothing may call.
	resolved, err := Resolve("collaborator", nil, nil)
	if err != nil {
		t.Fatalf("resolving under an off ceiling: %v", err)
	}
	if resolved.AllowsCalls() {
		t.Errorf("an off ceiling still allowed calls: %+v", resolved)
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
