package deploy

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const lockFileName = "deploy.lock"

// errLockHeld is returned by lockFileExclusive when another open file holds
// the lock.
var errLockHeld = errors.New("deploy lock is held by another process")

// Locker provides file-based locking for deploy operations on one
// environment. It uses an OS-level lock (flock on Unix, LockFileEx on
// Windows) to prevent concurrent deploy commands from different processes
// targeting the same environment of the same project directory.
//
// The OS releases the lock when the holding process exits, however it exits,
// so a leftover deploy.lock file never blocks anyone. The file is therefore
// never deleted: removing it while held would let a second process lock a
// fresh file at the same path while the first still runs.
type Locker struct {
	baseDir string
	env     string
	file    *os.File
}

// NewLocker creates a Locker for environment env of the given project
// directory. An invalid environment name is reported by Lock.
func NewLocker(baseDir, env string) *Locker {
	return &Locker{baseDir: baseDir, env: env}
}

// lockPath returns the full path to the lock file.
func (l *Locker) lockPath() string {
	return filepath.Join(envDirFor(l.baseDir, l.env), lockFileName)
}

// Lock acquires an exclusive file lock. Returns an error if the lock
// is already held by another process or if the locker already holds a lock.
func (l *Locker) Lock() error {
	if l.file != nil {
		return fmt.Errorf("locker already holds a lock")
	}
	if err := ValidateEnvironmentName(l.env); err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(l.lockPath()), stateDirPerm); err != nil {
		return fmt.Errorf("failed to create state directory: %w", err)
	}

	f, err := os.OpenFile(l.lockPath(), os.O_CREATE|os.O_RDWR, stateFilePerm)
	if err != nil {
		return fmt.Errorf("failed to open lock file: %w", err)
	}

	if err := lockFileExclusive(f); err != nil {
		_ = f.Close()
		if errors.Is(err, errLockHeld) {
			return l.heldError()
		}
		return err
	}

	// Record the holder so a contending command can name it. Best-effort: the
	// lock is the flock, not the file's contents.
	if err := f.Truncate(0); err == nil {
		_, _ = f.WriteAt([]byte(strconv.Itoa(os.Getpid())+"\n"), 0)
	}

	l.file = f
	return nil
}

// heldError builds the contention message, naming the holder's pid when the
// lock file records one (it is unreadable while locked on Windows).
func (l *Locker) heldError() error {
	holder := ""
	if data, err := os.ReadFile(l.lockPath()); err == nil {
		if pid := strings.TrimSpace(string(data)); pid != "" {
			holder = " (pid " + pid + ")"
		}
	}
	return fmt.Errorf("%w%s for environment %q; wait for that deploy command to finish. "+
		"The lock is released automatically when the process exits, so there is no lock file to remove",
		errLockHeld, holder, l.env)
}

// Unlock releases the lock. The lock file is left in place (see Locker).
// If no lock is held, Unlock is a no-op and returns nil.
func (l *Locker) Unlock() error {
	if l.file == nil {
		return nil
	}

	if err := unlockFile(l.file); err != nil {
		return fmt.Errorf("failed to release deploy lock: %w", err)
	}

	if err := l.file.Close(); err != nil {
		return fmt.Errorf("failed to close lock file: %w", err)
	}

	l.file = nil
	return nil
}
