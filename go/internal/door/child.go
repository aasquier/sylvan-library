package door

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"syscall"
	"time"
)

// Child is the supervised Python server: the door starts it, relays a
// shutdown to it, and exits when it exits -- so the container has one
// process to watch and a crashed half is a restart rather than a door
// answering 502 to every request forever.
type Child struct {
	cmd  *exec.Cmd
	done chan error
	log  *slog.Logger
}

// StartChild runs argv with the door's own environment and stdio, so the
// server's log lines land where the door's do.
func StartChild(argv []string, log *slog.Logger) (*Child, error) {
	if len(argv) == 0 {
		return nil, errors.New("no command to run")
	}
	cmd := exec.Command(argv[0], argv[1:]...) // #nosec G204 -- the operator's own command line
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = nil
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start %q: %w", argv[0], err)
	}
	c := &Child{cmd: cmd, done: make(chan error, 1), log: log}
	go func() { c.done <- cmd.Wait() }()
	log.Info("started the library behind the door", "pid", cmd.Process.Pid, "command", argv[0])
	return c, nil
}

// Done reports the child's exit, once.
func (c *Child) Done() <-chan error { return c.done }

// Stop asks the child to finish (SIGTERM, which uvicorn handles as a graceful
// shutdown and flushes the visitor ledger on), and kills it if it has not
// gone by the deadline.
func (c *Child) Stop(ctx context.Context) error {
	if c.cmd.Process == nil {
		return nil
	}
	_ = c.cmd.Process.Signal(syscall.SIGTERM)
	select {
	case err := <-c.done:
		return exitError(err)
	case <-ctx.Done():
		c.log.Warn("the library did not stop in time; killing it")
		_ = c.cmd.Process.Kill()
		select {
		case err := <-c.done:
			return exitError(err)
		case <-time.After(5 * time.Second):
			return errors.New("the child did not die")
		}
	}
}

// exitError reads a Wait() result: a clean exit or a signal-terminated exit
// after we asked it to stop is not an error worth reporting.
func exitError(err error) error {
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		if status, ok := exit.Sys().(syscall.WaitStatus); ok && status.Signaled() {
			return nil
		}
	}
	return err
}
