package common

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
)

const (
	DefaultRingBufferSize = 16384
	headerSize            = 8          // 4 bytes for head, 4 bytes for tail
	magicNumber           = 0x12345678 // Used to verify buffer integrity
)

var (
	ErrBufferFull     = errors.New("ring buffer is full")
	ErrInvalidSize    = errors.New("invalid message size")
	ErrBufferCorrupt  = errors.New("ring buffer is corrupted")
	ErrInvalidPointer = errors.New("invalid pointer position")
)

// RingBuffer implements a memory-mapped ring buffer.
// Memory layout:
// [magic(4)][head(4)][tail(4)][data...]
type RingBuffer struct {
	buf        []byte
	size       int
	writeMu    sync.Mutex // Write lock
	readMu     sync.Mutex // Read lock
	eventfd    int
	writefd    int
	writeCount uint64
}

// newEventfd creates an event notification fd (Linux: eventfd, macOS: pipe)
func newEventfd() (eventfd int, writefd int, err error) {
	if runtime.GOOS == "darwin" {
		var fds [2]int
		if err := syscall.Pipe(fds[:]); err != nil {
			return -1, -1, err
		}
		// Set non-blocking mode
		if err := syscall.SetNonblock(fds[0], true); err != nil {
			syscall.Close(fds[0])
			syscall.Close(fds[1])
			return -1, -1, err
		}
		if err := syscall.SetNonblock(fds[1], true); err != nil {
			syscall.Close(fds[0])
			syscall.Close(fds[1])
			return -1, -1, err
		}
		return fds[0], fds[1], nil
	} else if runtime.GOOS == "linux" {
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
	return -1, -1, syscall.EINVAL
}

// NewRingBuffer creates a new mmap-backed ring buffer file
func NewRingBuffer(mmapFileName string) (*RingBuffer, error) {
	file, err := os.OpenFile(mmapFileName, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// Ensure file size is correct
	if err := file.Truncate(int64(DefaultRingBufferSize)); err != nil {
		return nil, err
	}

	buf, err := syscall.Mmap(int(file.Fd()), 0, DefaultRingBufferSize, syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
	if err != nil {
		return nil, err
	}

	// Zero out the entire buffer
	for i := range buf {
		buf[i] = 0
	}

	efd, wfd, err := newEventfd()
	if err != nil {
		syscall.Munmap(buf)
		return nil, err
	}

	rb := &RingBuffer{
		buf:     buf,
		size:    DefaultRingBufferSize,
		eventfd: efd,
		writefd: wfd,
	}

	// Initialize the buffer
	rb.initialize()

	return rb, nil
}

// initialize initializes the ring buffer
func (r *RingBuffer) initialize() {
	// Write magic number
	binary.LittleEndian.PutUint32(r.buf[0:4], magicNumber)
	// Initialize head and tail
	r.setHead(headerSize)
	r.setTail(headerSize)
}

// verify checks if the ring buffer is valid
func (r *RingBuffer) verify() bool {
	return binary.LittleEndian.Uint32(r.buf[0:4]) == magicNumber
}

// getHead returns the current head pointer
func (r *RingBuffer) getHead() uint32 {
	return binary.LittleEndian.Uint32(r.buf[4:8])
}

// getTail returns the current tail pointer
func (r *RingBuffer) getTail() uint32 {
	return binary.LittleEndian.Uint32(r.buf[8:12])
}

// setHead sets the head pointer
func (r *RingBuffer) setHead(val uint32) {
	binary.LittleEndian.PutUint32(r.buf[4:8], val)
}

// setTail sets the tail pointer
func (r *RingBuffer) setTail(val uint32) {
	binary.LittleEndian.PutUint32(r.buf[8:12], val)
}

// GetHead returns the current head pointer
func (r *RingBuffer) GetHead() uint32 {
	return r.getHead()
}

// GetTail returns the current tail pointer
func (r *RingBuffer) GetTail() uint32 {
	return r.getTail()
}

// notifyEvent signals the consumer via eventfd/pipe
func (r *RingBuffer) notifyEvent() {
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], 1)
	if runtime.GOOS == "darwin" {
		if r.writefd > 0 {
			// Use non-blocking write
			_, err := syscall.Write(r.writefd, buf[:])
			if err == syscall.EAGAIN {
				// If pipe is full, drain it
				var tmp [8]byte
				for {
					_, err := syscall.Read(r.eventfd, tmp[:])
					if err != nil {
						break
					}
				}
				// Retry write
				_, _ = syscall.Write(r.writefd, buf[:])
			}
		}
	} else {
		if r.eventfd > 0 {
			_, _ = syscall.Write(r.eventfd, buf[:])
		}
	}
}

// WriteMsg writes a message to the ring buffer
// Returns true if successful, false if buffer is full
func (r *RingBuffer) WriteMsg(msg []byte) bool {
	r.writeMu.Lock()
	defer r.writeMu.Unlock()

	msgLen := uint32(len(msg))
	if msgLen == 0 || msgLen > uint32(r.size-headerSize-4) {
		fmt.Printf("[RingBuffer] Invalid message length: %d (max: %d)\n", msgLen, r.size-headerSize-4)
		return false
	}

	head := r.getHead()
	tail := r.getTail()
	dataCap := uint32(r.size)

	// Calculate available space
	var free uint32
	if head >= tail {
		// head is after tail, available space is from tail to head
		free = dataCap - head + tail - headerSize
	} else {
		// head is before tail, available space is from head to tail
		free = tail - head - 1
	}

	// Required space: message length(4) + message content
	need := 4 + msgLen
	if free < need {
		fmt.Printf("[RingBuffer] Buffer full: head=%d, tail=%d, free=%d, need=%d\n", head, tail, free, need)
		return false
	}

	// Write message length
	if head+4 > dataCap {
		head = headerSize
	}
	binary.LittleEndian.PutUint32(r.buf[head:head+4], msgLen)

	// Write message content
	writeEnd := head + 4 + msgLen
	if writeEnd <= dataCap {
		// Message can be written continuously
		copy(r.buf[head+4:writeEnd], msg)
	} else {
		// Message needs to be written in two parts
		firstPart := dataCap - head - 4
		if firstPart > 0 {
			copy(r.buf[head+4:dataCap], msg[:firstPart])
		}
		secondPart := msgLen - firstPart
		if secondPart > 0 {
			copy(r.buf[headerSize:headerSize+secondPart], msg[firstPart:])
		}
		writeEnd = headerSize + secondPart
	}

	// Update head pointer
	r.setHead(writeEnd)

	// Update write count
	atomic.AddUint64(&r.writeCount, 1)

	// Ensure notification is sent
	r.notifyEvent()
	return true
}

// ReadMsg reads a message from the ring buffer
// Returns the message and true if successful, nil and false if buffer is empty
func (r *RingBuffer) ReadMsg() ([]byte, bool) {
	head := r.getHead()
	tail := r.getTail()
	dataCap := uint32(r.size)

	// Check if there is data to read
	if head == tail {
		return nil, false
	}

	// Read message length
	if tail+4 > dataCap {
		tail = headerSize
	}

	msgLen := binary.LittleEndian.Uint32(r.buf[tail : tail+4])

	// Validate message length
	if msgLen == 0 || msgLen > uint32(r.size-headerSize-4) {
		// Skip current message
		if tail+4 < dataCap {
			r.setTail(tail + 4)
		} else {
			r.setTail(headerSize)
		}
		return nil, false
	}

	readEnd := tail + 4 + msgLen
	var msg []byte
	if readEnd <= dataCap {
		// Message can be read continuously
		msg = make([]byte, msgLen)
		copy(msg, r.buf[tail+4:readEnd])
	} else {
		// Message needs to be read in two parts
		firstPart := dataCap - tail - 4
		secondPart := msgLen - firstPart
		msg = make([]byte, msgLen)
		if firstPart > 0 {
			copy(msg[:firstPart], r.buf[tail+4:dataCap])
		}
		if secondPart > 0 {
			copy(msg[firstPart:], r.buf[headerSize:headerSize+secondPart])
		}
		readEnd = headerSize + secondPart
	}

	// Update tail pointer
	r.setTail(readEnd)

	return msg, true
}

// WaitForEvent blocks until eventfd/pipe is readable and drains all pending notifications.
func (r *RingBuffer) WaitForEvent() error {
	var buf [8]byte
	for {
		var n int
		var err error
		if runtime.GOOS == "darwin" {
			n, err = syscall.Read(r.eventfd, buf[:])
		} else {
			n, err = syscall.Read(r.eventfd, buf[:])
		}
		if err != nil {
			if err == syscall.EINTR {
				continue
			}
			return err
		}
		if n == 8 {
			break
		}
	}
	return nil
}

// Close releases mmap and closes eventfd/pipe
func (r *RingBuffer) Close() error {
	var err error
	if r.buf != nil {
		err = syscall.Munmap(r.buf)
		r.buf = nil
	}
	if r.eventfd > 0 {
		syscall.Close(r.eventfd)
		r.eventfd = -1
	}
	if r.writefd > 0 {
		syscall.Close(r.writefd)
		r.writefd = -1
	}
	return err
}

// Eventfd returns the eventfd/pipe read fd
func (r *RingBuffer) Eventfd() int {
	return r.eventfd
}

// GetWriteCount returns the total number of messages written
func (r *RingBuffer) GetWriteCount() uint64 {
	return atomic.LoadUint64(&r.writeCount)
}
