package pool_test

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/aasquier/sylvan-library/go/internal/pool"
	"github.com/aasquier/sylvan-library/go/internal/pool/pooltest"
)

// The two halves of the DuckDB lock, gated.
//
// **A refresh on the live instance would report a broken database and it was
// a timing problem**: the serving process holds the pool file read-only, a
// read-only holder excludes a writer completely, and two health checks that
// did not know about each other kept the lease standing almost continuously.
// `Pool.UseWithoutHolding` carries the measurement and the argument.
//
// What is checkable here is the mechanism rather than the deployment:
//
//   - that a probe's read does not renew the lease and hands the file back;
//   - that `pool.Locked` really recognises the error DuckDB gives for this,
//     which needs a **second process**, because DuckDB caches its instance per
//     file inside one and a same-process second open fails with an entirely
//     different message.
//
// The second is the one worth the machinery. A classifier written from a
// remembered error string is a classifier that quietly stops classifying, and
// what it would cost here is a `data refresh` waiting a patient minute for a
// permission error to heal itself.

// helperEnv turns a run of this test binary into the process that holds the
// file. Go's standard shape for "I need a second process and I have one right
// here"; `-test.run` picks the holder out and the environment tells it so.
const helperEnv = "MTGLAB_TEST_HOLD_POOL"

// TestAHolderIsASecondProcess is the helper, not a test of anything. It opens
// the pool read-only and sits on it, which is exactly what a running `mtglab
// ui` does between requests.
func TestAHolderIsASecondProcess(t *testing.T) {
	// **No `t.Parallel()`, and it is not a serial test either** -- it is not a
	// test. In the parent run it skips immediately; in the child it *is* the
	// second process, and it sits on the pool file for thirty seconds on
	// purpose. Parallelising it would mean a thirty-second sleep running
	// alongside the package's real tests in every ordinary run, which is a
	// suite that takes half a minute longer for nothing.
	//
	// It is spelled `TestX` because that is Go's only way to reach a function
	// in a test binary from outside it; `-test.run` picks it out and
	// `helperEnv` tells it which role it is playing.
	path := os.Getenv(helperEnv)
	if path == "" {
		t.Skip("not the holder")
	}
	db, err := pool.Open(context.Background(), path)
	if err != nil {
		t.Fatalf("hold: %v", err)
	}
	defer func() { _ = db.Close() }()
	fmt.Println("held")
	// Long enough for the parent to take its measurement and kill this; short
	// enough that a parent which died leaves nothing behind for long.
	time.Sleep(30 * time.Second)
}

func TestLockedRecognisesARealConflict(t *testing.T) {
	// **Not parallel**, and the reason is not shared state in this package: it
	// starts a second process that opens a database file, and a machine
	// running several of those alongside a sixteen-way suite is a machine
	// measuring its own load. It is also over in about a second.
	// **A copy, at a path this process has never opened.** DuckDB caches its
	// instance per file *inside* a process, so a parent that had already built
	// the fixture here would meet its own cached handle rather than the child's
	// lock, and the error would be a different one entirely ("Can't open a
	// connection to same database file with a different configuration",
	// measured). Copying the built file out to a path only the child ever opens
	// is what makes this a genuine cross-process conflict.
	path := filepath.Join(t.TempDir(), "conflict.duckdb")
	built, err := os.ReadFile(pooltest.Build(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, built, 0o600); err != nil {
		t.Fatal(err)
	}
	held := exec.Command(os.Args[0], "-test.run", "TestAHolderIsASecondProcess")
	held.Env = append(os.Environ(), helperEnv+"="+path)
	out, err := held.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := held.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = held.Process.Kill()
		_ = held.Wait()
	})
	// Wait for the holder to say it has the file, rather than sleeping at it.
	// A fixed sleep against an unbounded open is how the port test in this
	// repo once produced a flake that skipped a deploy.
	if line, err := bufio.NewReader(out).ReadString('\n'); err != nil {
		t.Fatalf("the holder never took the file: %v (%q)", err, line)
	}

	_, err = pool.OpenWriter(context.Background(), path)
	if err == nil {
		t.Fatal("a writer opened a file another process is holding read-only")
	}
	// **The whole point of the test.** If DuckDB rewords this, the failure
	// lands here rather than in a refresh that waits a minute for the wrong
	// kind of error.
	if !pool.Locked(err) {
		t.Fatalf("pool.Locked said no to a real lock conflict: %v", err)
	}
}

func TestLockedSaysNoToEverythingElse(t *testing.T) {
	t.Parallel()
	// The half that matters for an operator's time: waiting only ever helps a
	// lock, so nothing else may be classified as one.
	for _, err := range []error{
		nil,
		errors.New("permission denied"),
		errors.New("no such file or directory"),
		errors.New("Serialization Error: Conflict on tuple deletion"),
	} {
		if pool.Locked(err) {
			t.Fatalf("waiting will not fix this and Locked said it would: %v", err)
		}
	}
}

func TestOpenWriterWaitingGivesUpOnAnythingElse(t *testing.T) {
	t.Parallel()
	said := 0
	// A directory where a file has to be: not a lock, and not something a
	// minute of patience improves.
	dir := t.TempDir()
	start := time.Now()
	_, err := pool.OpenWriterWaiting(context.Background(), dir, time.Minute,
		func() { said++ })
	if err == nil {
		t.Fatal("opened a directory as a database")
	}
	if said != 0 {
		t.Fatal("announced a wait for an error that is not a lock")
	}
	if took := time.Since(start); took > 5*time.Second {
		t.Fatalf("waited %s on an error waiting cannot fix", took)
	}
}

func TestOpenWriterWaitingTakesTheFileWhenItIsFree(t *testing.T) {
	t.Parallel()
	said := 0
	db, err := pool.OpenWriterWaiting(context.Background(),
		t.TempDir()+"/fresh.duckdb", time.Minute, func() { said++ })
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	// **An ordinary refresh must look exactly as it always has.** The line is
	// only worth printing when there is something to wait for; printing it
	// every time would teach an operator to ignore it.
	if said != 0 {
		t.Fatal("announced a wait for a door that was already open")
	}
}

func TestAProbeDoesNotHoldTheFile(t *testing.T) {
	t.Parallel()
	p := pooltest.Open(t)
	ctx := context.Background()

	// **The shape that locked the instance out.** Two health checks, thirty
	// seconds apart in principle and fifteen in the worst phase, each renewing
	// a ten-second lease: the file was never free for a `data refresh` to take.
	// A quiet read leaves it free the moment it is done.
	if err := p.UseWithoutHolding(ctx, func(c *pool.Conn) error {
		_, err := pool.Count(ctx, c.DB(), "oracle_cards")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if p.Held() {
		t.Fatal("a probe left the pool open behind it")
	}

	// **And it is a probe that is quiet, not the pool.** Somebody reading the
	// library still gets the burst the lease was built for.
	if err := p.Use(ctx, func(*pool.Conn) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if !p.Held() {
		t.Fatal("an ordinary use did not take the lease")
	}
	// A probe arriving in the middle of somebody's visit must not shut the
	// door on them — which is the whole reason this asks the reaper's question
	// rather than simply closing.
	if err := p.UseWithoutHolding(ctx, func(*pool.Conn) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if !p.Held() {
		t.Fatal("a probe reaped the pool out from under a live visit")
	}
}

func TestAProbeNeverPushesSomebodyElsesLeaseOut(t *testing.T) {
	t.Parallel()
	p := pooltest.Open(t)
	p.SetIdle(time.Hour)
	ctx := context.Background()
	if err := p.Use(ctx, func(*pool.Conn) error { return nil }); err != nil {
		t.Fatal(err)
	}
	// A visitor, five minutes ago, on an hour-long lease: the pool is theirs
	// for another fifty-five minutes.
	p.ForceLease(0, time.Now().Add(-5*time.Minute))
	if err := p.UseWithoutHolding(ctx, func(*pool.Conn) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if !p.Held() {
		t.Fatal("the probe closed a pool still inside somebody's lease")
	}
	// **And it did not extend it either**, which is the half a `Held()` check
	// cannot see: if the probe had stamped `lastUsed`, the reaper would now be
	// counting from the probe rather than from the visitor.
	p.SetIdle(time.Minute)
	if !p.ReapOnce() {
		t.Fatal("the probe renewed a lease that belonged to somebody else")
	}
}
