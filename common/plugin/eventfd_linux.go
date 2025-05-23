//go:build linux

package plugin

import (
	"golang.org/x/sys/unix"
	"os"
)

func NewEventfd() (fd int, file *os.File, err error) {
	fd, err = unix.Eventfd(0, unix.EFD_NONBLOCK)
	if err != nil {
		return -1, nil, err
	}
	return fd, os.NewFile(uintptr(fd), "eventfd"), nil
}
