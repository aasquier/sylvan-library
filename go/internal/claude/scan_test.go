package claude

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

// The camera's reader, held to Python by testdata/scan.json.

type scanCorpus struct {
	MaxBytes       int      `json:"max_bytes"`
	DefaultPreset  string   `json:"default_preset"`
	MediaTypes     []string `json:"media_types"`
	MediaTypeCases []struct {
		MediaType string `json:"media_type"`
		OK        bool   `json:"ok"`
		Error     string `json:"error"`
	} `json:"media_type_cases"`
	Captures []struct {
		Note  string `json:"note"`
		Image string `json:"image"`
		Data  string `json:"data"`
		Error string `json:"error"`
	} `json:"captures"`
	Sizes []struct {
		Note  string `json:"note"`
		Bytes int    `json:"bytes"`
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	} `json:"sizes"`
	Sightings []struct {
		Note     string            `json:"note"`
		Refused  bool              `json:"refused"`
		Text     string            `json:"text"`
		Sighting map[string]string `json:"sighting"`
	} `json:"sightings"`
	Stances []struct {
		Note      string          `json:"note"`
		Requested json.RawMessage `json:"requested"`
		Ceiling   *string         `json:"ceiling"`
		Stance    json.RawMessage `json:"stance"`
		Error     string          `json:"error"`
	} `json:"stances"`
	Message struct {
		Role   string `json:"role"`
		Blocks []struct {
			Type       string `json:"type"`
			SourceType string `json:"source_type"`
			MediaType  string `json:"media_type"`
			Text       string `json:"text"`
		} `json:"blocks"`
	} `json:"message"`
}

func loadScanCorpus(t *testing.T) scanCorpus {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "scan.json"))
	if err != nil {
		t.Fatalf("reading the scan corpus: %v", err)
	}
	var c scanCorpus
	if err := json.Unmarshal(raw, &c); err != nil {
		t.Fatalf("decoding the scan corpus: %v", err)
	}
	if len(c.Captures) == 0 {
		t.Fatal("the corpus is empty; run `python tests/go_fixtures.py`")
	}
	return c
}

func TestTheScanConstantsArePythons(t *testing.T) {
	c := loadScanCorpus(t)
	if c.MaxBytes != ScanMaxBytes {
		t.Errorf("the cap is %d, Python's is %d", ScanMaxBytes, c.MaxBytes)
	}
	if c.DefaultPreset != ScanDefaultPreset {
		t.Errorf("the default preset is %q, Python's is %q", ScanDefaultPreset, c.DefaultPreset)
	}
	got := make([]string, 0, len(ScanMediaTypes))
	for name := range ScanMediaTypes {
		got = append(got, name)
	}
	sortStrings(got)
	if !reflect.DeepEqual(got, c.MediaTypes) {
		t.Errorf("the media types are %v, Python's are %v", got, c.MediaTypes)
	}
}

// The label is matched **exactly**, and the refusal names the four in sorted
// order. The tolerant version -- trim, lowercase, take the part before a
// semicolon -- is what a port writes by reflex and what a browser would make
// work by accident, so the uppercase and padded spellings are here as
// refusals rather than as passes.
func TestTheMediaTypeIsMatchedExactly(t *testing.T) {
	c := loadScanCorpus(t)
	good := base64.StdEncoding.EncodeToString([]byte("xxxx"))
	for _, row := range c.MediaTypeCases {
		_, _, err := ScanMessage(good, row.MediaType)
		if row.OK {
			if err != nil {
				t.Errorf("%q was refused with %q where Python accepted it", row.MediaType, err)
			}
			continue
		}
		if err == nil {
			t.Errorf("%q was accepted where Python refused it", row.MediaType)
			continue
		}
		if err.Error() != row.Error {
			t.Errorf("%q refused with\n  %q\nPython says\n  %q", row.MediaType, err, row.Error)
		}
		if !errors.Is(err, ErrScanRefused) {
			t.Errorf("%q refused with something the route will not read as a 422", row.MediaType)
		}
	}
}

// `base64.b64decode(s, validate=True)` against Go's `StdEncoding`.
//
// **The strictness is load-bearing, not fussy.** The default `b64decode`
// silently *discards* characters outside the alphabet, so a capture that
// picked up a newline would decode shorter and go to the API as a corrupt
// image rather than being refused -- and the model would answer something
// about it. `validate=True` is what makes the newline a refusal.
//
// **Go's `StdEncoding` does NOT draw that line in the same place, and this
// corpus is how we know.** Its decoder skips `\r` and `\n` wherever they
// appear, so the first version of `pyB64Decode` -- a straight
// `StdEncoding.DecodeString` -- accepted two captures Python refuses, one of
// them a plain trailing newline, which is exactly what a client that wrapped
// its base64 would send. The alphabet and the padding are checked explicitly
// now. It is a question about two libraries, and a reading of either would
// have got it wrong.
//
// The interesting row is `YW==`, whose final quantum has non-zero trailing
// bits: **both decoders accept it**, and both then re-encode to the canonical
// `YQ==` -- so the round trip is a normalisation and not the no-op it looks
// like. A port that skipped the decode and passed the string through would
// hand the API a byte sequence Python never would.
func TestTheBase64StrictnessIsPythons(t *testing.T) {
	c := loadScanCorpus(t)
	refusals := 0
	for _, row := range c.Captures {
		_, data, err := ScanMessage(row.Image, "image/jpeg")
		if row.Error != "" {
			refusals++
			if err == nil {
				t.Errorf("%s: accepted %q where Python refused with %q", row.Note, row.Image, row.Error)
				continue
			}
			if err.Error() != row.Error {
				t.Errorf("%s: refused with\n  %q\nPython says\n  %q", row.Note, err, row.Error)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: refused with %q where Python read it", row.Note, err)
			continue
		}
		if data != row.Data {
			t.Errorf("%s: encoded\n  %q\nPython encodes\n  %q", row.Note, data, row.Data)
		}
	}
	// A corpus that had drifted to all-accepting would pass every assertion
	// above and prove nothing about the strictness this test is named for.
	if refusals < 8 {
		t.Errorf("only %d refusals in the corpus; it no longer exercises "+
			"validate=True's strictness", refusals)
	}
}

// The size gate, at the boundary. Checked by generating the bytes here rather
// than carrying 4MB of base64 in a corpus for one assertion.
func TestTheCaptureSizeGateIsPythons(t *testing.T) {
	c := loadScanCorpus(t)
	if len(c.Sizes) == 0 {
		t.Fatal("no size cases")
	}
	for _, row := range c.Sizes {
		_, _, err := ScanMessage(make([]byte, row.Bytes), "image/jpeg")
		if row.OK {
			if err != nil {
				t.Errorf("%s (%d bytes): refused with %q", row.Note, row.Bytes, err)
			}
			continue
		}
		if err == nil {
			t.Errorf("%s (%d bytes): accepted where Python refused", row.Note, row.Bytes)
			continue
		}
		if err.Error() != row.Error {
			t.Errorf("%s: refused with\n  %q\nPython says\n  %q", row.Note, err, row.Error)
		}
	}
}

// A finished turn read back as a `Sighting`. Every failure is "nothing
// legible" and none of them raises, which is what lets the job hand the result
// to `identify` unconditionally.
func TestTheSightingReadsBackLikePythons(t *testing.T) {
	c := loadScanCorpus(t)
	for _, row := range c.Sightings {
		turn := Turn{Mode: "scan", Model: "m", StopReason: "end_turn",
			Text: row.Text, Refused: row.Refused}
		got := ScanSighting(turn)
		want := ScanRead{Title: row.Sighting["title"], Corner: row.Sighting["corner"]}
		if got != want {
			t.Errorf("%s: read %+v, Python reads %+v", row.Note, got, want)
		}
		// **And the marshalled bytes**, because the key order and the absence
		// of an unread field are both the wire and neither is a field
		// comparison. Python builds this dict by looping ("title", "corner")
		// and only adds a key whose stripped value is non-empty; a
		// `map[string]string` would alphabetise it and an empty string would
		// render as a present, blank field.
		raw, err := json.Marshal(got)
		if err != nil {
			t.Fatal(err)
		}
		wantRaw, err := json.Marshal(orderedSighting(row.Sighting))
		if err != nil {
			t.Fatal(err)
		}
		if string(raw) != string(wantRaw) {
			t.Errorf("%s: marshals as %s, Python marshals as %s", row.Note, raw, wantRaw)
		}
	}
}

// The mode's own stance: `consultant`, and emphatically not `off`.
func TestTheScanStanceIsPythons(t *testing.T) {
	c := loadScanCorpus(t)
	for _, row := range c.Stances {
		ceiling := ""
		if row.Ceiling != nil {
			ceiling = *row.Ceiling
		}
		t.Run(row.Note, func(t *testing.T) {
			withDialEnv(t, ceiling)
			var requested any
			if len(row.Requested) > 0 && string(row.Requested) != "null" {
				// **UseNumber, because production does.** Python's json tells
				// `7` from `7.5` by the literal rather than by the value, so
				// "cannot read a stance from int" and "...from float" are two
				// different refusals -- and a plain float64 decode collapses
				// them into one. Caught by this corpus on its first run,
				// which is the second time that decode has been the bug
				// rather than the code.
				decoder := json.NewDecoder(bytes.NewReader(row.Requested))
				decoder.UseNumber()
				if err := decoder.Decode(&requested); err != nil {
					t.Fatal(err)
				}
			}
			got, err := ScanStanceFor(requested, nil)
			if row.Error != "" {
				if err == nil {
					t.Fatalf("resolved where Python refused with %q", row.Error)
				}
				if err.Error() != row.Error {
					t.Errorf("refused with\n  %q\nPython says\n  %q", err, row.Error)
				}
				return
			}
			if err != nil {
				t.Fatalf("refused with %q", err)
			}
			assertSameJSONValue(t, "stance", Describe(got), row.Stance)
		})
	}
}

// The image block goes **first**, which is the documented ordering for vision
// requests and the order the instruction reads in -- the model is told what to
// do with a picture it has already been shown.
//
// Asserted on the marshalled request rather than on the Go value, because the
// SDK's builders are what decide the wire shape and the claim is about the
// wire.
func TestTheImageBlockComesFirst(t *testing.T) {
	c := loadScanCorpus(t)
	message, _, err := ScanMessage(base64.StdEncoding.EncodeToString([]byte("xxxxxxxxxxxx")), "image/webp")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(message)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Role    string `json:"role"`
		Content []struct {
			Type   string `json:"type"`
			Text   string `json:"text"`
			Source struct {
				Type      string `json:"type"`
				MediaType string `json:"media_type"`
				Data      string `json:"data"`
			} `json:"source"`
		} `json:"content"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got.Role != c.Message.Role {
		t.Errorf("role is %q, Python's is %q", got.Role, c.Message.Role)
	}
	if len(got.Content) != len(c.Message.Blocks) {
		t.Fatalf("%d blocks, Python sends %d", len(got.Content), len(c.Message.Blocks))
	}
	for i, want := range c.Message.Blocks {
		block := got.Content[i]
		if block.Type != want.Type {
			t.Errorf("block %d is %q, Python's is %q", i, block.Type, want.Type)
			continue
		}
		switch want.Type {
		case "image":
			if block.Source.Type != want.SourceType || block.Source.MediaType != want.MediaType {
				t.Errorf("block %d source is %s/%s, Python's is %s/%s", i,
					block.Source.Type, block.Source.MediaType, want.SourceType, want.MediaType)
			}
		case "text":
			if block.Text != want.Text {
				t.Errorf("block %d says\n  %q\nPython says\n  %q", i, block.Text, want.Text)
			}
		}
	}
}

// **A wart, pinned.** A capture that is neither a string nor bytes reaches
// `len()` in Python and raises an uncaught `TypeError`, so the route answers
// 500 to a request that is plainly malformed.
//
// It is the theme proposal's budget bug in a second module -- that one was
// ruled with Aaron on 2026-08-23 and fixed in both runtimes at once; this one
// was found the same day and is reproduced pending the same call. The test
// asserts it is NOT an `ErrScanRefused`, because that distinction is the whole
// mechanism: the route maps refusals to 422 and this to Python's 500.
func TestANonStringCaptureIsPythonsUncaughtTypeError(t *testing.T) {
	for _, image := range []any{[]any{1, 2, 3}, map[string]any{"a": 1}, float64(7), true} {
		_, _, err := ScanMessage(image, "image/jpeg")
		if err == nil {
			t.Errorf("%T was accepted", image)
			continue
		}
		if !ErrScanImageType(err) {
			t.Errorf("%T refused with %q, want the type wart", image, err)
		}
		if errors.Is(err, ErrScanRefused) {
			t.Errorf("%T reads as a 422; Python answers 500 for it", image)
		}
	}
	// And the two that ARE refusals, so this is not passing because
	// everything takes the wart branch.
	for _, image := range []any{"", nil} {
		_, _, err := ScanMessage(image, "image/jpeg")
		if !errors.Is(err, ErrScanRefused) || ErrScanImageType(err) {
			t.Errorf("%#v refused with %q, want the empty-capture 422", image, err)
		}
	}
}

func sortStrings(in []string) {
	for i := 1; i < len(in); i++ {
		for j := i; j > 0 && strings.Compare(in[j-1], in[j]) > 0; j-- {
			in[j-1], in[j] = in[j], in[j-1]
		}
	}
}

// orderedSighting renders the corpus's sighting the way Python's json does:
// insertion order, which for `scan.sighting` is title then corner, and only
// the keys it actually added.
func orderedSighting(in map[string]string) json.RawMessage {
	parts := []string{}
	for _, key := range []string{"title", "corner"} {
		if value, ok := in[key]; ok {
			encoded, err := json.Marshal(value)
			if err != nil {
				panic(err)
			}
			parts = append(parts, strconv.Quote(key)+":"+string(encoded))
		}
	}
	return json.RawMessage("{" + strings.Join(parts, ",") + "}")
}
