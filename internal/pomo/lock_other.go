//go:build !darwin && !linux && !freebsd

package pomo

import "errors"

func acquireTimerLock(string) (func(), error) {
	return nil, errors.New("interactive timers currently support macOS, Linux, and FreeBSD; desktop notifications support macOS")
}
