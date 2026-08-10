package fakebackend

import (
	"fmt"
	"io"
	"net"
	"sync"
)

const (
	ModeEcho  = "echo"
	ModeFixed = "fixed"
)

type FakeBackend struct {
	Addr      string
	Mode      string
	FixedResp []byte

	ln net.Listener
	wg sync.WaitGroup
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

	payload, err := io.ReadAll(conn)
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
		resp = []byte(fmt.Sprintf("[fakebackend]unknown mode: %s", f.Mode))
	}

	_, _ = conn.Write(resp)
}
