package wire

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMarshalWritesWhatStarletteWrites(t *testing.T) {
	// Compact separators, HTML characters untouched, unicode as it is,
	// no trailing newline: `json.dumps(v, ensure_ascii=False,
	// separators=(",", ":"))`.
	got, err := Marshal(map[string]any{"a": []any{1, "x<y & z"}, "b": "—"})
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"a":[1,"x<y & z"],"b":"—"}`; string(got) != want {
		t.Fatalf("Marshal = %s, want %s", got, want)
	}
}

func TestDetailIsTheEnvelope(t *testing.T) {
	rec := httptest.NewRecorder()
	Detail(rec, 404, "no deck 'x'")
	if rec.Code != 404 || rec.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("%d %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	if rec.Body.String() != `{"detail":"no deck 'x'"}` {
		t.Fatalf("body %s", rec.Body.String())
	}
	if rec.Header().Get("Content-Length") != "24" {
		t.Fatalf("content-length %q", rec.Header().Get("Content-Length"))
	}
}

func TestUnprocessableIsFastAPIsValidationList(t *testing.T) {
	rec := httptest.NewRecorder()
	Unprocessable(rec, IntParsing("query", "limit", "abc"),
		GreaterThanEqual("query", "limit", "0", 1))
	if rec.Code != 422 {
		t.Fatal(rec.Code)
	}
	var body struct {
		Detail []map[string]any `json:"detail"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Detail) != 2 {
		t.Fatalf("%d errors", len(body.Detail))
	}
	first := body.Detail[0]
	if first["type"] != "int_parsing" || first["input"] != "abc" ||
		!strings.Contains(first["msg"].(string), "valid integer") {
		t.Fatalf("first = %v", first)
	}
	if _, has := first["ctx"]; has {
		t.Fatal("int_parsing carries no ctx")
	}
	if loc, _ := first["loc"].([]any); len(loc) != 2 || loc[0] != "query" || loc[1] != "limit" {
		t.Fatalf("loc = %v", first["loc"])
	}
	second := body.Detail[1]
	if ctx, _ := second["ctx"].(map[string]any); ctx["ge"] != float64(1) {
		t.Fatalf("second = %v", second)
	}
	// Key order is pydantic's, which the wire keeps.
	if !strings.HasPrefix(rec.Body.String(), `{"detail":[{"type":"int_parsing","loc":["query","limit"],"msg":`) {
		t.Fatalf("order: %s", rec.Body.String())
	}
	// An empty list is a list, never null.
	rec = httptest.NewRecorder()
	Unprocessable(rec)
	if rec.Body.String() != `{"detail":[]}` {
		t.Fatalf("empty = %s", rec.Body.String())
	}
}

func TestQuoteWritesWhatPythonWrites(t *testing.T) {
	for in, want := range map[string]string{
		"nope":     `'nope'`,
		"no'pe":    `"no'pe"`,
		`say "hi"`: `'say "hi"'`,
		`it's "x"`: `'it\'s "x"'`,
		"a\\b":     `'a\\b'`,
		"a\nb\tc":  `'a\nb\tc'`,
		"café":     `'café'`,
		"x\x01y":   `'x\x01y'`,
		"":         `''`,
		"Æon — é":  `'Æon — é'`,
	} {
		if got := Quote(in); got != want {
			t.Errorf("Quote(%q) = %s, want %s", in, got, want)
		}
	}
}
