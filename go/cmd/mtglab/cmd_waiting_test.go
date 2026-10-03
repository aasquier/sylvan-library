package main

import (
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/pool"
)

// The sentence an operator reads while the pool is busy.
//
// It fires only while *another process* is holding the pool's lease, which is
// why the two one-line closures that used to carry it -- one in `data refresh`,
// one in `data snapshot` -- were the last two unreached statements in this file.
// That the pool calls its watcher when the file is locked is `internal/pool`'s
// own proof, with a holder in a child process (`writerlock_test.go`); what
// nothing checked is what the callers then *say*, and that is the half a person
// actually meets.
//
// **The failure it replaced is why the words matter.** A `data refresh` against
// the serving instance used to report a conflicting lock and a path, and then
// succeed a minute later for no visible reason -- so an operator refreshing a
// library somebody was reading was told the database was broken. This line is
// the fix for the symptom: it says somebody is reading, it says how long this
// will wait, and it names nothing underneath (commandment 10).
func TestTheWaitingNoticeSaysSomebodyIsReadingAndHowLongItWillWait(t *testing.T) {
	t.Parallel()
	var said strings.Builder
	waitingOn(&said)()

	out := said.String()
	for _, want := range []string{"the app is reading the pool", "waiting for it"} {
		if !strings.Contains(out, want) {
			t.Errorf("the notice is missing %q: %q", want, out)
		}
	}
	// The budget, from the one constant that bounds the wait rather than from
	// a number written beside it.
	if !strings.Contains(out, pool.WriterWait.String()) {
		t.Errorf("the notice does not say how long it will wait (%s): %q",
			pool.WriterWait, out)
	}
	if !strings.HasSuffix(out, "\n") {
		t.Errorf("the notice does not end its own line: %q", out)
	}
	// Nothing underneath is named: a lock, a file path, a process id and a
	// database's name are all machinery, and the line an operator reads is
	// about the library.
	for _, never := range []string{"lock", "PID", "duckdb", "SQL"} {
		if strings.Contains(out, never) {
			t.Errorf("the notice names %q, which is machinery: %q", never, out)
		}
	}
}
