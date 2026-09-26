package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/auth"
	"github.com/aasquier/sylvan-library/go/internal/claude"
)

// The refusals a person actually reads, taken at the mapper rather than
// through a route.
//
// Each of these functions turns something that went wrong into a status and a
// sentence, and both halves are decisions somebody argued: the status is what
// a browser branches on, and the sentence is the whole of what a newcomer is
// told (commandment 2). Driving them through a route needs the failure to
// happen for real, which is why the interesting arms of all three had never
// run -- and the arm that had never run is the one a player meets on the worst
// day.
//
// The assertion that matters most is the **last** arm of each, where the cause
// is something nobody predicted: the body must carry the room's own words and
// the cause must go to the log instead (commandment 10 -- no machinery renders,
// and a raw error is machinery wearing a sentence's clothes).

// logged builds an API whose only working part is its log, and hands back the
// buffer it writes to. Every function under test here reads `a.log` and
// nothing else; anything that reached further would fail loudly on a nil
// field rather than quietly passing.
func logged() (*API, *bytes.Buffer) {
	var buf bytes.Buffer
	return &API{log: slog.New(slog.NewTextHandler(&buf, nil))}, &buf
}

// detailOf reads the one key every refusal in this tree answers with.
func detailOf(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct{ Detail string }
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("the refusal was not a JSON object: %v (%q)", err, rec.Body.String())
	}
	return body.Detail
}

// The theme surface's refusals, one status per class.
//
// `refuseTheme` exists rather than a shared `refuseClaude` because of its
// first arm: a readiness floor that has not been met is a 409 and nothing else
// is. The three arms below it are the ones that had never run, and the last is
// the one worth the test -- an unrecognised cause must not be handed to the
// querent verbatim.
func TestEveryThemeRefusalCarriesItsOwnStatus(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		err  error
		want int
		// says is a fragment the body must carry; empty means the body must
		// carry the error's own words.
		says string
	}{
		{"nothing went wrong", nil, 0, ""},
		{
			"the floor is not met",
			&claude.ErrNotReady{Msg: "there is not enough here yet"},
			http.StatusConflict, "",
		},
		{
			"the transcript was refused",
			&claude.ErrTranscriptRejected{Msg: "that reads as somebody else's words"},
			http.StatusUnprocessableEntity, "",
		},
		{
			"the persona is not one of ours",
			&claude.UnknownPersonaError{Requested: "the archmage"},
			http.StatusUnprocessableEntity, "",
		},
		{
			"the stance could not be read",
			fmt.Errorf("reading the dial: %w", claude.ErrStanceRejected),
			http.StatusUnprocessableEntity, "",
		},
		{
			"the pipe is shut",
			fmt.Errorf("asking: %w", claude.ErrUnavailable),
			http.StatusServiceUnavailable, "",
		},
		{
			// The arm nobody designed for, and the reason the flavoured
			// sentence is written out rather than interpolated.
			"something nobody predicted",
			errors.New("dial tcp 10.0.0.1:443: i/o timeout"),
			http.StatusInternalServerError,
			"could not answer that right now",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			a, log := logged()
			rec := httptest.NewRecorder()
			answered := a.refuseTheme(rec, "theme ask", tc.err)

			if tc.want == 0 {
				if answered {
					t.Fatalf("a nil error was refused with %d", rec.Code)
				}
				if rec.Body.Len() != 0 {
					t.Errorf("a nil error wrote a body: %q", rec.Body.String())
				}
				return
			}
			if !answered {
				t.Fatalf("%v was not refused at all, so the route would answer twice", tc.err)
			}
			if rec.Code != tc.want {
				t.Errorf("status %d, want %d", rec.Code, tc.want)
			}

			detail := detailOf(t, rec)
			if tc.says == "" {
				// These four are the querent's own mistake or the room's own
				// state, and the error text is written for them.
				if detail != tc.err.Error() {
					t.Errorf("body %q, want the error's own words %q", detail, tc.err.Error())
				}
				return
			}
			if !strings.Contains(detail, tc.says) {
				t.Errorf("body %q does not carry %q", detail, tc.says)
			}
			// The half that commandment 10 is actually about.
			if strings.Contains(detail, "dial tcp") || strings.Contains(detail, "i/o timeout") {
				t.Errorf("the cause reached the querent: %q", detail)
			}
			if !strings.Contains(log.String(), "i/o timeout") {
				t.Errorf("the cause reached nobody -- the log says %q", log.String())
			}
		})
	}
}

// The admin surface's write refusals, including the arm that is not ADR 17's.
//
// ADR 17 argues two statuses for the account core's two named refusals; the
// third arm is everything else, and it must fall through to the deck
// surface's 500 rather than inventing a status of its own.
func TestEveryAdminWriteRefusalCarriesItsOwnStatus(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		err  error
		want int
		says string
	}{
		{
			"the last admin",
			fmt.Errorf("%w: refusing to disable", auth.ErrLastAdmin),
			http.StatusConflict, "refusing to disable",
		},
		{
			"a tier nobody serves",
			fmt.Errorf("%w: archmage", auth.ErrUnknownTier),
			http.StatusUnprocessableEntity, "no such tier",
		},
		{
			"something nobody predicted",
			errors.New("database is locked"),
			http.StatusInternalServerError, "could not answer that right now",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			a, log := logged()
			rec := httptest.NewRecorder()
			a.refuseAdminWrite(rec, tc.err)

			if rec.Code != tc.want {
				t.Errorf("status %d, want %d", rec.Code, tc.want)
			}
			if detail := detailOf(t, rec); !strings.Contains(detail, tc.says) {
				t.Errorf("body %q does not carry %q", detail, tc.says)
			}
			if tc.want == http.StatusInternalServerError {
				if detail := detailOf(t, rec); strings.Contains(detail, "database") {
					t.Errorf("the cause reached the admin: %q", detail)
				}
				if !strings.Contains(log.String(), "database is locked") {
					t.Errorf("the cause reached nobody -- the log says %q", log.String())
				}
			}
		})
	}
}

// The sentence for names the pool does not know, in both of its shapes.
//
// It is the commander box's answer to a typo, and commandment 2 is the whole
// specification: it says which names, offers the way forward, and does not
// scold. The plural shape had never run, which is the shape somebody pasting
// a decklist's commander line meets.
func TestTheUnknownNameSentenceNamesThemAndOffersAWayOn(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		missing []string
		// Each name must appear, and each of these fragments too.
		carries []string
	}{
		{
			"one name",
			[]string{"Syr Gwynn"},
			[]string{"Check the spelling", "pick one below", "leave this blank"},
		},
		{
			"several names",
			[]string{"Syr Gwynn", "Arahboo", "Atla Palanii"},
			[]string{"3 of those names", "Pick from below", "leave this blank"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := unknownSentence(tc.missing)
			for _, name := range tc.missing {
				if !strings.Contains(got, name) {
					t.Errorf("the sentence does not name %q: %q", name, got)
				}
			}
			for _, want := range tc.carries {
				if !strings.Contains(got, want) {
					t.Errorf("the sentence is missing %q: %q", want, got)
				}
			}
			// Commandment 2 in its checkable half: the words a scolding
			// interface reaches for are not in this room.
			for _, scold := range []string{"invalid", "error", "wrong", "failed", "bad "} {
				if strings.Contains(strings.ToLower(got), scold) {
					t.Errorf("the sentence scolds with %q: %q", scold, got)
				}
			}
		})
	}
}
