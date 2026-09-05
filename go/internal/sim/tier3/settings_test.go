package tier3

import "testing"

// TestWorkerHalfSetIsExactlyTheDialOnAndTheWholeUnconfigured holds the
// predicate to its definition rather than restating its cases: half-set means
// the operator turned the dial and [Settings.Configured] still reads false,
// and those are the only settings it may name. Derived over the whole
// combination space, so a predicate edited to drift from Configured — the
// pair's one job is agreeing — fails on the combination it drifted at.
func TestWorkerHalfSetIsExactlyTheDialOnAndTheWholeUnconfigured(t *testing.T) {
	t.Parallel()
	for _, enabled := range []bool{false, true} {
		for _, token := range []string{"", "not_a_real_token"} {
			for _, url := range []string{"", "http://127.0.0.1:8791"} {
				s := Settings{WorkerEnabled: enabled, FlyAPIToken: token, WorkerURL: url}
				want := enabled && !s.Configured()
				if got := s.WorkerHalfSet(); got != want {
					t.Errorf("WorkerHalfSet() = %v on enabled=%v token=%q url=%q; "+
						"the dial-on-but-unconfigured reading says %v",
						got, enabled, token, url, want)
				}
			}
		}
	}
}
