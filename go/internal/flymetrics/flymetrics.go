// Package flymetrics is the admin dashboard's platform panel: what the
// platform sees, when the platform is asked — Fly's managed Prometheus,
// behind a read-only token the maintainer mints. Off unless configured, and
// that is the design: unset, the widget hides itself rather than looking
// broken.
//
// The panel's hard-won lessons are carried as code, not prose: the
// Authorization value is sent **verbatim when it already carries a scheme**
// (a Fly macaroon begins `FlyV1 fm2_...`; wrapping that in `Bearer ` is two
// schemes and no valid credential — the panel spent a fortnight dead on
// exactly that), the User-Agent is explicit and set above the seam (a stock
// library User-Agent reads as a banned browser signature to at least one
// WAF, and Go's default UA earns no more trust), and an empty edge counter
// is settled against the 2xx witness, because Prometheus has no zero and an
// empty vector for "no 5xx today" is byte-identical to an empty vector for
// "this query has never worked" (#172 is the proof that is not
// hypothetical).
package flymetrics

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/aasquier/sylvan-library/go/internal/floats"
	"github.com/aasquier/sylvan-library/go/internal/wire"
)

const (
	userAgent = "mtg-lab/0.1 (personal deckbuilding tool; instance metrics)"
	timeout   = 8 * time.Second
	// CacheSeconds holds an answer long enough that a dashboard left open
	// all afternoon is a handful of requests, short enough that a number
	// nobody trusts is never what is shown. Failures are cached too, so a
	// broken token is not retried per tile.
	CacheSeconds = 300
)

// query is one instant query and its name on the wire, in declaration order
// — the recorded key order, which the payload's `values` renders in.
type query struct{ name, promql string }

func queries(app string) []query {
	sel := fmt.Sprintf("{app=%q}", app)
	edge := func(class string) string {
		return fmt.Sprintf("sum(increase(fly_edge_http_responses_count{app=%q,"+
			"status=~%q}[24h]))", app, class)
	}
	return []query{
		// Memory as the platform accounts for it — derived, because Fly
		// publishes no "resident" series; `used = total - available`.
		{"memory_bytes", "sum(fly_instance_memory_mem_total" + sel + ") " +
			"- sum(fly_instance_memory_mem_available" + sel + ")"},
		{"memory_total_bytes", "sum(fly_instance_memory_mem_total" + sel + ")"},
		// `status` carries full codes (`200`, `301`), so the class is
		// `=~"2.."` — two dots, because a status is exactly three characters.
		{"edge_2xx", edge("2..")},
		{"edge_4xx", edge("4..")},
		{"edge_5xx", edge("5..")},
	}
}

// edgeWitness proves the edge series exists: any app the edge serves at all
// has answered a 2xx in the last day. edgeSilent are the counters whose
// absence it disambiguates into a real zero.
const edgeWitness = "edge_2xx"

var edgeSilent = [...]string{"edge_4xx", "edge_5xx"}

// Transport is the seam a test injects: `(url, headers) -> (status, body)`.
type Transport func(url string, headers map[string]string) (int, []byte, error)

// Panel is the cached view. One lives on the API for the process lifetime —
// a process-wide cache with a home rather than a global.
//
// The three settings below are the same seam the transport and the clock
// already were, and they arrived for the same reason: every one of them used
// to be read off the process at the moment it was wanted, so the only way to
// describe a configured instance in a test was to install one on the whole
// binary with [testing.T.Setenv] — which Go refuses inside a parallel test,
// and which held all four of this package's tests serial. Nil and empty still
// ask the process, because that is what the composition root wants and what a
// deployment does; a test hands in a value and never touches the environment.
type Panel struct {
	Transport Transport // nil means real HTTP
	Now       func() time.Time
	// Token is asked for the read-only Fly credential on every fetch that
	// misses the cache. A function rather than a string because **read
	// fresh, never held** is a property this panel keeps on purpose: a token
	// filled in after the process booted takes effect on the next look
	// rather than at the next restart, which is the behaviour
	// TestUnconfiguredHidesAndIsNotCached describes. Nil asks the process
	// for FLY_METRICS_TOKEN.
	Token func() string
	// App and Org name the instance the queries ask Fly about. Empty asks
	// the process (FLY_APP_NAME, FLY_ORG_SLUG) and falls back to this app's
	// own names — unlike the token these cannot change under a live
	// container, so a string is the honest shape.
	App, Org string
	Log      *slog.Logger

	mu     sync.Mutex
	at     time.Time
	cached wire.OrderedMap
}

// token is the read-only Fly token, or empty — blank counts as absent (an
// empty string presented as a credential is how a 401 gets mistaken for a
// bug).
func (p *Panel) token() string {
	if p.Token != nil {
		return strings.TrimSpace(p.Token())
	}
	return strings.TrimSpace(os.Getenv("FLY_METRICS_TOKEN"))
}

// Authorization is the header value for secret — scheme included, or added.
// A value already carrying a scheme (first word, no underscore) goes out
// verbatim; only a bare token gets `Bearer ` put in front.
func Authorization(secret string) string {
	head, rest, _ := strings.Cut(secret, " ")
	if rest != "" && !strings.Contains(head, "_") {
		return secret
	}
	return "Bearer " + secret
}

func realTransport(target string, headers map[string]string) (int, []byte, error) {
	req, err := http.NewRequest(http.MethodGet, target, nil)
	if err != nil {
		return 0, nil, err
	}
	for name, value := range headers {
		req.Header.Set(name, value)
	}
	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return resp.StatusCode, body, err
}

// Fetch is every query, once, cached. Never raises: a metrics panel that can
// 500 the admin page is a monitoring tool that takes the dashboard down with
// the thing it monitors, so a failure is `configured: true, ok: false` with
// the reason.
func (p *Panel) Fetch() wire.OrderedMap {
	now := time.Now
	if p.Now != nil {
		now = p.Now
	}
	stamp := now()
	p.mu.Lock()
	if p.cached != nil && stamp.Sub(p.at) < CacheSeconds*time.Second {
		out := p.cached
		p.mu.Unlock()
		return out
	}
	p.mu.Unlock()

	secret := p.token()
	if secret == "" {
		// Not cached: configuring the token should take effect on the next
		// look rather than five minutes later.
		return wire.OrderedMap{
			{Key: "configured", Value: false},
			{Key: "ok", Value: false},
			{Key: "values", Value: wire.OrderedMap{}},
		}
	}

	get := p.Transport
	if get == nil {
		get = realTransport
	}
	headers := map[string]string{
		"Authorization": Authorization(secret),
		"Accept":        "application/json",
		"User-Agent":    userAgent,
	}

	app := valueOr(p.App, "FLY_APP_NAME", "sylvan-library")
	org := valueOr(p.Org, "FLY_ORG_SLUG", "personal")
	base := "https://api.fly.io/prometheus/" + org + "/api/v1/query"

	values := wire.OrderedMap{}
	byName := map[string]*float64{}
	for _, q := range queries(app) {
		status, body, err := get(base+"?query="+url.QueryEscape(q.promql), headers)
		if err != nil {
			return p.failed(fmt.Sprintf("could not reach Fly: %v", err), stamp)
		}
		if status < 200 || status >= 300 {
			return p.failed(fmt.Sprintf("Fly answered HTTP %d", status), stamp)
		}
		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err != nil {
			return p.failed("Fly's answer was not the JSON this expects", stamp)
		}
		v := scalar(payload)
		byName[q.name] = v
		values = append(values, wire.KV{Key: q.name, Value: nil})
	}
	settleEdge(byName)
	for i := range values {
		if v := byName[values[i].Key]; v != nil {
			values[i].Value = floats.Float(*v)
		}
	}
	answer := wire.OrderedMap{
		{Key: "configured", Value: true},
		{Key: "ok", Value: true},
		{Key: "values", Value: values},
		{Key: "app", Value: app},
		{Key: "org", Value: org},
	}
	p.mu.Lock()
	p.at, p.cached = stamp, answer
	p.mu.Unlock()
	return answer
}

func (p *Panel) failed(reason string, stamp time.Time) wire.OrderedMap {
	log := p.Log
	if log == nil {
		log = slog.Default()
	}
	log.Warn("fly metrics unavailable", "reason", reason)
	answer := wire.OrderedMap{
		{Key: "configured", Value: true},
		{Key: "ok", Value: false},
		{Key: "error", Value: reason},
		{Key: "values", Value: wire.OrderedMap{}},
	}
	p.mu.Lock()
	p.at, p.cached = stamp, answer
	p.mu.Unlock()
	return answer
}

// scalar is the one number out of a Prometheus instant-vector response, or
// nil — absent and zero are genuinely different answers, and collapsing them
// is how a broken query gets read as good news.
func scalar(payload map[string]any) *float64 {
	data, _ := payload["data"].(map[string]any)
	result, _ := data["result"].([]any)
	if len(result) == 0 {
		return nil
	}
	first, _ := result[0].(map[string]any)
	pair, _ := first["value"].([]any)
	if len(pair) < 2 {
		return nil
	}
	text, ok := pair[1].(string)
	if !ok {
		return nil
	}
	var f float64
	if _, err := fmt.Sscanf(text, "%g", &f); err != nil {
		return nil
	}
	return &f
}

// settleEdge turns an edge counter's ambiguous absence into the zero it
// means, but only with the witness populated — with the witness itself
// absent nothing is known, and every counter stays an em-dash.
func settleEdge(values map[string]*float64) {
	if values[edgeWitness] == nil {
		return
	}
	zero := 0.0
	for _, name := range edgeSilent {
		if v, present := values[name]; present && v == nil {
			values[name] = &zero
		}
	}
}

// valueOr prefers what the panel was handed, then what the process was told,
// then the name this app answers to — three sources in the order of how
// deliberately each was chosen.
func valueOr(given, name, fallback string) string {
	if given != "" {
		return given
	}
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}
