package door

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// What one response costs the door, compressed and not — the instrument behind
// `gzip.go`'s pool and the one a future change to that path is measured with.
//
// **Read the allocation figures and distrust the clock.** The pair exists as a
// pair for exactly that reason: `BenchmarkOnePlainResponse` never touches the
// compressor, so it is the control. Across the two runs that argued the pool in
// (2026-09-26, this Mac, ambient load 2.2 then 147) the control's `B/op` was
// **22,704 both times, to the byte**, while its `ns/op` moved 6.1µs → 14µs on
// the load alone. So a wall-clock delta here is worth nothing without
// `benchstat` over interleaved runs, and `B/op` and `allocs/op` are worth
// reading straight.
//
//	go test -run '^$' -bench Response -benchmem -count=6 ./internal/door/

// A body well over `gzipFloor`, so the writer commits to the compressed path.
const benchBody = "Syr Gwyn, Hero of Ashvale. "

func BenchmarkOneGzippedResponse(b *testing.B) {
	benchmarkOneResponse(b, true)
}

func BenchmarkOnePlainResponse(b *testing.B) {
	benchmarkOneResponse(b, false)
}

func benchmarkOneResponse(b *testing.B, compress bool) {
	body := []byte(strings.Repeat(benchBody, 400)) // ~10.8 kB
	h := gzipped(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(body)
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if compress {
		req.Header.Set("Accept-Encoding", "gzip")
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		h.ServeHTTP(httptest.NewRecorder(), req)
	}
}
