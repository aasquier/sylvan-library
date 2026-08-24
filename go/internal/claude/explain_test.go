package claude

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
)

// The sentences somebody reads at 2am, and the stance comparison the modes
// gate themselves on.
//
// `Explain` exists because an API failure has to say **what to do next**, and
// the four it names are four different next moves: a lapsed key is replaced
// at platform.claude.com, a 403 is usually a model the workspace cannot
// reach, a 429 is waited out, and "could not reach api.anthropic.com" is a
// network problem rather than an Anthropic one. Collapsing any of them into
// "the request failed" costs an hour of looking in the wrong place -- and the
// expiry case is not hypothetical here, since this project's key carries a
// fixed lifetime.

// Each status gets its own sentence, naming its own next move.
func TestEachFailureNamesItsOwnNextMove(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		status int
		wants  []string
	}{
		{"a rejected key", 401, []string{"401", "expired", "platform.claude.com"}},
		{"a refused request", 403, []string{"403", "workspace", "claude-opus-5"}},
		{"rate limiting", 429, []string{"429", "Retry"}},
		{"no such model", 404, []string{"404", "claude-opus-5"}},
		{"anything else", 500, []string{"500"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := Explain(apiError(tc.status), "claude-opus-5")
			for _, want := range tc.wants {
				if !strings.Contains(got, want) {
					t.Errorf("a %d says %q, want it to mention %q", tc.status, got, want)
				}
			}
			// Never a bare status: the number alone is not a next move.
			if strings.TrimSpace(got) == fmt.Sprint(tc.status) {
				t.Errorf("a %d says only its own number", tc.status)
			}
		})
	}
}

// A transport failure is **not** an Anthropic failure, and saying so is the
// difference between checking the network and checking the key.
func TestATransportFailureSaysTheNetworkRatherThanTheKey(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		err  error
	}{
		{"connection refused", errors.New("dial tcp 1.2.3.4:443: connect: connection refused")},
		{"no such host", errors.New("dial tcp: lookup api.anthropic.com: no such host")},
		{"a bare dial failure", errors.New("dial tcp: some other problem")},
		{"a timeout", &timeoutError{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := Explain(tc.err, "claude-opus-5")
			if !strings.Contains(got, "could not reach api.anthropic.com") {
				t.Errorf("a transport failure says %q", got)
			}
			// It does not blame the key, which is the wrong hour to spend.
			if strings.Contains(got, "expired") || strings.Contains(got, "401") {
				t.Errorf("a network problem was reported as a key problem: %q", got)
			}
		})
	}

	// Something that is neither is passed through rather than dressed up as
	// a network failure.
	other := errors.New("something else entirely")
	if got := Explain(other, "claude-opus-5"); got != other.Error() {
		t.Errorf("an unrelated error was rewritten as %q", got)
	}
	// And no error at all says nothing at all.
	if got := Explain(nil, "claude-opus-5"); got != "" {
		t.Errorf("no error explained as %q", got)
	}
}

// The connection predicate matches on shape where the standard library gives
// one, and falls back to text only for what it does not.
func TestTheConnectionPredicateMatchesShapeBeforeText(t *testing.T) {
	t.Parallel()
	// A real net.Error, matched structurally.
	if !isConnection(&timeoutError{}) {
		t.Error("a net.Error timeout was not recognised")
	}
	// Wrapped, because a caller's context is always in the way.
	if !isConnection(fmt.Errorf("asking Anthropic: %w", &timeoutError{})) {
		t.Error("a wrapped timeout was not recognised")
	}
	for _, text := range []string{
		"dial tcp 1.2.3.4:443: connect: connection refused",
		"lookup api.anthropic.com: no such host",
		"dial tcp: i/o timeout",
	} {
		if !isConnection(errors.New(text)) {
			t.Errorf("%q was not recognised as a transport failure", text)
		}
	}
	// And what is not one.
	for _, text := range []string{
		"invalid request", "the model refused", "context canceled",
	} {
		if isConnection(errors.New(text)) {
			t.Errorf("%q was read as a transport failure", text)
		}
	}
}

// The unavailable sentinel is passed through whole: it is already the
// sentence a person should read, and re-explaining it would bury it.
func TestTheUnavailableSentinelIsPassedThroughWhole(t *testing.T) {
	t.Parallel()
	got := Explain(ErrUnavailable, "claude-opus-5")
	if got != ErrUnavailable.Error() {
		t.Errorf("the unavailable sentinel was rewritten as %q", got)
	}
	// Wrapped, likewise -- a caller's context must not lose it.
	wrapped := fmt.Errorf("checking the pipe: %w", ErrUnavailable)
	if got := Explain(wrapped, "claude-opus-5"); !strings.Contains(got, ErrUnavailable.Error()) {
		t.Errorf("a wrapped sentinel explained as %q", got)
	}
}

// **A stance can only ever be quieter than intended, never louder.** The
// comparison the modes gate themselves on has to agree with that: a level
// nobody defined is an error rather than a silent pass.
func TestAStanceComparesAlongItsOwnAxesAndRefusesTheRest(t *testing.T) {
	t.Parallel()

	// The bottom of every axis is at least itself and nothing above it.
	for _, axis := range Axes {
		at, err := Off.AtLeast(axis, Off.get(axis))
		if err != nil {
			t.Fatalf("%s: %v", axis, err)
		}
		if !at {
			t.Errorf("%s: the floor is not at least itself", axis)
		}
	}

	// An axis nobody has, and a level nobody defined, are both errors --
	// never a quiet false, which would read as "this mode is not allowed"
	// and never a quiet true, which would be the dangerous direction.
	if _, err := Off.AtLeast("nonsense", "off"); err == nil {
		t.Error("an axis nobody has compared successfully")
	}
	if _, err := Off.AtLeast("initiative", "nonsense"); err == nil {
		t.Error("a level nobody defined compared successfully")
	}

	// A stance above the floor is at least the floor, and the floor is not
	// at least it -- the ordering is real rather than an equality test.
	louder := Stance{"volunteers", "flagged", "none"}
	at, err := louder.AtLeast("initiative", "off")
	if err != nil || !at {
		t.Errorf("a louder initiative is not at least the floor: %v %v", at, err)
	}
	at, err = Off.AtLeast("initiative", "volunteers")
	if err != nil {
		t.Fatal(err)
	}
	if at {
		t.Error("the floor is at least a louder initiative")
	}
}

// The write axis is what every edit gates on, and the floor cannot write.
func TestTheWriteFloorCannotWrite(t *testing.T) {
	t.Parallel()
	if Off.MayWrite() {
		t.Error("the floor may write -- a half-written stance would be louder than intended")
	}
	if !(Stance{"off", "flagged", "propose"}).MayWrite() {
		t.Error("a stance above the floor may not write")
	}
}

// apiError builds an SDK error with the given status, the way the transport
// hands one back. The type is the SDK's own, so `errors.As` matches it
// exactly as it does in production -- a hand-rolled stand-in would prove
// nothing about the branch that actually runs.
func apiError(status int) error {
	// The default branch renders through the SDK's own `Error()`, which
	// reads the request and the response -- so both are present here, the
	// way a real transport hands them back.
	req, _ := http.NewRequest(http.MethodPost, "https://api.anthropic.com/v1/messages", nil)
	return &anthropic.Error{
		StatusCode: status,
		Request:    req,
		Response:   &http.Response{StatusCode: status},
	}
}

// timeoutError is a net.Error, which is what the structural match looks for.
type timeoutError struct{}

func (*timeoutError) Error() string   { return "i/o timeout" }
func (*timeoutError) Timeout() bool   { return true }
func (*timeoutError) Temporary() bool { return true }

var _ net.Error = (*timeoutError)(nil)
