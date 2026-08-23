package claude

import (
	"bytes"
	"encoding/json"
	"os"
	"reflect"
	"slices"
	"testing"
)

// The dial, held to Python's own payload.
//
// The corpus records `service.claude_status`'s whole answer as **serialised
// JSON**, so this compares marshalled bytes and not fields. That is the
// `tier1.Number` lesson in its home package: a struct whose values are all
// right and whose field order is wrong is bit-exact by every field-by-field
// assertion and still wrong on the wire, and the settings gear renders this
// payload in order.

// dialRow is one case of stance.json's `dial` table.
type dialRow struct {
	Note       string          `json:"note"`
	Requested  json.RawMessage `json:"requested"`
	DeckStatus *string         `json:"deck_status"`
	Surface    *string         `json:"surface"`
	Ceiling    *string         `json:"ceiling"`
	Payload    json.RawMessage `json:"payload"`
	Error      string          `json:"error"`
}

// withDialEnv pins the three environment facts the dial reads at call time, to
// the same values `dial_cases` rendered under. Without this the corpus would
// be a record of whichever shell rendered it: a key in the environment makes
// `configured` true, and `MTGLAB_CLAUDE_MODEL` makes `model` anything at all.
func withDialEnv(t *testing.T, ceiling string) {
	t.Helper()
	for _, name := range []string{"ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN", "MTGLAB_CLAUDE_MODEL"} {
		t.Setenv(name, "")
		if err := os.Unsetenv(name); err != nil {
			t.Fatalf("unset %s: %v", name, err)
		}
	}
	if ceiling == "" {
		t.Setenv(CeilingEnv, "")
		if err := os.Unsetenv(CeilingEnv); err != nil {
			t.Fatalf("unset %s: %v", CeilingEnv, err)
		}
		return
	}
	t.Setenv(CeilingEnv, ceiling)
}

func TestTheDialAgreesWithPython(t *testing.T) {
	corpus := loadStanceCorpus(t)
	if len(corpus.Dial) == 0 {
		t.Fatal("stance.json carries no dial cases; run `python tests/go_fixtures.py`")
	}
	for _, row := range corpus.Dial {
		t.Run(row.Note, func(t *testing.T) {
			withDialEnv(t, deref(row.Ceiling))

			// `payload.get("stance") or None` is the route's; the corpus
			// records what reached `claude_status`, which is already that.
			var requested any
			if len(row.Requested) > 0 && string(row.Requested) != "null" {
				// UseNumber, for the reason the stance parser needs it:
				// Python's json tells `7` from `7.5` by the literal, so a
				// plain float64 decode collapses two different refusal
				// sentences into one.
				decoder := json.NewDecoder(bytes.NewReader(row.Requested))
				decoder.UseNumber()
				if err := decoder.Decode(&requested); err != nil {
					t.Fatalf("decoding the requested stance: %v", err)
				}
			}
			var deck DeckStatused
			if row.DeckStatus != nil {
				deck = DeckWithStatus(*row.DeckStatus)
			}

			got, err := Status(requested, deck, deref(row.Surface))
			if row.Error != "" {
				if err == nil {
					t.Fatalf("answered where Python refused with %q", row.Error)
				}
				if err.Error() != row.Error {
					t.Errorf("refused with\n  %q\nPython says\n  %q", err, row.Error)
				}
				return
			}
			if err != nil {
				t.Fatalf("refused with %q where Python answered", err)
			}
			assertSameJSONValue(t, "dial", got, row.Payload)
		})
	}
}

// The list of modes the dial publishes is **six of the seven**, in Python's
// order, and `scan` is the one missing.
//
// This test exists to pin a wart in both directions at once, which is unusual
// enough to argue. `service.claude_status` builds its list by naming five
// functions and splatting a sixth; ADR 34's scan landed in #180 and never
// joined it, so the payload whose own comment calls itself "the modes that
// exist" has been one short since. Reproducing that is the rule -- a flip that
// changes behaviour is not a flip -- but reproducing it silently would make
// the Go side look like it had merely forgotten too.
//
// So: the ORDER is asserted, because it is contract; the ABSENCE is asserted,
// because it is a bug; and the absence is asserted against the *derived* list
// rather than a second literal, so the day a mode is added this fails and asks
// whether the dial should learn about it.
func TestTheDialListsPythonsSixModesAndNotTheSeventh(t *testing.T) {
	got := DialModes()
	names := make([]string, 0, len(got))
	for _, m := range got {
		names = append(names, m.Name)
	}
	want := []string{
		ModeRationaleInterview, ModeSlotArgument, ModeCommanderDossier,
		ModeResearch, ModeThemeConversation, ModeThemeProposal,
	}
	if !reflect.DeepEqual(names, want) {
		t.Errorf("the dial lists\n  %v\nPython lists\n  %v", names, want)
	}
	// Every mode that exists, minus the ones the dial names: exactly `scan`.
	missing := []string{}
	for _, name := range ModeNames() {
		if !slices.Contains(names, name) {
			missing = append(missing, name)
		}
	}
	if !reflect.DeepEqual(missing, []string{ModeScan}) {
		t.Errorf("the dial omits %v; Python omits exactly [scan]. If a mode "+
			"was just added, the question is whether `/api/claude` should "+
			"report it -- in BOTH runtimes, since this reproduces Python's "+
			"list rather than deriving one", missing)
	}
}

// `?surface=scan` resolves `off`, and `scan.stance_for` exists to stop exactly
// that. The same omission as the modes list, one layer along: `_SURFACE_DEFAULTS`
// names `theme` and `research` and was never extended when ADR 34 landed.
//
// Pinned rather than fixed, and pinned with the two surfaces that DO work
// beside it -- so this reads as "scan is the odd one out" rather than as an
// assertion that off is right.
func TestAScanSurfaceStillResolvesOffWhichIsPythonsWart(t *testing.T) {
	withDialEnv(t, "")
	for surface, want := range map[string]string{
		"theme":    "second-opinion",
		"research": "second-opinion",
		"scan":     "off",
		"":         "off",
	} {
		dial, err := Status(nil, nil, surface)
		if err != nil {
			t.Fatalf("surface %q: %v", surface, err)
		}
		if dial.Stance.Preset == nil || *dial.Stance.Preset != want {
			t.Errorf("surface %q resolved %v, Python resolves %q",
				surface, dial.Stance.Preset, want)
		}
	}
}

// The one field with no Python analogue, argued rather than assumed.
//
// `installed` asks whether `import anthropic` works, because the SDK rides
// with the `claude` extra. Go has no such question -- the SDK is linked into
// this binary -- so the answer is a constant. The two agree everywhere the
// door runs: the container installs `.[api,claude]`, and after Phase 8 there
// is no extra left to be missing.
func TestInstalledIsAConstantBecauseTheSDKIsLinkedIn(t *testing.T) {
	withDialEnv(t, "")
	dial, err := Status(nil, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if !dial.Installed {
		t.Error("installed is false, which this binary cannot be")
	}
	// And `configured` is genuinely a question, which is the distinction the
	// two fields exist to draw.
	if dial.Configured {
		t.Error("configured is true with no credential in the environment")
	}
	t.Setenv("ANTHROPIC_API_KEY", "sk-test")
	dial, err = Status(nil, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if !dial.Configured {
		t.Error("configured is false with a credential in the environment")
	}
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
