package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
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

// uiCommand is `mtglab ui`: the app served whole from one process — every
// route, the shell from --web-dist, the tarot pictures from --tarot.
//
// Auth follows MTGLAB_REQUIRE_AUTH and MTGLAB_SECURE_COOKIES, and `app.db`
// and the card pool are found under MTGLAB_DATA_DIR (`internal/config`); the
// server has no `.env` reader, so on a laptop export what you need, or leave
// auth off, which is the local default.
func uiCommand() *cobra.Command {
	var (
		host, port, webDist, tarot string
		noOpen                     bool
	)
	cmd := &cobra.Command{
		Use:   "ui",
		Short: "Serve the app",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return serve(host, port, webDist, tarot)
		},
	}
	f := cmd.Flags()
	f.StringVar(&host, "host", "127.0.0.1", "address to listen on")
	f.StringVar(&port, "port", "8765", "port to listen on")
	f.StringVar(&webDist, "web-dist", envOr("MTGLAB_WEB_DIST", filepath.Join("src", "mtglab", "web_dist")),
		"the built frontend (MTGLAB_WEB_DIST)")
	f.StringVar(&tarot, "tarot", envOr("MTGLAB_TAROT_DIR", filepath.Join("src", "mtglab", "assets", "tarot")),
		"the packaged tarot art (MTGLAB_TAROT_DIR)")
	f.BoolVar(&noOpen, "no-open", false, "serve without opening a browser")
	_ = noOpen
	return cmd
}

func envOr(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}

func serve(host, port, webDist, tarot string) error {
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	// The schema ladder, before anything opens the file: creating `app.db`
	// and bringing it to `auth.SchemaVersion` is this command's job, and a
	// ladder that cannot be applied is a refusal to serve, not a warning — a
	// request answered over a half-migrated file is worse than no answer.
	if err := auth.Migrate(config.AppDBPath()); err != nil {
		return err
	}
	// The maintainer, reconciled to admin at every start (ADR 17). A no-op
	// unless MTGLAB_ADMIN_EMAIL is set, which is what a laptop wants.
	if err := ensureMaintainerAtBoot(); err != nil {
		return err
	}
	requireAuth := config.RequireAuth()
	d, err := door.New(door.Config{
		RequireAuth:   requireAuth,
		SecureCookies: config.SecureCookies(),
		AppDB:         config.AppDBPath(),
		WebDist:       webDist,
		TarotDir:      tarot,
		// The pool, on a lease: opened at the first ask, handed back when
		// idle, re-opened when `data refresh` replaces the file.
		Pool:        pool.New(config.DBPath(), log),
		PoolPath:    config.DBPath(),
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

	// Advisory only: `auth.Migrate` above created the file and the schema,
	// so a failure here is a genuinely unreadable database rather than a
	// fresh instance -- and the app still answers anonymous, which is the
	// degraded state a maintainer can sign in past once the disk is fixed.
	if err := d.Check(context.Background()); err != nil && requireAuth {
		log.Warn("app.db is not readable yet; every session is anonymous until it is", "error", err)
	}

	errs := make(chan error, 1)
	go func() { errs <- server.Serve(listener) }()

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)

	var serveErr error
	select {
	case sig := <-signals:
		log.Info("stopping", "signal", sig.String())
	case err := <-errs:
		if !errors.Is(err, http.ErrServerClosed) {
			log.Error("the server stopped serving", "error", err)
			serveErr = err
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_ = server.Shutdown(ctx)
	return serveErr
}

// ensureMaintainerAtBoot opens app.db briefly for the reconciliation the
// serving process owes ADR 17, and closes it again — the door opens its own
// handles with its own lifetimes.
func ensureMaintainerAtBoot() error {
	if config.AdminEmail() == "" {
		return nil
	}
	db, err := auth.OpenReadWrite(config.AppDBPath())
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	return auth.EnsureMaintainer(context.Background(), db)
}
