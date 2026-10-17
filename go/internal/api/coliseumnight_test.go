package api

import (
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/aasquier/sylvan-library/go/internal/night"
)

// The hall tells the settings room whether the torches are lit.
//
// The room used to carry the sentence itself — "the torches are not lit yet"
// — and it was true for a month and then false for a fortnight, because the
// two switches that open the arena were set on the instance one night and no
// string in a bundle can know that. So the fact travels on the wire now, read
// off the runner the door ties in, and these tests hold the three states the
// hall can be in: no runner at all (an instance with no `app.db`), a runner
// with no window, and a scheduled night with its window and zone echoed back
// the way the operator wrote them.

func TestWithNoRunnerTheHallSaysTheNightIsNotScheduled(t *testing.T) {
	t.Parallel()
	a := New(Config{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	body := coliseumRoom(t, a)
	n, _ := body["night"].(map[string]any)
	if n == nil {
		t.Fatalf("the hall answered without a night: %v", body)
	}
	if n["scheduled"] != false || n["window"] != "" || n["zone"] != "" {
		t.Errorf("a hall with no runner reports night=%v", n)
	}
}

func TestARunnerWithNoWindowIsNotAScheduledNight(t *testing.T) {
	t.Parallel()
	a := New(Config{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	// The admin's sample hour still works on such a runner; a scheduled night
	// is the window and nothing else.
	runner := night.NewRunner(night.RunnerConfig{
		Settings: night.Settings{Bouts: 6, BoutsPerAccount: 2, Games: 10}, Log: a.log})
	t.Cleanup(runner.Stop)
	a.SetNightRunner(runner)
	n, _ := coliseumRoom(t, a)["night"].(map[string]any)
	if n["scheduled"] != false || n["window"] != "" || n["zone"] != "" {
		t.Errorf("a runner with no window reports night=%v", n)
	}
}

func TestAScheduledNightEchoesItsWindowAndZoneAsTheOperatorWroteThem(t *testing.T) {
	t.Parallel()
	a := New(Config{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	window, err := night.ParseWindow("01:00-04:00")
	if err != nil {
		t.Fatal(err)
	}
	zone, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatal(err)
	}
	runner := night.NewRunner(night.RunnerConfig{
		Settings: night.Settings{Scheduled: true, Window: window, Zone: zone,
			Bouts: 6, BoutsPerAccount: 2, Games: 10},
		Log: a.log})
	t.Cleanup(runner.Stop)
	a.SetNightRunner(runner)
	n, _ := coliseumRoom(t, a)["night"].(map[string]any)
	if n["scheduled"] != true {
		t.Errorf("a scheduled night reports scheduled=%v", n["scheduled"])
	}
	// Echoed, not reformatted: the same two strings `fly secrets list` would
	// show, so a person comparing the room to the instance compares like with
	// like. The words are the frontend's job.
	if n["window"] != "01:00-04:00" {
		t.Errorf("window = %v, want 01:00-04:00", n["window"])
	}
	if n["zone"] != "America/Los_Angeles" {
		t.Errorf("zone = %v, want America/Los_Angeles", n["zone"])
	}
}
