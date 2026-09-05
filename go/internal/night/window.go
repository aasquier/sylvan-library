package night

import (
	"fmt"
	"time"
)

// Window is the nightly stretch the arena is open: two wall-clock minutes,
// "HH:MM-HH:MM", read in whichever zone the deployment configured. A night
// is a window and not a moment (ADR 46 decision 2): open, run until it
// closes, so overrun is a state rather than an accident.
//
// The wall clock is read literally, which is the whole of this type's
// position on daylight saving. On a shift night the window is simply an hour
// shorter or longer in absolute time — it opens when the wall says open and
// closes when the wall says close — and a window aimed into the skipped
// spring-forward hour lands an hour earlier than its label, because that is
// where the runtime resolves a wall time that never happened. Measured, not
// assumed: [Window.At]'s tests pin all three shift behaviours against a real
// zone.
type Window struct {
	// Minutes since local midnight. close <= open means the window runs
	// through midnight and closes the next day.
	open, close int
}

// ParseWindow reads "HH:MM-HH:MM", 24-hour, both ends required. The two ends
// on the same minute are refused rather than read as a zero or 24-hour
// window: neither reading is obviously the one meant, and a night that runs
// all day on a typo is the worse guess.
func ParseWindow(s string) (Window, error) {
	const shape = "MTGLAB_NIGHT_WINDOW %q is not HH:MM-HH:MM (\"23:30-01:30\" opens before midnight and closes after)"
	if len(s) != 11 || s[5] != '-' {
		return Window{}, fmt.Errorf(shape, s)
	}
	open, ok := clockMinutes(s[:5])
	if !ok {
		return Window{}, fmt.Errorf(shape, s)
	}
	closeAt, ok := clockMinutes(s[6:])
	if !ok {
		return Window{}, fmt.Errorf(shape, s)
	}
	if open == closeAt {
		return Window{}, fmt.Errorf(
			"MTGLAB_NIGHT_WINDOW %q opens and closes on the same minute; give the night some room", s)
	}
	return Window{open: open, close: closeAt}, nil
}

// clockMinutes reads a strict "HH:MM" into minutes since midnight.
func clockMinutes(s string) (int, bool) {
	if len(s) != 5 || s[2] != ':' {
		return 0, false
	}
	for _, i := range []int{0, 1, 3, 4} {
		if s[i] < '0' || s[i] > '9' {
			return 0, false
		}
	}
	h := int(s[0]-'0')*10 + int(s[1]-'0')
	m := int(s[3]-'0')*10 + int(s[4]-'0')
	if h > 23 || m > 59 {
		return 0, false
	}
	return h*60 + m, true
}

// Crossing reports whether the window runs through midnight.
func (w Window) Crossing() bool { return w.close < w.open }

// String renders the window back the way it was configured, for a log line.
func (w Window) String() string {
	return fmt.Sprintf("%02d:%02d-%02d:%02d", w.open/60, w.open%60, w.close/60, w.close%60)
}

// At reports whether the window is open at t, read on the wall clock of t's
// own location — hand it `now.In(zone)`. When it is open, it also answers
// the two facts a run row needs: the night's key (the local date the window
// opened, so a night that crosses midnight keeps the key of the evening it
// started) and the instant the window closes.
func (w Window) At(t time.Time) (open bool, nightKey string, closes time.Time) {
	year, month, day := t.Date()
	loc := t.Location()
	// The window that opened today — and, when the window crosses midnight,
	// the one that opened yesterday and is still running into this morning.
	openDays := []int{day}
	if w.Crossing() {
		openDays = append(openDays, day-1)
	}
	for _, d := range openDays {
		closeDay := d
		if w.Crossing() {
			closeDay = d + 1
		}
		// time.Date normalises both an out-of-range day number and a wall
		// time the zone skipped, which is the literal-wall-clock rule above.
		from := time.Date(year, month, d, w.open/60, w.open%60, 0, 0, loc)
		until := time.Date(year, month, closeDay, w.close/60, w.close%60, 0, 0, loc)
		if !t.Before(from) && t.Before(until) {
			key := time.Date(year, month, d, 12, 0, 0, 0, loc)
			return true, key.Format("2006-01-02"), until
		}
	}
	return false, "", time.Time{}
}
