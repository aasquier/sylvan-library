package claude

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/aasquier/sylvan-library/go/internal/textutil"
	"github.com/aasquier/sylvan-library/go/internal/wire"
)

// The scan mode: Claude reads a photographed card, and reads it as a camera
// would. The seventh mode, and the smallest.
//
// It exists for the cards the browser's own reader cannot do, and there are a
// lot of them: **cards printed before mid-2015 carry no collector number on
// the face at all**. The bottom-left info line arrived with the Magic Origins
// frame, so every dual land, every Ravnica shock, every Innistrad flip card
// reads nothing down there -- which is exactly the deep cuts this library is
// full of.
//
// **What this mode does not do is name the card**, and that is the whole
// design (ADR 34). Naming a card from a photograph has a right answer, so by
// ADR 14 it belongs to deterministic code -- but *transcribing pixels into
// text* has no offline implementation here, which is why the reader exists at
// all. So the boundary runs between the two: Claude returns what is printed,
// verbatim, and `internal/cards` decides what card that is. The response
// schema has **no field for a card name**, which lives in `modes.json` and
// is pinned by its own test; the absence is the feature.
//
// So this is a better camera, not a better judge. It has no tools, it never
// sees a deck, and its answer is two short strings.

// ScanMediaTypes is what the model may be handed. Anything else is refused
// before a request is built -- the API accepts these four and a wrong label is
// a 400 that costs a round trip to learn.
//
// **Matched exactly, byte for byte.** No trimming and no case folding:
// `IMAGE/JPEG` and `" image/jpeg "` are both refused. Measured, because the
// tolerant version is the one a reimplementation writes by reflex.
var ScanMediaTypes = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
	"image/webp": true,
	"image/gif":  true,
}

// ScanMaxBytes is the largest capture accepted, in decoded bytes. The API's own
// ceiling is 5MB; this sits under it so an oversized frame is refused here,
// with a sentence somebody can act on, rather than by the platform.
const ScanMaxBytes = 4 * 1024 * 1024

// ErrScanRefused marks a capture refused before any request was built.
//
// A sentinel wrapped by every refusal rather than four distinct errors,
// because the route maps all of them to one 422 and the *sentence* is what
// differs. `errors.Is` is how the handler tells this from a stance rejection,
// which is a different 422 with a different cause.
var ErrScanRefused = errors.New("the capture was refused")

func refuseScan(format string, args ...any) error {
	return fmt.Errorf("%w%.0w", fmt.Errorf(format, args...), ErrScanRefused)
}

// ScanDefaultPreset is the narrowest preset that still permits a call.
//
// Deliberately not `second-opinion`, which the two other deckless surfaces
// use: those are conversations where volunteering is the feature, and this is
// a transcription where it is the failure mode.
const ScanDefaultPreset = "consultant"

// ScanStanceFor is this mode's stance, and the reason it takes one at all.
//
// There is no deck to derive a default from, and `Resolve(nil, nil)` is `off`
// -- the right answer for "no idea what this is about" and the wrong one for a
// button somebody just pressed on a photograph.
//
// **Nothing here is meaningfully steerable**: a stance widens what a mode
// does, and there is no more or less forward way to read a collector number
// off a picture. It resolves so the call happens, not so it changes, which is
// why it takes the narrowest preset that is not `off`.
//
// **And `/api/claude` does not ask it**, which is the very thing this
// function exists to prevent. The dial's surface table names `theme` and
// `research` and was never extended when ADR 34 landed, so `?surface=scan`
// answers `off`. See `dialSurfaces`; recorded, not fixed.
func ScanStanceFor(requested any, limit *Stance) (Stance, error) {
	if requested == nil {
		ceil := Ceiling()
		if limit != nil {
			ceil = *limit
		}
		preset, err := Preset(ScanDefaultPreset)
		if err != nil {
			return Stance{}, err
		}
		return Clamp(preset, ceil), nil
	}
	return Resolve(requested, nil, limit)
}

// scanPayload validates a capture and returns it base64-encoded.
//
// Refuses here rather than at the API for the usual reason: a 400 from the
// platform arrives after a round trip and says nothing a person can act on.
//
// The `image` is whatever came out of the JSON body. A string is decoded as
// base64 and re-encoded, which is not the no-op it looks like -- it is the
// validation, and it also normalises what the API is handed.
func scanPayload(image any, mediaType string) (string, error) {
	if !ScanMediaTypes[mediaType] {
		names := make([]string, 0, len(ScanMediaTypes))
		for name := range ScanMediaTypes {
			names = append(names, name)
		}
		sort.Strings(names)
		return "", refuseScan("%s is not an image this reads. Expected one of: %s.",
			wire.Quote(mediaType), strings.Join(names, ", "))
	}
	var raw []byte
	switch value := image.(type) {
	case string:
		decoded, err := strictB64Decode(value)
		if err != nil {
			return "", refuseScan("the capture was not valid base64")
		}
		raw = decoded
	case []byte:
		raw = value
	case nil:
		// The route reads an absent or falsy image as empty bytes, so it
		// arrives here as nil and refuses as empty below, never as a type.
		raw = nil
	default:
		// **Refused since 2026-08-23, and a 500 before that.** The old
		// fallback took whatever it was handed, so a list or an object fell
		// through to an uncaught internal error -- for a request that is
		// plainly malformed, which is the theme proposal's budget wart in a
		// second module. Found on the day that one was ruled, and ruled with
		// it: a bad field is a 422 here, in a sentence we wrote.
		return "", refuseScan("the capture must be a base64 string")
	}
	if len(raw) == 0 {
		return "", refuseScan("the capture was empty")
	}
	if len(raw) > ScanMaxBytes {
		return "", refuseScan(
			"the capture is %dKB, over the %dKB limit. Photograph one card, closer.",
			len(raw)/1024, ScanMaxBytes/1024)
	}
	return base64.StdEncoding.EncodeToString(raw), nil
}

// strictB64Decode is strict base64: the alphabet and the padding are checked
// here, explicitly, and `StdEncoding` is asked only for the arithmetic.
//
// **The strictness is load-bearing, not fussy.** A lenient decoder silently
// discards characters outside the alphabet, so a capture that picked up a
// newline would decode shorter and go to the API as a corrupt image rather
// than being refused -- and the model would answer something about it. The
// strictness is what makes that newline a refusal.
//
// **And Go's `StdEncoding.DecodeString` is NOT this strictness**, which the
// corpus caught and a reading of the library would not have: Go's decoder
// skips `\r` and `\n` wherever they appear, so `DecodeString` on its own
// accepts a capture the recorded contract refuses. That is the one
// difference that matters most here, since a newline is exactly what a
// client that wrapped its base64 would send.
//
// Three rules, each of which a corpus row exercises:
//
//   - every byte is in `A-Za-z0-9+/=`, so a newline, a space, a NUL and the
//     URL-safe `-_` are all refused;
//   - `=` appears only as the last one or two bytes, so leading, embedded and
//     quadruple padding are refused;
//   - the length is a multiple of four, which is what refuses `YWJjZA` and
//     `YWJjZA===`.
//
// Non-canonical trailing bits are **not** refused, because the recorded
// contract does not refuse them: `YW==` decodes to `a` and re-encodes to the
// canonical `YQ==`. That makes the round trip a normalisation rather than
// the no-op it looks like -- passing the string through untouched would hand
// the API bytes the contract never would.
func strictB64Decode(s string) ([]byte, error) {
	if len(s)%4 != 0 {
		return nil, errNotBase64
	}
	body := s
	for len(body) > 0 && body[len(body)-1] == '=' {
		body = body[:len(body)-1]
	}
	if len(s)-len(body) > 2 {
		return nil, errNotBase64
	}
	for i := 0; i < len(body); i++ {
		c := body[i]
		switch {
		case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9',
			c == '+', c == '/':
		default:
			return nil, errNotBase64
		}
	}
	// An all-padding string has an empty body and non-zero padding: that
	// counts as leading padding and is refused.
	if body == "" && len(s) > 0 {
		return nil, errNotBase64
	}
	return base64.StdEncoding.DecodeString(s)
}

// errNotBase64 is internal: the caller turns every decode failure into the
// one recorded sentence, which never says which rule was broken.
var errNotBase64 = errors.New("not base64")

// ScanMessage is the one user message: the picture, then the ask.
//
// The image block goes **first**. That is the documented ordering for vision
// requests, and it is also the order the instruction reads in -- the model is
// told what to do with a picture it has already been shown.
func ScanMessage(image any, mediaType string) (anthropic.MessageParam, string, error) {
	data, err := scanPayload(image, mediaType)
	if err != nil {
		return anthropic.MessageParam{}, "", err
	}
	return anthropic.NewUserMessage(
		anthropic.NewImageBlockBase64(mediaType, data),
		anthropic.NewTextBlock("Transcribe this card's name and its bottom-left block."),
	), data, nil
}

// ScanRead is what the camera saw: the two strings, in the recorded key
// order.
//
// **A struct and not a `map[string]string`**, which is the difference between
// this shipping right and shipping like the Notes tab did. The recorded wire
// carries title first; `encoding/json` sorts a map's keys and would carry
// `corner` first. This value goes out as the job's `transcribed`,
// which the review screen renders beside the reading so somebody can see what
// was actually read -- so its order is somebody's reading order.
//
// `omitempty` is the other half: the recorded shape only carries a key whose
// stripped value is non-empty, so an unreadable field is an absent key
// rather than an empty string. The two spellings render the same on a screen
// and are different bytes.
type ScanRead struct {
	Title  string `json:"title,omitempty"`
	Corner string `json:"corner,omitempty"`
}

// Empty reports whether nothing legible came back, which is what decides
// between handing `identify` one sighting and handing it none.
func (s ScanRead) Empty() bool { return s.Title == "" && s.Corner == "" }

// ScanSighting is a finished turn as the `Sighting` that
// `/api/cards/identify` already takes.
//
// **The deliberate absence here is any resolution step.** What comes back is
// what the reader saw, in the same shape the browser's reader produces, and
// the pool does the rest -- so a card named by Claude and a card named by
// WebAssembly travel exactly the same path and get exactly the same scrutiny.
//
// Every failure is "nothing legible" rather than an error: a refusal, an
// unparseable answer, an answer that is not an object, a field that is not a
// string, a field that is only whitespace. A response schema makes the middle
// three close to impossible, and "close to" is why the branches exist -- a
// truncated answer is still JSON's opposite, and it must read as an empty
// sighting rather than raise.
func ScanSighting(turn Turn) ScanRead {
	var out ScanRead
	if turn.Refused {
		return out
	}
	text := turn.Text
	if text == "" {
		text = "{}"
	}
	var read map[string]any
	if err := json.Unmarshal([]byte(text), &read); err != nil {
		return out
	}
	// A strict string assertion: a number or a null is not a transcription,
	// and `plain` would turn one into the word "None".
	//
	// `textutil.Strip` and not `strings.TrimSpace` -- the two disagree on
	// which code points are whitespace, and a transcription can carry
	// anything the model typed. The corpus holds U+00A0 and U+2028 for
	// exactly that.
	if value, ok := read["title"].(string); ok {
		out.Title = textutil.Strip(value)
	}
	if value, ok := read["corner"].(string); ok {
		out.Corner = textutil.Strip(value)
	}
	return out
}
