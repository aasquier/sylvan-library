package jobs

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/aasquier/sylvan-library/go/internal/wire"
)

// The recorded corpus: `testdata/jobs.json` is a frozen golden of what the
// registry answers, and this file holds the package to it.
//
// The behaviour half of the registry -- one locked step, a lane that runs one
// thing at a time -- cannot be recorded, so `registry_test.go` and the race
// detector carry it. What is recorded here is the arithmetic and the
// formatting, which is where an implementation drifts *quietly*: a
// percentage a
// point out and a timestamp with a `Z` on it both look entirely plausible.

type jobsCorpus struct {
	Lanes    map[string]string `json:"lanes"`
	Live     []string          `json:"live"`
	MaxJobs  int               `json:"max_jobs"`
	IDLength int               `json:"id_length"`

	UnknownLane []struct {
		Lane  string `json:"lane"`
		Error string `json:"error"`
	} `json:"unknown_lane"`

	Percent []struct {
		Done  int `json:"done"`
		Total int `json:"total"`
		Want  int `json:"want"`
	} `json:"percent"`

	Stamps []struct {
		At   []int  `json:"at"`
		Want string `json:"want"`
	} `json:"stamps"`

	Payloads []struct {
		Name string `json:"name"`
		Job  struct {
			ID     string `json:"id"`
			Kind   string `json:"kind"`
			Status string `json:"status"`
			Done   int    `json:"done"`
			Total  int    `json:"total"`
			// The nested values arrive as **bytes, not as parsed JSON**, and
			// that is the corpus reporting a real constraint rather than a
			// convenience. `result` and `partial` can hold anything, and the
			// recorded bodies keep insertion order; a `map[string]any`
			// marshals
			// with its keys sorted, so a result carried through a
			// map could not reproduce the body whatever else it did right.
			// Every job family therefore owes its result a struct
			// with the fields in the recorded order.
			ResultJSON  *string `json:"result_json"`
			PartialJSON *string `json:"partial_json"`
			Error       *string `json:"error"`
			Label       string  `json:"label"`
			Owner       *int64  `json:"owner"`
			Key         string  `json:"key"`
			CreatedAt   string  `json:"created_at"`
		} `json:"job"`
		WantJSON string `json:"want_json"`
	} `json:"payloads"`
}

var (
	corpusOnce sync.Once
	corpusData jobsCorpus
	corpusErr  error
)

func corpus(t *testing.T) jobsCorpus {
	t.Helper()
	corpusOnce.Do(func() {
		raw, err := os.ReadFile(filepath.Join("testdata", "jobs.json"))
		if err != nil {
			corpusErr = err
			return
		}
		corpusErr = json.Unmarshal(raw, &corpusData)
	})
	if corpusErr != nil {
		t.Fatalf("the jobs corpus will not load (testdata/jobs.json is a "+
			"frozen golden -- restore it from version control): %v", corpusErr)
	}
	return corpusData
}

func TestTheCorpusIsWholeEnoughToProveAnything(t *testing.T) {
	// A corpus that quietly lost a section still passes every case left in
	// it, which is the failure this project has been bitten by three times.
	c := corpus(t)
	if len(c.Percent) < 40 || len(c.Stamps) < 8 || len(c.Payloads) < 10 ||
		len(c.UnknownLane) < 5 {
		t.Fatalf("the corpus has shrunk: %d percent, %d stamps, %d payloads, "+
			"%d lane refusals", len(c.Percent), len(c.Stamps),
			len(c.Payloads), len(c.UnknownLane))
	}
	// And the half that makes it worth having: at least one percentage that
	// is an exact tie, where half-to-even and half-away-from-zero differ.
	ties := 0
	for _, cse := range c.Percent {
		if cse.Total != 0 && (200*cse.Done)%cse.Total == 0 &&
			(100*cse.Done)%cse.Total != 0 {
			ties++
		}
	}
	if ties < 4 {
		t.Fatalf("only %d exact ties in the percent corpus; without them an "+
			"implementation that used math.Round would pass", ties)
	}
}

func TestThePercentagesAreTheRecordedRounding(t *testing.T) {
	for _, cse := range corpus(t).Percent {
		if got := percent(cse.Done, cse.Total); got != cse.Want {
			t.Errorf("percent(%d, %d) = %d, the corpus says %d",
				cse.Done, cse.Total, got, cse.Want)
		}
	}
}

func TestTheStampsAreIsoformat(t *testing.T) {
	for _, cse := range corpus(t).Stamps {
		at := cse.At
		if len(at) != 7 {
			t.Fatalf("a stamp case has %d fields, want 7", len(at))
		}
		when := time.Date(at[0], time.Month(at[1]), at[2], at[3], at[4],
			at[5], at[6]*1000, time.UTC)
		if got := stamp(when); got != cse.Want {
			t.Errorf("stamp(%v) = %q, the corpus says %q", at, got, cse.Want)
		}
	}
}

func TestAStampIsTruncatedToTheMicrosecond(t *testing.T) {
	// Go's clock has nanoseconds and the recorded format does not, so the
	// nanoseconds
	// have to go somewhere. The recorded answer is truncation -- the format
	// simply has no place to put them -- and the case that catches a rounding
	// implementation is the one that would otherwise carry a fraction into a
	// stamp that must have none.
	when := time.Date(2026, 8, 22, 13, 10, 20, 999, time.UTC)
	if got, want := stamp(when), "2026-08-22T13:10:20+00:00"; got != want {
		t.Errorf("stamp with 999ns = %q, want %q", got, want)
	}
	when = time.Date(2026, 8, 22, 13, 10, 20, 1999, time.UTC)
	if got, want := stamp(when), "2026-08-22T13:10:20.000001+00:00"; got != want {
		t.Errorf("stamp with 1999ns = %q, want %q", got, want)
	}
}

func TestAStampSortsAsTextTheWayItSortsAsTime(t *testing.T) {
	// The job listing sorts on this string, not on an instant, so the elided
	// fraction has to sort below every six-digit one within the same second.
	// It does, because `+` is 0x2B and every digit is above 0x30 -- but that
	// is an argument, and an argument about ordering is a thing to check.
	base := time.Date(2026, 8, 22, 13, 10, 20, 0, time.UTC)
	zero := stamp(base)
	for _, micro := range []int{1, 60, 100000, 999999} {
		later := stamp(base.Add(time.Duration(micro) * time.Microsecond))
		if zero >= later {
			t.Errorf("%q should sort below %q", zero, later)
		}
	}
	next := stamp(base.Add(time.Second))
	if stamp(base.Add(999999*time.Microsecond)) >= next {
		t.Errorf("the last microsecond of a second must sort below the next")
	}
}

// raw carries a nested value into a job as the recorded bytes, so the
// comparison below is of the whole body and not only of the envelope around
// it. A nil stays nil, which is the `null` a poll must see.
func raw(text *string) any {
	if text == nil {
		return nil
	}
	return json.RawMessage(*text)
}

func TestThePayloadIsTheRecordedBytes(t *testing.T) {
	for _, cse := range corpus(t).Payloads {
		t.Run(cse.Name, func(t *testing.T) {
			job := &Job{
				ID: cse.Job.ID, Kind: cse.Job.Kind, Label: cse.Job.Label,
				Key: cse.Job.Key, CreatedAt: cse.Job.CreatedAt,
				status: cse.Job.Status, done: cse.Job.Done,
				total: cse.Job.Total, result: raw(cse.Job.ResultJSON),
				partial: raw(cse.Job.PartialJSON), err: cse.Job.Error,
			}
			if cse.Job.Owner != nil {
				job.Owner = *cse.Job.Owner
			}
			raw, err := wire.Marshal(job.Payload())
			if err != nil {
				t.Fatalf("the payload will not encode: %v", err)
			}
			if string(raw) != cse.WantJSON {
				t.Errorf("payload bytes differ\n got %s\nwant %s",
					raw, cse.WantJSON)
			}
		})
	}
}

func TestNeitherTheOwnerNorTheKeyEverSerialises(t *testing.T) {
	// The payload test above would catch this only because every corpus case
	// happens to carry one; said directly, it is a rule rather than a
	// coincidence. An owner must not serialise because a caller who can see a
	// job already knows whose it is and one who cannot must not learn it
	// exists (ADR 5); a key must not because for the dossier it is the cache
	// key, which names a card.
	job := &Job{ID: "abc", Kind: "claude.dossier", Owner: 41,
		Key: "oracle:deadbeef", CreatedAt: "2026-08-22T00:00:00+00:00",
		status: Queued}
	raw, err := wire.Marshal(job.Payload())
	if err != nil {
		t.Fatalf("the payload will not encode: %v", err)
	}
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatalf("the payload is not JSON: %v", err)
	}
	for _, forbidden := range []string{"owner", "key", "seq"} {
		if _, found := fields[forbidden]; found {
			t.Errorf("%q reached the wire: %s", forbidden, raw)
		}
	}
	want := []string{"id", "kind", "status", "done", "total", "percent",
		"partial", "label", "result", "error", "created_at"}
	if len(fields) != len(want) {
		t.Errorf("the payload has %d fields, want %d: %s",
			len(fields), len(want), raw)
	}
	for _, key := range want {
		if _, found := fields[key]; !found {
			t.Errorf("%q is missing from the payload: %s", key, raw)
		}
	}
}

func TestTheLaneNamesAndTheBoundsAreTheRecordedOnes(t *testing.T) {
	c := corpus(t)
	for name, want := range map[string]Lane{
		"cpu": CPU, "net": NET, "forge": FORGE,
	} {
		if got := c.Lanes[name]; got != string(want) {
			t.Errorf("lane %s is %q here and %q in the corpus", name, want, got)
		}
	}
	if c.MaxJobs != MaxJobs {
		t.Errorf("MaxJobs is %d here and %d in the corpus", MaxJobs, c.MaxJobs)
	}
	if len(c.Live) != 2 || !live(c.Live[0]) || !live(c.Live[1]) {
		t.Errorf("the corpus's live set is %v and this package does not agree", c.Live)
	}
	for _, status := range []string{Done, Errored} {
		if live(status) {
			t.Errorf("%q must not be joinable", status)
		}
	}
}

func TestTheRefusedLaneSaysWhatTheCorpusSays(t *testing.T) {
	r := quietRegistry(t, Config{})
	for _, cse := range corpus(t).UnknownLane {
		if cse.Lane == "" {
			// **The one recorded divergence, and it is Go's zero value
			// rather than a decision.** The recorded refusal treats an
			// explicitly passed `""` as a third
			// thing, distinct from "no lane given". A string field cannot
			// tell "not set"
			// from "set to empty", so `Options{}` -- which is what "no lane
			// given" looks like -- has to mean CPU. Nothing can observe the
			// difference: `Plan.Lane` defaults to CPU as well, and every
			// planner passes a real lane, so no caller has ever produced the
			// empty one. Recorded here rather than left out of the corpus,
			// because a case silently skipped is a case nobody re-examines.
			continue
		}
		_, err := r.Submit("test", func(Progress) (any, error) { return nil, nil },
			Options{Lane: Lane(cse.Lane)})
		if err == nil {
			t.Fatalf("lane %q was accepted", cse.Lane)
		}
		if err.Error() != cse.Error {
			t.Errorf("lane %q refused with %q, the corpus says %q",
				cse.Lane, err, cse.Error)
		}
	}
	// And nothing was recorded, which is why the lane is checked before the
	// insert: a typo must be an error out of the route rather than a job that
	// sits `queued` for ever because nothing was ever going to run it.
	if got := r.All(0); len(got) != 0 {
		t.Errorf("a refused lane recorded %d jobs", len(got))
	}
}

func TestAnIDIsTwelveHexCharacters(t *testing.T) {
	// Twelve lowercase hex characters, and the length is in the corpus so an
	// implementation that
	// reached for a full uuid or a base64 would be caught rather than merely
	// look different.
	want := corpus(t).IDLength
	for range 32 {
		id := randomID()
		if len(id) != want {
			t.Fatalf("id %q is %d characters, the corpus says %d", id, len(id), want)
		}
		for _, c := range id {
			if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
				t.Fatalf("id %q is not lowercase hex", id)
			}
		}
	}
}
