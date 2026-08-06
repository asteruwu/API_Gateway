package connector

import (
	"API_Gateway/builder"
	tcpfilter "API_Gateway/connector/tcp_filter"
	"net"
)

// 读取配置，初始化
// 监听服务端口
// 接收连接，主流程

type Listener struct {
	addr    string
	filters []tcpfilter.TCPFilter
	next    func(net.Conn) error
}

func NewListener(cfg builder.ConnectorConfig, next func(net.Conn) error) *Listener {
	// 初始化 filters
	return &Listener{
		next: next,
	}
}

func (l *Listener) Process(conn net.Conn) {
	// TODO
	l.next(conn)
}

func (l *Listener) Connect() {
	listener, err := net.Listen("tcp", l.addr)
	if err != nil {
		// TODO
		return
	}
	defer listener.Close()

	for {
		conn, err := listener.Accept()
		if err != nil {
			// TODO
			continue
		}
		go l.Process(conn)
	}
}

func (l *Listener) Close() {
	// TODO
}
