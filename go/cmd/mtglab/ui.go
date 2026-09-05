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
	"github.com/aasquier/sylvan-library/go/internal/claude"
	"github.com/aasquier/sylvan-library/go/internal/config"
	"github.com/aasquier/sylvan-library/go/internal/door"
	"github.com/aasquier/sylvan-library/go/internal/night"
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
func uiCommand(cfg config.Config, forge tier3.Settings) *cobra.Command {
	var host, port, webDist, tarot string
	cmd := &cobra.Command{
		Use:   "ui",
		Short: "Serve the app",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return serve(cfg, forge, host, port, webDist, tarot)
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
func bootSummary(cfg config.Config, forge tier3.Settings, webDist, tarot string, poolPresent bool) []any {
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
		"forge_worker", forge.Configured(),
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

// shutdownGrace is how long a stop waits for the requests already in flight
// before it drops them and goes.
//
// It is a named constant rather than a number at the call site because the
// boot test waits on this same stop, and that test's budget used to be a
// **peer** of this one — 25 seconds against 20. Five seconds apart is not a
// margin: it means the first stop that legitimately spends its whole grace
// draining a connection decides the test by coin flip. The test derives its
// wait from this constant now (`serve_test.go`), so the two cannot drift back
// into being peers without somebody moving them both on purpose.
const shutdownGrace = 20 * time.Second

// serve is the whole app coming up on `host:port` and serving until a signal
// stops it.
func serve(cfg config.Config, forge tier3.Settings, host, port, webDist, tarot string) error {
	addr := net.JoinHostPort(host, port)
	return serveOn(cfg, forge, webDist, tarot, func() (net.Listener, error) {
		l, err := net.Listen("tcp", addr)
		if err != nil {
			return nil, fmt.Errorf("listen on %s: %w", addr, err)
		}
		return l, nil
	})
}

// serveOn is [serve] with the listener supplied rather than named, acquired at
// exactly the point in the boot where the listener has always been acquired —
// after the ladder, the summary, the reconciliation and the door, so the fixed
// order this package's comment argues for is unchanged. One thing now precedes
// all of it, and deliberately: the signal handler, argued at its own call
// below.
//
// **The listener is a parameter because an address is not a reservation.** A
// caller that can only say "port 41019" has to let this function bind it, and
// between the caller choosing that number and the bind here there is a window
// in which anything on the machine may take it. For the boot test that window
// was measured at 0.2s idle and 5.3s on a loaded machine — and because
// nothing held the port during it, every probe fast-failed with
// ECONNREFUSED, so the test's ~2.2s of polling (100 sleeps of 20ms, a wall
// clock a busy CPU never stretches) expired against a boot whose own cost is
// unbounded. The result was `the server never answered`, which is a lie about
// a server that was still coming up. A caller that hands over a listener it is
// already holding has no window and needs no clock: the connection is accepted
// into the backlog the moment it is made, and the probe waits for the boot
// instead of racing it.
func serveOn(cfg config.Config, forge tier3.Settings, webDist, tarot string,
	listen func() (net.Listener, error)) error {
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))

	// The night's switches, resolved first and allowed to refuse — the one
	// exception to `configComplaints`' warnings-never-refusal rule, argued at
	// [night.SettingsFromConfig]: those complaints are about settings the
	// site does not need to serve an anonymous page, while a misconfigured
	// night is a scheduler that would quietly run on the wrong clock at an
	// hour nobody is watching. Before the ladder and the signal handler,
	// because nothing has been touched yet and the sentence is the fix.
	nightSet, err := night.SettingsFromConfig(cfg)
	if err != nil {
		return err
	}

	// **The stop is armed before anything it could have to interrupt** — the
	// one ordering in this function that is not about the boot.
	//
	// This call used to sit at the bottom, three statements below
	// `server.Serve`, and everything above it therefore ran unguarded: the
	// ladder, the reconciliation, the door, the bind, and the first requests
	// answered over that listener. [signal.Notify] is the only thing that
	// takes SIGTERM away from the runtime, whose default action is to kill the
	// process where it stands, so a stop arriving in that window was not
	// *delayed*, it was **gone**. The boot test hung on it four times in one
	// day on CI, and the goroutine dump was unambiguous: `serveOn` parked in
	// the select below, `Serve` still accepting, nothing inside `Shutdown`.
	//
	// The buffered channel is the other half. A signal taken during the boot
	// is **held**, so `auth.Migrate` — a forward-only ladder that runs on
	// every deploy (ADR 23) — finishes rather than dying halfway, and the
	// select finds the stop already waiting. The cost is stated rather than
	// hidden: a boot that genuinely wedges can no longer be SIGTERMed, and
	// SIGKILL is the right tool for that.
	//
	// **Here and not in `main`.** Measured against the built binary, a SIGTERM
	// inside the first ~50ms still takes the default action — the process is
	// in runtime and CGO start-up and flag parsing — and from ~100ms it stops
	// cleanly. That window is the right one to leave open: nothing in it has
	// bound a port or touched the volume, so dying there is a process that
	// never started. Arming in `main` would close it by suppressing the
	// default action for the **whole binary**, and a `sim` run that ignores
	// Ctrl-C is a worse bug than this one.
	//
	// Stopped on the way out: the registration is process-wide, and un-stopped
	// every call left a channel registered for the life of the process. That
	// does mean the two tests driving `serve` into an early refusal now touch
	// a process-global; they stay parallel, because delivery is a broadcast to
	// every registered channel rather than a handoff to one, so no
	// registration can take a signal away from another.
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(signals)

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
	log.Info("configuration", bootSummary(cfg, forge, webDist, tarot, poolErr == nil)...)
	for _, complaint := range configComplaints(cfg) {
		log.Warn(complaint)
	}
	// The first scheduler this app has ever had says so where a `fly logs`
	// tail can see it; an unscheduled night logs nothing, because sample
	// runs are asked for rather than waited on.
	if nightSet.Scheduled {
		log.Info("the coliseum runs at night", "window", nightSet.Window.String(),
			"zone", nightSet.Zone.String(), "bouts", nightSet.Bouts,
			"per_account", nightSet.BoutsPerAccount, "games", nightSet.Games)
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
		// Read once here, like every other setting (ADR 39): the routes are
		// handed where the calls go rather than looking it up per request.
		Forge: forge, Claude: claude.SettingsFromEnv(),
		// The Coliseum at Night (ADR 46), already resolved above; the door
		// starts the runner and stops it with its own Close.
		Night:  nightSet,
		Logger: log,
	})
	if err != nil {
		return err
	}
	defer func() { _ = d.Close() }()

	// The listener, where the listener has always been: after the door, so a
	// boot that dies below never opened a port at all.
	listener, err := listen()
	if err != nil {
		return err
	}
	// The address the kernel actually gave, not the one that was asked for —
	// with port 0 those differ, and the one worth printing is the real one.
	addr := listener.Addr().String()
	server := &http.Server{
		Addr:              addr,
		Handler:           d.Handler(),
		ReadHeaderTimeout: 30 * time.Second,
		IdleTimeout:       120 * time.Second,
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

	// Whichever comes first, and the signal channel above may already be
	// holding one from during the boot — which is the point of arming it up
	// there rather than here.
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

	ctx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
	defer cancel()
	// **A stop that ran out of grace says so.** This used to be discarded, so
	// a process that gave up on connections still in flight logged exactly
	// what a process that drained cleanly logged — nothing — and the one
	// question a deploy actually wants answered ("did it go quietly?") had no
	// answer anywhere. It stays a warning rather than a returned error: the
	// process is going down either way, and turning an ordinary drain
	// timeout into a non-zero exit would make every crowded deploy read as a
	// crash.
	if err := server.Shutdown(ctx); err != nil {
		log.Warn("the stop ran out of grace with requests still in flight",
			"grace", shutdownGrace, "error", err)
	}
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
