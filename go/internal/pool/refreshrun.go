package pool

import (
	"context"
	"errors"
	"time"
)

// One refresh, as a callable sequence rather than as a command's body.
//
// **It exists because there are now two callers and one of them is a
// button.** `mtglab data refresh` had the whole sequence inline; ADR 6's last
// line always said the next step would be "an authenticated admin endpoint
// that starts a refresh as a background job", and two spellings of a
// five-minute, ~500MB, exclusive-lock operation is two things to keep in
// step. This is the one spelling: the command prints from it, the job reports
// progress from it, and neither can drift into a different order of
// operations.
//
// The shape is unchanged from the command's -- take the writer (waiting out
// the app's read lease), fetch the oracle bulk file, load it, then the
// printings -- because that order is what the recorded behaviour is. What is
// new is that failures are *classified*: a caller now knows which of the
// three phases broke without reading the message, which is what lets a
// user-facing surface say what happened in its own words (commandment 10).

// RefreshPhase is where a refresh was when it stopped.
type RefreshPhase string

const (
	// PhaseShelves is taking the pool read-write: the app's own read lease,
	// another process, a permission, a missing directory.
	PhaseShelves RefreshPhase = "shelves"
	// PhaseGather is asking for and downloading a bulk file: the network, and
	// everything on the other end of it.
	PhaseGather RefreshPhase = "gather"
	// PhaseShelve is reading a downloaded file into the pool.
	PhaseShelve RefreshPhase = "shelve"
)

// RefreshError is a failed refresh with its phase attached.
//
// The wrapped error is the real one and `errors.Is`/`errors.As` reach it, so
// nothing loses information by this being here -- a log still prints the
// whole diagnosis. What the phase buys is a caller that can answer *which
// part of the work broke* without matching on a message, which is the
// difference between a room that says "the library was busy just now" and one
// that recites a database's lock error at a person.
type RefreshError struct {
	Phase RefreshPhase
	Err   error
}

// Error is the wrapped error's own message, **unprefixed on purpose.** The
// phase is a classification carried alongside, not a decoration: `mtglab data
// refresh` printed these messages before this type existed and prints exactly
// the same ones now, so wrapping the sequence changed no operator-visible
// text anywhere.
func (e *RefreshError) Error() string { return e.Err.Error() }
func (e *RefreshError) Unwrap() error { return e.Err }

// PhaseOf is the phase a refresh failed in, or empty for an error that did
// not come from one.
func PhaseOf(err error) RefreshPhase {
	var refresh *RefreshError
	if errors.As(err, &refresh) {
		return refresh.Phase
	}
	return ""
}

// RefreshOptions is one run: where the pool is, where the downloads are
// parked, and how patient to be at the writer's door.
//
// A struct of values rather than a `config.Config`, for ADR 39/40's reason:
// this package must not learn how a deployment is described, and a test must
// be able to describe one without installing it on the process.
type RefreshOptions struct {
	// DBPath is the pool file.
	DBPath string
	// ScryfallDir is where the bulk downloads are parked and re-used.
	ScryfallDir string
	// OracleOnly skips the printings half: much smaller, and prices and
	// per-printing art keep their last refresh.
	OracleOnly bool
	// Wait is how long to stand at the writer's door. Zero takes
	// [WriterWait].
	Wait time.Duration
	// IndexURL is the bulk index. Zero takes [BulkIndex]; it is a field for
	// the reason [DownloadBulkFrom] takes a parameter -- a package-level
	// variable a test swapped would be shared state, and every test that
	// touched it would have to be serial.
	IndexURL string
}

// RefreshCounts is what a finished refresh put on the shelves. Printings is
// zero for an oracle-only run, which is not the same claim as "there are no
// printings" -- the ones already there were left alone.
type RefreshCounts struct {
	Oracle    int64
	Printings int64
}

// RefreshWatcher is how a caller follows along. Every field may be nil, and a
// zero watcher is a silent refresh.
//
// Callbacks rather than a channel: both callers want to be told *while
// holding their own state* -- the command writes to its own stdout, the job
// writes to its own progress reporter -- and neither wants a goroutine and a
// drain to do it.
type RefreshWatcher struct {
	// Waiting fires at most once, the first time the writer's door is found
	// shut. An operator watching a refresh sit silent for forty seconds has
	// no way to tell waiting from hung.
	Waiting func()
	// Gathering fires before a bulk file is asked for, Gathered once it is on
	// disk, and Shelved once its rows are in. `kind` is the bulk name, and
	// the two the app uses are the two the command names.
	Gathering func(kind string)
	Gathered  func(kind, path string)
	Shelved   func(kind string, n int64)
}

func (w RefreshWatcher) waiting() {
	if w.Waiting != nil {
		w.Waiting()
	}
}

func (w RefreshWatcher) gathering(kind string) {
	if w.Gathering != nil {
		w.Gathering(kind)
	}
}

func (w RefreshWatcher) gathered(kind, path string) {
	if w.Gathered != nil {
		w.Gathered(kind, path)
	}
}

func (w RefreshWatcher) shelved(kind string, n int64) {
	if w.Shelved != nil {
		w.Shelved(kind, n)
	}
}

// The two bulk files the app loads, named once so the command, the job and
// the tests all say the same words.
const (
	OracleBulk    = "oracle_cards"
	PrintingsBulk = "default_cards"
)

// Refresh downloads Scryfall's bulk data and rebuilds the pool.
//
// **It takes the writer first and holds it for the whole run**, downloads
// included, which is the order the command has always used and is left alone
// deliberately: a refresh that downloaded first would leave a window in which
// the operator's mental model -- "while this runs, the library is shut" --
// stops being true for the first several minutes of it.
func Refresh(ctx context.Context, opt RefreshOptions, watch RefreshWatcher) (RefreshCounts, error) {
	wait := opt.Wait
	if wait <= 0 {
		wait = WriterWait
	}
	index := opt.IndexURL
	if index == "" {
		index = BulkIndex
	}

	db, err := OpenWriterWaiting(ctx, opt.DBPath, wait, watch.waiting)
	if err != nil {
		return RefreshCounts{}, &RefreshError{Phase: PhaseShelves, Err: err}
	}
	defer func() { _ = db.Close() }()

	var counts RefreshCounts

	watch.gathering(OracleBulk)
	oracle, err := DownloadBulkFrom(ctx, index, OracleBulk, opt.ScryfallDir)
	if err != nil {
		return counts, &RefreshError{Phase: PhaseGather, Err: err}
	}
	watch.gathered(OracleBulk, oracle)
	counts.Oracle, err = LoadOracle(ctx, db, oracle)
	if err != nil {
		return counts, &RefreshError{Phase: PhaseShelve, Err: err}
	}
	watch.shelved(OracleBulk, counts.Oracle)

	if opt.OracleOnly {
		return counts, nil
	}

	watch.gathering(PrintingsBulk)
	printings, err := DownloadBulkFrom(ctx, index, PrintingsBulk, opt.ScryfallDir)
	if err != nil {
		return counts, &RefreshError{Phase: PhaseGather, Err: err}
	}
	watch.gathered(PrintingsBulk, printings)
	counts.Printings, err = LoadPrintings(ctx, db, printings)
	if err != nil {
		return counts, &RefreshError{Phase: PhaseShelve, Err: err}
	}
	watch.shelved(PrintingsBulk, counts.Printings)

	return counts, nil
}
