package pool_test

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/pool"
)

// The download half of `mtglab data refresh`, driven against a stub Scryfall.
//
// Two properties here are the reason this is worth testing rather than
// trusting: **the dated skip** (a refresh that re-downloaded a gigabyte every
// time would be a bug nobody notices except on the bill) and **the `.part`
// rename** (an interrupted download that left a truncated file under the real
// name would be mistaken for a valid cached copy forever after, and the next
// refresh would skip it).
//
// The format dispatch is the third: Scryfall has served both JSONL and a
// single array, either gzipped, and the reader decides on the first token
// rather than the filename -- so a file somebody decompressed or renamed by
// hand still reads.

// bulkUpdatedAt is the index's `updated_at`, which is what the local copy is
// dated by -- a constant, because the dated skip is what is under test rather
// than the date itself.
const bulkUpdatedAt = "2026-08-24T09:00:00.000+00:00"

// stubScryfall serves a bulk index pointing at its own download endpoint.
type stubScryfall struct {
	*httptest.Server
	downloads int
	body      []byte
	// name is the download URL's last segment, which decides the suffix the
	// local copy is parked under.
	name    string
	updated string
	status  int
}

func newStubScryfall(t *testing.T, name string, body []byte) *stubScryfall {
	t.Helper()
	s := &stubScryfall{body: body, name: name, updated: bulkUpdatedAt, status: http.StatusOK}
	mux := http.NewServeMux()
	mux.HandleFunc("/bulk-data", func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("User-Agent"); got != pool.RefreshUserAgent {
			t.Errorf("the index was asked with User-Agent %q, want the "+
				"long-standing %q -- Scryfall identifies us by it",
				got, pool.RefreshUserAgent)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []any{
				map[string]any{"type": "rulings", "download_uri": "http://example.invalid/rulings.json"},
				map[string]any{
					"type":               "oracle_cards",
					"updated_at":         s.updated,
					"jsonl_download_uri": s.URL + "/files/" + s.name,
				},
			},
		})
	})
	mux.HandleFunc("/files/", func(w http.ResponseWriter, r *http.Request) {
		s.downloads++
		if got := r.Header.Get("User-Agent"); got != pool.RefreshUserAgent {
			t.Errorf("the download was asked with User-Agent %q", got)
		}
		if s.status != http.StatusOK {
			w.WriteHeader(s.status)
			return
		}
		_, _ = w.Write(s.body)
	})
	s.Server = httptest.NewServer(mux)
	t.Cleanup(s.Close)
	return s
}

// The happy path, and the dated skip that keeps a refresh from paying twice.
func TestABulkFileIsParkedUnderItsDateAndSkippedNextTime(t *testing.T) {
	t.Parallel()
	body := []byte(`{"name":"Sol Ring"}` + "\n" + `{"name":"Forest"}` + "\n")
	scryfall := newStubScryfall(t, "oracle-cards.jsonl", body)
	dest := t.TempDir()
	ctx := context.Background()

	path, err := pool.DownloadBulkFrom(ctx, scryfall.URL+"/bulk-data", "oracle_cards", dest)
	if err != nil {
		t.Fatalf("downloading: %v", err)
	}
	if got := filepath.Base(path); got != "oracle_cards-2026-08-24.jsonl" {
		t.Errorf("parked as %q, want the kind and the date", got)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != string(body) {
		t.Errorf("the file is %q, want it exactly as served", raw)
	}
	if scryfall.downloads != 1 {
		t.Fatalf("the first refresh downloaded %d times", scryfall.downloads)
	}

	// The dated copy is already there, so the second refresh does not pay
	// for it again.
	again, err := pool.DownloadBulkFrom(ctx, scryfall.URL+"/bulk-data", "oracle_cards", dest)
	if err != nil {
		t.Fatal(err)
	}
	if again != path {
		t.Errorf("the second refresh answered %q", again)
	}
	if scryfall.downloads != 1 {
		t.Errorf("the second refresh downloaded again (%d total) -- the "+
			"dated skip is not working", scryfall.downloads)
	}
}

// The suffix follows the URL rather than the kind, because a caller reading
// the file decides how to decompress it from the name.
func TestTheLocalSuffixFollowsWhatWasServed(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ served, want string }{
		{"oracle-cards.jsonl.gz", "oracle_cards-2026-08-24.jsonl.gz"},
		{"oracle-cards.jsonl", "oracle_cards-2026-08-24.jsonl"},
		{"oracle-cards.json.gz", "oracle_cards-2026-08-24.json.gz"},
		{"oracle-cards.json", "oracle_cards-2026-08-24.json"},
		// Anything unrecognised falls back to `.json` rather than to no
		// suffix at all.
		{"oracle-cards.bin", "oracle_cards-2026-08-24.json"},
	} {
		t.Run(tc.served, func(t *testing.T) {
			t.Parallel()
			scryfall := newStubScryfall(t, tc.served, []byte("{}\n"))
			path, err := pool.DownloadBulkFrom(context.Background(),
				scryfall.URL+"/bulk-data", "oracle_cards", t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			if got := filepath.Base(path); got != tc.want {
				t.Errorf("parked as %q, want %q", got, tc.want)
			}
		})
	}
}

// **An interrupted download is never mistaken for a valid cached copy.** The
// bytes go to a `.part` and the rename happens only once the body is
// complete, so a failure leaves nothing under the real name -- and nothing
// for the next refresh's dated skip to trip over.
func TestAFailedDownloadLeavesNothingUnderTheRealName(t *testing.T) {
	t.Parallel()
	scryfall := newStubScryfall(t, "oracle-cards.jsonl", nil)
	scryfall.status = http.StatusInternalServerError
	dest := t.TempDir()

	_, err := pool.DownloadBulkFrom(context.Background(),
		scryfall.URL+"/bulk-data", "oracle_cards", dest)
	if err == nil {
		t.Fatal("a 500 was treated as a download")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("the failure said %q", err)
	}

	entries, err := os.ReadDir(dest)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".part") {
			t.Errorf("a failed download left %q under a real name -- the "+
				"next refresh would skip it forever", e.Name())
		}
	}
}

// A bulk type Scryfall does not serve, and an entry with no download URL at
// all, are different failures and say so -- the second because Scryfall's
// index format has changed before and the message has to say what was there.
func TestTheIndexsOwnFailuresAreNamed(t *testing.T) {
	t.Parallel()
	scryfall := newStubScryfall(t, "oracle-cards.jsonl", []byte("{}\n"))
	ctx := context.Background()

	_, err := pool.DownloadBulkFrom(ctx, scryfall.URL+"/bulk-data", "no_such_kind", t.TempDir())
	if err == nil {
		t.Fatal("an unknown bulk type was downloaded")
	}
	if !strings.Contains(err.Error(), "no_such_kind") {
		t.Errorf("the failure did not name the type: %q", err)
	}

	// An entry with neither download key: the message names the keys that
	// were there, because that is the evidence for "the format changed".
	_, err = pool.BulkDownloadURL(map[string]any{"type": "oracle_cards", "size": 100})
	if err == nil {
		t.Fatal("an entry with no download URL produced one")
	}
	for _, want := range []string{"oracle_cards", "index format", "size"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the failure does not mention %q: %q", want, err)
		}
	}

	// And an index that is not reachable at all.
	_, err = pool.DownloadBulkFrom(ctx, "http://127.0.0.1:1/bulk-data",
		"oracle_cards", t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "bulk index") {
		t.Errorf("an unreachable index said %q", err)
	}
}

// The JSONL key wins over the plain one: it is the current format and the
// one the reader is fastest on.
func TestTheJSONLLinkIsPreferredOverThePlainOne(t *testing.T) {
	t.Parallel()
	got, err := pool.BulkDownloadURL(map[string]any{
		"download_uri":       "http://example.invalid/cards.json",
		"jsonl_download_uri": "http://example.invalid/cards.jsonl",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(got, ".jsonl") {
		t.Errorf("chose %q, want the JSONL link", got)
	}

	// The plain one is still taken when it is all there is.
	got, err = pool.BulkDownloadURL(map[string]any{
		"download_uri": "http://example.invalid/cards.json"})
	if err != nil || !strings.HasSuffix(got, ".json") {
		t.Errorf("the plain link came back as %q (%v)", got, err)
	}
	// An empty string is not a link.
	if _, err := pool.BulkDownloadURL(map[string]any{"download_uri": ""}); err == nil {
		t.Error("an empty download URL was accepted")
	}
}

// Both formats Scryfall has served, gzipped and not, dispatched on the first
// token rather than the filename -- so a file somebody decompressed or
// renamed by hand still reads.
func TestBothBulkFormatsReadWhateverTheFileIsCalled(t *testing.T) {
	t.Parallel()
	cards := []string{"Sol Ring", "Llanowar Elves", "Forest"}

	jsonl := ""
	for _, name := range cards {
		jsonl += fmt.Sprintf(`{"name":%q,"cmc":1}`+"\n", name)
	}
	array := "[\n"
	for i, name := range cards {
		if i > 0 {
			array += ",\n"
		}
		array += fmt.Sprintf(`  {"name":%q,"cmc":1}`, name)
	}
	array += "\n]\n"

	for _, tc := range []struct {
		name, filename, body string
		gzipped              bool
	}{
		{"jsonl", "cards.jsonl", jsonl, false},
		{"a json array", "cards.json", array, false},
		{"gzipped jsonl", "cards.jsonl.gz", jsonl, true},
		{"a gzipped array", "cards.json.gz", array, true},
		// The dispatch is on content, so a JSONL file named `.json` reads.
		{"jsonl under the wrong name", "cards.json", jsonl, false},
		{"an array under the wrong name", "cards.jsonl", array, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), tc.filename)
			write(t, path, tc.body, tc.gzipped)

			var seen []string
			if err := pool.IterCards(path, func(card map[string]any) error {
				name, _ := card["name"].(string)
				seen = append(seen, name)
				return nil
			}); err != nil {
				t.Fatalf("reading: %v", err)
			}
			if len(seen) != len(cards) {
				t.Fatalf("read %v, want %v", seen, cards)
			}
			for i := range cards {
				if seen[i] != cards[i] {
					t.Errorf("card %d is %q, want %q -- the order is the file's", i, seen[i], cards[i])
				}
			}
		})
	}
}

// An empty file is no cards rather than an error, and a file that is neither
// format is an error rather than no cards -- the difference between "this
// refresh found nothing" and "this file is not a bulk file" matters to
// whoever is looking at an empty pool.
func TestAnEmptyBulkFileIsNoCardsAndAGarbledOneIsAnError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	empty := filepath.Join(dir, "empty.jsonl")
	write(t, empty, "", false)
	count := 0
	if err := pool.IterCards(empty, func(map[string]any) error { count++; return nil }); err != nil {
		t.Errorf("an empty file errored: %v", err)
	}
	if count != 0 {
		t.Errorf("an empty file yielded %d cards", count)
	}

	garbled := filepath.Join(dir, "garbled.jsonl")
	write(t, garbled, "this is not JSON at all\n", false)
	if err := pool.IterCards(garbled, func(map[string]any) error { return nil }); err == nil {
		t.Error("a file that is not a bulk file read as one")
	}

	// A file that starts as JSON but is not an object or an array.
	scalar := filepath.Join(dir, "scalar.jsonl")
	write(t, scalar, "42\n", false)
	err := pool.IterCards(scalar, func(map[string]any) error { return nil })
	if err == nil {
		t.Error("a bare number read as a bulk file")
	} else if !strings.Contains(err.Error(), "unrecognised bulk format") {
		t.Errorf("the refusal said %q", err)
	}

	// A file that is not there at all.
	if err := pool.IterCards(filepath.Join(dir, "gone.jsonl"),
		func(map[string]any) error { return nil }); err == nil {
		t.Error("a missing file read as an empty one")
	}

	// A `.gz` that is not gzip.
	fake := filepath.Join(dir, "fake.jsonl.gz")
	write(t, fake, "not gzipped\n", false)
	if err := pool.IterCards(fake, func(map[string]any) error { return nil }); err == nil {
		t.Error("a file that only claims to be gzipped read anyway")
	}
}

// The yield's own error stops the walk, which is how a caller aborts a
// refresh part-way without reading the rest of a gigabyte.
func TestTheWalkStopsWhenTheCallerSaysSo(t *testing.T) {
	t.Parallel()
	stop := errors.New("that is enough")

	for _, tc := range []struct{ name, body string }{
		{"jsonl", `{"name":"a"}` + "\n" + `{"name":"b"}` + "\n" + `{"name":"c"}` + "\n"},
		{"an array", `[{"name":"a"},{"name":"b"},{"name":"c"}]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "cards.jsonl")
			write(t, path, tc.body, false)

			seen := 0
			err := pool.IterCards(path, func(map[string]any) error {
				seen++
				if seen == 2 {
					return stop
				}
				return nil
			})
			if !errors.Is(err, stop) {
				t.Fatalf("the walk answered %v, want the caller's own error", err)
			}
			if seen != 2 {
				t.Errorf("the walk read %d cards after being told to stop", seen)
			}
		})
	}
}

// Numbers survive as `json.Number`, which is what keeps a mana value of `1`
// from becoming `1.0` on the way into the pool.
func TestNumbersSurviveTheWalkWithoutBecomingFloats(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "cards.jsonl")
	write(t, path, `{"name":"Sol Ring","cmc":1,"edhrec_rank":42}`+"\n", false)

	var card map[string]any
	if err := pool.IterCards(path, func(c map[string]any) error {
		card = c
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"cmc", "edhrec_rank"} {
		if _, ok := card[key].(json.Number); !ok {
			t.Errorf("%s decoded as %T, not json.Number", key, card[key])
		}
	}
}

// write puts text at path, gzipped if asked.
func write(t *testing.T, path, text string, gzipped bool) {
	t.Helper()
	fh, err := os.Create(path) //nolint:gosec // a test's own temp dir
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = fh.Close() }()
	var w io.Writer = fh
	if gzipped {
		gz := gzip.NewWriter(fh)
		defer func() { _ = gz.Close() }()
		w = gz
	}
	if _, err := io.WriteString(w, text); err != nil {
		t.Fatal(err)
	}
}
