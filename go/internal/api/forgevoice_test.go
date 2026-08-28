package api

import (
	"errors"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/sim/tier3"
)

// **The words that reached a person on the live site, and must not again.**
//
// Recorded verbatim from the deployed instance on 2026-08-25, when a Tier 3
// match whose worker would not come up answered in the room with the host's
// control plane, the app's name, a machine id, an HTTP status and raw JSON.
// Commandment 10 says no technology backing this site ever renders.
const recitedTheStack = `forge worker: Machines API GET ` +
	`/apps/sylvan-library/machines/080e90dec3d918/wait?state=started&timeout=60 ` +
	`answered 408: {"error":"deadline_exceeded: machine failed to reach ` +
	`desired state, started, currently stopped"}`

// Fragments of the stack, each of which is a thing a player may never be shown.
// Deliberately a list of *substrings* rather than a check that the sentence
// differs from the error — a paraphrase that still said "machine" would pass
// that and fail this.
var neverSaid = []string{
	"Machines API", "machines.dev", "080e90dec3d918", "deadline_exceeded",
	"408", "HTTP", "http", "/apps/", "sylvan-library", "JSON", "{",
	"MTGLAB_", "FLY_", "fly.toml", "Fly", "worker", "machine", "shim",
	"Forge", "forge", "JVM", "Java", "timeout", "API", "504", "503",
}

// Every kind of trouble this surface can meet, said in the room's own words —
// and nothing else. The table is the argument: whatever the arena's own account
// of itself was, what crosses to a person names nothing that computes.
func TestTheArenaNeverRecitesTheStack(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		err  error
	}{
		{"the recorded failure", tier3.NotReady("%s", recitedTheStack)},
		{"a machine that will not wake",
			tier3.NotReady("forge worker: machine started but the shim never " +
				"answered (dial tcp: connection refused)")},
		{"a token that expired",
			tier3.NotReady("forge worker: Machines API GET /apps/x/machines " +
				`answered 401: {"error":"token expired"}`)},
		{"no arena at all",
			tier3.NotInstalled("no Java 21+ found -- set MTGLAB_JAVA to one")},
		{"a match that broke off",
			errors.New("forge worker: the match stream ended without a result")},
		{"a plain unwrapped failure", errors.New("dial tcp 172.19.0.2:8080: i/o timeout")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			said := forgeTrouble(tc.err)
			if said == "" {
				t.Fatal("a failure with nothing to say leaves the room silent")
			}
			for _, leak := range neverSaid {
				if strings.Contains(said, leak) {
					t.Errorf("the room said %q, which contains %q: %q",
						said, leak, tc.err)
				}
			}
			// And it is a sentence rather than a shrug: a person who is told
			// only that something went wrong has been told nothing.
			if len(said) < 40 {
				t.Errorf("the room said %q, which is not an explanation", said)
			}
		})
	}
}

// **The two kinds of trouble get two different sentences**, which is the whole
// reason [tier3.ErrWorkerNotReady] exists. Telling somebody to try again when
// nothing here can ever play a game sends them round a loop with no exit;
// telling them the arena is dark when it is merely opening loses them a match
// they could have had.
func TestComeBackLaterAndNeverAreDifferentSentences(t *testing.T) {
	t.Parallel()
	later := forgeTrouble(tier3.NotReady("the machine is still coming up"))
	never := forgeTrouble(tier3.NotInstalled("no Forge card data"))
	if later == never {
		t.Fatalf("both said %q", later)
	}
	if !strings.Contains(later, "again") {
		t.Errorf("a transient fault does not invite another go: %q", later)
	}
	if strings.Contains(never, "again") {
		t.Errorf("an arena that will never open invites another go: %q", never)
	}
	// Both reassure about the decks, because the decks are the thing a person
	// will otherwise assume they got wrong.
	for _, said := range []string{later, never} {
		if !strings.Contains(said, "decks") {
			t.Errorf("%q leaves a player wondering about their decks", said)
		}
	}
}

// Nothing to say for nothing gone wrong — so a caller can hand this an error
// that may be nil without dressing up a success as a failure.
func TestNoTroubleIsNoSentence(t *testing.T) {
	t.Parallel()
	if said := forgeTrouble(nil); said != "" {
		t.Errorf("a nil error said %q", said)
	}
}
