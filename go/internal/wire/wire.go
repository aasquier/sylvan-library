// Package wire writes what FastAPI writes, so a response the Go side answers
// is indistinguishable on the wire from the one Python answered yesterday.
// The frontend is the reason it has to be: `web/src/lib/api.ts` reads
// `detail` off every non-2xx body and nothing else, and zero changes under
// `web/src` is a stated invariant of the migration (docs/go-migration/
// PLAN.md section 3, invariant 1). The contract suite (`tests/contract/`)
// is what holds the two doors to it.
//
// Three shapes, and they are the whole vocabulary:
//
//   - a 2xx body, as `application/json` with no charset, compact separators,
//     UTF-8 as it is;
//   - the error envelope, `{"detail": "<a sentence>"}`, for every refusal a
//     handler makes itself -- a 404 for a deck that is not there, a 422 for
//     a body that does not say what it must;
//   - the 422 FastAPI itself writes when a query parameter or a body fails
//     validation: `detail` is then a *list* of `{type, loc, msg, input}`
//     objects (plus `ctx` for a bound), which is pydantic's error shape with
//     the `url` key stripped, exactly as FastAPI ships it.
package wire

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

// Marshal encodes v the way Starlette's JSONResponse does -- `json.dumps(...,
// ensure_ascii=False, allow_nan=False, separators=(",", ":"))`: compact, and
// with `<`, `>` and `&` written as themselves. That last is the one place
// encoding/json's default differs (it writes `<` for a browser's sake),
// and the prose carries those characters, so the encoder is told not to.
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
// compacted once at start -- with the content type and length FastAPI sends.
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

// ValidationError is one entry of the 422 FastAPI writes for a request that
// failed validation before the handler ran. Key order is pydantic's: type,
// loc, msg, input, then ctx when the error has a bound to report.
type ValidationError struct {
	Type  string         `json:"type"`
	Loc   []any          `json:"loc"`
	Msg   string         `json:"msg"`
	Input any            `json:"input"`
	Ctx   map[string]any `json:"ctx,omitempty"`
}

// Unprocessable answers 422 with FastAPI's validation list.
func Unprocessable(w http.ResponseWriter, errs ...ValidationError) {
	if errs == nil {
		errs = []ValidationError{}
	}
	JSON(w, http.StatusUnprocessableEntity, map[string]any{"detail": errs})
}

// The pydantic v2 sentences FastAPI relays for the parameter kinds the
// ported routes declare. Recorded here verbatim (pydantic 2.x, 2026-08-21)
// so the wire says the same words from either door; the contract suite pins
// the shape and the frontend shows the sentence.

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
// spellings pydantic accepts.
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
// It is the answer to more than it looks. Starlette parses a request body as
// JSON **only** when the content type says `application/json` (or
// `application/…+json`); with any other type, or none, the raw bytes are
// handed through as a *string* -- which is not a dictionary, so a perfectly
// well-formed object posted without a content type lands here with itself as
// the `input`.
func DictType(where string, input any) ValidationError {
	return ValidationError{Type: "dict_type", Loc: []any{where},
		Msg: "Input should be a valid dictionary", Input: input}
}

// JSONInvalid is a body the JSON parser refused.
//
// `loc` carries the position the parser stopped at, one-based as CPython
// reports it, and `input` is an empty object rather than the body -- both are
// pydantic's choices, not this project's.
//
// **`ctx.error` is the one field in this file that is not word for word
// Python's.** It is CPython's `json` scanner talking ("Expecting property name
// enclosed in double quotes"), and reproducing that scanner's sentences and
// offsets exactly would be a parser rewrite to improve a diagnostic nobody's
// client reads: the app's own frontend always sends valid JSON with a content
// type, so this is reachable only from a shell. The type, the shape and the
// status are identical, which is what a client can depend on.
func JSONInvalid(where string, at int, reason string) ValidationError {
	return ValidationError{Type: "json_invalid", Loc: []any{where, at},
		Msg: "JSON decode error", Input: map[string]any{},
		Ctx: map[string]any{"error": reason}}
}
