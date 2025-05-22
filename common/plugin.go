package common

import (
	"encoding/binary"
	rb "github.com/EBWi11/mmap_ringbuffer"
	"github.com/bwmarrin/snowflake"
)

const MmapFileSize = 1024 * 1024 * 32

type Plugin struct {
	Name        string
	WRingBuffer *rb.RingBuffer
	RRingBuffer *rb.RingBuffer
	Result      map[uint64][]byte
}

func createPluginRingBufferMmapFilePath(name string, RW string) string {
	return "/tmp/agentsmith-hub-plugin/" + name + "_" + RW
}

func PluginInit(name string) (*Plugin, error) {
	var err error
	p := &Plugin{Name: name}
	p.WRingBuffer, err = rb.NewRingBuffer(createPluginRingBufferMmapFilePath(name, "w"), MmapFileSize, true)
	if err != nil {
		return nil, err
	}
	p.RRingBuffer, err = rb.NewRingBuffer(createPluginRingBufferMmapFilePath(name, "r"), MmapFileSize, true)
	if err != nil {
		_ = p.WRingBuffer.Close()
		return nil, err
	}

	p.Result = make(map[uint64][]byte)

	return p, nil
}

func (p *Plugin) GetResultl() {
	for {
		tmpres, err := p.RRingBuffer.ReadMsg()

		if err != nil {

		}

		id := binary.BigEndian.Uint64(tmpres[0:8])
		res := tmpres[8:]

		p.Result[id] = res
	}
}

func (p *Plugin) Run(parameter []byte) ([]byte, error) {
	node, _ := snowflake.NewNode(1)
	idUint64 := uint64(node.Generate().Int64())

	id := make([]byte, 8)
	binary.LittleEndian.PutUint64(id, idUint64)
	_, err := p.WRingBuffer.WriteMsg(append(id, parameter...))

	if err != nil {

	}

	res := p.Result[idUint64]

	return res, err
}

func (p *Plugin) Close() {
	_ = p.WRingBuffer.Close()
	_ = p.RRingBuffer.Close()
	if p.Result != nil {
		for k := range p.Result {
			delete(p.Result, k)
		}
	}
}
