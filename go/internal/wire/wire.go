// Package wire writes the app's recorded response vocabulary, so a response
// answered today is indistinguishable on the wire from the one recorded
// yesterday. The frontend is the reason it has to be: `web/src/lib/api.ts`
// reads `detail` off every non-2xx body and nothing else, and the wire
// contract is frozen precisely so nothing under `web/src` ever has to move
// in step with the server. The in-package tests hold every shape here to
// the recorded corpus.
//
// Three shapes, and they are the whole vocabulary:
//
//   - a 2xx body, as `application/json` with no charset, compact separators,
//     UTF-8 as it is;
//   - the error envelope, `{"detail": "<a sentence>"}`, for every refusal a
//     handler makes itself -- a 404 for a deck that is not there, a 422 for
//     a body that does not say what it must;
//   - the validation 422, written when a query parameter or a body fails its
//     declared shape before the handler runs: `detail` is then a *list* of
//     `{type, loc, msg, input}` objects (plus `ctx` for a bound), the
//     recorded validation shape, with no `url` key.
package wire

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

// Marshal encodes v as the recorded body encoding: compact separators, no
// trailing newline, UTF-8 written as itself, and `<`, `>` and `&` written
// as themselves. That last is the one place encoding/json's default differs
// (it escapes them for a browser's sake), and the prose carries those
// characters, so the encoder is told not to.
func Marshal(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

// JSON answers v with status. A value that will not encode is a 500 in the
// envelope rather than a half-written body -- the handler has already been
// given its chance to refuse; this is the last line.
func JSON(w http.ResponseWriter, status int, v any) {
	raw, err := Marshal(v)
	if err != nil {
		raw = []byte(`{"detail":"internal error"}`)
		status = http.StatusInternalServerError
	}
	Raw(w, status, raw)
}

// Raw answers bytes that are already JSON -- the embedded reference prose,
// compacted once at start -- with the recorded content type (no charset)
// and an explicit length.
func Raw(w http.ResponseWriter, status int, raw []byte) {
	h := w.Header()
	h.Set("Content-Type", "application/json")
	h.Set("Content-Length", fmt.Sprint(len(raw)))
	w.WriteHeader(status)
	_, _ = w.Write(raw)
}

// Detail is the error envelope: `{"detail": detail}` with status.
func Detail(w http.ResponseWriter, status int, detail string) {
	JSON(w, status, map[string]string{"detail": detail})
}

// ValidationError is one entry of the validation 422 -- the refusal written
// when a request fails its declared shape before the handler runs. Key
// order is the recorded one: type, loc, msg, input, then ctx when the
// error has a bound to report.
type ValidationError struct {
	Type  string         `json:"type"`
	Loc   []any          `json:"loc"`
	Msg   string         `json:"msg"`
	Input any            `json:"input"`
	Ctx   map[string]any `json:"ctx,omitempty"`
}

// Unprocessable answers 422 with the validation list.
func Unprocessable(w http.ResponseWriter, errs ...ValidationError) {
	if errs == nil {
		errs = []ValidationError{}
	}
	JSON(w, http.StatusUnprocessableEntity, map[string]any{"detail": errs})
}

// The refusal sentences for the parameter kinds the routes declare.
// Recorded here verbatim (2026-08-21) so the wire always says the same
// words; the in-package tests pin the shape and the frontend shows the
// sentence.

// IntParsing is a query parameter declared `int` that did not parse.
func IntParsing(where, name string, input any) ValidationError {
	return ValidationError{Type: "int_parsing", Loc: []any{where, name},
		Msg: "Input should be a valid integer, unable to parse string as an integer", Input: input}
}

// FloatParsing is a query parameter declared `float` that did not parse.
func FloatParsing(where, name string, input any) ValidationError {
	return ValidationError{Type: "float_parsing", Loc: []any{where, name},
		Msg: "Input should be a valid number, unable to parse string as a number", Input: input}
}

// BoolParsing is a query parameter declared `bool` that was none of the
// spellings the boolean grammar accepts.
func BoolParsing(where, name string, input any) ValidationError {
	return ValidationError{Type: "bool_parsing", Loc: []any{where, name},
		Msg: "Input should be a valid boolean, unable to interpret input", Input: input}
}

// GreaterThanEqual is a bound failed on the low side (`Query(..., ge=n)`).
func GreaterThanEqual(where, name string, input any, ge int) ValidationError {
	return ValidationError{Type: "greater_than_equal", Loc: []any{where, name},
		Msg: fmt.Sprintf("Input should be greater than or equal to %d", ge), Input: input,
		Ctx: map[string]any{"ge": ge}}
}

// LessThanEqual is a bound failed on the high side (`Query(..., le=n)`).
func LessThanEqual(where, name string, input any, le int) ValidationError {
	return ValidationError{Type: "less_than_equal", Loc: []any{where, name},
		Msg: fmt.Sprintf("Input should be less than or equal to %d", le), Input: input,
		Ctx: map[string]any{"le": le}}
}

// Missing is a required field that was not there.
func Missing(loc ...any) ValidationError {
	return ValidationError{Type: "missing", Loc: loc, Msg: "Field required", Input: nil}
}

// DictType is a body that arrived and is not a mapping.
//
// It is the answer to more than it looks. A request body is parsed as JSON
// **only** when the content type says `application/json` (or
// `application/…+json`); with any other type, or none, the raw bytes stand
// as a *string* -- which is not a dictionary, so a perfectly well-formed
// object posted without a content type lands here with itself as the
// `input`. That decision procedure is the recorded one, not a preference.
func DictType(where string, input any) ValidationError {
	return ValidationError{Type: "dict_type", Loc: []any{where},
		Msg: "Input should be a valid dictionary", Input: input}
}

// JSONInvalid is a body the JSON parser refused.
//
// `loc` carries the position the parser stopped at, one-based, and `input`
// is an empty object rather than the body -- both recorded choices,
// inherited rather than designed here.
//
// **`ctx.error` is the one field in this file whose wording is this
// parser's own rather than the recorded corpus's.** The corpus has a
// scanner with its own sentences and offsets ("Expecting property name
// enclosed in double quotes"), and reproducing those exactly would be a
// parser rewrite to improve a diagnostic nobody's client reads: the app's
// own frontend always sends valid JSON with a content type, so this is
// reachable only from a shell. The type, the shape and the status match
// the recording, which is what a client can depend on.
func JSONInvalid(where string, at int, reason string) ValidationError {
	return ValidationError{Type: "json_invalid", Loc: []any{where, at},
		Msg: "JSON decode error", Input: map[string]any{},
		Ctx: map[string]any{"error": reason}}
}
