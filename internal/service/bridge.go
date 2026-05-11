package service

import "github.com/dmwork-org/octo-daemon-cli/internal"

// internalDataDir is a thin wrapper to keep Platform files independent of the
// internal package's exact API surface. Currently just delegates.
func internalDataDir() string { return internal.DataDir() }
