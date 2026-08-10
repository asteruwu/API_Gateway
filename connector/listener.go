package connector

import (
	"API_Gateway/builder"
	tcpfilter "API_Gateway/connector/tcp_filter"
	"errors"
	"fmt"
	"net"
)

// 读取配置，初始化
// 监听服务端口
// 接收连接，主流程

type Listener struct {
	addr     string
	listener net.Listener
	filters  []tcpfilter.TCPFilter
	next     func(net.Conn) error
}

const defaultListenerPort = "6666"

func NewListener(cfg builder.ConnectorConfig, next func(net.Conn) error) *Listener {
	var port string
	if cfg.Port == "" {
		port = defaultListenerPort
	} else {
		port = cfg.Port
	}

	// 初始化 filters

	return &Listener{
		addr: fmt.Sprintf("%s:%s", "localhost", port),
		next: next,
	}
}

func (l *Listener) Process(conn net.Conn) {
	// TODO
	defer conn.Close()
	fmt.Println("[connector]pass connection to handler")
	l.next(conn)
}

func (l *Listener) Connect() error {
	listener, err := net.Listen("tcp", l.addr)
	if err != nil {
		// TODO
		return err
	}
	l.listener = listener
	defer l.Close()

	for {
		conn, err := listener.Accept()
		if errors.Is(err, net.ErrClosed) {
			return err
		}
		if err != nil {
			// TODO
			fmt.Println("[connector]failed to accept connection, drop")
			continue
		}
		go l.Process(conn)
	}
}

func (l *Listener) Close() {
	// TODO
	l.listener.Close()
}
