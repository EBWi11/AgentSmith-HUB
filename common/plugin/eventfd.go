package plugin

import (
	"encoding/binary"
	"golang.org/x/sys/unix"
	"runtime"
	"syscall"
)

func ReadEfd(efd uintptr) bool {
	var buf [8]byte
	if runtime.GOOS == "linux" {
		_, err := unix.Read(int(efd), buf[:])
		if err != nil {
			return false
		}
		return true
	} else {
		_, err := syscall.Read(int(efd), buf[:1]) // read 1 byte as signal
		if err != nil {
			return false
		}
		return true
	}
}

func WriteEfd(efd uintptr) error {
	if runtime.GOOS == "linux" {
		var buf [8]byte
		binary.LittleEndian.PutUint64(buf[:], 1)
		_, err := unix.Write(int(efd), buf[:])
		return err
	} else {
		var b [1]byte
		b[0] = 1
		_, err := syscall.Write(int(efd), b[:])
		return err
	}
}
