package api

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// The scan route (ADR 34). `internal/claude`'s corpus holds the reader itself
// -- the media types, base64's strictness, the sighting -- so what these prove
// is the layer above it: which refusal lands on which status and BEFORE any
// job, that the dedupe is the picture, that the result is the recorded four
// keys in the recorded order, and that what comes back is not a card.

const scanAt = "/api/claude/scan"

// aCapture is a small valid PNG-ish blob, base64 as the browser sends it.
func aCapture(seed string) string {
	return base64.StdEncoding.EncodeToString(append([]byte("\x89PNG\r\n\x1a\n"), seed...))
}

func scanBody(fields string) string { return "{" + fields + "}" }

// ---- what is refused, and in what order ---------------------------------

// Everything refusable is refused in the request, each with its own status.
// Carried into a worker they would arrive as a job in state `error` -- one
// string for three cases and a status code for none.
func TestTheScanRouteRefusesWhatItCanBeforeAnyJob(t *testing.T) {
	noCredential(t)
	rig := newJobRig(t)
	defer rig.close()
	good := aCapture("x")
	for _, row := range []struct {
		body   string
		status int
		detail string
	}{
		{`{}`, 422, "the capture was empty"},
		{`{"image":null}`, 422, "the capture was empty"},
		{`{"image":""}`, 422, "the capture was empty"},
		{`{"image":"!!!!"}`, 422, "the capture was not valid base64"},
		// The newline is the case a straight Go base64 decoder accepts and
		// the recorded decoder refuses -- a client that wrapped its base64
		// at 76 columns.
		{fmt.Sprintf(`{"image":%q}`, good[:8]+"\n"+good[8:]), 422, "the capture was not valid base64"},
		{fmt.Sprintf(`{"image":%q,"media_type":"image/tiff"}`, good), 422,
			"'image/tiff' is not an image this reads"},
		// `str()` and not a cast: an int media type refuses by name.
		{fmt.Sprintf(`{"image":%q,"media_type":7}`, good), 422, "'7' is not an image this reads"},
		// ...and a list renders as its repr, which repr-quotes with DOUBLE
		// quotes because the content already holds single ones.
		{fmt.Sprintf(`{"image":%q,"media_type":["a"]}`, good), 422, `"['a']" is not an image this reads`},
		{fmt.Sprintf(`{"image":%q,"stance":"garbage"}`, good), 422, "is not a stance preset"},
		// And the key, last of the four.
		{fmt.Sprintf(`{"image":%q}`, good), 503, "no ANTHROPIC_API_KEY"},
	} {
		status, payload, raw := callAs(t, rig.api, alice, "POST", scanAt, row.body)
		if status != row.status {
			t.Errorf("%.60s answered %d, want %d: %s", row.body, status, row.status, raw)
			continue
		}
		if detail, _ := payload["detail"].(string); !strings.Contains(detail, row.detail) {
			t.Errorf("%.60s said %q, want it to contain %q", row.body, detail, row.detail)
		}
	}
	if got := rig.jobs.All(alice.UserID); len(got) != 0 {
		t.Errorf("%d jobs were queued by refused requests", len(got))
	}
}

// The capture is checked **before** the stance and before the key, so a
// photograph nobody could read never reports itself as a missing credential.
func TestTheCaptureIsRefusedBeforeTheStanceAndTheKey(t *testing.T) {
	noCredential(t)
	rig := newJobRig(t)
	defer rig.close()
	// All three wrong at once: the capture wins.
	status, payload, raw := callAs(t, rig.api, alice, "POST", scanAt,
		`{"image":"!!!!","media_type":"image/tiff","stance":"garbage"}`)
	if status != 422 {
		t.Fatalf("%d %s", status, raw)
	}
	// The media type is checked before the image inside `_payload`, so of the
	// two capture refusals it is the media type that answers.
	if detail, _ := payload["detail"].(string); !strings.Contains(detail, "image/tiff") {
		t.Errorf("said %q, want the media type -- it is checked first", detail)
	}
	// A good capture with a bad stance is the stance's 422, not the key's 503.
	status, payload, _ = callAs(t, rig.api, alice, "POST", scanAt,
		fmt.Sprintf(`{"image":%q,"stance":"garbage"}`, aCapture("x")))
	if status != 422 {
		t.Errorf("a bad stance answered %d, want 422 before the key's 503", status)
	}
	if detail, _ := payload["detail"].(string); strings.Contains(detail, "ANTHROPIC") {
		t.Errorf("the key was checked before the stance: %q", detail)
	}
}

// A capture that is neither a string nor bytes is a **422 like every other bad
// capture**, and it was an uncaught 500 until 2026-08-23.
//
// The capture read once took whatever it was handed, so a list or an
// object crashed out of the route uncaught. It is the theme
// proposal's budget wart in a second file, found on
// the day that one was ruled and ruled with it. This test is the wart's own,
// kept and inverted.
func TestACaptureThatIsNotTextIsA422(t *testing.T) {
	noCredential(t)
	rig := newJobRig(t)
	defer rig.close()
	for _, image := range []string{`[1,2,3]`, `{"a":1}`, `7`, `7.5`, `true`} {
		status, payload, raw := callAs(t, rig.api, alice, "POST", scanAt,
			scanBody(`"image":`+image))
		if status != 422 {
			t.Errorf("image %s answered %d, want 422: %s", image, status, raw)
			continue
		}
		if detail, _ := payload["detail"].(string); detail != "the capture must be a base64 string" {
			t.Errorf("image %s said %q", image, detail)
		}
	}
	// `payload.get("image") or b""` means a FALSY value never reaches the type
	// branch, so those keep their own sentence -- the fix did not flatten
	// three cases into one.
	for _, image := range []string{`null`, `""`, `0`, `false`, `[]`} {
		status, payload, raw := callAs(t, rig.api, alice, "POST", scanAt,
			scanBody(`"image":`+image))
		if status != 422 {
			t.Errorf("image %s answered %d, want 422: %s", image, status, raw)
			continue
		}
		if detail, _ := payload["detail"].(string); detail != "the capture was empty" {
			t.Errorf("falsy image %s said %q, want the empty-capture sentence", image, detail)
		}
	}
	if got := rig.jobs.All(alice.UserID); len(got) != 0 {
		t.Errorf("%d jobs were queued by refused requests", len(got))
	}
}

// ---- what a run answers with --------------------------------------------

// A whole scan: the mode transcribes, `identify` decides, and the job's result
// carries the recorded four keys in the recorded order.
//
// The card is `Black Lotus`, which `tiny_pool` holds -- so this also proves the half
// that matters most: the transcription is looked up rather than believed.
func TestAScanTranscribesAndThePoolNamesTheCard(t *testing.T) {
	rig := newJobRig(t)
	defer rig.close()
	script := &scriptedClaude{replies: []string{
		answer("end_turn", said(`{"title":"  Black Lotus  ","corner":""}`)),
	}}
	script.start(t)

	status, payload, raw := callAs(t, rig.api, alice, "POST", scanAt,
		scanBody(fmt.Sprintf(`"image":%q`, aCapture("lotus"))))
	if status != 200 {
		t.Fatalf("%d %s", status, raw)
	}
	id, _ := payload["id"].(string)
	done, doneRaw := rig.await(t, id)
	if done["status"] != "done" {
		t.Fatalf("job %s: %v", done["status"], doneRaw)
	}
	result, _ := done["result"].(map[string]any)

	// `transcribed` is what was read, stripped -- and it is the whole of what
	// the model contributed.
	transcribed, _ := result["transcribed"].(map[string]any)
	if transcribed["title"] != "Black Lotus" {
		t.Errorf("transcribed %v, want the stripped title", transcribed)
	}
	// An unreadable corner is an ABSENT key, not an empty one.
	if _, present := transcribed["corner"]; present {
		t.Errorf("an empty corner was carried as a key: %v", transcribed)
	}
	// And the reading is the pool's, not the model's: a title only ever
	// OFFERS, so `via` is "title" and there is no resolved card.
	reading, ok := result["reading"].(map[string]any)
	if !ok {
		t.Fatalf("no reading: %v", result)
	}
	if reading["via"] != "title" {
		t.Errorf("via is %v, want title -- a title offers and never resolves", reading["via"])
	}
	if reading["resolved"] != nil {
		t.Errorf("a title resolved a card: %v", reading["resolved"])
	}
	candidates, _ := reading["candidates"].([]any)
	if len(candidates) == 0 {
		t.Fatal("the shortlist is empty; the pool never saw the transcription")
	}
	first, _ := candidates[0].(map[string]any)
	if first["name"] != "Black Lotus" {
		t.Errorf("the shortlist leads with %v, want Black Lotus", first["name"])
	}

	// **The four keys, in the recorded order.** `reading, transcribed,
	// refused,
	// model` -- a map would alphabetise them, which is the rule written
	// down after the Notes tab shipped alphabetised for seven versions.
	if got := resultKeys(t, doneRaw); got != "reading,transcribed,refused,model" {
		t.Errorf("result keys are %s, the record says reading,transcribed,refused,model", got)
	}
}

// Nothing legible is a null reading rather than a reading of nothing: an
// empty sighting hands the identifier an empty list, so no lookup
// happens at all.
func TestAnUnreadableCaptureIsANullReading(t *testing.T) {
	rig := newJobRig(t)
	defer rig.close()
	script := &scriptedClaude{replies: []string{
		answer("end_turn", said(`{"title":"","corner":"   "}`)),
	}}
	script.start(t)
	status, payload, raw := callAs(t, rig.api, alice, "POST", scanAt,
		scanBody(fmt.Sprintf(`"image":%q`, aCapture("blur"))))
	if status != 200 {
		t.Fatalf("%d %s", status, raw)
	}
	done, doneRaw := rig.await(t, payload["id"].(string))
	result, _ := done["result"].(map[string]any)
	if result["reading"] != nil {
		t.Errorf("reading is %v, want null", result["reading"])
	}
	transcribed, _ := result["transcribed"].(map[string]any)
	if len(transcribed) != 0 {
		t.Errorf("transcribed is %v, want {}", transcribed)
	}
	if got := resultKeys(t, doneRaw); got != "reading,transcribed,refused,model" {
		t.Errorf("result keys are %s", got)
	}
}

// **The dedupe is the picture.** Two presses on one shot are one paid call;
// a different photograph is different work.
func TestTwoPressesOnOnePhotographAreOneJob(t *testing.T) {
	rig := newJobRig(t)
	defer rig.close()
	hold := make(chan struct{})
	script := &scriptedClaude{
		replies: []string{
			answer("end_turn", said(`{"title":"Black Lotus","corner":""}`)),
			answer("end_turn", said(`{"title":"Black Lotus","corner":""}`)),
		},
		hold: hold,
	}
	script.start(t)

	same := aCapture("one-shot")
	first := submitScan(t, rig, same)
	second := submitScan(t, rig, same)
	if first != second {
		t.Errorf("two presses made two jobs (%s, %s)", first, second)
	}
	// A different photograph is its own job, so this is not passing because
	// the key is constant.
	other := submitScan(t, rig, aCapture("another-shot"))
	if other == first {
		t.Errorf("a different photograph joined the first job (%s)", other)
	}
	close(hold)
	rig.jobs.Wait()
}

// The key is the **re-encoded** capture, so two spellings of one photograph
// are one job. `YW==` and `YQ==` decode to the same byte and the digest is
// over the re-encoding, never what arrived.
func TestTwoSpellingsOfOnePhotographAreOneJob(t *testing.T) {
	rig := newJobRig(t)
	defer rig.close()
	hold := make(chan struct{})
	script := &scriptedClaude{
		replies: []string{answer("end_turn", said(`{"title":"x","corner":""}`))},
		hold:    hold,
	}
	script.start(t)
	if a, b := submitScan(t, rig, "YW=="), submitScan(t, rig, "YQ=="); a != b {
		t.Errorf("the same byte under two spellings made two jobs (%s, %s)", a, b)
	}
	close(hold)
	rig.jobs.Wait()
}

// A scan waits on the NET lane, not behind a Tier 1 sweep: it is a socket wait
// with a picture on it, and queueing it behind the simulator's single CPU
// worker would be minutes of stall for nothing.
func TestAScanWaitsOnTheNetworkLane(t *testing.T) {
	rig := newJobRig(t)
	defer rig.close()
	script := &scriptedClaude{replies: []string{
		answer("end_turn", said(`{"title":"Black Lotus","corner":""}`)),
	}}
	script.start(t)
	id := submitScan(t, rig, aCapture("lane"))
	rig.jobs.Wait()
	found := false
	for _, j := range rig.jobs.All(alice.UserID) {
		if j.ID == id {
			found = true
			if j.Kind != ScanKind {
				t.Errorf("kind is %q, want %q", j.Kind, ScanKind)
			}
			if j.Label != scanLabel {
				t.Errorf("label is %q, want %q", j.Label, scanLabel)
			}
		}
	}
	if !found {
		t.Errorf("the scan job is not in the owner's list")
	}
}

// A call that comes back unusable is a job in state `error`, which is where it
// belongs once the response has already been sent -- and the message is the
// explanation, never a stack trace and never with a mode name prefixed onto
// it.
func TestAFailedScanCallIsAFailedJob(t *testing.T) {
	rig := newJobRig(t)
	defer rig.close()
	// 401 and not 500: the SDK retries a 500 five times, and what this
	// asserts is what a failed call leaves in the job, not the retry policy.
	script := &scriptedClaude{replies: []string{"!401"}}
	script.start(t)
	id := submitScan(t, rig, aCapture("boom"))
	done, raw := rig.await(t, id)
	if done["status"] != "error" {
		t.Fatalf("job is %v, want error: %s", done["status"], raw)
	}
	message, _ := done["error"].(string)
	if message == "" {
		t.Fatal("a failed job with no error message")
	}
	if strings.HasPrefix(message, "scan:") || strings.HasPrefix(message, "scan ") {
		t.Errorf("the error carries a mode-name prefix the record never writes: %q", message)
	}
}

func submitScan(t *testing.T, rig *jobRig, image string) string {
	t.Helper()
	status, payload, raw := callAs(t, rig.api, alice, "POST", scanAt,
		scanBody(fmt.Sprintf(`"image":%q`, image)))
	if status != 200 {
		t.Fatalf("submitting a scan answered %d: %s", status, raw)
	}
	id, _ := payload["id"].(string)
	if id == "" {
		t.Fatalf("no job id: %s", raw)
	}
	return id
}

// resultKeys reads the job payload's `result` keys in wire order, which a
// decoded map has already lost.
func resultKeys(t *testing.T, raw []byte) string {
	t.Helper()
	var envelope struct {
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatal(err)
	}
	dec := json.NewDecoder(strings.NewReader(string(envelope.Result)))
	if _, err := dec.Token(); err != nil {
		t.Fatal(err)
	}
	var keys []string
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			t.Fatal(err)
		}
		keys = append(keys, tok.(string))
		var skip json.RawMessage
		if err := dec.Decode(&skip); err != nil {
			t.Fatal(err)
		}
	}
	return strings.Join(keys, ",")
}
