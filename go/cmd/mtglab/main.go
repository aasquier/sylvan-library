// Command mtglab is the binary that carries the whole deployed surface and
// its runbook: `ui` serves the app, `forge-shim` is the worker's door, and
// `users`, `decks`, `sim`, `data` and `claude check` are what a
// `fly ssh console` reaches for. The dev bench -- `animist` and
// `cardmotion`, and nothing else -- lives in `tools/` under its own name and
// never ships. (`bench` and `mutate` were Python-era subcommands of this
// binary and did not cross: mutation sampling is `gremlins`, installed on
// demand, and there is no benchmark harness beyond `go test -bench`.)
//
// cobra throughout, by Aaron's explicit requirement (ADR 38 decision 5).
package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/aasquier/sylvan-library/go/internal/claude"
	"github.com/aasquier/sylvan-library/go/internal/config"
	"github.com/aasquier/sylvan-library/go/internal/sim/tier3"
)

// newRoot is the whole command tree, assembled around one [config.Config].
//
// **The configuration is an argument, not a lookup.** ADR 39 made the
// settings a value but stopped at `cmd/`, where a `settings()` helper called
// [config.Load] inside every `RunE` -- so the tree still read the process
// environment, just later and from thirty-one places instead of one. The cost
// was invisible and large: a test could only say "this deployment has its
// decks over here" by *writing* the process environment through
// [testing.T.Setenv], which Go panics on inside a parallel test. Eighty-eight
// tests in this package were serial for that reason alone, and not one of
// them was about the environment. Now they build a [config.Config] literal
// and hand it to this function, which is what a test could always have done
// if the door had been open.
//
// Separate from [main] for the older reason too: a test can ask the binary
// what verbs it actually has rather than restate a list beside it -- which is
// how prose in this repository came to name `mtglab animist`, a family that
// moved to `tools/` and left the sentences behind.
func newRoot(cfg config.Config, forge tier3.Settings, pipe claude.Endpoint) *cobra.Command {
	root := &cobra.Command{
		Use:           "mtglab",
		Short:         "The sylvan library: a Commander toolkit",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(uiCommand(cfg, forge))
	root.AddCommand(shimCommand(forge))
	root.AddCommand(dataCommand(cfg))
	root.AddCommand(usersCommand(cfg))
	root.AddCommand(decksCommand(cfg))
	root.AddCommand(simCommand(cfg, forge))
	root.AddCommand(cardsCommand(cfg))
	root.AddCommand(claudeCommand(pipe))
	root.AddCommand(probeCommand())
	return root
}

func main() {
	err := newRoot(config.Load(), tier3.LoadSettings(), claude.EndpointFromEnv()).Execute()
	switch {
	case err == nil:
		return
	case errors.Is(err, errFailedGate):
		// The verdict is the status, not a sentence: `decks validate` has
		// already printed the report and its counts, and a line saying so
		// again is noise in a script that only wanted the code.
		os.Exit(1)
	default:
		fmt.Fprintln(os.Stderr, "mtglab:", err)
		os.Exit(1)
	}
}

// The environment is read exactly three times in this process, all of them on
// the line above: [config.Load] for where things live and which switches are
// on, [tier3.LoadSettings] for the Forge and Fly half, and
// [claude.EndpointFromEnv] for the Anthropic credential. Each is a value from
// that point on. There is deliberately no `settings()` helper any more -- a
// function that reads the environment on demand is a global variable wearing
// a call, and this package spent eighty-eight serial tests proving it.
