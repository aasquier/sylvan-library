package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"net/http"

	"github.com/aasquier/sylvan-library/go/internal/claude"
	"github.com/aasquier/sylvan-library/go/internal/pool"
	"github.com/aasquier/sylvan-library/go/internal/wheel"
	"github.com/aasquier/sylvan-library/go/internal/wire"
)

// deckWheel is `POST /api/decks/{owner}/{slug}/wheel` — `service.wheel_spin`.
// A POST because each spin is a fresh draw, but read-only with respect to the
// deck: readers may spin a shared deck's wheel, exactly as they may read its
// stats. `seed` replays a spin; absent, the server rolls one and reports it.
//
// The seed goes through the recorded integer grammar (`claude.IntValue`) —
// a float truncates, `"１２"` reads as twelve — and a
// value the grammar refuses raises, which is an **uncaught
// 500**: the plain-text `Internal Server Error`, not a JSON detail.
// Recorded rather than tidied, and measured on the live wire before it was
// written down.
func (a *API) deckWheel(w http.ResponseWriter, r *http.Request) {
	body, ok := readOptionalBody(w, r)
	if !ok {
		return
	}
	src, ok := a.sourceFor(w, r)
	if !ok {
		return
	}
	d, err := src.Get(r.Context(), r.PathValue("slug"))
	if a.refuse(w, "wheel", err) {
		return
	}
	var seed *big.Int
	if raw, present := body["seed"]; present && raw != nil {
		if seed, err = claude.IntValue(raw); err != nil {
			uncaught500(w, a.log, "wheel", err)
			return
		}
	}
	var spun wire.OrderedMap
	err = a.usePool(r.Context(), func(c *pool.Conn) error {
		identity := map[string]bool{}
		if len(d.Commander) > 0 {
			cards, err := c.GetCards(r.Context(), d.Commander[:1])
			if err != nil {
				return err
			}
			if rec := cards[d.Commander[0]]; rec != nil {
				for _, col := range rec.ColorIdentity {
					identity[col] = true
				}
			}
		}
		var spinErr error
		spun, spinErr = wheel.Spin(r.Context(), d, identity, c, seed)
		return spinErr
	})
	if errors.Is(err, pool.ErrNoPool) {
		wire.JSON(w, http.StatusOK, wire.OrderedMap{
			{Key: "pool_available", Value: false},
			{Key: "card", Value: nil},
			{Key: "symbol", Value: nil},
			{Key: "message", Value: noPoolMessage},
		})
		return
	}
	if a.refuse(w, "wheel", err) {
		return
	}
	wire.JSON(w, http.StatusOK,
		append(wire.OrderedMap{{Key: "pool_available", Value: true}}, spun...))
}

// uncaught500 answers an error no handler owns: `text/plain; charset=utf-8`,
// the three words, nothing about the cause. The cause goes to the log
// instead — the detail lands server-side, never on the wire — and the shape
// is recorded, so it stays plain text rather than growing a JSON body.
func uncaught500(w http.ResponseWriter, log interface {
	Error(string, ...any)
}, where string, err error) {
	log.Error("the "+where+" route raised", "error", err)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusInternalServerError)
	_, _ = w.Write([]byte("Internal Server Error"))
}

// readOptionalBody is `body: dict[str, Any] | None = None`: an absent or
// null body is an empty mapping rather than a 422, and everything else is
// exactly `readBody`'s contract — a non-JSON content type or a non-object
// value is `dict_type`, JSON that will not parse is `json_invalid`.
func readOptionalBody(w http.ResponseWriter, r *http.Request) (map[string]any, bool) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		wire.Unprocessable(w, wire.Missing("body"))
		return nil, false
	}
	if len(raw) == 0 {
		return map[string]any{}, true
	}
	if !isJSONRequest(r.Header.Get("Content-Type")) {
		wire.Unprocessable(w, wire.DictType("body", string(raw)))
		return nil, false
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		wire.Unprocessable(w, wire.JSONInvalid("body", decodeOffset(err), err.Error()))
		return nil, false
	}
	if decoder.More() {
		wire.Unprocessable(w, wire.JSONInvalid("body", int(decoder.InputOffset())+1, "Extra data"))
		return nil, false
	}
	if value == nil {
		return map[string]any{}, true
	}
	body, ok := value.(map[string]any)
	if !ok {
		wire.Unprocessable(w, wire.DictType("body", value))
		return nil, false
	}
	return body, true
}
