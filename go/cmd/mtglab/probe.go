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
			client := &http.Client{Timeout: 4 * time.Second}
			resp, err := client.Get(url)
			if err != nil {
				return err
			}
			defer func() { _ = resp.Body.Close() }()
			_, _ = io.Copy(io.Discard, resp.Body)
			if resp.StatusCode < 200 || resp.StatusCode > 299 {
				return fmt.Errorf("%s answered %s", url, resp.Status)
			}
			return nil
		},
	}
}
