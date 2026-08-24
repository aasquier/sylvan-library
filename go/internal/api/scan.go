package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/aasquier/sylvan-library/go/internal/auth"
	"github.com/aasquier/sylvan-library/go/internal/cards"
	"github.com/aasquier/sylvan-library/go/internal/claude"
	"github.com/aasquier/sylvan-library/go/internal/claude/tools"
	"github.com/aasquier/sylvan-library/go/internal/jobs"
	"github.com/aasquier/sylvan-library/go/internal/pool"
	"github.com/aasquier/sylvan-library/go/internal/wire"
)

// The scan route (ADR 34): `POST /api/claude/scan`, a **job**, over
// `api/scanruns.py`.
//
// **This is the one route in the app that receives a photograph**, and it
// never receives one by accident: the browser's local reader sends nothing but
// two short strings, and a capture arrives here only because somebody pressed
// a button on that specific card, having been told what pressing it does. The
// image is passed to Anthropic, is not written to disk, and is not logged.
//
// **What comes back is not a card.** The mode transcribes what is printed and
// `identify` decides what it is, so a corner resolves only against the pool's
// real set codes and a title still only ever offers a shortlist -- the same
// scrutiny the WebAssembly reader's output gets, which is what keeps ADR 14
// intact with a model in the loop. The job calls `identifyAgainst`, the same
// function `POST /api/cards/identify` calls, because "the same scrutiny" has
// to mean the same code.
//
// **A job from its first commit, and its duration is unmeasured** -- which is
// the reason it is a job rather than an argument that it needs to be one. The
// sentence this project has been burned by three times is *"it is a few
// seconds"*: the theme turn carried it in a docstring and ran 4.3-133.8s, the
// dossier was never re-measured after ADR 20 and presented deployed as a
// spinner and then Safari's `Load failed`, and the proposal was 226s. A vision
// call at `low` effort returning two short strings ought to be quick. "Ought
// to be" is not a measurement, and being wrong costs a transport error with no
// status code and no access-log line.
//
// **Checking happens in the request; calling happens in the job.** A capture
// that is not an image this reads, is empty, or is over the size cap is a 422;
// a malformed stance is a 422; no key is a 503. Carried into a worker they
// would all arrive as a job in state `error` -- one string for three cases and
// a status code for none.
//
// **Nothing is cached**, because a photograph is not a question anybody asks
// twice: the next capture is a different card, or the same card photographed
// better. What *is* deduplicated is the double press -- the key is a digest of
// the encoded image, so two clicks on one shot are one paid call, matched per
// owner like every other key here (ADR 5's shape, one layer down).

// ScanKind is what `/api/jobs` calls one of these.
const ScanKind = "claude.scan"

// scanLabel is `plan_scan`'s label, which is a constant: there is nothing
// about a photograph worth putting in a job list.
const scanLabel = "scan: a photographed card"

// scanResult is the job's answer, in Python's key order.
//
// A struct and not a map, for the rule Phase 5 wrote down: `encoding/json`
// sorts a map's keys and a dict does not, so a job result must be a struct
// with its fields in Python's order.
type scanResult struct {
	// Reading is `read["readings"][0] if read["readings"] else None` -- the
	// pool's verdict on what was transcribed, or null when nothing legible
	// came back and nothing was looked up.
	Reading *identifyReading `json:"reading"`
	// Transcribed is what was actually read, carried so the page can show it.
	// A wrong reading beside the words it came from is a mistake somebody can
	// see; a wrong reading alone is one they cannot.
	Transcribed claude.ScanRead `json:"transcribed"`
	Refused     bool            `json:"refused"`
	Model       string          `json:"model"`
}

// claudeScan is `POST /api/claude/scan` -- `scanruns.plan_scan` behind
// `app.claude_scan`. Returns a job.
func (a *API) claudeScan(w http.ResponseWriter, r *http.Request) {
	body, ok := readBody(w, r)
	if !ok {
		return
	}
	// `payload.get("image") or b""`: a falsy image is empty bytes, which
	// refuses as "the capture was empty" rather than as a type.
	var image any
	if pyTruthy(body["image"]) {
		image = body["image"]
	}
	// `str(payload.get("media_type") or "image/jpeg")`: `str` and not a cast,
	// so an int media type becomes "7" and refuses by name rather than by
	// type. Measured -- a list becomes "['a']" and repr-quotes with double
	// quotes in the refusal, which is `wire.Quote`'s job.
	mediaType := "image/jpeg"
	if pyTruthy(body["media_type"]) {
		mediaType = claude.Plain(body["media_type"])
	}

	// Built here, before anything is queued: this is what validates the
	// capture, and every one of its refusals is a 422 the page acts on.
	// Every way a capture can fail is one 422, including the one that used to
	// be a 500: an image that is neither a string nor bytes reached `len()` in
	// Python and raised an uncaught `TypeError`. Ruled with Aaron 2026-08-23
	// alongside the theme proposal's identical `float(budget)` wart, and fixed
	// in both runtimes at once. See `claude.scanPayload`.
	ask, data, err := claude.ScanMessage(image, mediaType)
	if err != nil {
		wire.Detail(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	var requested any
	if pyTruthy(body["stance"]) {
		requested = body["stance"]
	}
	stance, err := claude.ScanStanceFor(requested, nil)
	if err != nil {
		wire.Detail(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	// The digest is over the **encoded** capture, which is what Python hashes
	// (`ask["content"][0]["source"]["data"]`) -- and it matters that it is the
	// re-encoded form rather than what arrived, since `YW==` and `YQ==` are
	// the same photograph and must be the same job.
	sum := sha256.Sum256([]byte(data))
	key := "scan:" + hex.EncodeToString(sum[:])

	// Raised here rather than a minute into a job that was never going to
	// work. `Require` and not `Connect`, for the reason themeruns gives: this
	// only needs to know *whether* a call is possible.
	if err := claude.Require(); err != nil {
		wire.Detail(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	tier := auth.ScopeFrom(r.Context()).ModelTier
	ledgerOf := a.claudeLedger
	a.submit(w, r, jobs.Plan{
		Kind: ScanKind, Label: scanLabel, Lane: jobs.NET, Key: key,
		Run: func(rep jobs.Progress) (any, error) {
			// **Its own context**: the request is over by the time this runs,
			// and `r.Context()` is cancelled the moment the handler returns.
			ctx := context.Background()
			mode, err := claude.GetMode(claude.ModeScan)
			if err != nil {
				return nil, err
			}
			turn, err := claude.Converse(ctx, mode, claude.Request{
				Messages: []anthropic.MessageParam{ask},
				Stance:   stance,
				// **No deps at all, and the nil is the contract.** This mode
				// has no tools and never sees a deck: it is a camera, and the
				// pool is consulted afterwards by `identify` rather than by
				// anything the model can reach.
				Deps:   tools.Deps{},
				Tier:   tier,
				Ledger: ledgerOf,
				OnTurn: func(done, max int) { rep.Report(done, max) },
			})
			if err != nil {
				return nil, claudeJobError(err)
			}
			seen := claude.ScanSighting(turn)
			out := scanResult{Transcribed: seen, Refused: turn.Refused, Model: turn.Model}
			// `[seen] if seen else []`: nothing legible means nothing is
			// looked up, which is a null reading rather than a reading of
			// nothing.
			if seen.Empty() {
				return out, nil
			}
			// Through the same door as the browser's reader. Nothing about
			// this reading is trusted more for having come from a better
			// camera.
			sightings := []cards.Sighting{{Title: seen.Title, Corner: seen.Corner}}
			err = a.leasePool(ctx, func(c *pool.Conn) error {
				answer, readErr := identifyAgainst(ctx, c, sightings)
				if readErr != nil {
					return readErr
				}
				if len(answer.Readings) > 0 {
					reading := answer.Readings[0]
					out.Reading = &reading
				}
				return nil
			})
			// No pool is not a failure here, it is a reading of nothing --
			// `service.identify_cards` answers with an empty `readings` list
			// and a message when it cannot connect, and the job's `reading`
			// is null either way.
			if err != nil && !errors.Is(err, pool.ErrNoPool) {
				return nil, err
			}
			return out, nil
		},
	})
}
