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
	"github.com/aasquier/sylvan-library/go/internal/sim/tier3"
)

// uiCommand is `mtglab ui`: the app served whole from one process — every
// route, the shell from --web-dist, the tarot pictures from --tarot.
//
// Auth follows MTGLAB_REQUIRE_AUTH and MTGLAB_SECURE_COOKIES, and `app.db`
// and the card pool are found under MTGLAB_DATA_DIR (`internal/config`); the
// server has no `.env` reader, so on a laptop export what you need, or leave
// auth off, which is the local default.
func uiCommand() *cobra.Command {
	var host, port, webDist, tarot string
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
	f.StringVar(&webDist, "web-dist", envOr("MTGLAB_WEB_DIST", "web_dist"),
		"the built frontend (MTGLAB_WEB_DIST)")
	f.StringVar(&tarot, "tarot", envOr("MTGLAB_TAROT_DIR", filepath.Join("assets", "tarot")),
		"the packaged tarot art (MTGLAB_TAROT_DIR)")
	// There is deliberately no `--no-open`. It existed here, parsed into a
	// variable and thrown away with `_ = noOpen`, while its help text promised
	// "serve without opening a browser" -- a promise about behaviour this
	// command has never had, since nothing in the process opens anything. For
	// an operator `--help` is the only documentation there is, so a flag that
	// describes an action the code does not take is worse than no flag.
	return cmd
}

func envOr(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}

// bootSummary is what this process decided, said once, in one line.
//
// It exists so that a `fly logs` tail answers "is this thing configured the
// way I think it is" in two seconds rather than by shell-ing in and reading
// the environment. It is handed the same [config.Config] the process is
// serving with -- resolved once by [config.Load] -- so the line cannot drift
// from the behaviour it describes without the behaviour changing too.
//
// **Never a secret and never a value that might be one.** A key in a log is a
// leak forever, and this app's own rule is that it does not hold most of them
// at all: the mail provider renders as *which sender was chosen*, never as its
// credential, and no ANTHROPIC_ or _TOKEN value appears in any form.
// `TestTheBootSummaryLeaksNoSecret` holds that.
func bootSummary(cfg config.Config, webDist, tarot string, poolPresent bool) []any {
	state := func(on bool, yes, no string) string {
		if on {
			return yes
		}
		return no
	}
	return []any{
		"auth", state(cfg.RequireAuth, "on", "off"),
		"cookies", state(cfg.SecureCookies, "secure", "plain"),
		"schema", auth.SchemaVersion,
		"data_dir", cfg.DataDir,
		"decks_dir", cfg.DecksDir,
		"web_dist", webDist,
		"tarot", tarot,
		"pool", state(poolPresent, "present", "absent"),
		"base_url", cfg.BaseURL,
		"mail", state(cfg.ResendAPIKey != "", "provider", "console"),
		"forge_worker", tier3.Configured(),
	}
}

// configComplaints are the settings that must agree with each other, checked
// at boot instead of on the request that needed them.
//
// A setting on its own is right or wrong only against the others: nothing is
// broken about an unset MTGLAB_EMAIL_FROM until the first person asks for a
// password reset, and by then the person discovering the misconfiguration is
// the one worst placed to fix it. Each of these is a deployment that will fail
// on one specific request, days later, silently.
//
// They are **warnings, never a refusal.** Merging deploys (ADR 23), so a boot
// that refuses here would take the site down for a setting the site does not
// need to serve a single anonymous page. Loud and serving beats silent and
// serving; refusing to serve is worse than both.
func configComplaints(cfg config.Config) []string {
	if !cfg.RequireAuth {
		// Auth off is a laptop, one person, no invites and no reset mail. Every
		// relationship below is about a deployment.
		return nil
	}
	var out []string
	if cfg.ResendAPIKey == "" {
		out = append(out, "auth is on and RESEND_API_KEY is unset: invites and "+
			"password resets are refused rather than sent, because the console "+
			"fallback would print addresses into this log (docs/HOSTING.md §7)")
	}
	if cfg.BaseURLIsDefault() {
		out = append(out, "auth is on and MTGLAB_BASE_URL is still "+
			config.DefaultBaseURL+": every invite and reset link mailed from "+
			"here points at a loopback address")
	}
	if cfg.EmailFromIsDefault() {
		out = append(out, "auth is on and MTGLAB_EMAIL_FROM is still the "+
			"built-in default: a provider refuses any From address outside a "+
			"sending domain you have verified with it")
	}
	if cfg.AdminEmail == "" {
		out = append(out, "auth is on and MTGLAB_ADMIN_EMAIL is unset: nobody "+
			"is reconciled to admin at boot, so an accidental demotion cannot "+
			"repair itself with a restart (ADR 17)")
	}
	return out
}

func serve(host, port, webDist, tarot string) error {
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	// The environment, read once, here. Everything below is handed the
	// resulting value, so the door and the summary describing it cannot
	// disagree about what this process was configured to be.
	cfg := settings()
	// The schema ladder, before anything opens the file: creating `app.db`
	// and bringing it to `auth.SchemaVersion` is this command's job, and a
	// ladder that cannot be applied is a refusal to serve, not a warning — a
	// request answered over a half-migrated file is worse than no answer.
	if err := auth.Migrate(cfg.AppDBPath()); err != nil {
		return err
	}
	// After the ladder, because `schema` in the summary is a fact about the
	// file on disk rather than about this binary's ambition for it, and before
	// anything can fail below, because a boot that dies wants its own
	// configuration in the log above the error.
	_, poolErr := os.Stat(cfg.DBPath())
	log.Info("configuration", bootSummary(cfg, webDist, tarot, poolErr == nil)...)
	for _, complaint := range configComplaints(cfg) {
		log.Warn(complaint)
	}
	// The maintainer, reconciled to admin at every start (ADR 17). A no-op
	// unless MTGLAB_ADMIN_EMAIL is set, which is what a laptop wants.
	if err := ensureMaintainerAtBoot(cfg); err != nil {
		return err
	}
	requireAuth := cfg.RequireAuth
	d, err := door.New(door.Config{
		RequireAuth:   requireAuth,
		SecureCookies: cfg.SecureCookies,
		AppDB:         cfg.AppDBPath(),
		WebDist:       webDist,
		TarotDir:      tarot,
		// The pool, on a lease: opened at the first ask, handed back when
		// idle, re-opened when `data refresh` replaces the file.
		Pool:        pool.New(cfg.DBPath(), log),
		PoolPath:    cfg.DBPath(),
		DecksDir:    cfg.DecksDir,
		ScryfallDir: cfg.ScryfallDir(),
		DataDir:     cfg.DataDir,
		AdminEmail:  cfg.AdminEmail,
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
func ensureMaintainerAtBoot(cfg config.Config) error {
	if cfg.AdminEmail == "" {
		return nil
	}
	db, err := auth.OpenReadWrite(cfg.AppDBPath())
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	return auth.EnsureMaintainer(context.Background(), db, cfg)
}
