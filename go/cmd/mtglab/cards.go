package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aasquier/sylvan-library/go/internal/config"
	"github.com/aasquier/sylvan-library/go/internal/pool"
)

// cardsCommand is `mtglab cards`: the pool asked directly from the shell.
// `show` is rule 1 of the non-negotiables made a command — "never evaluate a
// card from memory; look it up" deserves a first-class door rather than a
// snippet pasted around CLAUDE.md and the mtg-lab skill, so the lookup is
// the binary's own.
func cardsCommand(cfg config.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cards",
		Short: "Ask the card pool directly",
	}
	cmd.AddCommand(cardsShowCommand(cfg))
	return cmd
}

func cardsShowCommand(cfg config.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "show <name>...",
		Short: "A card's facts from the pool: cost, types, identity, text",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			ctx := cmd.Context()
			p := pool.New(cfg.DBPath(), nil)
			defer p.Close()
			var missing []string
			err := p.Use(ctx, func(c *pool.Conn) error {
				found, err := c.GetCards(ctx, args)
				if err != nil {
					return err
				}
				// In the order asked, not the map's.
				for _, name := range args {
					rec, ok := found[name]
					if !ok {
						missing = append(missing, name)
						continue
					}
					cost := ""
					if rec.ManaCost != nil && *rec.ManaCost != "" {
						cost = "   " + *rec.ManaCost
					}
					fmt.Fprintf(out, "  %s%s\n", rec.Name, cost)
					identity := "colorless"
					if len(rec.ColorIdentity) > 0 {
						identity = strings.Join(rec.ColorIdentity, ", ")
					}
					fmt.Fprintf(out, "  %s   [%s]\n", rec.TypeLine, identity)
					for _, line := range strings.Split(rec.OracleText, "\n") {
						fmt.Fprintf(out, "    %s\n", line)
					}
					fmt.Fprintln(out)
				}
				return nil
			})
			if err != nil {
				return refused("the pool is not readable (%s) -- run `mtglab data refresh`", err)
			}
			if len(missing) > 0 {
				sort.Strings(missing)
				return refused("not in the pool: %s", strings.Join(missing, ", "))
			}
			return nil
		},
	}
}
