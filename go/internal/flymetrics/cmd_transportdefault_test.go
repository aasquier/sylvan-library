package flymetrics

import (
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"testing"
)

// The seam's own default: which transport a panel reaches for when nobody
// handed it one, and which of the three sources an app name comes from.
//
// Both are lever 36 in docs/polish/COVERAGE.md — the line the deployed process
// runs and no test does. [Panel.Fetch] is driven a dozen times here with a
// transport injected, so the two statements that choose the *real* one ran only
// on Fly; the same is true of the environment arm of [valueOr], which every
// other test steps over by handing the panel an app and an org outright. A
// wrong answer in either is invisible rather than loud: a panel that asked a
// mirror, or one that monitored an app with somebody else's name.

// A panel with no transport asks real HTTP, and a panel that was handed one
// asks that.
//
// Identity rather than behaviour, because the alternative is a test that
// reaches api.fly.io: the whole point of the branch is *which function*, and
// what [realTransport] does with it is already driven directly in
// `realtransport_test.go`.
func TestAPanelWithNoTransportReachesForRealHttp(t *testing.T) {
	t.Parallel()
	chosen := (&Panel{}).transport()
	if chosen == nil {
		t.Fatal("a panel with no transport chose nothing, so the first look " +
			"at the metrics would be a nil call")
	}
	if reflect.ValueOf(chosen).Pointer() != reflect.ValueOf(Transport(realTransport)).Pointer() {
		t.Error("a panel with no transport of its own chose something other " +
			"than real HTTP")
	}

	// And the injected half, proved by calling it: a panel that was handed a
	// transport must not quietly prefer the real one.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))
	defer srv.Close()
	asked := ""
	handed := &Panel{Transport: func(url string, _ map[string]string) (int, []byte, error) {
		asked = url
		return http.StatusOK, []byte(`{"status":"success"}`), nil
	}}
	status, _, err := handed.transport()(srv.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	if asked != srv.URL || status != http.StatusOK {
		t.Errorf("the handed transport was not the one asked: url %q, status %d",
			asked, status)
	}
}

// [valueOr]'s middle source, which is the one a deployed machine uses: Fly
// names the app and the org in the process rather than in this app's own
// configuration, so the panel reads them from there when nobody overrode them.
//
// `PATH` stands in for `FLY_APP_NAME` because it is the one variable set in
// every process that can run this test, which makes the assertion a fact about
// this process rather than a restatement of the code (lever 36). All three
// sources are asked in one test, because the order between them is the whole
// of what this function is.
func TestAnAppNamePrefersWhatItWasHandedThenWhatTheProcessSays(t *testing.T) {
	t.Parallel()
	inProcess := os.Getenv("PATH")
	if inProcess == "" {
		t.Skip("this process has no PATH, so there is no variable to read")
	}

	if got := valueOr("chosen-by-hand", "PATH", "the-fallback"); got != "chosen-by-hand" {
		t.Errorf("a value handed in lost to something else: %q", got)
	}
	if got := valueOr("", "PATH", "the-fallback"); got != inProcess {
		t.Errorf("an unset field did not read the process: %q", got)
	}
	if got := valueOr("", "MTGLAB_A_VARIABLE_NOBODY_SETS", "the-fallback"); got != "the-fallback" {
		t.Errorf("a silent process did not fall back: %q", got)
	}
}
