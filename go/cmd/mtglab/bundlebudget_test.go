package main

import (
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

// **The entry chunk has a budget, and the budget lives here, not in prose.**
//
// `web/README.md` carried the entry chunk's size as a written figure twice --
// "~266 kB", then "285 kB raw / 91 kB gzipped, measured 2026-08-24" -- and
// both rotted, because nothing re-measured them; by 2026-09-12 the real chunk
// was 316,739 bytes and the prose was two corrections behind. A number in
// prose is a claim; this file is the same number as a gate, which is the only
// form of it that cannot rot silently. The README now points here instead of
// quoting a figure, and the measured trend lives in the polish ledger's Black
// section, run by run.
//
// **What the budget is for**: the entry chunk is the one script every visit
// downloads before anything renders, and the README's lazy-route rule exists
// to keep it small. The failure this gate catches is the catastrophic shape,
// bought once already: a heavyweight dependency landing in the entry chunk --
// `recharts` was once a *static* import of three lazy routes, which put 113 kB
// gzipped on pages that drew no chart, and the size table looked fine because
// the chunk was split; only the load wasn't. That mistake against today's
// chunk would land ~100 kB gzipped in one merge and fail here by a margin no
// honest feature ever will.
//
// **When it fires**: first suspect a new import in `web/src/App.tsx` or
// anything the eager screens (`Library`, `Login`, `Claim`) pull in -- the fix
// is a `React.lazy` line or a deferred component (`lib/deferred.tsx`), not a
// bigger budget. If the growth is real, deliberate, and argued, raise the
// constant in the same commit that earns it, and say so in the PR. Do not
// raise it to make a red check green.
const entryChunkGzipBudget = 128 * 1024

// gzippedSize is the chunk as the door actually ships it: the serving path
// compresses at the same maximum level the recorded measurements use, so the
// number this gate bounds is the number a browser downloads, not a raw size
// that flatters minified text.
func gzippedSize(t *testing.T, path string) int {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	var n countingWriter
	zw, err := gzip.NewWriterLevel(&n, gzip.BestCompression)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := zw.Write(raw); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return int(n)
}

type countingWriter int64

func (c *countingWriter) Write(p []byte) (int, error) {
	*c += countingWriter(len(p))
	return len(p), nil
}

func TestTheEntryChunkFitsItsBudget(t *testing.T) {
	t.Parallel()
	root := repoRoot(t)
	dist := filepath.Join(root, "web_dist")
	if _, err := os.Stat(dist); err != nil {
		t.Skipf("no built bundle at web_dist: %v", err)
	}
	// The entry chunk by its stable name (`web/vite.config.ts` builds without
	// content hashes, deliberately -- the bundle is committed). If this file
	// is ever renamed, the gate must fail rather than skip: a budget that a
	// rename dodges is prose again.
	entry := filepath.Join(dist, "assets", "app.js")
	if _, err := os.Stat(entry); err != nil {
		t.Fatalf("the entry chunk is not at its recorded name: %v -- if the "+
			"bundle layout changed, move this gate with it", err)
	}
	got := gzippedSize(t, entry)
	if got > entryChunkGzipBudget {
		t.Fatalf("the entry chunk is %d bytes gzipped against a budget of %d "+
			"-- a visit pays this before anything renders. Suspect an eager "+
			"import that belongs behind React.lazy or lib/deferred.tsx; raise "+
			"the budget only for growth that is argued, in the commit that "+
			"earns it", got, entryChunkGzipBudget)
	}
}
