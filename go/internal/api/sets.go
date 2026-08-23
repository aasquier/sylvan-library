package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"sort"
	"time"

	"github.com/aasquier/sylvan-library/go/internal/wire"
)

// scryfallSets and setsUserAgent are `service.SCRYFALL_SETS` and
// `service.USER_AGENT`, verbatim. The URL is a var for exactly one caller:
// the tests point it at a local server, the way the Claude client's tests
// ride ANTHROPIC_BASE_URL rather than growing a production seam.
var scryfallSets = "https://api.scryfall.com/sets"

const setsUserAgent = "mtg-lab/0.1 (local personal deckbuilding tool)"

// upcomingSets is `GET /api/sets/upcoming` — `service.upcoming_sets`: the
// one route that reaches the network on demand. Spoiler scanning is
// meaningless against a card pool that by definition does not have the cards
// yet, so the set list has to come from upstream. Cached for the process
// lifetime keyed on today's date, as the marshalled bytes, so a replay is
// byte-identical to the answer it replays.
//
// A transport failure is a 503 that says so plainly — the only route where
// "could not reach Scryfall" is a state and not a bug — while a payload that
// is not the JSON this expects raises where Python's `json.load` raises:
// uncaught, plain-text 500. (The 503's detail carries this runtime's own
// rendering of the failure; the *shape* is the contract, the prose of an
// exception never was.)
func (a *API) upcomingSets(w http.ResponseWriter, r *http.Request) {
	today := time.Now().Format("2006-01-02")
	a.setsMu.Lock()
	if a.setsDay == today && a.setsBody != nil {
		body := a.setsBody
		a.setsMu.Unlock()
		wire.Raw(w, http.StatusOK, body)
		return
	}
	a.setsMu.Unlock()

	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, scryfallSets, nil)
	if err != nil {
		a.fail(w, "sets", err)
		return
	}
	req.Header.Set("User-Agent", setsUserAgent)
	req.Header.Set("Accept", "application/json")
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		wire.Detail(w, http.StatusServiceUnavailable,
			"could not reach Scryfall: "+err.Error())
		return
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		// urllib raises HTTPError -- an OSError -- for these, so Python's
		// route answers 503 here too.
		wire.Detail(w, http.StatusServiceUnavailable,
			"could not reach Scryfall: HTTP "+resp.Status)
		return
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		wire.Detail(w, http.StatusServiceUnavailable,
			"could not reach Scryfall: "+err.Error())
		return
	}
	var payload map[string]any
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&payload); err != nil {
		pythonUncaught(w, a.log, "sets", err)
		return
	}

	body, err := wire.MarshalOrdered(upcomingFrom(payload, today))
	if err != nil {
		a.fail(w, "sets", err)
		return
	}
	a.setsMu.Lock()
	a.setsDay, a.setsBody = today, body
	a.setsMu.Unlock()
	wire.Raw(w, http.StatusOK, body)
}

// upcomingFrom filters and shapes the payload exactly as Python does:
// unreleased (string comparison is date comparison for ISO dates), not
// digital, six keys per set in declaration order, stably sorted on
// `released_at` so ties keep Scryfall's own order.
func upcomingFrom(payload map[string]any, today string) wire.OrderedMap {
	data, _ := payload["data"].([]any)
	type entry struct {
		released string
		row      wire.OrderedMap
	}
	out := []entry{}
	for _, item := range data {
		s, ok := item.(map[string]any)
		if !ok {
			continue
		}
		released, _ := s["released_at"].(string)
		digital, _ := s["digital"].(bool)
		if released == "" || released <= today || digital {
			continue
		}
		out = append(out, entry{released, wire.OrderedMap{
			{Key: "code", Value: s["code"]},
			{Key: "name", Value: s["name"]},
			{Key: "released_at", Value: released},
			{Key: "card_count", Value: s["card_count"]},
			{Key: "icon", Value: s["icon_svg_uri"]},
			{Key: "set_type", Value: s["set_type"]},
		}})
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].released < out[j].released
	})
	sets := make([]wire.OrderedMap, len(out))
	for i, e := range out {
		sets[i] = e.row
	}
	return wire.OrderedMap{
		{Key: "sets", Value: sets},
		{Key: "as_of", Value: today},
	}
}
