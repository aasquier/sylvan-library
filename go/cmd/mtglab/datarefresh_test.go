package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/config"
	"github.com/aasquier/sylvan-library/go/internal/pool"
)

// `mtglab data refresh`, printed end to end, without the network.
//
// **`ci.yml` lists this command as unreachable because it "downloads from
// Scryfall", and the download has had a seam for longer than that note has:**
// [pool.RefreshOptions] carries its own index URL, precisely so a stand-in can
// answer instead. What was missing was one argument between here and there,
// and this is what it buys — the command's *printing*, which is the only part
// of `data refresh` that lives in this package at all.
//
// The printing is the product. An operator watching a refresh on the deployed
// box reads these lines and nothing else: which file is being fetched, where
// it landed, how many rows went in, and whether anything was swept up
// afterwards. Every one of them was written by nothing until now.

// bulkIndex is a stand-in for Scryfall's bulk index, serving two small files
// in the shape the loader reads.
func bulkIndex(t *testing.T) string {
	t.Helper()
	oracle := []map[string]any{{
		"oracle_id": "o-sol-ring", "name": "Sol Ring", "layout": "normal",
		"mana_cost": "{1}", "cmc": 1, "type_line": "Artifact",
		"set": "tst", "released_at": "1993-08-05",
		"color_identity": []string{}, "colors": []string{},
	}, {
		"oracle_id": "o-forest", "name": "Forest", "layout": "normal",
		"cmc": 0, "type_line": "Basic Land -- Forest",
		"set": "tst", "released_at": "1993-08-05",
		"color_identity": []string{"G"}, "colors": []string{},
	}, {
		// Skipped by the loader's own filter, so the count the command prints
		// is the count that went in rather than the count that was offered.
		"oracle_id": "o-token", "name": "Treasure", "layout": "token",
		"set": "tst",
	}}
	printings := []map[string]any{{
		"id": "p-sol-ring", "oracle_id": "o-sol-ring", "name": "Sol Ring",
		"set": "tst", "set_name": "Test", "collector_number": "1",
		"rarity": "uncommon", "released_at": "1993-08-05",
		"finishes": []string{"nonfoil"}, "prices": map[string]any{"usd": "1.50"},
	}}

	var srv *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("/bulk-data", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{
			map[string]any{"type": pool.OracleBulk,
				"updated_at":         "2026-08-24T09:00:00.000+00:00",
				"jsonl_download_uri": srv.URL + "/files/" + pool.OracleBulk + ".jsonl"},
			map[string]any{"type": pool.PrintingsBulk,
				"updated_at":         "2026-08-24T09:00:00.000+00:00",
				"jsonl_download_uri": srv.URL + "/files/" + pool.PrintingsBulk + ".jsonl"},
		}})
	})
	mux.HandleFunc("/files/", func(w http.ResponseWriter, r *http.Request) {
		rows := oracle
		if strings.Contains(r.URL.Path, pool.PrintingsBulk) {
			rows = printings
		}
		for _, row := range rows {
			raw, err := json.Marshal(row)
			if err != nil {
				t.Errorf("encoding a bulk row: %v", err)
				return
			}
			_, _ = w.Write(append(raw, '\n'))
		}
	})
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv.URL + "/bulk-data"
}

// runRefresh drives the command with its index pointed at a stand-in, and
// hands back what an operator would have watched scroll past.
func runRefresh(t *testing.T, cfg config.Config, index string, args ...string) (string, error) {
	t.Helper()
	cmd := dataRefreshCommand(cfg, index)
	cmd.SetArgs(args)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(io.Discard)
	cmd.SilenceUsage, cmd.SilenceErrors = true, true
	err := cmd.Execute()
	return out.String(), err
}

// The whole refresh, in the order an operator reads it: each file announced
// before it is fetched, the path it landed at, then the rows that went in.
//
// **The order is the assertion rather than the lines alone**, because the
// order is what makes the transcript legible — a count printed before the
// download it belongs to would be a count of the previous run.
func TestARefreshSaysWhatItIsFetchingBeforeItFetchesIt(t *testing.T) {
	t.Parallel()
	d := scratchDeployment(t)

	out, err := runRefresh(t, d.Config, bulkIndex(t))
	if err != nil {
		t.Fatalf("a refresh against a stand-in index failed: %v\n%s", err, out)
	}
	t.Logf("data refresh:\n%s", out)

	// The printings download wears `(large)` because it is half a gigabyte
	// and the operator is about to wait for it.
	want := []string{
		"downloading oracle_cards ...",
		"loaded 2 oracle cards",
		"downloading default_cards (large) ...",
		"loaded 1 printings",
	}
	at := 0
	for _, line := range strings.Split(out, "\n") {
		if at < len(want) && strings.Contains(line, want[at]) {
			at++
		}
	}
	if at != len(want) {
		t.Errorf("the transcript reached %q and no further:\n%s", want[min(at, len(want)-1)], out)
	}
	// The token row never went in, so the count is what was shelved rather
	// than what was offered.
	if strings.Contains(out, "loaded 3 oracle cards") {
		t.Errorf("a token was counted as a card:\n%s", out)
	}
	// Each file's path is printed under its own announcement, because the
	// next thing an operator does about a bad download is look at it.
	if !strings.Contains(out, filepath.Join(d.ScryfallDir(), pool.OracleBulk)) {
		t.Errorf("the transcript never said where the oracle file landed:\n%s", out)
	}
	// And the pool is really there afterwards -- the transcript is not a
	// story about a file that was never written.
	if _, err := d.run(t, "cards", "show", "Sol Ring"); err != nil {
		t.Errorf("the refreshed pool cannot answer for a card it loaded: %v", err)
	}
}

// `--oracle-only` is the small refresh: the printings are never asked for, and
// the prices and per-printing art already in the pool keep their last refresh.
//
// The flag's promise is the *absence* of a line, which is the sort of thing
// that rots silently -- a refresh that quietly fetched half a gigabyte anyway
// would look exactly like this one from the outside except for the wait.
func TestAnOracleOnlyRefreshNeverAsksForThePrintings(t *testing.T) {
	t.Parallel()
	d := scratchDeployment(t)

	out, err := runRefresh(t, d.Config, bulkIndex(t), "--oracle-only")
	if err != nil {
		t.Fatalf("an oracle-only refresh failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "loaded 2 oracle cards") {
		t.Errorf("the oracle half did not land:\n%s", out)
	}
	if strings.Contains(out, "default_cards") {
		t.Errorf("an oracle-only refresh fetched the printings anyway:\n%s", out)
	}
}

// A refresh that cannot take the writer is a refusal rather than a half-built
// pool: the door is taken first and held for the whole run, downloads
// included, so a volume that will not have it is found before a byte is
// fetched.
func TestARefreshOntoAVolumeThatWillNotTakeItIsRefused(t *testing.T) {
	t.Parallel()
	out, err := runRefresh(t, unmounted(t).Config, bulkIndex(t))
	if err == nil {
		t.Fatalf("a refresh onto a volume that did not mount reported success:\n%s", out)
	}
	if strings.Contains(out, "loaded") {
		t.Errorf("the refresh claimed to have shelved something:\n%s", out)
	}
}

// The four sentences a refresh prints, each asked directly, because each of
// them is a rendering rule rather than a step: which file wears `(large)`,
// which count is of printings and which of cards, the line that appears only
// when there is something to wait for, and the sweep that appears only when
// there was something to sweep.
func TestTheRefreshSaysEachOfItsFourSentencesTheRecordedWay(t *testing.T) {
	t.Parallel()

	// `(large)` is on the printings download and nowhere else -- it is there
	// because the operator is about to wait for half a gigabyte.
	if got := bulkLabel(pool.PrintingsBulk); got != pool.PrintingsBulk+" (large)" {
		t.Errorf("the printings download announced itself as %q", got)
	}
	if got := bulkLabel(pool.OracleBulk); got != pool.OracleBulk {
		t.Errorf("the oracle download announced itself as %q", got)
	}
	// The two counts are named for what a person recognises rather than for
	// the bulk file they came out of.
	if got := rowsLabel(pool.PrintingsBulk); got != "printings" {
		t.Errorf("the printings count is labelled %q", got)
	}
	if got := rowsLabel(pool.OracleBulk); got != "oracle cards" {
		t.Errorf("the oracle count is labelled %q", got)
	}

	// The waiting line appears only when there is something to wait for, and
	// says how long it is prepared to wait -- an operator watching a refresh
	// sit silent has no way to tell waiting from hung.
	var waiting bytes.Buffer
	sayWaiting(&waiting)
	if !strings.Contains(waiting.String(), "the app is reading the pool") ||
		!strings.Contains(waiting.String(), pool.WriterWait.String()) {
		t.Errorf("the waiting line reads %q", waiting.String())
	}

	// The sweep is news rather than furniture: silent when there was nothing
	// to tidy, and agreeing with its own count when there was.
	for _, tc := range []struct {
		what  string
		swept pool.SweepCounts
		want  string
	}{
		{"a shelf that was already tidy", pool.SweepCounts{}, ""},
		{"one older copy", pool.SweepCounts{Files: 1, Bytes: 2048},
			"swept 1 older bulk file (2,048 bytes freed)\n"},
		{"several", pool.SweepCounts{Files: 3, Bytes: 1234567},
			"swept 3 older bulk files (1,234,567 bytes freed)\n"},
	} {
		var out bytes.Buffer
		sayTheSweep(&out, tc.swept)
		if out.String() != tc.want {
			t.Errorf("%s swept as %q, want %q", tc.what, out.String(), tc.want)
		}
	}
}
