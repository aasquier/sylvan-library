package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/aasquier/sylvan-library/go/internal/api"
	"github.com/aasquier/sylvan-library/go/internal/auth"
	"github.com/aasquier/sylvan-library/go/internal/auth/authtest"
	"github.com/aasquier/sylvan-library/go/internal/prices"

	_ "modernc.org/sqlite"
)

// `mtglab claude usage`, the spend ledger's shell door.
//
// It exists because the Admin panel is the only other reader and a panel needs
// a browser and a session -- so `fly ssh console -C "mtglab claude usage"` is
// the only way the instance's own bill can be read from a terminal. Which
// makes the last test in this file the important one: the two surfaces answer
// the same question about the same rows, and if they ever disagree, the one
// nobody is looking at is the one that will be believed.

// spend is one recorded conversation, with the day it happened on.
type spend struct {
	day, mode, model             string
	requests, in, out, cacheRead int
}

// withLedger writes an app.db carrying these conversations, at
// `<data>/app.db`, which is where [config.Config.AppDBPath] looks.
//
// Inserted directly rather than through `ledger.Record`, which stamps the
// clock: every question here is about which window a conversation falls in, so
// the day it happened on has to be an argument.
func (d deployment) withLedger(t *testing.T, rows ...spend) deployment {
	t.Helper()
	path := d.AppDBPath()
	if err := authtest.NewScratchDB(path); err != nil {
		t.Fatalf("seeding a scratch app.db: %v", err)
	}
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	for _, row := range rows {
		if _, err := db.Exec(
			"INSERT INTO claude_usage (created_at, mode, model, stop_reason,"+
				" requests, input_tokens, output_tokens, cache_read_tokens)"+
				" VALUES (?, ?, ?, 'end_turn', ?, ?, ?, ?)",
			row.day+"T09:00:00.000000+00:00", row.mode, row.model,
			row.requests, row.in, row.out, row.cacheRead); err != nil {
			t.Fatalf("seeding %+v: %v", row, err)
		}
	}
	return d
}

// straddle is a model whose rate is known to move and the two days either side
// of the move, read off the price table rather than typed here.
func straddle(t *testing.T) (model, before, after string) {
	t.Helper()
	for name, priced := range prices.Table {
		if priced.Then == nil || priced.Until == "" {
			continue
		}
		day, err := time.Parse("2006-01-02", priced.Until)
		if err != nil {
			t.Fatalf("%s: Until %q is not a date", name, priced.Until)
		}
		return name, priced.Until, day.AddDate(0, 0, 1).Format("2006-01-02")
	}
	t.Skip("no rate in the table is scheduled to move")
	return "", "", ""
}

// A box that has never asked Claude anything says so, and says where it
// looked. It does **not** mint the database on the way past: a reporting verb
// that creates an accounts file by being asked a question is one nobody should
// trust with the answer.
func TestClaudeUsageOnABoxWithNoLedgerSaysSoAndCreatesNothing(t *testing.T) {
	t.Parallel()
	d := scratchDeployment(t)
	out, err := d.run(t, "claude", "usage")
	if err != nil {
		t.Fatalf("usage on a bare box: %v", err)
	}
	if !strings.Contains(out, "no ledger at") || !strings.Contains(out, d.AppDBPath()) {
		t.Errorf("a bare box answered:\n%s", out)
	}
	if _, err := auth.Open(d.AppDBPath()); err == nil {
		if db, _ := auth.Open(d.AppDBPath()); db != nil {
			var n int
			if err := db.QueryRow("SELECT count(*) FROM claude_usage").Scan(&n); err == nil {
				t.Error("reading the spend created an app.db")
			}
			_ = db.Close()
		}
	}
}

// Both axes, because they answer different questions -- and dollars against
// the models only, since a mode is spread across every model that served it
// and a figure in that column would look like arithmetic.
func TestClaudeUsageRollsUpBothAxesWithDollarsAgainstTheModels(t *testing.T) {
	t.Parallel()
	model, _, after := straddle(t)
	d := scratchDeployment(t).withLedger(t,
		spend{after, "commander-dossier", model, 4, 900_000, 40_000, 1_000_000},
		spend{after, "rationale-interview", model, 1, 3_000, 200, 0})

	out, err := d.run(t, "claude", "usage")
	if err != nil {
		t.Fatalf("usage: %v", err)
	}
	for _, want := range []string{
		"mode", "model", "commander-dossier", "rationale-interview", model,
		"total", "floor", "rates", prices.Checked,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the report never says %q:\n%s", want, out)
		}
	}
	// The mode rows carry no money, the model row does. Both facts read off
	// the same page, so a column that drifted into the wrong table fails here.
	modeLine := lineWith(t, out, "commander-dossier")
	if strings.Contains(modeLine, "$") {
		t.Errorf("a mode row carries a dollar figure, which it cannot honestly "+
			"know: %q", modeLine)
	}
	modelLine := lineWith(t, out, model)
	if !strings.Contains(modelLine, "$") {
		t.Errorf("the model row carries no dollar figure: %q", modelLine)
	}
	// And the two axes agree about the tokens, because they are the same rows
	// counted twice.
	if modeTotal, modelTotal := sumColumn(t, out, "mode"), sumColumn(t, out, "model"); modeTotal != modelTotal {
		t.Errorf("the mode roll-up counts %d input tokens and the model roll-up %d",
			modeTotal, modelTotal)
	}
}

// A window that crosses a rate change is priced on both sides of it, so the
// figure is what was spent rather than what the same traffic would cost this
// morning. Two identical conversations, one either side of the change: the
// total must be less than twice the dearer one.
func TestClaudeUsagePricesEachSideOfARateChangeAtItsOwnRate(t *testing.T) {
	t.Parallel()
	model, before, after := straddle(t)
	one := spend{after, "research", model, 2, 1_000_000, 100_000, 0}
	older := one
	older.day = before
	d := scratchDeployment(t).withLedger(t, one, older)

	out, err := d.run(t, "claude", "usage")
	if err != nil {
		t.Fatalf("usage: %v", err)
	}
	got := money(t, lineWith(t, out, model))

	row := prices.Row{Model: model, Conversations: 1, InputTokens: int64(one.in),
		OutputTokens: int64(one.out), CacheRead: int64(one.cacheRead)}
	honest := prices.Sum([]prices.Estimate{
		prices.Over([]prices.Row{row}, before),
		prices.Over([]prices.Row{row}, after),
	})
	flat := prices.Over([]prices.Row{row, row}, after)
	if honest.USD == flat.USD {
		t.Fatalf("both rates price this spend at %v, so nothing is being asked", flat.USD)
	}
	if want := round4(honest.USD); got != want {
		t.Errorf("the report says $%.4f; the two days at their own rates come "+
			"to $%.4f (all of it at the later rate would be $%.4f)",
			got, want, round4(flat.USD))
	}
}

// A model the table cannot price is named, not charged at nothing. This is the
// line the Admin panel sends people here for -- it counts the unpriced
// conversations and will not render the id itself, because a model id is not
// something the site shows anybody.
func TestClaudeUsageNamesAModelItCannotPrice(t *testing.T) {
	t.Parallel()
	d := scratchDeployment(t).withLedger(t,
		spend{"2026-09-02", "research", "claude-from-the-future", 1, 10_000, 500, 0})

	out, err := d.run(t, "claude", "usage")
	if err != nil {
		t.Fatalf("usage: %v", err)
	}
	if !strings.Contains(out, "unpriced") {
		t.Errorf("an unpriced conversation went unmentioned:\n%s", out)
	}
	// Named in the unpriced list, not merely present in the table -- the id is
	// on its own line under the count. Asserting `Contains` alone would pass
	// on the table row that is there anyway, which is how a test can watch the
	// one thing it exists for be deleted and say nothing.
	listed := false
	for _, line := range strings.Split(out, "\n") {
		if fields := strings.Fields(line); len(fields) == 1 &&
			fields[0] == "claude-from-the-future" {
			listed = true
		}
	}
	if !listed {
		t.Errorf("the unpriced model is never listed under the count, which is "+
			"the one thing the Admin panel sends people here for:\n%s", out)
	}
}

// `--since` narrows the window, and a `since` the ledger could only compare as
// nonsense is refused rather than answered.
//
// The comparison is textual against an ISO-8601 stamp, which is what makes it
// cheap and what makes a typo silent: "1 Sep" sorts after every timestamp ever
// written, so it would answer "nothing recorded" with a straight face.
func TestClaudeUsageNarrowsOnSinceAndRefusesOneItCannotCompare(t *testing.T) {
	t.Parallel()
	model, before, after := straddle(t)
	d := scratchDeployment(t).withLedger(t,
		spend{before, "research", model, 1, 111, 1, 0},
		spend{after, "commander-dossier", model, 1, 222, 2, 0})

	whole, err := d.run(t, "claude", "usage")
	if err != nil {
		t.Fatalf("usage: %v", err)
	}
	if !strings.Contains(whole, "research") || !strings.Contains(whole, "commander-dossier") {
		t.Errorf("the unbounded window is missing a mode:\n%s", whole)
	}
	narrow, err := d.run(t, "claude", "usage", "--since", after)
	if err != nil {
		t.Fatalf("usage --since: %v", err)
	}
	if strings.Contains(narrow, "research") {
		t.Errorf("--since %s kept a conversation from %s:\n%s", after, before, narrow)
	}
	if !strings.Contains(narrow, "commander-dossier") {
		t.Errorf("--since %s dropped a conversation from that very day:\n%s", after, narrow)
	}

	for _, bad := range []string{
		"1 Sep", "yesterday", "2026-13-01", "2026/09/01",
		// And the RFC3339 spelling, refused on purpose: the ledger writes its
		// offset as `+00:00` and `Z` sorts after the microseconds' dot, so this
		// would quietly drop the first moments of the day it names.
		"2026-09-01T00:00:00Z",
	} {
		if _, err := d.run(t, "claude", "usage", "--since", bad); err == nil {
			t.Errorf("--since %q was accepted", bad)
		}
	}
}

// **The balancing test.** The shell and the Admin panel roll up the same rows
// for the same person, and they must not be able to disagree.
//
// They are one code path (`ledger.Window`) and this is what holds them to it:
// the same app.db, both surfaces driven, every token and every dollar
// compared. A second implementation of this arithmetic would pass its own
// tests and fail this one, which is the point -- the surface nobody is looking
// at is the one that would go quietly wrong.
func TestTheShellAndTheAdminPanelAgreeOnTheSameRows(t *testing.T) {
	t.Parallel()
	model, before, after := straddle(t)
	d := scratchDeployment(t).withLedger(t,
		spend{before, "commander-dossier", model, 4, 900_000, 40_000, 1_000_000},
		spend{after, "commander-dossier", model, 2, 15_000, 900, 30_000},
		spend{after, "rationale-interview", model, 1, 3_000, 200, 0},
		spend{after, "research", "claude-from-the-future", 1, 10_000, 500, 0})

	shell, err := d.run(t, "claude", "usage")
	if err != nil {
		t.Fatalf("usage: %v", err)
	}
	panel := adminClaudeStats(t, d.AppDBPath())

	// The all-time window is the one the shell answers with no `--since`.
	all, ok := panel["windows"].(map[string]any)["all"].(map[string]any)
	if !ok {
		t.Fatalf("the panel's all-time window is missing: %v", panel)
	}
	spendPane, ok := all["estimated_usd"].(map[string]any)
	if !ok {
		t.Fatalf("the panel carries no estimate: %v", all)
	}

	// The dollars, to the panel's own four places.
	panelUSD, _ := spendPane["usd"].(float64)
	if got := money(t, lineWith(t, shell, "total")); got != round4(panelUSD) {
		t.Errorf("the shell totals $%.4f and the panel $%.4f", got, round4(panelUSD))
	}
	// The unpriced count and the model behind it.
	unpriced, _ := spendPane["unpriced"].(float64)
	if unpriced == 0 {
		t.Fatal("this fixture was meant to carry an unpriced conversation")
	}
	if !strings.Contains(shell, "claude-from-the-future") {
		t.Errorf("the panel counts %v unpriced and the shell does not name the "+
			"model:\n%s", unpriced, shell)
	}

	// And every row, on both axes: the panel's numbers must appear on the
	// shell's own line for that mode or model.
	for _, axis := range []string{"by_mode", "by_model"} {
		rows, _ := all[axis].([]any)
		if len(rows) == 0 {
			t.Fatalf("the panel's %s is empty", axis)
		}
		for _, raw := range rows {
			row, _ := raw.(map[string]any)
			name, _ := row[strings.TrimPrefix(axis, "by_")].(string)
			line := lineWith(t, shell, name)
			for _, field := range []string{"conversations", "requests",
				"input_tokens", "output_tokens", "cache_read_tokens"} {
				n, _ := row[field].(float64)
				if !hasNumber(line, int64(n)) {
					t.Errorf("%s %s: the panel says %s=%d and the shell's line "+
						"is %q", axis, name, field, int64(n), line)
				}
			}
		}
	}
}

// adminClaudeStats drives `GET /api/admin/stats/claude` against one app.db,
// through the served route table rather than by calling the handler -- so the
// bytes compared are the bytes the browser gets.
func adminClaudeStats(t *testing.T, appDBPath string) map[string]any {
	t.Helper()
	a := api.New(api.Config{AppDBPath: appDBPath})
	var handler http.HandlerFunc
	for _, route := range a.Routes() {
		if route.Method == http.MethodGet && route.Pattern == "/api/admin/stats/claude" {
			handler = route.Handler
		}
	}
	if handler == nil {
		t.Fatal("the app no longer serves GET /api/admin/stats/claude")
	}
	req := httptest.NewRequest(http.MethodGet, "/api/admin/stats/claude", nil)
	req = req.WithContext(auth.WithScope(context.Background(),
		auth.Scope{UserID: 1, Username: "alice", IsAdmin: true, Authenticated: true}))
	rec := httptest.NewRecorder()
	handler(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("the panel answered %d: %s", rec.Code, rec.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("the panel answered non-JSON: %v", err)
	}
	return payload
}

// lineWith is the one line of a report mentioning `needle`, or a failure
// naming what it looked through -- a report that lost a row should say which.
func lineWith(t *testing.T, report, needle string) string {
	t.Helper()
	for _, line := range strings.Split(report, "\n") {
		if strings.Contains(line, needle) {
			return line
		}
	}
	t.Fatalf("no line mentions %q:\n%s", needle, report)
	return ""
}

// money reads the dollar figure off a line.
func money(t *testing.T, line string) float64 {
	t.Helper()
	m := regexp.MustCompile(`\$([0-9]+\.[0-9]+)`).FindStringSubmatch(line)
	if m == nil {
		t.Fatalf("no dollar figure on %q", line)
	}
	usd, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		t.Fatalf("%q: %v", line, err)
	}
	return usd
}

// sumColumn totals the input-token column of one table in the report, so the
// two axes can be held to each other. The tables are told apart by their own
// header line, which is what a reader does.
func sumColumn(t *testing.T, report, header string) int64 {
	t.Helper()
	var total int64
	inTable := false
	for _, line := range strings.Split(report, "\n") {
		fields := strings.Fields(line)
		switch {
		case len(fields) == 0:
			inTable = false
		case fields[0] == header:
			inTable = true
		case inTable && len(fields) >= 5:
			// mode/model, conv, requests, in, out, cached: the fourth column.
			n, err := strconv.ParseInt(fields[3], 10, 64)
			if err != nil {
				t.Fatalf("%q: column four is not a number: %v", line, err)
			}
			total += n
		}
	}
	if total == 0 {
		t.Fatalf("the %s table has no rows in it:\n%s", header, report)
	}
	return total
}

// hasNumber reports whether a line carries `n` as its own field, so 1 does not
// match 1000 and a token count is not found inside a dollar figure.
func hasNumber(line string, n int64) bool {
	want := strconv.FormatInt(n, 10)
	for _, field := range strings.Fields(line) {
		if field == want {
			return true
		}
	}
	return false
}

// round4 is the panel's own rounding, so the two are compared at the precision
// the wire actually carries rather than at float equality.
func round4(usd float64) float64 {
	rounded, err := strconv.ParseFloat(strconv.FormatFloat(usd, 'f', 4, 64), 64)
	if err != nil {
		return usd
	}
	return rounded
}
