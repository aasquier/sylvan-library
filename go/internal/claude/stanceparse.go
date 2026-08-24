package claude

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/aasquier/sylvan-library/go/internal/wire"
)

// StanceFromObj parses a stance from a preset name or a mapping of axes,
// which is what arrives on the wire: null, a string, or an object.
//
// Accepts a partial mapping: unnamed axes take Off's value rather than the
// most permissive one, so a malformed or half-written request can only ever be
// quieter than intended. That default is the whole reason this is not a plain
// json.Unmarshal into a Stance — a zero Stance is three empty strings, which
// is not Off, and would fail validation instead of falling back to it.
func StanceFromObj(obj any) (Stance, error) {
	switch v := obj.(type) {
	case nil:
		return Off, nil
	case Stance:
		return v, nil
	case string:
		return Preset(v)
	case map[string]any:
		return stanceFromMap(v)
	case json.RawMessage:
		return stanceFromRaw(v)
	}
	return Stance{}, fmt.Errorf("cannot read a stance from %s", typeName(obj))
}

func stanceFromMap(m map[string]any) (Stance, error) {
	var unknown []string
	for k := range m {
		if k != "initiative" && k != "scope" && k != "write" {
			unknown = append(unknown, k)
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		// The refusal interpolates the sorted axes as a rendered list —
		// square brackets, single-quoted elements, ", " between — and the
		// string reaches a 422 body, so the punctuation is the recorded shape.
		quoted := make([]string, len(unknown))
		for i, u := range unknown {
			quoted[i] = wire.Quote(u)
		}
		return Stance{}, fmt.Errorf("[%s] are not stance axes; expected %s",
			strings.Join(quoted, ", "), strings.Join(Axes, ", "))
	}
	out := Off
	for _, axis := range Axes {
		raw, ok := m[axis]
		if !ok {
			continue
		}
		level, ok := raw.(string)
		if !ok {
			// A non-string level is refused by value, naming the axis and
			// rendering what arrived as a literal — quoting included, since
			// the sentence is a recorded shape.
			return Stance{}, fmt.Errorf("%s is not a %s level; expected one of %s",
				literalAny(raw), axis, strings.Join(levels[axis], ", "))
		}
		out.set(axis, level)
	}
	if err := out.Validate(); err != nil {
		return Stance{}, err
	}
	return out, nil
}

// stanceFromRaw decodes with UseNumber so that 7 and 7.5 stay distinguishable.
//
// They must be. The refusal names the type from the literal — `7` earns
// "cannot read a stance from int" and `7.5` "...from float". Go's default
// decoder makes both a float64 and the two sentences collapse into one. The
// corpus records both, which is how this was found rather than shipped: a
// plain json.Unmarshal passes every structural test and is wrong on exactly
// one character of one error string.
func stanceFromRaw(raw json.RawMessage) (Stance, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return Off, nil
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var decoded any
	if err := dec.Decode(&decoded); err != nil {
		return Stance{}, fmt.Errorf("cannot read a stance from %s", typeName(raw))
	}
	return StanceFromObj(decoded)
}

// typeName spells a decoded JSON value in the refusals' own type vocabulary
// — NoneType, bool, int, float, str, list, dict — because the refusal text
// reaches a 422 body and the corpus holds it.
//
// json.Number carries its own literal, so int and float are told apart by
// the source text: whether it had a decimal point or an exponent, never
// whether the value happens to be integral. `7.0` is a float.
func typeName(obj any) string {
	switch v := obj.(type) {
	case nil:
		return "NoneType"
	case bool:
		return "bool"
	case json.Number:
		if strings.ContainsAny(v.String(), ".eE") {
			return "float"
		}
		return "int"
	case float64:
		return "float"
	case int, int64:
		return "int"
	case string:
		return "str"
	case []any:
		return "list"
	case map[string]any:
		return "dict"
	}
	return "object"
}

// literalAny renders one decoded scalar as a literal for a refusal sentence.
// Only scalars get the exact rendering — a nested list or object under an
// axis key is refused by value like anything else, through the `%v` arm
// rather than the full literal rendering, a residue the corpus never reaches.
func literalAny(v any) string {
	switch x := v.(type) {
	case nil:
		return "None"
	case bool:
		if x {
			return "True"
		}
		return "False"
	case string:
		return wire.Quote(x)
	case json.Number:
		return x.String()
	}
	return fmt.Sprintf("%v", v)
}

// ErrStanceRejected marks a refusal that came from reading the caller's own
// stance rather than from anything the conversation did.
//
// It exists because a route has to answer differently: a malformed stance is
// a 422 and a failed call is a 502, and a mode's orchestration does both in
// one function -- reading the stance and running the conversation -- so the
// difference has to travel on the error rather than on which line failed.
//
// **Wrapped in `Resolve` rather than in each mode**, so the sixth mode inherits
// it the way it inherits everything else. A rule enforced by nothing drifts,
// and this one would drift silently: a mode that forgot would answer 502 for a
// typo in a client's stance, which reads as "the model broke" rather than "you
// sent nonsense".
//
// The message is the parser's own, unchanged -- the stance corpus pins the
// refusal text byte for byte, so this may add a category and never a word.
var ErrStanceRejected = errors.New("the stance could not be read")

// stanceRejection carries a parse refusal under ErrStanceRejected while
// answering with exactly the sentence the parser wrote.
type stanceRejection struct{ err error }

func (e *stanceRejection) Error() string { return e.err.Error() }

// Is makes `errors.Is(err, ErrStanceRejected)` true without putting the
// sentinel's own words anywhere a caller could read them.
func (e *stanceRejection) Is(target error) bool { return target == ErrStanceRejected }

func (e *stanceRejection) Unwrap() error { return e.err }

// Resolve is the stance that actually applies: what was asked, defaulted, then
// capped. The one function callers should need.
//
// requested may be a Stance, a preset name, a partial mapping, or nil — nil
// means "use the deck's default", which is how a UI that has never been
// touched still behaves sensibly. A nil deck with a nil request is Off, not
// Consultant: the deck default exists only when there is a deck to read.
func Resolve(requested any, deck DeckStatused, limit *Stance) (Stance, error) {
	var asked Stance
	if requested == nil {
		if deck == nil {
			asked = Off
		} else {
			asked = DefaultFor(deck)
		}
	} else {
		parsed, err := StanceFromObj(requested)
		if err != nil {
			return Stance{}, &stanceRejection{err: err}
		}
		asked = parsed
	}
	ceil := Ceiling()
	if limit != nil {
		ceil = *limit
	}
	return Clamp(asked, ceil), nil
}

// AxisReadout is one axis of a described stance. A struct in the recorded
// key order, never a map: encoding/json sorts a map's keys and this goes on
// the wire beside a payload the client renders in order.
type AxisReadout struct {
	Axis     string   `json:"axis"`
	Question string   `json:"question"`
	Level    string   `json:"level"`
	Means    string   `json:"means"`
	Levels   []string `json:"levels"`
}

// StanceReadout is a stance rendered for a UI: the axes, their levels, and
// what each means.
//
// Preset is a *string so that "no preset matches" marshals as null rather
// than as "" -- null is the recorded shape, and it is what the client checks
// for.
type StanceReadout struct {
	Preset      *string       `json:"preset"`
	AllowsCalls bool          `json:"allows_calls"`
	MayWrite    bool          `json:"may_write"`
	Axes        []AxisReadout `json:"axes"`
}

// Describe renders a stance for the dial, including preset when the stance
// happens to equal one — so a dial that was set by name can show the name back
// rather than three axis readings.
func Describe(s Stance) StanceReadout {
	var named *string
	for _, name := range PresetNames {
		if presets[name] == s {
			n := name
			named = &n
			break
		}
	}
	out := StanceReadout{Preset: named, AllowsCalls: s.AllowsCalls(), MayWrite: s.MayWrite()}
	for _, axis := range Axes {
		level := s.get(axis)
		out.Axes = append(out.Axes, AxisReadout{
			Axis:     axis,
			Question: axisQuestions[axis],
			Level:    level,
			Means:    levelMeanings[[2]string{axis, level}],
			Levels:   levels[axis],
		})
	}
	return out
}
