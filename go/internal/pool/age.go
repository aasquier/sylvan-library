package pool

import (
	"os"
	"time"
)

// How old the library's rows are.
//
// **Nothing in the app reported this, and four polish runs said so.**
// [Stale] answers a different question — whether the pool predates the
// *columns* this binary reads — so it says `false` for a pool of any age at
// all. A library six weeks behind is a perfectly healthy pool full of out of
// date legality and out of date prices, and the only age signal anywhere in
// the product was a date sitting inside a filename that somebody had to go and
// read.
//
// The date is Scryfall's own, off the bulk files a refresh parked. That makes
// it the age of the **data** rather than of the file: a rebuild rewrites
// `mtg.duckdb` and moves its mtime, so a pool rebuilt this morning out of a
// month-old download would read as a day old by the file and a month old by
// this, and the second is the one anybody watching cares about.

// BulkDataDay is the day the rows in a pool built from dir's bulk files were
// published — and for a shelf holding more than one kind, **the oldest of
// them.**
//
// The oldest, not the newest, and that is the whole reading. A refresh may load
// one kind and skip the other (`--oracle-only` is supported, and a run that
// fell over between its two loads leaves the same shape), so a shelf can hold a
// day-old oracle file beside a month-old printings file. The pool built from
// those is a month behind on prices and printings whatever the oracle half
// says, and answering with the newer date would be the reassuring number rather
// than the true one.
//
// Per kind it is the *newest* copy, because the older ones are the rollbacks
// [SweepBulk] has not taken yet and describe rows that are no longer loaded.
//
// False when nothing can be read: no directory, a directory that will not
// open, or nothing on the shelf this code recognises. A caller reports that as
// "cannot tell" and never as a number.
func BulkDataDay(dir string) (time.Time, bool) {
	if dir == "" {
		return time.Time{}, false
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return time.Time{}, false
	}
	kinds := make([]string, 0, len(sweptKinds))
	for kind := range sweptKinds {
		kinds = append(kinds, kind)
	}

	newest := map[string]time.Time{}
	for _, entry := range entries {
		// Regular files only, on [SweepBulk]'s terms: a directory wearing a
		// plausible name is not a download.
		if !entry.Type().IsRegular() {
			continue
		}
		kind, stamp := parkedBulkDay(entry.Name(), kinds)
		if kind == "" {
			continue
		}
		// `datedDay` has already agreed the shape is ten characters of digits
		// and hyphens; this is what refuses the ones that are that and still
		// not a day -- a month of 13, a 31st of February. UTC because the stamp
		// is a date with no place in it, and a local zone would make the answer
		// depend on where the machine is.
		day, err := time.ParseInLocation(time.DateOnly, stamp, time.UTC)
		if err != nil {
			continue
		}
		if was, seen := newest[kind]; !seen || day.After(was) {
			newest[kind] = day
		}
	}

	var oldest time.Time
	var found bool
	for _, day := range newest {
		if !found || day.Before(oldest) {
			oldest, found = day, true
		}
	}
	return oldest, found
}
