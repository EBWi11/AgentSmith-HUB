package plugin

import (
	"encoding/binary"
	"fmt"
	rb "github.com/EBWi11/mmap_ringbuffer"
	"github.com/bwmarrin/snowflake"
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"time"
)

const MmapFileSize = 1024 * 1024 * 32

type PluginServer struct {
	name        string
	path        string
	wRingBuffer *rb.RingBuffer
	rRingBuffer *rb.RingBuffer
	result      map[uint64][]byte
	cmd         *exec.Cmd
	efd         uintptr
	efdFile     *os.File // for macOS pipe compatibility
}

func PluginRingBufferMmapFilePath(name string, RW string) string {
	return "/tmp/agentsmith-hub-plugin/" + name + "_" + RW
}

func (p *PluginServer) Shutdown() {
	syscall.Kill(p.cmd.Process.Pid, syscall.SIGKILL)
}

func (p *PluginServer) RunPlugin() error {
	var errFile *os.File
	errFile, err := os.OpenFile(p.path+".stderr", os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0o0600)
	outFile, err := os.OpenFile(p.path+".stdout", os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0o0600)

	if err != nil {
		return err
	}

	efd, efdFile, err := NewEventfd()
	if err != nil {
		return err
	}
	p.efd = uintptr(efd)
	p.efdFile = efdFile

	p.cmd = exec.Command(p.path, strconv.Itoa(efd))

	defer errFile.Close()
	p.cmd.Stderr = errFile

	p.cmd.ExtraFiles = []*os.File{efdFile}
	p.cmd.Stderr = errFile
	p.cmd.Stdout = outFile

	err = p.cmd.Start()
	if err != nil {
		return err
	}

	go p.getResult()

	return nil
}

func PluginServerInit(name string, path string) (*PluginServer, error) {
	var err error
	_ = os.Mkdir("/tmp/agentsmith-hub-plugin/", os.ModePerm)
	p := &PluginServer{name: name, path: path}
	p.wRingBuffer, err = rb.NewRingBuffer(PluginRingBufferMmapFilePath(name, "w"), MmapFileSize, true)
	if err != nil {
		return nil, err
	}
	p.rRingBuffer, err = rb.NewRingBuffer(PluginRingBufferMmapFilePath(name, "r"), MmapFileSize, true)
	if err != nil {
		_ = p.wRingBuffer.Close()
		return nil, err
	}

	p.result = make(map[uint64][]byte)

	return p, nil
}

func (p *PluginServer) Exec(parameter []byte) ([]byte, error) {
	node, _ := snowflake.NewNode(1)
	idUint64 := uint64(node.Generate().Int64())

	id := make([]byte, 8)
	binary.LittleEndian.PutUint64(id, idUint64)
	_, err := p.wRingBuffer.WriteMsg(append(id, parameter...))

	if err != nil {
		return nil, err
	}

	err = WriteEfd(p.efd)
	if err != nil {
		return nil, err
	}

	time.Sleep(1 * time.Second)

	res := p.result[idUint64]
	return res, err
	return nil, fmt.Errorf("no result")
}

func (p *PluginServer) getResult() {
	for {
		if ReadEfd(p.efd) {
			tmpres, err := p.rRingBuffer.ReadMsg()
			if err == nil {
				fmt.Println("!!!! GET DATA:", string(tmpres))
				id := binary.BigEndian.Uint64(tmpres[0:8])
				res := tmpres[8:]
				p.result[id] = res
			}
		}
	}
}

func (p *PluginServer) ringbufferClose() {
	_ = p.wRingBuffer.Close()
	_ = p.rRingBuffer.Close()
	if p.result != nil {
		for k := range p.result {
			delete(p.result, k)
		}
	}
	_ = os.Remove(PluginRingBufferMmapFilePath(p.name, "w"))
	_ = os.Remove(PluginRingBufferMmapFilePath(p.name, "w"))
}
