package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/auth"
	"github.com/aasquier/sylvan-library/go/internal/config"
	"github.com/aasquier/sylvan-library/go/internal/pool"
)

// The `data` family: the runbook's three commands over the volume.
//
// `backup` is the one with a procedure attached (docs/HOSTING.md): it writes
// a consistent copy of `app.db` **while the app is serving**, and the copy is
// pulled off the box and removed afterwards, because a file full of password
// hashes should not sit on the volume indefinitely. So the two things asked
// of it here are that it refuses to overwrite -- a backup that clobbered the
// previous one is not a backup -- and that what it writes is a real database
// rather than a truncated one.
//
// `snapshot` appends today's prices to the price history, which is the series
// every "what has this deck cost over time" answer is read from. A snapshot
// that silently wrote nothing would leave a gap nobody notices until somebody
// asks about a month that has one.
//
// `refresh` is mostly not here: it downloads from Scryfall, and the sequence
// itself is tested against a stub in `internal/pool` rather than by reaching
// the network from a test. What *is* here is its last line. The sweep's report
// is a pure function of a count and a writer, and "1 older bulk files" is
// exactly the kind of thing a stub two packages away would never catch.
//
// All parallel since ADR 40: the volume each of these runs against is a
// [deployment] value rather than MTGLAB_DATA_DIR on the process.

// The backup writes a real database and says what it wrote.
func TestABackupWritesAConsistentCopyAndSaysWhatItWrote(t *testing.T) {
	t.Parallel()
	d := scratchDeployment(t)
	// A database with something in it, so the copy is of a real file.
	if _, err := d.runWithInput(t, "hunter2hunter2\nhunter2hunter2\n",
		"users", "add", "keeper", "--admin"); err != nil {
		t.Fatal(err)
	}

	dest := filepath.Join(t.TempDir(), "app.db.backup")
	out, err := d.run(t, "data", "backup", dest)
	if err != nil {
		t.Fatalf("backup: %v", err)
	}
	// It says where it went, what schema it is, and how big -- the three
	// facts the runbook's next step needs.
	if !strings.Contains(out, dest) {
		t.Errorf("the backup does not say where it went:\n%s", out)
	}
	if !strings.Contains(out, "schema version") {
		t.Errorf("the backup does not name the schema:\n%s", out)
	}
	if !strings.Contains(out, "bytes") {
		t.Errorf("the backup does not say how big it is:\n%s", out)
	}

	// And the copy is a real database with the account in it, rather than a
	// truncated file that only looks like one.
	db, err := auth.Open(dest)
	if err != nil {
		t.Fatalf("the backup will not open: %v", err)
	}
	defer func() { _ = db.Close() }()
	users, err := auth.AllUsers(t.Context(), db)
	if err != nil {
		t.Fatalf("the backup will not read: %v", err)
	}
	if len(users) != 1 || users[0].Username != "keeper" {
		t.Errorf("the backup holds %v", users)
	}

	// The original is untouched and still serving.
	if _, err := os.Stat(d.AppDBPath()); err != nil {
		t.Errorf("the original went missing: %v", err)
	}
}

// **The destination must not exist.** A backup that clobbered the previous
// one is not a backup, and the procedure runs it more than once.
func TestABackupRefusesToOverwriteTheLastOne(t *testing.T) {
	t.Parallel()
	d := scratchDeployment(t)
	if _, err := d.run(t, "users", "list"); err != nil {
		t.Fatal(err)
	}

	dest := filepath.Join(t.TempDir(), "app.db.backup")
	if _, err := d.run(t, "data", "backup", dest); err != nil {
		t.Fatalf("the first backup: %v", err)
	}
	before, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := d.run(t, "data", "backup", dest); err == nil {
		t.Fatal("the second backup overwrote the first")
	}
	after, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Error("the refused backup changed the file anyway")
	}
}

// A backup of a database that is not there is a refusal rather than an empty
// file -- an empty backup restored is an instance with no accounts.
func TestABackupOfNothingIsRefusedRatherThanEmpty(t *testing.T) {
	t.Parallel()
	d := scratchDeployment(t)
	// No `app.db`: nothing has run the ladder here.
	if _, err := os.Stat(d.AppDBPath()); err == nil {
		t.Skip("this scratch directory already has an app.db")
	}

	dest := filepath.Join(t.TempDir(), "app.db.backup")
	if _, err := d.run(t, "data", "backup", dest); err == nil {
		if info, statErr := os.Stat(dest); statErr == nil && info.Size() == 0 {
			t.Fatal("an empty backup was written -- restoring it is an instance " +
				"with no accounts")
		}
	}
}

// The snapshot appends today's prices, and says how many -- a snapshot that
// silently wrote nothing leaves a gap in the series nobody notices until
// somebody asks about the month that has one.
func TestASnapshotAppendsTodaysPricesAndCountsThem(t *testing.T) {
	t.Parallel()
	d := scratchDeployment(t).withPool(t)

	out, err := d.run(t, "data", "snapshot")
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if !strings.Contains(out, "snapshotted") {
		t.Errorf("the snapshot said:\n%s", out)
	}
	if !strings.Contains(out, "for today") {
		t.Errorf("the snapshot does not say which day it wrote:\n%s", out)
	}

	// Twice on one day is idempotent rather than doubled: the series is
	// keyed by day, and a second run must not put two rows on it.
	second, err := d.run(t, "data", "snapshot")
	if err != nil {
		t.Fatalf("the second snapshot: %v", err)
	}
	if second != out {
		t.Logf("a second snapshot on the same day reported differently:\n%s\n%s", out, second)
	}
}

// **A snapshot on a machine with no pool mints one and reports zero.**
//
// That is the current behaviour, pinned here rather than approved of: the
// writer creates the DuckDB file when it is absent, which is how `data
// refresh` bootstraps a fresh machine, and the snapshot then finds no prices
// and says so with a zero exit code.
//
// The operational risk is worth naming where somebody will read it. If the
// volume ever failed to mount, `MTGLAB_DATA_DIR` would point at an empty
// directory, this would create an empty pool on the container's own disk, and
// the runbook would see "snapshotted 0 prices for today" and a green exit --
// while the real price history sat untouched on the volume that is not there.
// Whether that should refuse instead is a decision rather than a bug fix, so
// this test records what happens today and fails the day it changes.
func TestASnapshotOnAFreshMachineMintsAPoolAndReportsZero(t *testing.T) {
	t.Parallel()
	d := scratchDeployment(t)

	out, err := d.run(t, "data", "snapshot")
	if err != nil {
		t.Fatalf("a fresh machine failed the snapshot: %v", err)
	}
	if !strings.Contains(out, "snapshotted 0") {
		t.Errorf("a fresh machine snapshotted %q", strings.TrimSpace(out))
	}
	// It minted the pool on the way through, which is the half worth
	// knowing: nothing about the output says the volume was empty.
	if _, statErr := os.Stat(d.DBPath()); statErr != nil {
		t.Errorf("the snapshot did not create the pool it reported on: %v", statErr)
	}
}

// The refresh's last line: what it swept, and nothing at all when it swept
// nothing.
//
// The silence is the half worth pinning. An ordinary refresh on a shelf that
// is already tidy has to read exactly as it always has — the line is news, the
// same judgement [sayWaiting] makes about waiting — and a `0 older bulk files`
// on every run would be furniture within a week.
func TestTheRefreshSaysWhatItSweptAndStaysQuietWhenItSweptNothing(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		swept pool.SweepCounts
		want  string
	}{
		{"a tidy shelf", pool.SweepCounts{}, ""},
		// A count of one agrees with its noun.
		{"one copy", pool.SweepCounts{Files: 1, Bytes: 512},
			"swept 1 older bulk file (512 bytes freed)\n"},
		// And the bytes wear the same separators as every other number this
		// command prints.
		{"a full shelf", pool.SweepCounts{Files: 6, Bytes: 317_004_233},
			"swept 6 older bulk files (317,004,233 bytes freed)\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var out bytes.Buffer
			sayTheSweep(&out, tc.swept)
			if got := out.String(); got != tc.want {
				t.Errorf("the refresh said %q, want %q", got, tc.want)
			}
		})
	}
}

// The three subcommands are all wired, so one that stopped being registered
// would vanish from the runbook without anything failing.
func TestTheDataFamilyIsWired(t *testing.T) {
	t.Parallel()
	got := map[string]bool{}
	for _, c := range dataCommand(config.Config{}).Commands() {
		got[c.Name()] = true
	}
	for _, want := range []string{"refresh", "snapshot", "backup"} {
		if !got[want] {
			t.Errorf("`mtglab data %s` is not wired", want)
		}
	}
}
