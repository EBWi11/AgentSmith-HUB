//go:build linux

package common

// newEventfd creates an event notification fd (Linux: eventfd, macOS: pipe)
func NewEventfd() (eventfd int, writefd int, err error) {
	// On Linux, use eventfd2 syscall
	fd, _, errno := syscall.Syscall(syscall.SYS_EVENTFD2, 0, 0, 0)
	if errno != 0 {
		return -1, -1, errno
	}
	// Set non-blocking mode
	flags, _, errno := syscall.Syscall(syscall.SYS_FCNTL, fd, syscall.F_GETFL, 0)
	if errno != 0 {
		syscall.Close(int(fd))
		return -1, -1, errno
	}
	_, _, errno = syscall.Syscall(syscall.SYS_FCNTL, fd, syscall.F_SETFL, flags|syscall.O_NONBLOCK)
	if errno != 0 {
		syscall.Close(int(fd))
		return -1, -1, errno
	}
	return int(fd), -1, nil
}
