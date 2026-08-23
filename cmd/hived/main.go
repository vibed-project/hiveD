// Command hived is the kubectl-style CLI for a hiveD Keeper.
package main

import (
	"fmt"
	"os"

	"github.com/vibed-project/hiveD/cmd/hived/cmd"
)

func main() {
	if err := cmd.Root().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
