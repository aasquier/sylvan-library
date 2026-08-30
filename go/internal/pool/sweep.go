package pool

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// The refresh's own housekeeping: the older dated copies it leaves behind.
//
// A download is parked as `<kind>-<date><suffix>` and re-used by date, which
// is what keeps a refresh from paying for half a gigabyte twice
// ([DownloadBulkFrom] argues that half). Nothing ever removed yesterday's, so
// the shelf only ever grew — the deployed volume was holding seven bulk files
// and 317MB of them, all but two of which no code would open again.
//
// **The sweep runs only once a refresh has finished shelving**, and that is
// the design rather than an implementation detail. The previous dated copy is
// the rollback: a run that fell over between the download and the load has
// left the pool holding the *old* rows, and the file those rows came from is
// the only thing on the box that can put them back without the network. So a
// failed refresh sweeps nothing — and a run that skipped its download because
// the date had not moved still sweeps, because how much the shelf is holding
// is not a fact about whether Scryfall published this morning.
//
// **It deletes by shape, never by "everything I did not recognise".** A name
// goes only when it is exactly `<kind>-<date><suffix>` for a kind this code
// downloads, with a real ten-character date and one of the suffixes
// [DownloadBulkFrom] itself parks files under. A `.part` from a download in
// flight, a file somebody decompressed by hand, a directory, a symlink, a
// bulk kind we do not fetch, and anything at all belonging to something else
// are left where they are: `/data/scryfall` is a directory on a volume that
// holds irreplaceable things elsewhere, and a cleaner that reasons from
// absence rather than from evidence is one surprise away from taking one.

// SweepCounts is what one sweep removed: how many files, and the bytes they
// were holding. Both are zero for a shelf that had nothing older on it, which
// is the ordinary case on a box that has been refreshed since this existed.
type SweepCounts struct {
	Files int
	Bytes int64
}

// bulkSuffixes is every suffix a parked bulk file can carry.
//
// One list, read twice: [DownloadBulkFrom] picks the first that matches what
// Scryfall served and falls back to `.json` (the last entry, and the fallback
// for a URL whose name says nothing), and [SweepBulk] matches names against
// the same list. A format Scryfall starts serving is then one edit rather than
// two — and the failure mode of two lists is a file this code downloads and
// never sweeps, which is the bug this whole file exists to fix.
var bulkSuffixes = []string{".jsonl.gz", ".jsonl", ".json.gz", ".json"}

// sweptKinds is the bulk kinds this code downloads, and so the only ones it
// will ever delete. A caller naming anything else sweeps nothing.
var sweptKinds = map[string]bool{OracleBulk: true, PrintingsBulk: true}

// SweepBulk removes the older dated bulk files from dir, keeping the file each
// named kind's refresh just used.
//
// `kept` maps a bulk kind to the path this run read for it, and it is **both
// the spare list and the permission slip**: a kind that is not a key here is
// not swept at all. That is what makes `--oracle-only` safe — such a run never
// looked at the printings, so the newest printings copy is still the only
// local record of rows that are on the shelves right now, and deleting it
// would throw away a rollback for work this run had no opinion about.
//
// **A file that will not delete is not a failed refresh.** The pool is loaded;
// the only casualty is a byte count. A removal that fails is skipped and goes
// uncounted. The error returned is the directory's own: a shelf that cannot be
// read at all is worth saying out loud, because nothing else on this path
// would notice it.
func SweepBulk(dir string, kept map[string]string) (SweepCounts, error) {
	var swept SweepCounts
	kinds := make([]string, 0, len(kept))
	spare := make(map[string]bool, len(kept))
	for kind, used := range kept {
		if !sweptKinds[kind] {
			continue
		}
		kinds = append(kinds, kind)
		if used != "" {
			spare[filepath.Base(used)] = true
		}
	}
	if len(kinds) == 0 {
		return swept, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return swept, fmt.Errorf("bulk sweep: %w", err)
	}
	for _, entry := range entries {
		name := entry.Name()
		// Regular files only: a directory and a symlink both fail this, and
		// neither is something a download put here.
		if !entry.Type().IsRegular() || spare[name] || !parkedBulk(name, kinds) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if err := os.Remove(filepath.Join(dir, name)); err != nil {
			continue
		}
		swept.Files++
		swept.Bytes += info.Size()
	}
	return swept, nil
}

// parkedBulk reports whether name is exactly what [DownloadBulkFrom] parks a
// file for one of kinds under: the kind, a hyphen, a date, and one of the
// suffixes it uses. Everything else on the shelf is somebody else's.
func parkedBulk(name string, kinds []string) bool {
	for _, kind := range kinds {
		rest, ok := strings.CutPrefix(name, kind+"-")
		if !ok {
			continue
		}
		for _, suffix := range bulkSuffixes {
			if stamp, cut := strings.CutSuffix(rest, suffix); cut && datedDay(stamp) {
				return true
			}
		}
	}
	return false
}

// datedDay is Scryfall's `updated_at` cut to its day — `2026-08-24`, exactly
// ten characters with hyphens in the two places.
//
// Deliberately stricter than what the download will write: an index that
// served a short or missing `updated_at` parks a file under a stamp this
// refuses, and a file the sweep cannot confidently name is a file it leaves
// alone. Erring that way costs a few megabytes; erring the other way costs
// something that was not ours to delete.
func datedDay(s string) bool {
	if len(s) != 10 {
		return false
	}
	for i := range len(s) {
		if i == 4 || i == 7 {
			if s[i] != '-' {
				return false
			}
			continue
		}
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}
