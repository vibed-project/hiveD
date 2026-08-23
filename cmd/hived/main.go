// Command hived is the kubectl-style CLI for a hiveD Keeper.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/vibed-project/hiveD/cmd/hived/cmd"
)

func main() {
	// ExecuteContext, not Execute: commands previously ran on
	// context.Background() with no deadline and no cancellation, so --timeout
	// could not take effect and Ctrl-C could not unwind a stream cleanly.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := cmd.Root().ExecuteContext(ctx); err != nil {
		// A cancelled context is the user pressing Ctrl-C, not a failure to
		// report as one; still exit non-zero so scripts see it.
		if !errors.Is(err, context.Canceled) {
			fmt.Fprintln(os.Stderr, "Error:", err)
		}
		os.Exit(1)
	}
}
