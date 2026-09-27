package door

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The door built on ground that is not what it was told.
//
// Every branch here is a deployment that went slightly wrong — a bundle
// directory the process cannot read, a tarot path that turned out to be a
// file, an `app.db` that is not a database — and in each case the question is
// the same one: does the door refuse at start, or does it take the port and
// discover the problem on somebody's first request? Both answers appear
// below, and which is which is the design: what the door can serve without is
// a warning, and what it cannot is a refusal.

// A bundle directory that exists and cannot be read is not "no frontend" —
// it is a directory whose contents this process was supposed to have. The
// door refuses to stand rather than serving a shell with no assets behind it.
func TestADoorRefusesToStandOnABundleItCannotRead(t *testing.T) {
	t.Parallel()
	if os.Geteuid() == 0 {
		t.Skip("root reads a 0o000 directory, so there is no fault to build")
	}
	web := filepath.Join(t.TempDir(), "web_dist")
	if err := os.MkdirAll(web, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(web, "index.html"), []byte("shell"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(web, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(web, 0o755) })

	_, err := New(Config{WebDist: web,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if err == nil {
		t.Fatal("a door stood on a bundle directory it cannot read")
	}
	if !strings.Contains(err.Error(), "web_dist") {
		t.Fatalf("the refusal does not name what it could not read: %q", err)
	}
}

// A path that is not a directory is the same fact as a path that is not
// there, and both are a warning rather than a refusal: an instance with no
// tarot art serves everything else perfectly well, and the reading is the one
// room it cannot open.
func TestAMisnamedTarotPathIsAWarningAndNotAMount(t *testing.T) {
	t.Parallel()
	web, _ := site(t)
	notADir := filepath.Join(t.TempDir(), "tarot")
	if err := os.WriteFile(notADir, []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}
	logged := &lockedLog{}
	d, err := New(Config{WebDist: web, TarotDir: notADir,
		Logger: slog.New(slog.NewTextHandler(logged, nil))})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(logged.String(), "no tarot art directory") {
		t.Fatalf("the door mounted a file as the tarot directory:\n%s", logged.String())
	}
	srv := httptest.NewServer(d.Handler())
	t.Cleanup(srv.Close)
	// With nothing mounted there, the path is not a picture at all — it falls
	// through to the shell, the way every route the frontend owns does. What
	// matters is that no bytes came out of the file somebody pointed at.
	resp := get(t, srv, "GET", "/tarot/00-fool.webp", "")
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "not a directory") {
		t.Fatalf("the file named as the tarot directory was served as art: %q", body)
	}
	if !strings.Contains(string(body), "shell") {
		t.Fatalf("an unmounted tarot path answered %d with %q", resp.StatusCode, body)
	}
	// And the shell it *can* serve is still served, which is what makes this
	// a warning rather than a refusal.
	if resp := get(t, srv, "GET", "/", ""); resp.StatusCode != http.StatusOK {
		t.Fatalf("the shell answered %d on an instance with no tarot art", resp.StatusCode)
	}
}

// A static mount serves files under it and nothing else. The guard is not
// "does the path contain ..": it is whether the cleaned path still lands
// *inside* the directory, which is the only form that cannot be talked out of
// by an encoding. The mount's own root is the case worth naming — it resolves
// to the directory itself, which is not a file and must not be listed.
func TestAPathThatResolvesToTheMountItselfIsNotFound(t *testing.T) {
	t.Parallel()
	srv := build(t, false, nil)
	for _, path := range []string{
		"/assets/%2e",     // the directory, spelled so nothing cleans it away
		"/assets/%2e/%2e", // twice, for the same place
		"/assets/",        // the mount root, plainly
		"/assets/app.js/", // a file named as though it were a directory
	} {
		if resp := get(t, srv, "GET", path, ""); resp.StatusCode != http.StatusNotFound {
			t.Errorf("%s answered %d, want 404", path, resp.StatusCode)
		}
	}
	// The file itself still serves, so the refusals above are about the
	// shape of the path rather than about the mount being broken.
	if resp := get(t, srv, "GET", "/assets/app.js", ""); resp.StatusCode != http.StatusOK {
		t.Fatalf("the asset itself answered %d", resp.StatusCode)
	}
}

// A directory under a mount opens perfectly well and is still not a file.
// Serving it would be a listing of the bundle's insides, which is a thing
// nobody asked this door to publish.
func TestADirectoryUnderAMountIsNotAFileToServe(t *testing.T) {
	t.Parallel()
	web, tarot := site(t)
	nested := filepath.Join(web, "assets", "chunks")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "one.js"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	d, err := New(Config{WebDist: web, TarotDir: tarot,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(d.Handler())
	t.Cleanup(srv.Close)

	if resp := get(t, srv, "GET", "/assets/chunks", ""); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("a directory under the mount answered %d", resp.StatusCode)
	}
	// A real file one level down is served, so the 404 above is about the
	// directory rather than about the depth.
	if resp := get(t, srv, "GET", "/assets/chunks/one.js", ""); resp.StatusCode != http.StatusOK {
		t.Fatalf("a nested asset answered %d", resp.StatusCode)
	}
}

// `app.db` is named and is not a database. This is the one thing the door
// refuses over rather than degrading past, and the rule is worth restating:
// an *absent* file is a real state (a test, a bare library use) and both
// halves of the write side say so; a file that is there and will not open is
// a volume problem, and a write that discovered it at the insert would be a
// write that has already changed a deck file.
func TestADoorRefusesToStandOnAnAppDatabaseThatWillNotOpen(t *testing.T) {
	t.Parallel()
	web, tarot := site(t)
	rubbish := filepath.Join(t.TempDir(), "app.db")
	if err := os.WriteFile(rubbish, []byte("this is not a database"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := New(Config{WebDist: web, TarotDir: tarot, AppDB: rubbish,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}); err == nil {
		t.Fatal("a door stood over a file that is named app.db and is not one")
	}

	// And the absent case, for the contrast the paragraph above is about: no
	// file, no refusal, a line in the log, and a door that serves.
	logged := &lockedLog{}
	d, err := New(Config{WebDist: web, TarotDir: tarot,
		AppDB:  filepath.Join(t.TempDir(), "never-made.db"),
		Logger: slog.New(slog.NewTextHandler(logged, nil))})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(logged.String(), "no app.db") {
		t.Fatalf("an absent app.db went unmentioned:\n%s", logged.String())
	}
	srv := httptest.NewServer(d.Handler())
	t.Cleanup(srv.Close)
	if resp := get(t, srv, "GET", "/", ""); resp.StatusCode != http.StatusOK {
		t.Fatalf("the shell answered %d on an instance with no app.db", resp.StatusCode)
	}
}
