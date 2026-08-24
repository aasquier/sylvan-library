package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/aasquier/sylvan-library/go/internal/auth"
	"github.com/aasquier/sylvan-library/go/internal/config"
	"github.com/aasquier/sylvan-library/go/internal/pool"
)

// dataCommand is `mtglab data`: the pool's care and feeding, on the box that
// serves it — the reason the runbook's `fly ssh console -C "mtglab data
// refresh"` had to survive the interpreter leaving the image.
func dataCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "data",
		Short: "Fetch and load the card pool",
	}
	cmd.AddCommand(dataRefreshCommand(), dataSnapshotCommand(), dataBackupCommand())
	return cmd
}

// dataBackupCommand is `mtglab data backup`: the runbook's online copy of
// `app.db`, safe while the app serves. The destination must not exist; the
// procedure pulls the copy off the box and removes it, because a backup of
// password hashes should not sit on the volume indefinitely.
func dataBackupCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "backup <destination>",
		Short: "Write a consistent copy of app.db, safe while the app runs",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			version, err := auth.Backup(cmd.Context(), config.AppDBPath(), args[0])
			if err != nil {
				return err
			}
			info, err := os.Stat(args[0])
			if err != nil {
				return err
			}
			fmt.Printf("wrote %s (schema version %d, %s bytes)\n",
				args[0], version, commas(info.Size()))
			return nil
		},
	}
}

func dataRefreshCommand() *cobra.Command {
	var oracleOnly bool
	cmd := &cobra.Command{
		Use:   "refresh",
		Short: "Download Scryfall's bulk data and rebuild the pool",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			db, err := pool.OpenWriter(ctx, config.DBPath())
			if err != nil {
				return err
			}
			defer func() { _ = db.Close() }()

			fmt.Println("downloading oracle_cards ...")
			oracle, err := pool.DownloadBulk(ctx, "oracle_cards", config.ScryfallDir())
			if err != nil {
				return err
			}
			fmt.Printf("  %s\n", oracle)
			n, err := pool.LoadOracle(ctx, db, oracle)
			if err != nil {
				return err
			}
			fmt.Printf("  loaded %s oracle cards\n", commas(n))

			if oracleOnly {
				return nil
			}
			fmt.Println("downloading default_cards (large) ...")
			printings, err := pool.DownloadBulk(ctx, "default_cards", config.ScryfallDir())
			if err != nil {
				return err
			}
			fmt.Printf("  %s\n", printings)
			m, err := pool.LoadPrintings(ctx, db, printings)
			if err != nil {
				return err
			}
			fmt.Printf("  loaded %s printings\n", commas(m))
			return nil
		},
	}
	cmd.Flags().BoolVar(&oracleOnly, "oracle-only", false,
		"skip the printings download and load (much smaller; prices and "+
			"per-printing art keep their last refresh)")
	return cmd
}

func dataSnapshotCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "snapshot",
		Short: "Append today's prices to the price history",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := pool.OpenWriter(cmd.Context(), config.DBPath())
			if err != nil {
				return err
			}
			defer func() { _ = db.Close() }()
			n, err := pool.SnapshotPrices(cmd.Context(), db)
			if err != nil {
				return err
			}
			fmt.Printf("snapshotted %s prices for today\n", commas(n))
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
