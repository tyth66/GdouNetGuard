//go:build !windows

package campus

import "errors"

// EnableStartup is not supported on non-Windows platforms.
func EnableStartup(cfg Config) error {
	return errors.New("-enable-startup requires Windows; use systemd or launchd on this OS")
}

// DisableStartup is not supported on non-Windows platforms.
func DisableStartup(taskName string) error {
	return errors.New("-disable-startup requires Windows")
}

// StartBackground is not supported on non-Windows platforms.
func StartBackground(cfg Config) error {
	return errors.New("-background requires Windows; use your init system or nohup on this OS")
}
