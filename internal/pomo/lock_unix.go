//go:build darwin || linux || freebsd

package pomo

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

func acquireTimerLock(dir string) (func(), error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(dir, "timer.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, fmt.Errorf("a timer is already running; use its controls to pause, extend, or start a break")
		}
		return nil, err
	}
	// Keep the lock file: unlinking it would permit two processes to lock different inodes.
	return func() { syscall.Flock(int(f.Fd()), syscall.LOCK_UN); f.Close() }, nil
}
