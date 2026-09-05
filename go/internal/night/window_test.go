package night_test

import (
	"strings"
	"testing"
	"time"

	"github.com/aasquier/sylvan-library/go/internal/night"
)

func mustWindow(t *testing.T, s string) night.Window {
	t.Helper()
	w, err := night.ParseWindow(s)
	if err != nil {
		t.Fatalf("ParseWindow(%q): %v", s, err)
	}
	return w
}

func zone(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatalf("LoadLocation(%q): %v", name, err)
	}
	return loc
}

func TestParseWindowReadsTheClockAndRefusesEverythingElse(t *testing.T) {
	t.Parallel()
	for _, good := range []string{"22:00-23:30", "23:30-01:30", "00:00-23:59"} {
		w := mustWindow(t, good)
		if w.String() != good {
			t.Errorf("ParseWindow(%q) renders back as %q", good, w)
		}
	}
	for _, bad := range []string{
		"", "night", "22:00", "22:00-", "-23:00", "24:00-01:00",
		"22:60-23:00", "22:00-23:5", "2:00-03:00", "22.00-23.00",
		"22:00 - 23:00", "22:0a-23:00", "22:00-25:00", "22:00-23:0a",
	} {
		if _, err := night.ParseWindow(bad); err == nil {
			t.Errorf("ParseWindow(%q) accepted a window it cannot mean", bad)
		} else if !strings.Contains(err.Error(), "MTGLAB_NIGHT_WINDOW") {
			t.Errorf("the refusal for %q does not name the switch: %v", bad, err)
		}
	}
	// The two ends on the same minute are neither a zero window nor a whole
	// day, so they are refused rather than guessed at.
	if _, err := night.ParseWindow("22:00-22:00"); err == nil {
		t.Error("a window that opens and closes on the same minute was accepted")
	}
}

func TestCrossingIsAboutMidnightOnly(t *testing.T) {
	t.Parallel()
	if mustWindow(t, "22:00-23:30").Crossing() {
		t.Error("an evening window does not cross midnight")
	}
	if !mustWindow(t, "23:30-01:30").Crossing() {
		t.Error("23:30-01:30 crosses midnight")
	}
}

func TestAtKnowsOpenFromClosedOnAPlainEvening(t *testing.T) {
	t.Parallel()
	la := zone(t, "America/Los_Angeles")
	w := mustWindow(t, "22:00-23:30")
	for _, tc := range []struct {
		name string
		at   time.Time
		open bool
	}{
		{"midday", time.Date(2026, 9, 6, 12, 0, 0, 0, la), false},
		{"the minute before", time.Date(2026, 9, 6, 21, 59, 59, 0, la), false},
		{"the opening minute", time.Date(2026, 9, 6, 22, 0, 0, 0, la), true},
		{"mid-window", time.Date(2026, 9, 6, 22, 45, 0, 0, la), true},
		{"the closing minute", time.Date(2026, 9, 6, 23, 30, 0, 0, la), false},
	} {
		open, key, closes := w.At(tc.at)
		if open != tc.open {
			t.Errorf("%s: open = %v, want %v", tc.name, open, tc.open)
			continue
		}
		if !open {
			continue
		}
		if key != "2026-09-06" {
			t.Errorf("%s: night key %q, want 2026-09-06", tc.name, key)
		}
		if want := time.Date(2026, 9, 6, 23, 30, 0, 0, la); !closes.Equal(want) {
			t.Errorf("%s: closes %v, want %v", tc.name, closes, want)
		}
	}
}

// A window through midnight belongs to the evening it opened: both sides of
// midnight answer the same night key and the same closing instant, which is
// what keeps "tonight" one night in the rows.
func TestAtCarriesACrossingWindowOverMidnight(t *testing.T) {
	t.Parallel()
	la := zone(t, "America/Los_Angeles")
	w := mustWindow(t, "23:30-01:30")
	wantClose := time.Date(2026, 9, 7, 1, 30, 0, 0, la)
	for _, tc := range []struct {
		name string
		at   time.Time
		open bool
	}{
		{"before the evening opens", time.Date(2026, 9, 6, 23, 0, 0, 0, la), false},
		{"before midnight", time.Date(2026, 9, 6, 23, 45, 0, 0, la), true},
		{"after midnight", time.Date(2026, 9, 7, 0, 30, 0, 0, la), true},
		{"the closing minute", time.Date(2026, 9, 7, 1, 30, 0, 0, la), false},
		{"the next afternoon", time.Date(2026, 9, 7, 15, 0, 0, 0, la), false},
	} {
		open, key, closes := w.At(tc.at)
		if open != tc.open {
			t.Errorf("%s: open = %v, want %v", tc.name, open, tc.open)
			continue
		}
		if !open {
			continue
		}
		if key != "2026-09-06" {
			t.Errorf("%s: night key %q, want the evening the window opened", tc.name, key)
		}
		if !closes.Equal(wantClose) {
			t.Errorf("%s: closes %v, want %v", tc.name, closes, wantClose)
		}
	}
}

// The zone is the window's whole meaning: one absolute instant is inside the
// window on one wall and outside it on another, so At reads the wall of the
// location it is handed.
func TestAtHonoursTheZoneItIsHanded(t *testing.T) {
	t.Parallel()
	la := zone(t, "America/Los_Angeles")
	ny := zone(t, "America/New_York")
	w := mustWindow(t, "22:00-23:30")
	instant := time.Date(2026, 9, 6, 22, 30, 0, 0, la)
	if open, _, _ := w.At(instant); !open {
		t.Fatal("22:30 on the Los Angeles wall should be inside 22:00-23:30")
	}
	// The same instant is 01:30 the next morning in New York.
	if open, _, _ := w.At(instant.In(ny)); open {
		t.Error("the same instant read on the New York wall should be outside the window")
	}
}

// The daylight-saving nights, measured rather than assumed (the Go runtime
// resolves wall times around a shift, and its choices are pinned here so a
// change in them is news rather than a mystery):
//
//   - Spring forward (2026-03-08, 02:00 -> 03:00 in Los Angeles): the wall
//     is read literally, so a window spanning the shift is an hour shorter
//     in absolute time. It still opens and still closes; nothing hangs.
//   - A window aimed entirely into the skipped hour lands an hour earlier
//     than its label — the runtime resolves the nonexistent wall time to
//     the standard-time instant — and the night still happens.
//   - Fall back (2026-11-01, 02:00 -> 01:00): the repeated hour makes a
//     crossing window an hour longer in absolute time, and an ambiguous
//     wall time resolves to its first occurrence.
func TestAtDoesSomethingSaneOnTheShiftNights(t *testing.T) {
	t.Parallel()
	la := zone(t, "America/Los_Angeles")

	spanning := mustWindow(t, "01:30-03:30")
	openedAt := time.Date(2026, 3, 8, 1, 30, 0, 0, la)
	open, key, closes := spanning.At(openedAt.Add(10 * time.Minute))
	if !open || key != "2026-03-08" {
		t.Fatalf("the spring-forward night did not open: open=%v key=%q", open, key)
	}
	if got := closes.Sub(openedAt); got != time.Hour {
		t.Errorf("01:30-03:30 across the skipped hour lasted %v; the literal wall reading makes it %v",
			got, time.Hour)
	}

	swallowed := mustWindow(t, "02:15-02:45")
	// 01:20 wall: before the window's label, but exactly where the runtime
	// resolves the skipped 02:15 to — so the window is open here.
	open, key, closes = swallowed.At(time.Date(2026, 3, 8, 1, 20, 0, 0, la))
	if !open || key != "2026-03-08" {
		t.Fatalf("the swallowed window never opened: open=%v key=%q", open, key)
	}
	if want := time.Date(2026, 3, 8, 1, 45, 0, 0, la); !closes.Equal(want) {
		t.Errorf("the swallowed window closes at %v, want the hour-early %v", closes, want)
	}

	fallback := mustWindow(t, "22:00-02:30")
	openedAt = time.Date(2026, 10, 31, 22, 0, 0, 0, la)
	open, key, closes = fallback.At(time.Date(2026, 11, 1, 0, 30, 0, 0, la))
	if !open || key != "2026-10-31" {
		t.Fatalf("the fall-back night did not open: open=%v key=%q", open, key)
	}
	if got, want := closes.Sub(openedAt), 5*time.Hour+30*time.Minute; got != want {
		t.Errorf("22:00-02:30 across the repeated hour lasted %v; the literal wall reading makes it %v",
			got, want)
	}
}
