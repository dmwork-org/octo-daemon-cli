package internal

import (
	"fmt"
	"os"
	"path/filepath"
)

func LockFilePath() string {
	return filepath.Join(dataDir(), "daemon.lock")
}

// TryLock attempts to acquire an exclusive lock on the lock file.
// Returns the file handle (caller must keep it open) or an error if locked.
func TryLock() (*os.File, error) {
	lockPath := LockFilePath()
	if err := os.MkdirAll(filepath.Dir(lockPath), 0700); err != nil {
		return nil, fmt.Errorf("create lock dir: %w", err)
	}

	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, fmt.Errorf("open lock file: %w", err)
	}

	if err := lockFile(f); err != nil {
		f.Close()
		return nil, fmt.Errorf("another daemon is already running. Use 'octo-daemon stop' first")
	}

	// Write PID into lock file for reference
	f.Truncate(0)
	f.Seek(0, 0)
	fmt.Fprintf(f, "%d\n", os.Getpid())
	f.Sync()

	return f, nil
}

// IsLocked checks if the daemon lock is held (non-destructive).
func IsLocked() bool {
	lockPath := LockFilePath()
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return false
	}
	defer f.Close()

	if err := lockFile(f); err != nil {
		return true // locked by another process
	}
	unlockFile(f)
	return false
}

// ReadLockPID reads the PID from the lock file without locking.
func ReadLockPID() (int, error) {
	data, err := os.ReadFile(LockFilePath())
	if err != nil {
		return 0, err
	}
	var pid int
	_, err = fmt.Sscanf(string(data), "%d", &pid)
	return pid, err
}
