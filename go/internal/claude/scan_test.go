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

// The camera's reader, held to the recorded corpus testdata/scan.json.

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
	NotAString []struct {
		Note  string          `json:"note"`
		Raw   json.RawMessage `json:"raw"`
		OK    bool            `json:"ok"`
		Error string          `json:"error"`
	} `json:"not_a_string"`
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
		t.Fatal("the corpus is empty; testdata/scan.json is a frozen golden and always carries captures")
	}
	return c
}

func TestTheScanConstantsAreTheRecordedOnes(t *testing.T) {
	t.Parallel()
	c := loadScanCorpus(t)
	if c.MaxBytes != ScanMaxBytes {
		t.Errorf("the cap is %d, the corpus says %d", ScanMaxBytes, c.MaxBytes)
	}
	if c.DefaultPreset != ScanDefaultPreset {
		t.Errorf("the default preset is %q, the corpus says %q", ScanDefaultPreset, c.DefaultPreset)
	}
	got := make([]string, 0, len(ScanMediaTypes))
	for name := range ScanMediaTypes {
		got = append(got, name)
	}
	sortStrings(got)
	if !reflect.DeepEqual(got, c.MediaTypes) {
		t.Errorf("the media types are %v, the corpus says %v", got, c.MediaTypes)
	}
}

// The label is matched **exactly**, and the refusal names the four in sorted
// order. The tolerant version -- trim, lowercase, take the part before a
// semicolon -- is what a reimplementation writes by reflex and what a
// browser would make work by accident, so the uppercase and padded spellings
// are here as refusals rather than as passes.
func TestTheMediaTypeIsMatchedExactly(t *testing.T) {
	t.Parallel()
	c := loadScanCorpus(t)
	good := base64.StdEncoding.EncodeToString([]byte("xxxx"))
	for _, row := range c.MediaTypeCases {
		_, _, err := ScanMessage(good, row.MediaType)
		if row.OK {
			if err != nil {
				t.Errorf("%q was refused with %q where the corpus accepts it", row.MediaType, err)
			}
			continue
		}
		if err == nil {
			t.Errorf("%q was accepted where the corpus refuses it", row.MediaType)
			continue
		}
		if err.Error() != row.Error {
			t.Errorf("%q refused with\n  %q\nthe corpus says\n  %q", row.MediaType, err, row.Error)
		}
		if !errors.Is(err, ErrScanRefused) {
			t.Errorf("%q refused with something the route will not read as a 422", row.MediaType)
		}
	}
}

// The strict base64 gate, against the recorded corpus.
//
// **The strictness is load-bearing, not fussy.** A lenient decoder silently
// *discards* characters outside the alphabet, so a capture that picked up a
// newline would decode shorter and go to the API as a corrupt image rather
// than being refused -- and the model would answer something about it. The
// strictness is what makes the newline a refusal.
//
// **Go's `StdEncoding` does NOT draw the line in the recorded place, and
// this corpus is how we know.** Its decoder skips `\r` and `\n` wherever
// they appear, so the first version of `strictB64Decode` -- a straight
// `StdEncoding.DecodeString` -- accepted two captures the corpus refuses,
// one of them a plain trailing newline, which is exactly what a client that
// wrapped its base64 would send. The alphabet and the padding are checked
// explicitly now. A reading of the library would have got it wrong; only the
// recorded rows said so.
//
// The interesting row is `YW==`, whose final quantum has non-zero trailing
// bits: **it is accepted**, and re-encodes to the canonical `YQ==` -- so the
// round trip is a normalisation and not the no-op it looks like. Skipping
// the decode and passing the string through would hand the API a byte
// sequence the recorded contract never would.
func TestTheBase64StrictnessMatchesTheRecordedCorpus(t *testing.T) {
	t.Parallel()
	c := loadScanCorpus(t)
	refusals := 0
	for _, row := range c.Captures {
		_, data, err := ScanMessage(row.Image, "image/jpeg")
		if row.Error != "" {
			refusals++
			if err == nil {
				t.Errorf("%s: accepted %q where the corpus refuses with %q", row.Note, row.Image, row.Error)
				continue
			}
			if err.Error() != row.Error {
				t.Errorf("%s: refused with\n  %q\nthe corpus says\n  %q", row.Note, err, row.Error)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: refused with %q where the corpus reads it", row.Note, err)
			continue
		}
		if data != row.Data {
			t.Errorf("%s: encoded\n  %q\nthe corpus encodes\n  %q", row.Note, data, row.Data)
		}
	}
	// A corpus that had drifted to all-accepting would pass every assertion
	// above and prove nothing about the strictness this test is named for.
	if refusals < 8 {
		t.Errorf("only %d refusals in the corpus; it no longer exercises "+
			"the decoder's strictness", refusals)
	}
}

// The size gate, at the boundary. Checked by generating the bytes here rather
// than carrying 4MB of base64 in a corpus for one assertion.
func TestTheCaptureSizeGateIsTheRecordedOne(t *testing.T) {
	t.Parallel()
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
			t.Errorf("%s (%d bytes): accepted where the corpus refuses", row.Note, row.Bytes)
			continue
		}
		if err.Error() != row.Error {
			t.Errorf("%s: refused with\n  %q\nthe corpus says\n  %q", row.Note, err, row.Error)
		}
	}
}

// A finished turn read back as a `Sighting`. Every failure is "nothing
// legible" and none of them raises, which is what lets the job hand the result
// to `identify` unconditionally.
func TestTheSightingReadsBackAsRecorded(t *testing.T) {
	t.Parallel()
	c := loadScanCorpus(t)
	for _, row := range c.Sightings {
		turn := Turn{Mode: "scan", Model: "m", StopReason: "end_turn",
			Text: row.Text, Refused: row.Refused}
		got := ScanSighting(turn)
		want := ScanRead{Title: row.Sighting["title"], Corner: row.Sighting["corner"]}
		if got != want {
			t.Errorf("%s: read %+v, the corpus reads %+v", row.Note, got, want)
		}
		// **And the marshalled bytes**, because the key order and the absence
		// of an unread field are both the wire and neither is a field
		// comparison. The recorded shape carries title before corner and only
		// a key whose stripped value is non-empty; a `map[string]string`
		// would alphabetise it and an empty string would render as a present,
		// blank field.
		raw, err := json.Marshal(got)
		if err != nil {
			t.Fatal(err)
		}
		wantRaw, err := json.Marshal(orderedSighting(row.Sighting))
		if err != nil {
			t.Fatal(err)
		}
		if string(raw) != string(wantRaw) {
			t.Errorf("%s: marshals as %s, the corpus marshals as %s", row.Note, raw, wantRaw)
		}
	}
}

// The mode's own stance: `consultant`, and emphatically not `off`.
func TestTheScanStanceMatchesTheCorpus(t *testing.T) {
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
				// **UseNumber, because production does.** The refusals tell
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
					t.Fatalf("resolved where the corpus refuses with %q", row.Error)
				}
				if err.Error() != row.Error {
					t.Errorf("refused with\n  %q\nthe corpus says\n  %q", err, row.Error)
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
	t.Parallel()
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
		t.Errorf("role is %q, the corpus says %q", got.Role, c.Message.Role)
	}
	if len(got.Content) != len(c.Message.Blocks) {
		t.Fatalf("%d blocks, the corpus sends %d", len(got.Content), len(c.Message.Blocks))
	}
	for i, want := range c.Message.Blocks {
		block := got.Content[i]
		if block.Type != want.Type {
			t.Errorf("block %d is %q, the corpus says %q", i, block.Type, want.Type)
			continue
		}
		switch want.Type {
		case "image":
			if block.Source.Type != want.SourceType || block.Source.MediaType != want.MediaType {
				t.Errorf("block %d source is %s/%s, the corpus says %s/%s", i,
					block.Source.Type, block.Source.MediaType, want.SourceType, want.MediaType)
			}
		case "text":
			if block.Text != want.Text {
				t.Errorf("block %d says\n  %q\nthe corpus says\n  %q", i, block.Text, want.Text)
			}
		}
	}
}

// A capture that is neither a string nor bytes is **one 422 like every other
// bad capture**, and it was an uncaught 500 until 2026-08-23.
//
// The old payload path took whatever it was handed, so a list or an object
// fell through to an uncaught internal error -- the theme proposal's budget
// wart in a second module, found on the day that one was ruled and ruled
// with it. This test is the wart's own, kept and inverted: it asserts the
// refusal IS an `ErrScanRefused` now, since that is the mechanism the route
// reads to answer 422 rather than 500.
func TestACaptureThatIsNotTextIsARefusalLikeAnyOther(t *testing.T) {
	t.Parallel()
	c := loadScanCorpus(t)
	if len(c.NotAString) == 0 {
		t.Fatal("the corpus no longer holds a capture that is not text")
	}
	for _, row := range c.NotAString {
		var raw any
		dec := json.NewDecoder(bytes.NewReader(row.Raw))
		dec.UseNumber()
		if err := dec.Decode(&raw); err != nil {
			t.Fatal(err)
		}
		_, _, err := ScanMessage(raw, "image/jpeg")
		if row.OK {
			if err != nil {
				t.Errorf("%s: refused with %q where the corpus accepts it", row.Note, err)
			}
			continue
		}
		if err == nil {
			t.Errorf("%s: accepted where the corpus refuses", row.Note)
			continue
		}
		if err.Error() != row.Error {
			t.Errorf("%s: refused with\n  %q\nthe corpus says\n  %q", row.Note, err, row.Error)
		}
		// The mechanism, not just the sentence: the route maps this sentinel
		// to 422, and before the ruling this branch deliberately did not
		// carry it.
		if !errors.Is(err, ErrScanRefused) {
			t.Errorf("%s: the refusal will not read as a 422 at the route", row.Note)
		}
		// Commandment 10: no type name, no language, nothing that computes.
		for _, leak := range []string{"TypeError", "[]interface", "map[", "float64", "int"} {
			if strings.Contains(err.Error(), leak) {
				t.Errorf("%s: the refusal leaks %q: %q", row.Note, leak, err)
			}
		}
	}
	// And the two that are EMPTY rather than mistyped keep their own sentence,
	// so the fix did not flatten three cases into one.
	for _, image := range []any{"", nil} {
		_, _, err := ScanMessage(image, "image/jpeg")
		if err == nil || err.Error() != "the capture was empty" {
			t.Errorf("%#v refused with %v, want the empty-capture sentence", image, err)
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

// orderedSighting renders the corpus's sighting in the recorded order --
// title then corner -- and only the keys it actually carries.
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
