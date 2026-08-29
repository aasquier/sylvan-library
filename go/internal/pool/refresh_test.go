package pool_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/pool"
	"github.com/aasquier/sylvan-library/go/internal/pool/pooltest"
)

type refreshFile struct {
	Oracle []struct {
		Raw  map[string]any `json:"raw"`
		Skip bool           `json:"skip"`
		Row  map[string]any `json:"row"`
	} `json:"oracle"`
	Printings []struct {
		Raw  map[string]any `json:"raw"`
		Skip bool           `json:"skip"`
		Row  map[string]any `json:"row"`
	} `json:"printings"`
}

func loadRefresh(t *testing.T) refreshFile {
	t.Helper()
	raw, err := os.ReadFile("testdata/refresh.json")
	if err != nil {
		t.Fatalf("refresh.json: %v (testdata/refresh.json is a frozen "+
			"golden -- restore it from version control)", err)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var fx refreshFile
	if err := dec.Decode(&fx); err != nil {
		t.Fatal(err)
	}
	return fx
}

// bulkFile writes the raw cards as the JSONL a bulk download holds.
func bulkFile(t *testing.T, cards []map[string]any) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "bulk.jsonl")
	fh, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	enc := json.NewEncoder(fh)
	for _, c := range cards {
		if err := enc.Encode(c); err != nil {
			t.Fatal(err)
		}
	}
	if err := fh.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestTheLoaderBuildsTheRecordedPool is the end-to-end gate: the raw
// tiny-pool cards through DownloadBulk's format, OpenWriter and the Appender
// loaders, read back through the same `GetCards` the app uses, against the
// pool `pooltest` builds from the recorded transformed rows. The two JSON
// text columns differ by encoder (ASCII escapes vs raw UTF-8) and are
// compared through the reader, which parses them — the one knowing
// divergence, invisible to every query.
func TestTheLoaderBuildsTheRecordedPool(t *testing.T) {
	t.Parallel()
	fx := loadRefresh(t)
	rawOracle := make([]map[string]any, 0, len(fx.Oracle))
	names := []string{}
	for _, c := range fx.Oracle {
		rawOracle = append(rawOracle, c.Raw)
		if !c.Skip {
			if name, ok := c.Raw["name"].(string); ok {
				names = append(names, name)
			}
		}
	}
	rawPrintings := make([]map[string]any, 0, len(fx.Printings))
	for _, p := range fx.Printings {
		rawPrintings = append(rawPrintings, p.Raw)
	}

	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "loaded.duckdb")
	db, err := pool.OpenWriter(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	loadedOracle, err := pool.LoadOracle(ctx, db, bulkFile(t, rawOracle))
	if err != nil {
		t.Fatalf("LoadOracle: %v", err)
	}
	loadedPrintings, err := pool.LoadPrintings(ctx, db, bulkFile(t, rawPrintings))
	if err != nil {
		t.Fatalf("LoadPrintings: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	wantOracle, wantPrintings := 0, 0
	for _, c := range fx.Oracle {
		if !c.Skip {
			wantOracle++
		}
	}
	for _, p := range fx.Printings {
		if !p.Skip {
			wantPrintings++
		}
	}
	if int(loadedOracle) != wantOracle || int(loadedPrintings) != wantPrintings {
		t.Fatalf("loaded %d/%d, want %d/%d",
			loadedOracle, loadedPrintings, wantOracle, wantPrintings)
	}

	sort.Strings(names)
	mine := pool.New(path, nil)
	t.Cleanup(mine.Close)
	theirs := pooltest.Open(t)

	var got, want map[string]*pool.CardRecord
	if err := mine.Use(ctx, func(c *pool.Conn) error {
		var err error
		got, err = c.GetCards(ctx, names)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := theirs.Use(ctx, func(c *pool.Conn) error {
		var err error
		want, err = c.GetCards(ctx, names)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if len(got) != len(want) {
		t.Fatalf("resolved %d names, fixture pool resolved %d", len(got), len(want))
	}
	for name, rec := range want {
		if !reflect.DeepEqual(got[name], rec) {
			t.Errorf("%s diverged:\n got %+v\nwant %+v", name, got[name], rec)
		}
	}
	if stale := staleOf(t, path); stale {
		t.Fatal("a freshly loaded pool reads as stale")
	}
}

func staleOf(t *testing.T, path string) bool {
	t.Helper()
	p := pool.New(path, nil)
	t.Cleanup(p.Close)
	var verdict bool
	if err := p.Use(context.Background(), func(c *pool.Conn) error {
		var err error
		verdict, err = pool.Stale(context.Background(), c)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return verdict
}

// TestEveryRowMatchesTheCorpus is the row-level corpus: each raw card
// through
// the builders against the recorded row, value for value —
// the two parsed-JSON columns compared as documents, everything else
// exactly, and the skip verdicts beside them.
func TestEveryRowMatchesTheCorpus(t *testing.T) {
	t.Parallel()
	fx := loadRefresh(t)
	for i, c := range fx.Oracle {
		if got := pool.SkipOracleLayout(c.Raw); got != c.Skip {
			t.Errorf("oracle %d: skip = %v, want %v", i, got, c.Skip)
		}
		compareRow(t, fmt.Sprintf("oracle %d (%v)", i, c.Raw["name"]),
			pool.OracleColumns, pool.OracleRow(c.Raw), c.Row,
			map[string]bool{"legalities": true, "card_faces": true})
	}
	for i, p := range fx.Printings {
		if got := pool.SkipPrinting(p.Raw); got != p.Skip {
			t.Errorf("printing %d: skip = %v, want %v", i, got, p.Skip)
		}
		compareRow(t, fmt.Sprintf("printing %d (%v)", i, p.Raw["name"]),
			pool.PrintingColumns, pool.PrintingRow(p.Raw), p.Row, nil)
	}
}

func compareRow(t *testing.T, label string, columns []string, got []any,
	want map[string]any, parsed map[string]bool) {
	t.Helper()
	if len(got) != len(columns) {
		t.Fatalf("%s: %d values for %d columns", label, len(got), len(columns))
	}
	for i, name := range columns {
		g, w := got[i], want[name]
		if parsed[name] {
			// **The builder hands these over as documents, not as text.** It
			// used to marshal them to a string, and this branch read only
			// that shape -- so when the string went away, `doc` stayed nil
			// and every JSON column compared as null. The recorded value is
			// untouched either way: `testdata/refresh.json` is a frozen
			// golden and this is the *comparison* learning the correct
			// representation, not the golden learning a new one.
			//
			// The string case stays because it is still legal input to this
			// helper and costs one branch. Both sides carry `json.Number`
			// (both were decoded with `UseNumber`), which marshals as a bare
			// number, so `jsonEqual` sees the same text for the same value.
			doc := g
			if s, ok := g.(string); ok {
				if err := json.Unmarshal([]byte(s), &doc); err != nil {
					t.Errorf("%s.%s: unparseable %q", label, name, s)
					continue
				}
			}
			if !jsonEqual(doc, w) {
				t.Errorf("%s.%s parsed documents differ:\n got %v\nwant %v",
					label, name, doc, w)
			}
			continue
		}
		if !valueEqual(g, w) {
			t.Errorf("%s.%s: got %#v, want %#v", label, name, g, w)
		}
	}
}

// valueEqual compares a builder value against the corpus's recorded
// reading: numbers through float64, lists elementwise, dates by their ISO
// text, nil against null.
func valueEqual(g, w any) bool {
	if g == nil || w == nil {
		return g == nil && w == nil
	}
	switch gv := g.(type) {
	case []string:
		wl, ok := w.([]any)
		if !ok || len(wl) != len(gv) {
			return false
		}
		for i := range gv {
			if wl[i] != gv[i] {
				return false
			}
		}
		return true
	case bool:
		wb, ok := w.(bool)
		return ok && wb == gv
	case string:
		ws, ok := w.(string)
		return ok && ws == gv
	case float64:
		return numEqual(w, gv)
	case int32:
		return numEqual(w, float64(gv))
	default:
		// A date reached the corpus as its ISO text.
		if tv, ok := g.(interface{ Format(string) string }); ok {
			ws, sok := w.(string)
			return sok && tv.Format("2006-01-02") == ws
		}
		return false
	}
}

func numEqual(w any, g float64) bool {
	switch wv := w.(type) {
	case json.Number:
		f, err := wv.Float64()
		return err == nil && f == g
	case float64:
		return wv == g
	}
	return false
}

func jsonEqual(a, b any) bool {
	ra, _ := json.Marshal(canonical(a))
	rb, _ := json.Marshal(canonical(b))
	return string(ra) == string(rb)
}

// canonical sorts object keys so two insertion orders compare equal — the
// documents are compared as documents, not as either runtime's text.
func canonical(v any) any {
	switch value := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(value))
		for k := range value {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		out := make(map[string]any, len(value))
		for _, k := range keys {
			out[k] = canonical(value[k])
		}
		return out
	case []any:
		out := make([]any, len(value))
		for i, item := range value {
			out[i] = canonical(item)
		}
		return out
	default:
		return v
	}
}
