package main

import (
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/spf13/cobra"
)

// probeCommand is the image's HEALTHCHECK: one GET, answered or not. The
// runtime image carries nothing but the binary, so the binary asks after
// its own health. Hidden -- it is plumbing for the container, not a command
// anybody runs by hand.
func probeCommand() *cobra.Command {
	return &cobra.Command{
		Use:    "probe [url]",
		Short:  "GET a URL and exit 0 on a 2xx",
		Hidden: true,
		Args:   cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			url := "http://127.0.0.1:8080/api/health"
			if len(args) == 1 {
				url = args[0]
			}
			// **Its own transport, never the process-global one.** A bare
			// `&http.Client{}` borrows `http.DefaultTransport`, which is
			// shared by everything in the process and keeps a pool of idle
			// connections — so a probe could be answered over a connection
			// somebody else opened and left, which is precisely the
			// connection a health check must not trust. A health probe that
			// reuses a stale socket can report health that the next real
			// request will not get.
			//
			// It also made the test for this flaky, which is how it was
			// found: seven parallel subtests sharing one pool, and a
			// `CloseIdleConnections` from any of them breaking a request in
			// another. Green on amd64, red on arm64, on the same commit.
			client := &http.Client{
				Timeout:   4 * time.Second,
				Transport: &http.Transport{DisableKeepAlives: true},
			}
			resp, err := client.Get(url)
			if err != nil {
				return err
			}
			defer func() { _ = resp.Body.Close() }()
			defer client.CloseIdleConnections()
			_, _ = io.Copy(io.Discard, resp.Body)
			if resp.StatusCode < 200 || resp.StatusCode > 299 {
				return fmt.Errorf("%s answered %s", url, resp.Status)
			}
			return nil
		},
	}
}
