package main

import (
	"errors"
	"fmt"
	"net"
	"strings"
	"testing"
)

// The bind, both answers.
//
// `serve` and `serveShim` hand [serveOn] and [serveShimOn] a listener as a
// function, because an address is not a reservation (the argument is made in
// full on [serveOn]). Every test that drove either of them drove the
// **refusal** -- a port somebody else is holding -- because the success half
// hands the socket straight into `Serve` and blocks there until a signal, so
// there was nowhere for a test to stand between the two. Pulled out as
// [listenOn] there is: asked for a port of its own it answers instantly, and
// the caller closes it again.
//
// What this holds is the pair, and the second half is the one that matters: a
// bind that fails has to say **which address** for the app's door and name the
// worker's door for the shim, because "address already in use" with no address
// in it is the least useful sentence a boot can end on.

func TestABindAnswersWithTheSocketOrWithTheAddressNobodyCouldHave(t *testing.T) {
	t.Parallel()
	refusal := func(err error) error { return fmt.Errorf("listen on the loopback: %w", err) }

	// A port of its own: the kernel chooses, the listener is real, and the
	// caller owns it afterwards.
	l, err := listenOn("127.0.0.1:0", refusal)()
	if err != nil {
		t.Fatalf("a bind to a port of its own was refused: %v", err)
	}
	addr, ok := l.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("the bind answered with a %T", l.Addr())
	}
	if addr.Port == 0 {
		t.Error("the listener came back on no port at all")
	}
	// It is really listening: the port it names accepts a connection.
	conn, err := net.Dial("tcp", l.Addr().String())
	if err != nil {
		t.Errorf("nothing was accepting on the port the bind named: %v", err)
	} else {
		_ = conn.Close()
	}
	if err := l.Close(); err != nil {
		t.Errorf("closing the listener the bind handed over: %v", err)
	}

	// And the refusal is the caller's own sentence, with the cause under it.
	called := 0
	_, err = listenOn("127.0.0.1:not-a-port", func(cause error) error {
		called++
		return fmt.Errorf("listen on the loopback: %w", cause)
	})()
	if err == nil {
		t.Fatal("a port that is not a number was bound anyway")
	}
	if called != 1 {
		t.Errorf("the caller's refusal was built %d times", called)
	}
	if !strings.Contains(err.Error(), "listen on the loopback") {
		t.Errorf("the refusal lost the caller's own sentence: %v", err)
	}
	// The cause is still underneath, so a log reader gets both halves.
	if errors.Unwrap(err) == nil {
		t.Errorf("the refusal dropped what actually went wrong: %v", err)
	}
}
