//go:build linux

package service

import (
	"errors"
)

type systemdUserService struct{}

// All Linux methods are stubs for now — implemented in Step 7 of the plan.
// Keeping the struct + interface satisfaction here so the build stays green
// on linux, and the New() factory can return a concrete value.

func (*systemdUserService) Install(cfg InstallConfig, force bool) error {
	return errors.New("systemd user service install is not yet implemented (see plan Step 7)")
}

func (*systemdUserService) Uninstall() error {
	return errors.New("systemd user service uninstall is not yet implemented")
}

func (*systemdUserService) Status() (StatusInfo, error) {
	return StatusInfo{}, errors.New("systemd user service status is not yet implemented")
}

func (*systemdUserService) Restart() error {
	return errors.New("systemd user service restart is not yet implemented")
}

func (*systemdUserService) LogPath() string {
	// Linux uses journald; CLI prints a journalctl command instead.
	return ""
}
