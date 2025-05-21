package common

import (
	"encoding/binary"
	"os"
	"runtime"
	"sync"
	"syscall"
)

const (
	RingBufferSize = 40960
	HeaderSize     = 16
	RMmapFileName  = "/tmp/go_plugin_ringbuffer.mmap"
)

type RingBuffer struct {
	buf     []byte
	size    int
	mu      sync.Mutex // Only lock for writing, not for reading
	eventfd int        // eventfd file descriptor (Linux), or pipe read fd (macOS)
	writefd int        // pipe write fd (macOS only)
}

// Eventfd returns the eventfd or pipe read fd for external event wait.
func (r *RingBuffer) Eventfd() int {
	return r.eventfd
}

// Close releases mmap and closes file descriptors.
func (r *RingBuffer) Close() error {
	var err1, err2, err3 error
	if r.buf != nil {
		err1 = syscall.Munmap(r.buf)
	}
	if r.eventfd > 0 {
		err2 = syscall.Close(r.eventfd)
	}
	if r.writefd > 0 {
		err3 = syscall.Close(r.writefd)
	}
	if err1 != nil {
		return err1
	}
	if err2 != nil {
		return err2
	}
	if err3 != nil {
		return err3
	}
	return nil
}

func newEventfd() (int, int, error) {
	if runtime.GOOS == "linux" {
		// On Linux, use eventfd
		fd, err := unixEventfd()
		if err != nil {
			return 0, 0, err
		}
		return fd, 0, nil
	} else {
		// On macOS, use pipe to simulate eventfd
		var fds [2]int
		if err := syscall.Pipe(fds[:]); err != nil {
			return 0, 0, err
		}
		return fds[0], fds[1], nil // readfd, writefd
	}
}

func unixEventfd() (int, error) {
	if runtime.GOOS == "linux" {
		// Use syscall.Syscall to avoid missing symbol on macOS
		fd, _, errno := syscall.Syscall(syscall.SYS_EVENTFD2, 0, 0, 0)
		if errno != 0 {
			return -1, errno
		}
		return int(fd), nil
	}
	return -1, syscall.ENOSYS
}

func NewRingBuffer(size int) (*RingBuffer, error) {
	file, err := os.OpenFile(RMmapFileName, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return nil, err
	}
	stat, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if stat.Size() == 0 {
		// New file, initialize the first 16 bytes to 0
		zero := make([]byte, HeaderSize)
		if _, err := file.WriteAt(zero, 0); err != nil {
			return nil, err
		}
	}
	if err := file.Truncate(int64(size)); err != nil {
		return nil, err
	}
	buf, err := syscall.Mmap(int(file.Fd()), 0, size, syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
	if err != nil {
		return nil, err
	}
	efd, wfd, err := newEventfd()
	if err != nil {
		return nil, err
	}
	return &RingBuffer{buf: buf, size: size, eventfd: efd, writefd: wfd}, nil
}

func (r *RingBuffer) getHeadTail() (head, tail int64) {
	head = int64(binary.LittleEndian.Uint64(r.buf[0:8]))
	tail = int64(binary.LittleEndian.Uint64(r.buf[8:16]))
	return
}

func (r *RingBuffer) setHeadTail(head, tail int64) {
	binary.LittleEndian.PutUint64(r.buf[0:8], uint64(head))
	binary.LittleEndian.PutUint64(r.buf[8:16], uint64(tail))
}

// Exported GetHeadTail method for external debugging
func (r *RingBuffer) GetHeadTail() (int64, int64) {
	return r.getHeadTail()
}

// notifyEvent uses eventfd on Linux, pipe on macOS (local conditional compilation)
func (r *RingBuffer) notifyEvent() {
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], 1)
	if runtime.GOOS == "darwin" {
		if r.writefd > 0 {
			_, err := syscall.Write(r.writefd, buf[:])
			// 可选：写入失败时打印日志
			_ = err
		}
	} else {
		if r.eventfd > 0 {
			_, err := syscall.Write(r.eventfd, buf[:])
			// 可选：写入失败时打印日志
			_ = err
		}
	}
}

func (r *RingBuffer) WriteMsg(msg []byte) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	msgLen := int32(len(msg))
	if msgLen <= 0 || msgLen > int32(r.size-HeaderSize-4) {
		return false
	}
	head, tail := r.getHeadTail()
	dataStart := HeaderSize
	bufCap := r.size - HeaderSize
	used := int((tail + int64(bufCap) - head) % int64(bufCap))
	free := bufCap - used
	need := 4 + int(msgLen)
	// Key correction: ringbuffer must leave at least one byte free to avoid head==tail when full/empty
	if need >= free {
		return false
	}
	writePos := dataStart + int(tail%int64(bufCap))
	if writePos+4 > r.size {
		writePos = dataStart
	}
	// Ensure not to write out of bounds
	if writePos+4 > r.size {
		return false
	}
	binary.LittleEndian.PutUint32(r.buf[writePos:writePos+4], uint32(msgLen))
	if writePos+4+int(msgLen) <= r.size {
		copy(r.buf[writePos+4:writePos+4+int(msgLen)], msg)
	} else {
		first := r.size - (writePos + 4)
		copy(r.buf[writePos+4:], msg[:first])
		copy(r.buf[dataStart:], msg[first:])
	}
	tail += int64(4 + msgLen)
	r.setHeadTail(head, tail)
	r.notifyEvent() // Notify eventfd after write
	return true
}

// ReadMsg is single-threaded, no lock needed.
func (r *RingBuffer) ReadMsg() ([]byte, bool) {
	head, tail := r.getHeadTail()
	if head == tail {
		return nil, false
	}
	dataStart := HeaderSize
	bufCap := r.size - HeaderSize
	readPos := dataStart + int(head%int64(bufCap))
	if readPos+4 > r.size {
		readPos = dataStart
	}
	if readPos+4 > r.size {
		return nil, false
	}
	msgLen := int(binary.LittleEndian.Uint32(r.buf[readPos : readPos+4]))
	if msgLen <= 0 || msgLen > bufCap-4 {
		return nil, false
	}
	var msg []byte
	if readPos+4+msgLen <= r.size {
		msg = make([]byte, msgLen)
		copy(msg, r.buf[readPos+4:readPos+4+msgLen])
		head += int64(4 + msgLen)
	} else {
		first := r.size - (readPos + 4)
		msg = make([]byte, msgLen)
		copy(msg, r.buf[readPos+4:])
		copy(msg[first:], r.buf[dataStart:dataStart+msgLen-first])
		head += int64(4 + msgLen)
	}
	r.setHeadTail(head, tail)
	return msg, true
}

// WaitForEvent blocks until eventfd/pipe is readable and drains all pending notifications.
func (r *RingBuffer) WaitForEvent() error {
	var buf [8]byte
	for {
		n, err := syscall.Read(r.eventfd, buf[:])
		if err != nil {
			if err == syscall.EINTR {
				continue
			}
			return err
		}
		// eventfd/pipe 可能累积多次通知，需全部读完
		if n == 8 {
			break
		}
	}
	return nil
}
