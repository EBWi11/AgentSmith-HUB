//go:build darwin

package common

import (
	"syscall"
)

// newEventfd creates an event notification fd (Linux: eventfd, macOS: pipe)
func NewEventfd() (eventfd int, writefd int, err error) {
	var fds [2]int
	if err := syscall.Pipe(fds[:]); err != nil {
		return -1, -1, err
	}
	// Set non-blocking mode
	if err := syscall.SetNonblock(fds[0], true); err != nil {
		_ = syscall.Close(fds[0])
		_ = syscall.Close(fds[1])
		return -1, -1, err
	}
	if err := syscall.SetNonblock(fds[1], true); err != nil {
		_ = syscall.Close(fds[0])
		_ = syscall.Close(fds[1])
		return -1, -1, err
	}
	return fds[0], fds[1], nil
}
