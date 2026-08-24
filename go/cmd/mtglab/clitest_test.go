package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/claude"
	"github.com/aasquier/sylvan-library/go/internal/config"
	"github.com/aasquier/sylvan-library/go/internal/pool/pooltest"
	"github.com/aasquier/sylvan-library/go/internal/sim/tier3"
)

// How every command in this package is driven: a described deployment in, the
// bytes it wrote out, and the error the root would have rendered.
//
// **Nothing here touches the process.** Before ADR 40 each command family had
// its own driver that swapped [os.Stdout] for a pipe and pointed the command
// at a scratch library through `t.Setenv` -- two process globals per test,
// which is why eighty-eight of them ran serially. Now the configuration is an
// argument and the output is a buffer Cobra was always willing to write to,
// so a test describes a machine and reads what one command said on it, and
// two hundred of them can do that at once.
//
// The commands are driven through [newRoot] rather than by constructing a
// subcommand directly, so a test exercises the same wiring `main` does --
// including the silences, which decide whether a refusal reaches the caller
// as an error or as a usage dump.

// deployment is a scratch machine: its own decks directory, its own data
// directory, and nothing shared with any other test.
type deployment struct {
	config.Config
	// Forge is where this machine's Forge is, if it has one. The zero value
	// is a machine with none, which is what CI has and what almost every test
	// here wants.
	Forge tier3.Settings
	// Pipe is this machine's Anthropic credential. The zero value has none,
	// so `claude check` reports unavailable without spending a call.
	Pipe claude.Endpoint
}

// scratchDeployment is a fresh machine with empty directories, no pool, no
// Forge and no credential -- the state every test starts from and changes the
// one field it is about.
func scratchDeployment(t *testing.T) deployment {
	t.Helper()
	dataDir := t.TempDir()
	decksDir := filepath.Join(t.TempDir(), "decks")
	if err := os.MkdirAll(decksDir, 0o750); err != nil {
		t.Fatal(err)
	}
	return deployment{Config: config.Config{
		DataDir:   dataDir,
		DecksDir:  decksDir,
		BaseURL:   config.DefaultBaseURL,
		EmailFrom: config.DefaultEmailFrom,
	}}
}

// withPool copies the 21-card fixture pool to `<data>/mtg.duckdb`, which is
// where [config.Config.DBPath] looks.
func (d deployment) withPool(t *testing.T) deployment {
	t.Helper()
	raw, err := os.ReadFile(pooltest.Build(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(d.DBPath(), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	return d
}

// run executes `mtglab <args...>` on this deployment and returns what the
// command wrote and the error the root would render as `mtglab: <err>`.
func (d deployment) run(t *testing.T, args ...string) (string, error) {
	t.Helper()
	return d.runWithInput(t, "", args...)
}

// runWithInput is [deployment.run] for the commands that prompt: `stdin` is
// what the operator types, handed to the command as its own reader rather
// than installed on the process.
func (d deployment) runWithInput(t *testing.T, stdin string, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	root := newRoot(d.Config, d.Forge, d.Pipe)
	root.SetArgs(args)
	root.SetOut(&out)
	// Prompts and warnings go to the error stream, which no test asserts on
	// and every test would otherwise see interleaved into its own output.
	root.SetErr(io.Discard)
	root.SetIn(strings.NewReader(stdin))
	// Execute first: Go evaluates a return statement's operands left to
	// right, so reading the buffer in the same expression would read it
	// empty.
	err := root.Execute()
	return out.String(), err
}
