package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// The configuration record is the pair of checked-in files that claim to
// describe how this binary is driven: `.env.example` names every switch the
// environment sets, and `.claude/launch.json` names the commands and flags a
// session actually starts. Both are prose about code, and prose about code
// rots at exactly the rate the code moves.
//
// `.env.example` had rotted by twenty names when this was written -- every
// `MTGLAB_FORGE_*`, the two Claude dials, both served-file paths -- while
// CLAUDE.md went on saying "`.env.example` documents the names". That sentence
// was true when somebody wrote it and had been false for a long time, and
// nothing anywhere could tell the difference. **The fix for a claim like that
// is never to reword it.**
//
// Two conservatisms, so this guard cannot cry wolf. Only a name inside its own
// quotes counts -- `os.Getenv("MTGLAB_X")` and `const e = "MTGLAB_X"` are
// switches, a name mentioned inside a longer sentence is prose. And only the
// four prefixes this project actually owns are scanned, so a variable belonging
// to the toolchain or the shell never lands in the list.

// envName is a switch this project reads: quoted, whole, and one of ours.
var envName = regexp.MustCompile(`"((?:MTGLAB|ANTHROPIC|RESEND|FLY)_[A-Z0-9_]+)"`)

// documentedName is an *entry* in `.env.example`: a name at the head of a
// line, commented or not, with the `=` that makes it a line somebody can
// uncomment and fill in.
//
// The `=` is load-bearing and was bought by this test failing to fire. Without
// it, `# MTGLAB_JAVA beats the JDK unpacked beside...` -- a sentence of the
// surrounding prose -- reads as documentation, so deleting the entry itself
// left the guard green. A name explained in a paragraph and missing from the
// list is exactly the shape of "documented" that helps nobody: the list is
// what an operator copies.
var documentedName = regexp.MustCompile(`(?m)^#? *((?:MTGLAB|ANTHROPIC|RESEND|FLY)_[A-Z0-9_]+)=`)

// switchesInGoSource returns the switch names read by code that ships and the
// ones read only by tests, separately: a test-only switch is not compiled into
// the binary at all, which is a stronger guarantee than any comment asking
// nicely that it not be set in production.
func switchesInGoSource(t *testing.T, root string) (shipped, testOnly map[string]string) {
	t.Helper()
	shipped, testOnly = map[string]string{}, map[string]string{}
	err := filepath.WalkDir(filepath.Join(root, "go"), func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		body, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		into := shipped
		if strings.HasSuffix(path, "_test.go") {
			into = testOnly
		}
		for _, m := range envName.FindAllStringSubmatch(string(body), -1) {
			if _, seen := into[m[1]]; !seen {
				into[m[1]] = rel
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// Anti-vacuity: a walk that matched nothing reads exactly like a tree with
	// nothing wrong in it, and a moved root is the likelier cause.
	if _, ok := shipped["MTGLAB_DATA_DIR"]; !ok {
		t.Fatalf("the walk from %s found no MTGLAB_DATA_DIR; it found %d names", root, len(shipped))
	}
	return shipped, testOnly
}

func documentedSwitches(t *testing.T, root string) map[string]bool {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(root, ".env.example"))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]bool{}
	for _, m := range documentedName.FindAllStringSubmatch(string(body), -1) {
		out[m[1]] = true
	}
	if len(out) == 0 {
		t.Fatal(".env.example documents no names at all; the extractor has stopped matching")
	}
	return out
}

func TestEveryEnvironmentSwitchIsDocumentedAndEveryDocumentedOneIsRead(t *testing.T) {
	t.Parallel()
	root := repoRoot(t)
	shipped, _ := switchesInGoSource(t, root)
	documented := documentedSwitches(t, root)

	for name, where := range shipped {
		if !documented[name] {
			t.Errorf("%s reads %s and .env.example does not name it -- "+
				"a switch nobody wrote down is a switch discovered in production", where, name)
		}
	}
	for name := range documented {
		if _, read := shipped[name]; !read {
			t.Errorf(".env.example documents %s and no shipping code reads it -- "+
				"dead config to delete, not a mystery to preserve", name)
		}
	}
}

// testSwitch is a name whose whole purpose is to change behaviour under test:
// the fake pool, the live-API opt-ins, the flag fixture.
var testSwitch = regexp.MustCompile(`^MTGLAB_(TEST|LIVE)_`)

func TestATestSwitchIsNotReachableInProduction(t *testing.T) {
	t.Parallel()
	root := repoRoot(t)
	shipped, testOnly := switchesInGoSource(t, root)

	for name, where := range shipped {
		if testSwitch.MatchString(name) {
			t.Errorf("%s reads %s from shipping code: a test switch that compiles "+
				"into the binary can be set on the instance", where, name)
		}
	}
	documented := documentedSwitches(t, root)
	for name := range documented {
		if testSwitch.MatchString(name) {
			t.Errorf(".env.example documents %s, which invites setting a test "+
				"switch on a real deployment", name)
		}
	}
	// Anti-vacuity again, and it is the load-bearing half here: the two
	// assertions above pass trivially on a tree that has no test switches left
	// to misplace, and this suite has several.
	found := 0
	for name := range testOnly {
		if testSwitch.MatchString(name) {
			found++
		}
	}
	if found == 0 {
		t.Fatal("no MTGLAB_TEST_/MTGLAB_LIVE_ switch found in any _test.go; " +
			"the boundary this test guards may no longer exist")
	}
}

// launchArgs is one entry of `.claude/launch.json` flattened to the command
// line it runs. The file mixes two shapes -- the binary invoked directly, and
// a `bash -c` string with the environment set in front of it -- and both are
// just tokens once joined.
func launchCommandLines(t *testing.T, root string) []string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(root, ".claude", "launch.json"))
	if err != nil {
		t.Fatal(err)
	}
	var file struct {
		Configurations []struct {
			Name              string   `json:"name"`
			RuntimeExecutable string   `json:"runtimeExecutable"`
			RuntimeArgs       []string `json:"runtimeArgs"`
		} `json:"configurations"`
	}
	if err := json.Unmarshal(body, &file); err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, c := range file.Configurations {
		out = append(out, c.RuntimeExecutable+" "+strings.Join(c.RuntimeArgs, " "))
	}
	if len(out) == 0 {
		t.Fatal(".claude/launch.json declares no configurations")
	}
	return out
}

// TestTheDevServersAskThisBinaryForCommandsItHas holds `.claude/launch.json`
// against [newRoot], because that file is how commandment 16's browser walk
// starts and a stale flag there costs Aaron the minutes he is waiting to look
// at something. cobra refuses an unknown flag outright, so the failure is a
// dev server that never comes up, at exactly the wrong moment.
func TestTheDevServersAskThisBinaryForCommandsItHas(t *testing.T) {
	t.Parallel()
	root := repoRoot(t)
	checked := 0
	for _, line := range launchCommandLines(t, root) {
		fields := strings.Fields(line)
		for i, tok := range fields {
			if tok != "mtglab" && !strings.HasSuffix(tok, "/mtglab") {
				continue
			}
			rest := fields[i+1:]
			if len(rest) == 0 || strings.HasPrefix(rest[0], "-") {
				continue
			}
			verb := rest[0]
			var cmd *cobra.Command
			for _, c := range newRoot().Commands() {
				if c.Name() == verb {
					cmd = c
				}
			}
			if cmd == nil {
				t.Errorf(".claude/launch.json runs `mtglab %s`, and this binary "+
					"has no such subcommand (it has %v)", verb, sortedKeys(commandNames()))
				continue
			}
			checked++
			for _, arg := range rest[1:] {
				if !strings.HasPrefix(arg, "--") {
					continue
				}
				name, _, _ := strings.Cut(strings.TrimPrefix(arg, "--"), "=")
				if cmd.Flags().Lookup(name) == nil {
					t.Errorf(".claude/launch.json passes --%s to `mtglab %s`, "+
						"which has no such flag; cobra exits rather than ignoring it",
						name, verb)
				}
				checked++
			}
		}
	}
	// The scanner finding nothing would pass every assertion above.
	if checked < 2 {
		t.Errorf("only %d command/flag pairs found in .claude/launch.json; "+
			"the scanner has probably stopped matching", checked)
	}
}

func commandNames() map[string]bool {
	have := map[string]bool{}
	for _, c := range newRoot().Commands() {
		have[c.Name()] = true
	}
	return have
}
