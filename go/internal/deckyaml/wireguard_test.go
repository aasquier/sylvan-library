package deckyaml

import (
	"encoding/json"
	"strings"
	"testing"
)

// The ordered mapping's own wire guard.
//
// `Map.MarshalJSON` writes the pairs in the file's order because the stock
// encoder sorts a map's keys, and a deck's `notes:` once reached the wire
// alphabetised for exactly that reason. Writing the bytes by hand means
// owning the failure too: a value the encoder refuses must stop the whole
// object rather than leaving half of one on the wire, where a client would
// read it as a truncated deck rather than as an error.
func TestAValueTheWireCannotCarryStopsTheWholeObject(t *testing.T) {
	t.Parallel()
	m := Map{
		{Key: "plan", Value: "mulligan for a two-drop"},
		{Key: "pitfalls", Value: make(chan int)}, // nothing JSON can carry
		{Key: "lines", Value: "the long game"},
	}
	got, err := json.Marshal(m)
	if err == nil {
		t.Fatalf("a value the wire cannot carry was written as %s", got)
	}
	if strings.Contains(string(got), "plan") {
		t.Errorf("half an object came back alongside the refusal: %s", got)
	}

	// The same mapping without it is whole, and in the file's order rather
	// than alphabetised.
	m[1].Value = "do not tap out"
	got, err = json.Marshal(m)
	if err != nil {
		t.Fatalf("a mapping the wire can carry was refused: %v", err)
	}
	if want := `{"plan":"mulligan for a two-drop","pitfalls":"do not tap out","lines":"the long game"}`; string(got) != want {
		t.Errorf("the wire reads\n got  %s\n want %s", got, want)
	}
}
