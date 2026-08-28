package api

import (
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/pool/pooltest"
)

// A faction's sigil reaches `/api/colors/{key}` whole, and it reaches it with
// no card pool at all.
//
// The second half is the point. Every other picture on the combination route
// is a card resolved through the pool and dropped when the pool lacks it, so
// a fresh clone sees names where a stocked one sees art. The sigil is the one
// image that must not work that way: it is a faction's heraldry rather than a
// card in a list, it carries its own art URL and the credit pinned to that
// exact printing, and a page built on it should draw the same on a laptop that
// has never run a data refresh. `reference.Sigil` argues why the credit and
// the URL travel together; this proves the pair survives the route.
//
// It is also the only thing that would catch the field being declared on the
// response and then never assigned -- which serialises as `null` on a real
// guild while every key-presence check in the suite still passes.
func TestTheCombinationRouteCarriesTheFactionsSigilWithOrWithoutAPool(t *testing.T) {
	t.Parallel()
	for name, a := range map[string]*API{
		"with a pool": New(Config{Pool: pooltest.Open(t)}),
		"with none":   New(Config{}),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			// Azorius: a guild, so a Signet, credited.
			status, body, raw := call(t, a, "GET", "/api/colors/WU", "")
			if status != 200 {
				t.Fatalf("WU: %d %s", status, raw)
			}
			sigil, ok := body["sigil"].(map[string]any)
			if !ok {
				t.Fatalf("WU has no sigil on the wire: %s", raw)
			}
			for _, key := range []string{"card", "artist", "printing", "art"} {
				if s, _ := sigil[key].(string); s == "" {
					t.Errorf("WU: sigil %q is empty: %s", key, raw)
				}
			}
			if card, _ := sigil["card"].(string); !strings.Contains(card, "Azorius") {
				t.Errorf("WU: sigil card is %q", card)
			}
			// Mono-Green: not a faction, so the key is there and holds
			// nothing -- a shape a reader can branch on.
			_, body, raw = call(t, a, "GET", "/api/colors/G", "")
			mono, present := body["sigil"]
			if !present {
				t.Fatalf("G is missing the sigil key entirely: %s", raw)
			}
			if mono != nil {
				t.Errorf("G is a mono-colour and was served a sigil: %s", raw)
			}
		})
	}
}
