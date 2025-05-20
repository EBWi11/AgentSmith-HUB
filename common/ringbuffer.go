package common

import (
	"encoding/binary"
	"os"
	"sync"
	"syscall"
)

const (
	RingBufferSize = 40960
	HeaderSize     = 16 // 8字节head+8字节tail
	RMmapFileName  = "/tmp/go_plugin_ringbuffer.mmap"
)

type RingBuffer struct {
	buf  []byte
	size int
	mu   sync.Mutex // 只对写加锁，读不加锁
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
		// 新建文件，初始化前16字节为0
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
	return &RingBuffer{buf: buf, size: size}, nil
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

// 导出GetHeadTail方法，便于外部调试
func (r *RingBuffer) GetHeadTail() (int64, int64) {
	return r.getHeadTail()
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
	// 关键修正：ringbuffer 必须至少留一个字节空闲，避免 head==tail 时无法区分满/空
	if need >= free {
		return false
	}
	writePos := dataStart + int(tail%int64(bufCap))
	if writePos+4 > r.size {
		writePos = dataStart
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
	return true
}

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
