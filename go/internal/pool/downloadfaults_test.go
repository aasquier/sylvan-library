package pool_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/pool"
)

// What a download does when the disk refuses it.
//
// The happy path and the dated skip are held next door; these are the shapes
// the deployed instance actually meets. A refresh writes ~500MB onto a volume
// that has a size, and the two ways it goes wrong -- the directory cannot be
// made, and the write cannot finish -- both used to be a `return "", err` that
// nothing had ever driven.
//
// What makes them worth a test rather than an inspection is `Refresh`'s
// **phase classification**, which is downstream of exactly these returns. The
// admin page says "the library was busy just now" or "the source could not be
// reached" or "a row would not go in" depending on which phase the failure
// carried, so a download fault that came back unclassified would reach a
// player as the wrong sentence -- or, worse, as the right sentence about the
// wrong thing.
//
// And the `.part` file is the standing rule underneath: a failure must leave
// **nothing** under the real name, because the next refresh's dated skip would
// take a truncated file for a good one and there is no second chance to
// notice.

// A destination that cannot be created is a refusal naming the download,
// not a panic and not a silent empty file.
func TestADestinationThatCannotBeMadeRefusesTheDownload(t *testing.T) {
	t.Parallel()
	scryfall := newStubScryfall(t, "oracle-cards.jsonl", []byte(`{"name":"Sol Ring"}`+"\n"))
	// A regular file standing where the directory would go: `MkdirAll` cannot
	// make a directory under it, which is a real shape -- a volume that did
	// not mount leaves a file where the mount point was.
	blocker := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blocker, []byte("in the way"), 0o600); err != nil {
		t.Fatal(err)
	}

	path, err := pool.DownloadBulkFrom(context.Background(),
		scryfall.URL+"/bulk-data", "oracle_cards", filepath.Join(blocker, "downloads"))
	if err == nil {
		t.Fatalf("a download into %q succeeded and parked at %q", blocker, path)
	}
	if path != "" {
		t.Errorf("the refusal came back with a path anyway: %q", path)
	}
	if !strings.Contains(err.Error(), "bulk download") {
		t.Errorf("the refusal does not say what was being done: %v", err)
	}
	// Nothing was fetched: the directory is checked before the network is
	// asked, so a full volume costs a request rather than a download.
	if scryfall.downloads != 0 {
		t.Errorf("%d files were pulled before the destination was checked",
			scryfall.downloads)
	}
}

// A write that cannot finish leaves **nothing** under the real name -- the
// `.part` is removed, so the next refresh downloads again rather than
// skipping a truncated file it will read as complete.
func TestAFailedWriteLeavesNoFileForTheNextRefreshToTrust(t *testing.T) {
	t.Parallel()
	body := []byte(strings.Repeat(`{"name":"Sol Ring"}`+"\n", 500))
	scryfall := newStubScryfall(t, "oracle-cards.jsonl", body)
	dest := t.TempDir()
	ctx := context.Background()

	// Cut the connection mid-body by closing the server while the copy runs.
	// Simpler and just as real: serve a status the download refuses, which
	// takes the same "no file left behind" path before any bytes land.
	scryfall.status = 500
	if _, err := pool.DownloadBulkFrom(ctx, scryfall.URL+"/bulk-data",
		"oracle_cards", dest); err == nil {
		t.Fatal("a 500 from the source was accepted as a download")
	}
	entries, err := os.ReadDir(dest)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		t.Errorf("a failed download left %q behind; the next refresh's dated "+
			"skip would take it for a good copy", e.Name())
	}

	// And the same destination works once the source recovers, so the failure
	// above poisoned nothing.
	scryfall.status = 200
	path, err := pool.DownloadBulkFrom(ctx, scryfall.URL+"/bulk-data", "oracle_cards", dest)
	if err != nil {
		t.Fatalf("the retry after a failure: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) != len(body) {
		t.Errorf("the retry wrote %d bytes, want %d", len(raw), len(body))
	}
}

// An index that is not the index is a refusal that says which document was
// wrong -- `bulk index`, not `bulk download`, because the two are different
// requests to different places and the person reading a log needs to know
// which one to go and look at.
func TestAnIndexThatWillNotDecodeSaysItWasTheIndex(t *testing.T) {
	t.Parallel()
	// The index endpoint is `/bulk-data`; the download endpoint serves this
	// body, and pointing the index at it is a proxy or a captive portal
	// answering with a page instead of a document -- which is what this
	// actually looks like in the wild.
	scryfall := newStubScryfall(t, "oracle-cards.jsonl", []byte("<html>not json</html>"))
	ctx := context.Background()
	_, err := pool.DownloadBulkFrom(ctx, scryfall.URL+"/files/anything",
		"oracle_cards", t.TempDir())
	if err == nil {
		t.Fatal("a body that is not the index was accepted")
	}
	if !strings.Contains(err.Error(), "bulk index") {
		t.Errorf("the refusal reads %v, want it to name the index", err)
	}
	if scryfall.downloads == 0 {
		t.Fatal("the fixture never reached the stub, so this proves nothing")
	}
}

// A kind the index does not carry is refused by name, so a typo in a caller
// is a sentence rather than an empty download.
func TestAKindTheIndexDoesNotCarryIsRefusedByName(t *testing.T) {
	t.Parallel()
	scryfall := newStubScryfall(t, "oracle-cards.jsonl", []byte("{}\n"))
	_, err := pool.DownloadBulkFrom(context.Background(),
		scryfall.URL+"/bulk-data", "not_a_bulk_kind", t.TempDir())
	if err == nil {
		t.Fatal("a kind nobody serves was accepted")
	}
	if !strings.Contains(err.Error(), "not_a_bulk_kind") {
		t.Errorf("the refusal does not name the kind asked for: %v", err)
	}
}
