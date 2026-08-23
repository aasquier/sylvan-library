package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/pool/pooltest"
)

// `cards show` against the 21-card pool: rule 1's lookup as a command.

func runCards(t *testing.T, args ...string) (string, error) {
	t.Helper()
	oldOut := os.Stdout
	t.Cleanup(func() { os.Stdout = oldOut })
	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = outW
	cmd := cardsCommand()
	cmd.SetArgs(args)
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	runErr := cmd.Execute()
	_ = outW.Close()
	out := make([]byte, 0, 1024)
	buf := make([]byte, 1024)
	for {
		n, readErr := outR.Read(buf)
		out = append(out, buf[:n]...)
		if readErr != nil {
			break
		}
	}
	os.Stdout = oldOut
	return string(out), runErr
}

func pointAtTinyPool(t *testing.T) {
	t.Helper()
	built := pooltest.Build(t)
	dir := t.TempDir()
	if err := os.Link(built, filepath.Join(dir, "mtg.duckdb")); err != nil {
		// A cross-device tmp layout: fall back to a copy.
		raw, readErr := os.ReadFile(built)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if writeErr := os.WriteFile(filepath.Join(dir, "mtg.duckdb"), raw, 0o644); writeErr != nil {
			t.Fatal(writeErr)
		}
	}
	t.Setenv("MTGLAB_DATA_DIR", dir)
}

func TestCardsShowPrintsThePoolsFacts(t *testing.T) {
	pointAtTinyPool(t)
	out, err := runCards(t, "show", "Sol Ring")
	if err != nil {
		t.Fatalf("show: %v", err)
	}
	for _, want := range []string{"Sol Ring", "{1}", "Artifact", "{C}{C}"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestCardsShowNamesWhatThePoolLacks(t *testing.T) {
	pointAtTinyPool(t)
	_, err := runCards(t, "show", "Sol Ring", "Imaginary Card, the Unprinted")
	if err == nil || !strings.Contains(err.Error(), "not in the pool: Imaginary Card, the Unprinted") {
		t.Fatalf("want the not-in-pool refusal, got %v", err)
	}
}

func TestClaudeCheckWithoutAKeyReportsUnavailable(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("ANTHROPIC_AUTH_TOKEN", "")
	oldOut := os.Stdout
	t.Cleanup(func() { os.Stdout = oldOut })
	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = outW
	cmd := claudeCommand()
	cmd.SetArgs([]string{"check"})
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	runErr := cmd.Execute()
	_ = outW.Close()
	out := make([]byte, 0, 512)
	buf := make([]byte, 512)
	for {
		n, readErr := outR.Read(buf)
		out = append(out, buf[:n]...)
		if readErr != nil {
			break
		}
	}
	os.Stdout = oldOut
	if runErr == nil {
		t.Fatal("a keyless check must exit non-zero")
	}
	text := string(out)
	if !strings.Contains(text, "status    unavailable") ||
		!strings.Contains(text, "reason") {
		t.Errorf("unexpected keyless report:\n%s", text)
	}
}
