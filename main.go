package main

import (
	"fmt"
	"os"

	"github.com/dmwork-org/octo-daemon-cli/cmd"
)

var (
	Version   = "dev"
	Commit    = "none"
	BuildDate = "unknown"
)

func main() {
	cmd.SetVersionInfo(Version, Commit, BuildDate)
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
