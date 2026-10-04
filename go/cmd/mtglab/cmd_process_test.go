package main

import (
	"strings"
	"testing"
)

// The binary's own body, asked a question without a shell.
//
// [main] used to be four statements -- the three environment reads, the
// execute, the status, the exit -- and not one of them could run in a test: a
// function that calls [os.Exit] cannot be called by a test binary that wants to
// report afterwards, and the argv it runs on belongs to the process. So the
// whole composition of this program, the one line that decides what `mtglab`
// *is*, sat at zero and the first thing to notice a wrong argument there would
// have been a deploy.
//
// Split the way every other process reader in this tree is split (ADR 39):
// [runProcess] takes the words and the error stream as values, `main` is the
// one line that reaches for [os.Args] and [os.Exit], and the thing between them
// is drivable.
//
// A word the binary does not have is the right question to ask it: cobra
// refuses it at the root, before any command's `RunE` opens a file or reads a
// directory, so this runs the real composition over this machine's real
// environment and touches nothing on the disk.
func TestTheBinaryRefusesAWordItDoesNotHaveAndNamesItOnTheErrorStream(t *testing.T) {
	t.Parallel()
	var said strings.Builder

	code := runProcess([]string{"frobnicate"}, &said)
	if code == 0 {
		t.Errorf("a word the binary does not have exited 0, which a runbook "+
			"line or a cron entry would read as success; it said %q", said.String())
	}
	// `mtglab: <sentence>` on the error stream, where it does not contaminate
	// output somebody is piping -- and naming what was actually typed, because
	// a refusal that does not quote the word is a refusal nobody can act on.
	if !strings.HasPrefix(said.String(), "mtglab:") {
		t.Errorf("the refusal reached the error stream as %q", said.String())
	}
	if !strings.Contains(said.String(), "frobnicate") {
		t.Errorf("the refusal does not say what was typed: %q", said.String())
	}
	// Nothing was printed twice: cobra's own error and usage dumps are
	// silenced at the root so the one sentence a script sees is this one.
	if strings.Count(said.String(), "frobnicate") != 1 {
		t.Errorf("the refusal was printed more than once: %q", said.String())
	}
}
