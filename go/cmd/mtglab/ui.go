package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/aasquier/sylvan-library/go/internal/auth"
	"github.com/aasquier/sylvan-library/go/internal/config"
	"github.com/aasquier/sylvan-library/go/internal/door"
	"github.com/aasquier/sylvan-library/go/internal/pool"
)

// uiCommand is `mtglab ui`: the front door on --host:--port, everything
// under /api proxied to --upstream, the shell and the tarot pictures served
// from --web-dist and --tarot, and -- when a command follows `--` -- that
// command started as a child and supervised, which is how the container runs
// the pair:
//
//	mtglab ui --host 0.0.0.0 --port 8080 --upstream http://127.0.0.1:8765 \
//	    -- mtglab ui --no-open --host 127.0.0.1 --port 8765
//
// Auth follows MTGLAB_REQUIRE_AUTH and MTGLAB_SECURE_COOKIES exactly as
// `config.py` reads them, and `app.db` and the card pool are found under
// MTGLAB_DATA_DIR (`internal/config`); the door has no `.env` reader, so on
// a laptop export what the Python server reads from its `.env`, or leave
// auth off, which is the local default.
func uiCommand() *cobra.Command {
	var (
		host, port, upstream, webDist, tarot string
		noOpen                               bool
	)
	cmd := &cobra.Command{
		Use:   "ui [flags] [-- command to supervise...]",
		Short: "Serve the app: the front door, with the library behind it",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			child := args
			if at := cmd.ArgsLenAtDash(); at >= 0 {
				child = args[at:]
			} else if len(args) > 0 {
				return fmt.Errorf("unexpected arguments %q (put a command to supervise after `--`)", args)
			}
			return serve(host, port, upstream, webDist, tarot, child)
		},
	}
	f := cmd.Flags()
	f.StringVar(&host, "host", "127.0.0.1", "address to listen on")
	f.StringVar(&port, "port", "8765", "port to listen on")
	f.StringVar(&upstream, "upstream", "http://127.0.0.1:8766",
		"where everything not yet served here goes")
	f.StringVar(&webDist, "web-dist", envOr("MTGLAB_WEB_DIST", filepath.Join("src", "mtglab", "web_dist")),
		"the built frontend (MTGLAB_WEB_DIST)")
	f.StringVar(&tarot, "tarot", envOr("MTGLAB_TAROT_DIR", filepath.Join("src", "mtglab", "assets", "tarot")),
		"the packaged tarot art (MTGLAB_TAROT_DIR)")
	f.BoolVar(&noOpen, "no-open", false, "accepted for parity with the Python `ui`; the door never opens a browser")
	_ = noOpen
	return cmd
}

func envOr(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}

func serve(host, port, upstream, webDist, tarot string, child []string) error {
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	up, err := url.Parse(upstream)
	if err != nil || up.Scheme == "" || up.Host == "" {
		return fmt.Errorf("--upstream must be a URL like http://127.0.0.1:8765, got %q", upstream)
	}
	// The schema ladder, before anything opens the file: creating `app.db`
	// and bringing it to `auth.SchemaVersion` is this command's job since
	// Phase 8, and a ladder that cannot be applied is a refusal to serve,
	// not a warning — a request answered over a half-migrated file is worse
	// than no answer.
	if err := auth.Migrate(config.AppDBPath()); err != nil {
		return err
	}
	requireAuth := config.RequireAuth()
	d, err := door.New(door.Config{
		RequireAuth:   requireAuth,
		SecureCookies: config.SecureCookies(),
		AppDB:         config.AppDBPath(),
		WebDist:       webDist,
		TarotDir:      tarot,
		Upstream:      up,
		// The pool, on a lease: opened at the first ask, handed back when
		// idle, re-opened when `data refresh` replaces the file.
		Pool:        pool.New(config.DBPath(), log),
		DecksDir:    config.DecksDir(),
		ScryfallDir: config.ScryfallDir(),
		DataDir:     config.DataDir(),
		AdminEmail:  config.AdminEmail(),
		Logger:      log,
	})
	if err != nil {
		return err
	}
	defer func() { _ = d.Close() }()

	addr := net.JoinHostPort(host, port)
	server := &http.Server{
		Addr:              addr,
		Handler:           d.Handler(),
		ReadHeaderTimeout: 30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", addr, err)
	}
	fmt.Printf("sylvan-library -> http://%s\n", addr)

	var kid *door.Child
	if len(child) > 0 {
		kid, err = door.StartChild(child, log)
		if err != nil {
			_ = listener.Close()
			return err
		}
	}
	// Advisory only: `auth.Migrate` above created the file and the schema,
	// so a failure here is a genuinely unreadable database rather than a
	// fresh instance -- and the door still answers anonymous, which is the
	// degraded state a maintainer can sign in past once the disk is fixed.
	if err := d.Check(context.Background()); err != nil && requireAuth {
		log.Warn("app.db is not readable yet; every session is anonymous until it is", "error", err)
	}

	errs := make(chan error, 1)
	go func() { errs <- server.Serve(listener) }()

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)

	var childErr error
	select {
	case sig := <-signals:
		log.Info("stopping", "signal", sig.String())
	case err := <-errs:
		if !errors.Is(err, http.ErrServerClosed) {
			log.Error("the door stopped serving", "error", err)
			childErr = err
		}
	case err := <-kidDone(kid):
		log.Error("the library behind the door stopped; stopping too", "error", err)
		childErr = fmt.Errorf("the supervised command exited: %w", err)
		if err == nil {
			childErr = errors.New("the supervised command exited")
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_ = server.Shutdown(ctx)
	if kid != nil {
		if err := kid.Stop(ctx); err != nil && childErr == nil {
			childErr = err
		}
	}
	return childErr
}

// kidDone is the child's exit channel, or one that never fires when there is
// no child -- so the select above reads the same either way.
func kidDone(c *door.Child) <-chan error {
	if c == nil {
		return nil
	}
	return c.Done()
}
