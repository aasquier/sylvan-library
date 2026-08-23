package pool

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"

	duckdb "github.com/duckdb/duckdb-go/v2"
)

// LoadOracle and LoadPrintings replace the table's rows from a bulk file.
//
// Two upgrades over `cards/db.py`'s loaders, both the phase plan's:
// **DuckDB's Appender** instead of a prepared statement per row (the
// ledger's queued 28-minute item — ~110 rows/second measured on the old
// path, most of it `duckdb::DataChunk::Initialize` churn), and **one
// transaction around the DELETE and the load**, so an interrupted refresh
// rolls back to the pool it started with instead of leaving a library with
// no printings at all — the sixteen-minute window `docs/polish/LEDGER.md`
// records.
//
// One behavioural note the swap makes sharp: Python's `INSERT OR REPLACE`
// absorbed a duplicate primary key silently, the Appender refuses it at
// flush. Within one run a collision means the bulk file itself repeated an
// id; refusing loudly beats absorbing invisibly, and the transaction makes
// the refusal safe.

func LoadOracle(ctx context.Context, db *sql.DB, path string) (int64, error) {
	return load(ctx, db, path, "oracle_cards", OracleColumns,
		SkipOracleLayout, OracleRow)
}

func LoadPrintings(ctx context.Context, db *sql.DB, path string) (int64, error) {
	return load(ctx, db, path, "printings", PrintingColumns,
		SkipPrinting, PrintingRow)
}

func load(ctx context.Context, db *sql.DB, path, table string,
	columns []string, skip func(map[string]any) bool,
	row func(map[string]any) []any) (int64, error) {

	conn, err := db.Conn(ctx)
	if err != nil {
		return 0, fmt.Errorf("load %s: %w", table, err)
	}
	defer func() { _ = conn.Close() }()

	if _, err := conn.ExecContext(ctx, "BEGIN TRANSACTION"); err != nil {
		return 0, fmt.Errorf("load %s: %w", table, err)
	}
	committed := false
	defer func() {
		if !committed {
			_, _ = conn.ExecContext(ctx, "ROLLBACK")
		}
	}()
	if _, err := conn.ExecContext(ctx, "DELETE FROM "+table); err != nil {
		return 0, fmt.Errorf("load %s: %w", table, err)
	}

	var total int64
	err = conn.Raw(func(driverConn any) error {
		dc, ok := driverConn.(driver.Conn)
		if !ok {
			return fmt.Errorf("load %s: not a duckdb connection", table)
		}
		appender, err := duckdb.NewAppenderWithColumns(dc, "", "", table, columns)
		if err != nil {
			return err
		}
		walkErr := IterCards(path, func(card map[string]any) error {
			if skip(card) {
				return nil
			}
			values := row(card)
			args := make([]driver.Value, len(values))
			for i, v := range values {
				args[i] = v
			}
			if err := appender.AppendRow(args...); err != nil {
				return err
			}
			total++
			return nil
		})
		if walkErr != nil {
			_ = appender.Close()
			return walkErr
		}
		return appender.Close()
	})
	if err != nil {
		return 0, fmt.Errorf("load %s: %w", table, err)
	}
	if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
		return 0, fmt.Errorf("load %s: %w", table, err)
	}
	committed = true
	return total, nil
}

// SnapshotPrices is `snapshot_prices`: append today's prices to
// price_history. One statement, exactly as Python runs it.
func SnapshotPrices(ctx context.Context, db *sql.DB) (int64, error) {
	if _, err := db.ExecContext(ctx, `
        INSERT OR REPLACE INTO price_history
        SELECT CURRENT_DATE, id, oracle_id, name, price_usd, price_usd_foil
        FROM printings
        WHERE price_usd IS NOT NULL OR price_usd_foil IS NOT NULL
    `); err != nil {
		return 0, fmt.Errorf("snapshot: %w", err)
	}
	var written int64
	err := db.QueryRowContext(ctx,
		"SELECT count(*) FROM price_history WHERE snapshot_date = CURRENT_DATE").
		Scan(&written)
	if err != nil {
		return 0, fmt.Errorf("snapshot: %w", err)
	}
	return written, nil
}
