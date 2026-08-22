package auth

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/aasquier/sylvan-library/go/internal/config"
)

// The `EmailSender` seam and the two implementations behind it --
// `mtglab/auth/mail.py`.
//
// ADR 16 puts a transactional email provider behind an interface for one
// stated reason: **"the seam is what keeps that dependency out of the tests."**
// No test in this module sends mail, the same rule that keeps the Claude tests
// off the network, and the way that rule is kept is that nothing above this
// file knows which implementation it is holding.
//
// Two of them. `ConsoleSender` writes the message to a stream -- the
// development default, and the reason `mtglab users invite` is usable on a
// laptop with no account anywhere: the link you need is the one it prints.
// `ResendSender` makes one HTTPS POST to `api.resend.com`.
//
// **Where the address is allowed to appear.** ADR 16 is unconditional that an
// email address must never reach a log line, an artifact or a Claude tool
// result, and this is the one file that handles addresses in bulk, so the line
// is drawn here rather than assumed:
//
//   - `ConsoleSender` prints it. That is what makes it useful in development --
//     you are standing at the terminal, you need the link -- and exactly what
//     makes it wrong in production, where "the console" is the platform's log
//     aggregator. So `SenderFromEnv` **refuses to fall back to it when auth is
//     on**, which is the flag that means deployed.
//   - `ResendSender` never puts one in an error. The provider's own error
//     bodies quote the recipient back at you, which is why the failure below
//     carries the status and the error *name* and drops the body.

// ResendEndpoint is the provider's one URL.
const ResendEndpoint = "https://api.resend.com/emails"

// UserAgent is sent on every request, and it is not a courtesy -- it is the
// difference between mail working and not.
//
// `api.resend.com` is behind Cloudflare, which answered Python's default
// `Python-urllib/3.12` with **403 and error code 1010**, the "banned browser
// signature" page. Measured on the instance 2026-08-13, on the first real
// invite ever sent: the same GET was 200 from a client that set no User-Agent
// at all and 403 from the one that set that. The domain was verified and the
// key was valid the whole time.
//
// Go's `net/http` sends `Go-http-client/1.1` by default, which is a signature
// of exactly the kind that was blocked. The header is set explicitly for that
// reason and not because anything has been observed to refuse it.
const UserAgent = "mtg-lab/0.1 (personal deckbuilding tool; transactional mail)"

// MailTimeout is long enough that a slow provider is not mistaken for a hung
// one, short enough that a hung one does not hold a worker for the length of a
// coffee break. The reset endpoint sends after the response has gone, so this
// is never on the path of a response a person is waiting for.
const MailTimeout = 10 * time.Second

// ErrEmailNotConfigured is mail being required with nothing set up to send it.
var ErrEmailNotConfigured = errors.New("email not configured")

// ErrEmailNotSent is the provider refusing or being unreachable.
//
// Deliberately carries no recipient and no response body -- see the file
// comment for why. What it does carry is enough to search the provider's
// dashboard with: a status code and their own name for the error.
var ErrEmailNotSent = errors.New("email not sent")

// Message is one plain-text message.
//
// Plain text and no HTML alternative, on purpose. The entire content of both
// messages this project sends is a sentence and a link; an HTML template would
// be a second thing to keep in step, a rendering surface to test, and a reason
// for a mail client to hide the URL behind a button somebody cannot read
// before clicking.
type Message struct {
	To      string
	Subject string
	Body    string
}

// EmailSender is anything that can send a Message. The seam ADR 16 asks for.
type EmailSender interface {
	Send(message Message) error
}

// ConsoleSender prints the message instead of sending it. Development and
// tests.
//
// Writes to stderr by default rather than stdout, so a command's own output
// stays parseable when a message goes out in the middle of it.
type ConsoleSender struct{ Stream io.Writer }

// Send writes the message where a person can read it.
func (c ConsoleSender) Send(message Message) error {
	out := c.Stream
	if out == nil {
		out = os.Stderr
	}
	var b strings.Builder
	b.WriteString("\n  -- no RESEND_API_KEY set, so this message was NOT sent ------\n")
	fmt.Fprintf(&b, "  to:      %s\n", message.To)
	fmt.Fprintf(&b, "  subject: %s\n\n", message.Subject)
	for _, line := range strings.Split(message.Body, "\n") {
		b.WriteString("  " + line + "\n")
	}
	b.WriteString("  -------------------------------------------------------------\n")
	_, err := io.WriteString(out, b.String())
	return err
}

// Transport is a seam inside the seam: `(request) -> (status, body)`. The
// EmailSender interface lets a test swap the whole sender, which proves
// everything except that the request ResendSender builds is the request Resend
// documents -- and the one thing that ever broke delivery was a header. So the
// headers are assembled above this line and a test can read them.
type Transport func(req *http.Request) (int, []byte, error)

// ResendSender is the deployed implementation. One POST, no SDK.
//
// The key is held rather than read per send, which is the opposite of what
// `config` does with the Anthropic key and worth saying why: that one is never
// bound because the SDK reads it itself, so holding a copy would serve nobody.
// This one has to be put in a header by this code. It is built once, at the
// edge, and never logged or serialised.
type ResendSender struct {
	apiKey string
	from   string
	post   Transport
}

// NewResendSender builds one, refusing an empty key.
func NewResendSender(apiKey, from string, transport Transport) (*ResendSender, error) {
	if strings.TrimSpace(apiKey) == "" {
		return nil, failf("%w: RESEND_API_KEY is empty", ErrEmailNotConfigured)
	}
	if transport == nil {
		transport = httpPost
	}
	return &ResendSender{apiKey: strings.TrimSpace(apiKey), from: from, post: transport}, nil
}

func httpPost(req *http.Request) (int, []byte, error) {
	client := &http.Client{Timeout: MailTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	// Bounded: the body is only ever read to name the error, and a provider
	// having a bad day must not be able to hand this process a large one.
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return resp.StatusCode, body, err
}

// Send posts one message.
func (r *ResendSender) Send(message Message) error {
	payload, err := json.Marshal(map[string]any{
		"from":    r.from,
		"to":      []string{message.To},
		"subject": message.Subject,
		"text":    message.Body,
	})
	if err != nil {
		return failf("%w: the message would not encode", ErrEmailNotSent)
	}
	req, err := http.NewRequest(http.MethodPost, ResendEndpoint, bytes.NewReader(payload))
	if err != nil {
		return failf("%w: %v", ErrEmailNotSent, err)
	}
	req.Header.Set("Authorization", "Bearer "+r.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", UserAgent)

	status, body, err := r.post(req)
	if err != nil {
		// Timeouts, DNS, TLS. This error is about the network, not about the
		// recipient, so it is safe to quote.
		return failf("%w: could not reach the mail provider: %v", ErrEmailNotSent, err)
	}
	if status < 200 || status >= 300 {
		return failf("%w: %s", ErrEmailNotSent, describeMailFailure(status, body))
	}
	return nil
}

// describeMailFailure is a failure worth logging, with the recipient stripped
// out.
//
// Resend's error bodies look like `{"statusCode": 422, "message": "...",
// "name": "validation_error"}` and the message quotes the address back. The
// name says what went wrong; the message is what would put somebody's address
// in the application log.
//
// **A body that is not their JSON is worth saying so**, rather than falling
// back to a bare status. That case is not the provider refusing at all -- it is
// something in front of the provider, and the two want completely different
// fixes. See UserAgent: a bare `HTTP 403` sent the first real diagnosis at the
// API key and the domain, both of which were fine. The body itself is still
// never quoted, because this string is logged.
func describeMailFailure(status int, body []byte) string {
	name := ""
	shape := "no error name"
	var parsed map[string]any
	if err := json.Unmarshal(body, &parsed); err != nil {
		shape = "and the body was not the provider's JSON"
	} else if v, ok := parsed["name"].(string); ok {
		name = v
	}
	if name == "" {
		name = shape
	}
	return fmt.Sprintf("the mail provider refused the message: HTTP %d (%s)", status, name)
}

// SenderFromEnv is the sender this process should use, decided when it is
// asked for.
//
// `RESEND_API_KEY` picks the real one. Without it:
//
//   - **auth off** -- a laptop, one person. The console sender is the right
//     answer and the printed link is the feature.
//   - **auth on** -- a deployment. Refused, loudly, rather than writing
//     addresses into whatever collects stdout there. `docs/HOSTING.md` §7 lists
//     the key as a deploy-day requirement and this is that list enforced.
//
// Read at call time and not at start, for the reason every other lookup in
// this project is: a value captured at start is a value a test cannot change
// and a container cannot set late.
func SenderFromEnv(transport Transport) (EmailSender, error) {
	if key := config.ResendAPIKey(); key != "" {
		return NewResendSender(key, config.EmailFrom(), transport)
	}
	if config.RequireAuth() {
		return nil, failf("%w: this instance requires authentication but has no "+
			"RESEND_API_KEY, so invites and password resets cannot be delivered -- "+
			"set one (`fly secrets set RESEND_API_KEY=...`) or the console fallback "+
			"would print email addresses into the application log", ErrEmailNotConfigured)
	}
	return ConsoleSender{}, nil
}
