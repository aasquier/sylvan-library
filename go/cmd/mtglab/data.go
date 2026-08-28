package main

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/aasquier/sylvan-library/go/internal/auth"
	"github.com/aasquier/sylvan-library/go/internal/config"
	"github.com/aasquier/sylvan-library/go/internal/pool"
)

// dataCommand is `mtglab data`: the pool's care and feeding, on the box
// that serves it — the runbook's `fly ssh console -C "mtglab data refresh"`
// depends on the binary alone.
func dataCommand(cfg config.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "data",
		Short: "Fetch and load the card pool",
	}
	cmd.AddCommand(dataRefreshCommand(cfg), dataSnapshotCommand(cfg), dataBackupCommand(cfg))
	return cmd
}

// dataBackupCommand is `mtglab data backup`: the runbook's online copy of
// `app.db`, safe while the app serves. The destination must not exist; the
// procedure pulls the copy off the box and removes it, because a backup of
// password hashes should not sit on the volume indefinitely.
func dataBackupCommand(cfg config.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "backup <destination>",
		Short: "Write a consistent copy of app.db, safe while the app runs",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			version, err := auth.Backup(cmd.Context(), cfg.AppDBPath(), args[0])
			if err != nil {
				return err
			}
			info, err := os.Stat(args[0])
			if err != nil {
				return err
			}
			fmt.Fprintf(out, "wrote %s (schema version %d, %s bytes)\n",
				args[0], version, commas(info.Size()))
			return nil
		},
	}
}

// openTheWriter takes the pool read-write, waiting out the running app rather
// than losing a race to it.
//
// **The failure this replaces read as a broken database and was a timing
// problem.** `mtglab data refresh` on the live instance would report
//
//	Could not set lock on file "/data/mtg.duckdb":
//	Conflicting lock is held in /usr/local/bin/mtglab (PID 654)
//
// and then succeed a minute later for no visible reason. It was the serving
// process's read lease, held almost continuously by two health checks that did
// not know about each other — [pool.Pool.UseWithoutHolding] is the fix for the
// cause and this is the fix for the symptom, which is worth having on its own:
// an operator refreshing a library somebody is *reading* should be told to wait
// rather than told the file is broken.
//
// The line only appears when there is something to wait for, so an ordinary
// refresh on a quiet instance looks exactly as it always has.
func openTheWriter(ctx context.Context, out io.Writer, path string) (*sql.DB, error) {
	return pool.OpenWriterWaiting(ctx, path, pool.WriterWait, func() { sayWaiting(out) })
}

// sayWaiting is that line, in one place, because `refresh` now reaches it
// through `pool.Refresh`'s watcher and `snapshot` still reaches it through
// [openTheWriter]. Two spellings of a sentence is one spelling that goes
// stale.
func sayWaiting(out io.Writer) {
	fmt.Fprintf(out, "the app is reading the pool; waiting for it "+
		"(up to %s) ...\n", pool.WriterWait)
}

// bulkLabel and rowsLabel are how `refresh` has always named the two bulk
// files and the two row counts. The `(large)` is on the printings download
// because it is 500MB and the operator is about to wait for it.
func bulkLabel(kind string) string {
	if kind == pool.PrintingsBulk {
		return kind + " (large)"
	}
	return kind
}

func rowsLabel(kind string) string {
	if kind == pool.PrintingsBulk {
		return "printings"
	}
	return "oracle cards"
}

func dataRefreshCommand(cfg config.Config) *cobra.Command {
	var oracleOnly bool
	cmd := &cobra.Command{
		Use:   "refresh",
		Short: "Download Scryfall's bulk data and rebuild the pool",
		Args:  cobra.NoArgs,
		// The sequence itself lives in `pool.Refresh`, because the admin page
		// now starts the same work as a background job (ADR 6's last line,
		// cashed in) and two spellings of it would be two things to keep in
		// step. What is left here is the printing -- the same lines, in the
		// same order, with the same words.
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			_, err := pool.Refresh(cmd.Context(), pool.RefreshOptions{
				DBPath:      cfg.DBPath(),
				ScryfallDir: cfg.ScryfallDir(),
				OracleOnly:  oracleOnly,
			}, pool.RefreshWatcher{
				Waiting: func() { sayWaiting(out) },
				Gathering: func(kind string) {
					fmt.Fprintf(out, "downloading %s ...\n", bulkLabel(kind))
				},
				Gathered: func(_, path string) { fmt.Fprintf(out, "  %s\n", path) },
				Shelved: func(kind string, n int64) {
					fmt.Fprintf(out, "  loaded %s %s\n", commas(n), rowsLabel(kind))
				},
			})
			return err
		},
	}
	cmd.Flags().BoolVar(&oracleOnly, "oracle-only", false,
		"skip the printings download and load (much smaller; prices and "+
			"per-printing art keep their last refresh)")
	return cmd
}

func dataSnapshotCommand(cfg config.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "snapshot",
		Short: "Append today's prices to the price history",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			db, err := openTheWriter(cmd.Context(), out, cfg.DBPath())
			if err != nil {
				return err
			}
			defer func() { _ = db.Close() }()
			n, err := pool.SnapshotPrices(cmd.Context(), db)
			if err != nil {
				return err
			}
			fmt.Fprintf(out, "snapshotted %s prices for today\n", commas(n))
			return nil
		},
	}
}

// commas renders n with thousands separators: 34,512.
func commas(n int64) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	var out []byte
	for i, ch := range []byte(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, ch)
	}
	return string(out)
}
