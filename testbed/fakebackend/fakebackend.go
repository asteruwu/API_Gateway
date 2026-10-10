package fakebackend

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
)

const (
	ModeEcho  = "echo"
	ModeFixed = "fixed"

	maxFrameSize = 16 << 20 // 16 MiB
)

type FakeBackend struct {
	Addr      string
	Mode      string
	FixedResp []byte

	ln       net.Listener
	wg       sync.WaitGroup
	accepted atomic.Int64
}

func (f *FakeBackend) Start() error {
	ln, err := net.Listen("tcp", f.Addr)
	if err != nil {
		return err
	}
	f.ln = ln
	f.wg.Add(1)
	go f.acceptLoop()
	return nil
}

func (f *FakeBackend) RealAddr() string {
	return f.ln.Addr().String()
}

// ConnsAccepted 返回已 accept 的连接总数，用于断言后端连接是否被复用。
func (f *FakeBackend) ConnsAccepted() int64 {
	return f.accepted.Load()
}

func (f *FakeBackend) Close() error {
	if f.ln == nil {
		return nil
	}
	err := f.ln.Close()
	f.wg.Wait()
	return err
}

func (f *FakeBackend) acceptLoop() {
	defer f.wg.Done()
	for {
		conn, err := f.ln.Accept()
		if err != nil {
			return
		}
		f.wg.Add(1)
		go f.handleConn(conn)
	}
}

func (f *FakeBackend) handleConn(conn net.Conn) {
	defer f.wg.Done()
	defer conn.Close()
	f.accepted.Add(1)

	// 帧循环：一条连接可连续处理多帧，直到对端关闭 —— 这是长连接复用的前提
	for {
		payload, err := readFrame(conn)
		if err != nil {
			return
		}

		var resp []byte
		switch f.Mode {
		case ModeEcho, "":
			resp = payload
		case ModeFixed:
			resp = f.FixedResp
		default:
			resp = fmt.Appendf([]byte{}, "[fakebackend]unknown mode: %s", f.Mode)
		}

		if err := writeFrame(conn, resp); err != nil {
			return
		}
	}
}

// readFrame / writeFrame 独立实现与 transformer 相同的模拟帧协议
func readFrame(r io.Reader) ([]byte, error) {
	var hdr [4]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return nil, err
	}
	n := binary.BigEndian.Uint32(hdr[:])
	if n > maxFrameSize {
		return nil, fmt.Errorf("frame too large: %d", n)
	}
	payload := make([]byte, n)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func writeFrame(w io.Writer, payload []byte) error {
	var hdr [4]byte
	binary.BigEndian.PutUint32(hdr[:], uint32(len(payload)))
	if _, err := w.Write(hdr[:]); err != nil {
		return err
	}
	_, err := w.Write(payload)
	return err
}
