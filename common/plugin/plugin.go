package plugin

import (
	"errors"
	rb "github.com/EBWi11/mmap_ringbuffer"
	"os/exec"
)

type Plugin struct {
	name        string
	path        string
	wRingBuffer *rb.RingBuffer
	rRingBuffer *rb.RingBuffer
	cmd         *exec.Cmd
	efd         uintptr
}

func (p *Plugin) Read() ([]byte, error) {
	if ReadEfd(p.efd) {
		return p.rRingBuffer.ReadMsg()
	}
	return nil, errors.New("no data")
}

func (p *Plugin) Write(data []byte) error {
	err := WriteEfd(p.efd)
	if err != nil {
		return err
	}
	_, err = p.wRingBuffer.WriteMsg(data)
	return err
}

func PluginInit(name string, path string) (*Plugin, error) {
	var err error
	efd := uintptr(3)

	p := &Plugin{name: name, path: path, efd: efd}
	p.wRingBuffer, err = rb.OpenRingBuffer(PluginRingBufferMmapFilePath(name, "r"))
	if err != nil {
		return nil, err
	}
	p.wRingBuffer, err = rb.OpenRingBuffer(PluginRingBufferMmapFilePath(name, "w"))
	if err != nil {
		return nil, err
	}

	return p, err
}
