//go:build darwin

package plugin

import (
	"os"
	"syscall"
)

func NewEventfd() (fd int, file *os.File, err error) {
	var fds [2]int
	if err := syscall.Pipe(fds[:]); err != nil {
		return -1, nil, err
	}
	// fds[0]: read, fds[1]: write
	return fds[0], os.NewFile(uintptr(fds[0]), "eventpipe"), nil
}
