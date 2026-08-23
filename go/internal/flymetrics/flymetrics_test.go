package flymetrics

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/aasquier/sylvan-library/go/internal/wire"
)

// The bug this function exists for: a Fly macaroon carries its own scheme,
// and wrapping it in `Bearer ` is two schemes and no valid credential — the
// panel spent a fortnight dead on exactly that. Detected as "first word, no
// underscore", never by matching `FlyV1` literally.
func TestAuthorizationKeepsASchemeAndAddsBearerOtherwise(t *testing.T) {
	for secret, want := range map[string]string{
		"FlyV1 fm2_lJPE":     "FlyV1 fm2_lJPE",
		"FlyV9 fm9_x y":      "FlyV9 fm9_x y",
		"fm2_lJPE":           "Bearer fm2_lJPE",
		"plain-api-key":      "Bearer plain-api-key",
		"fm2_with space fm2": "Bearer fm2_with space fm2",
	} {
		if got := Authorization(secret); got != want {
			t.Errorf("Authorization(%q) = %q, want %q", secret, got, want)
		}
	}
}

func vector(value string) []byte {
	return []byte(`{"data":{"result":[{"value":[1724444444.0,"` + value + `"]}]}}`)
}

var empty = []byte(`{"data":{"result":[]}}`)

func stub(t *testing.T, answers map[string][]byte, hits *int) Transport {
	t.Helper()
	return func(target string, headers map[string]string) (int, []byte, error) {
		*hits++
		if ua := headers["User-Agent"]; ua != userAgent {
			t.Errorf("User-Agent %q", ua)
		}
		if headers["Authorization"] == "" {
			t.Error("no Authorization header")
		}
		for name, body := range answers {
			if contains(target, name) {
				return 200, body, nil
			}
		}
		return 200, empty, nil
	}
}

func contains(target, name string) bool {
	// The query names its series; matching on the substring keeps the stub
	// honest about which counter it is answering.
	switch name {
	case "memory_bytes":
		return contains2(target, "mem_available")
	case "memory_total_bytes":
		return contains2(target, "mem_total") && !contains2(target, "mem_available")
	default:
		class := strings.Replace(name[len("edge_"):], "xx", "..", 1)
		return contains2(target, "%22"+class+"%22")
	}
}

func contains2(s, sub string) bool { return strings.Contains(s, sub) }

// The whole panel: values in query order, floats rendered as Python renders
// them, the app and org named — and the second ask served from the cache.
func TestFetchAnswersOnceAndCaches(t *testing.T) {
	t.Setenv("FLY_METRICS_TOKEN", "FlyV1 fm2_test")
	hits := 0
	now := time.Unix(1_000_000, 0)
	p := &Panel{Transport: stub(t, map[string][]byte{
		"memory_bytes":       vector("123456789"),
		"memory_total_bytes": vector("268435456"),
		"edge_2xx":           vector("1500.5"),
		"edge_4xx":           vector("12"),
		"edge_5xx":           vector("0"),
	}, &hits), Now: func() time.Time { return now }}
	first, _ := wire.MarshalOrdered(p.Fetch())
	want := `{"configured":true,"ok":true,"values":{"memory_bytes":123456789.0,` +
		`"memory_total_bytes":268435456.0,"edge_2xx":1500.5,"edge_4xx":12.0,` +
		`"edge_5xx":0.0},"app":"sylvan-library","org":"personal"}`
	if string(first) != want {
		t.Fatalf("got %s\nwant %s", first, want)
	}
	if hits != 5 {
		t.Fatalf("%d fetches for five queries", hits)
	}
	second, _ := wire.MarshalOrdered(p.Fetch())
	if string(second) != want || hits != 5 {
		t.Fatalf("the cache did not answer the second ask (%d hits)", hits)
	}
	// And past the TTL it asks again.
	now = now.Add((CacheSeconds + 1) * time.Second)
	_ = p.Fetch()
	if hits != 10 {
		t.Fatalf("an expired cache did not refetch (%d hits)", hits)
	}
}

// Prometheus has no zero: an empty 4xx/5xx beside a populated 2xx witness is
// a real zero, and with the witness absent nothing is known and every
// counter stays null.
func TestTheWitnessSettlesTheSilentCounters(t *testing.T) {
	t.Setenv("FLY_METRICS_TOKEN", "tok")
	hits := 0
	p := &Panel{Transport: stub(t, map[string][]byte{"edge_2xx": vector("9")}, &hits)}
	got, _ := wire.MarshalOrdered(p.Fetch())
	want := `{"configured":true,"ok":true,"values":{"memory_bytes":null,` +
		`"memory_total_bytes":null,"edge_2xx":9.0,"edge_4xx":0.0,` +
		`"edge_5xx":0.0},"app":"sylvan-library","org":"personal"}`
	if string(got) != want {
		t.Fatalf("got %s\nwant %s", got, want)
	}

	blind := &Panel{Transport: stub(t, nil, &hits)}
	got, _ = wire.MarshalOrdered(blind.Fetch())
	if string(got) != `{"configured":true,"ok":true,"values":{"memory_bytes":null,`+
		`"memory_total_bytes":null,"edge_2xx":null,"edge_4xx":null,`+
		`"edge_5xx":null},"app":"sylvan-library","org":"personal"}` {
		t.Fatalf("with no witness: %s", got)
	}
}

// An unset token is `configured: false` and is NOT cached — configuring it
// should take effect on the next look, not five minutes later.
func TestUnconfiguredHidesAndIsNotCached(t *testing.T) {
	t.Setenv("FLY_METRICS_TOKEN", "  ")
	hits := 0
	p := &Panel{Transport: stub(t, nil, &hits)}
	got, _ := wire.MarshalOrdered(p.Fetch())
	if string(got) != `{"configured":false,"ok":false,"values":{}}` {
		t.Fatalf("unconfigured: %s", got)
	}
	t.Setenv("FLY_METRICS_TOKEN", "tok")
	after, _ := wire.MarshalOrdered(p.Fetch())
	if string(after) == string(got) {
		t.Fatal("setting the token did not take effect on the next look")
	}
}

// A failure is `ok: false` with the reason — and cached, so a broken token
// is not retried per tile.
func TestAFailureIsCloudedGlassNotA500(t *testing.T) {
	t.Setenv("FLY_METRICS_TOKEN", "tok")
	hits := 0
	now := time.Unix(2_000_000, 0)
	p := &Panel{Now: func() time.Time { return now },
		Transport: func(string, map[string]string) (int, []byte, error) {
			hits++
			return 401, nil, nil
		}}
	got, _ := wire.MarshalOrdered(p.Fetch())
	if string(got) != `{"configured":true,"ok":false,"error":"Fly answered HTTP 401","values":{}}` {
		t.Fatalf("failure shape: %s", got)
	}
	_ = p.Fetch()
	if hits != 1 {
		t.Fatalf("a cached failure was retried (%d hits)", hits)
	}
	badJSON := &Panel{Transport: func(string, map[string]string) (int, []byte, error) {
		return 200, []byte("<html>"), nil
	}}
	got, _ = wire.MarshalOrdered(badJSON.Fetch())
	if string(got) != fmt.Sprintf(`{"configured":true,"ok":false,"error":%q,"values":{}}`,
		"Fly's answer was not the JSON this expects") {
		t.Fatalf("bad JSON shape: %s", got)
	}
}
