package connector

import (
	"API_Gateway/builder"
	tcpfilter "API_Gateway/connector/tcp_filter"
	gerrors "API_Gateway/pkg/errors"
	"errors"
	"fmt"
	"log"
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

func NewListener(cfg builder.ConnectorConfig, next func(net.Conn) error) (*Listener, error) {
	var port string
	if cfg.Port == "" {
		port = defaultListenerPort
	} else {
		port = cfg.Port
	}

	// 初始化 filters
	filters, err := tcpfilter.BuildTCPFilters(cfg.Filters)
	if err != nil {
		log.Printf("[connector]failed to initialize listener, errmsg: %s", err.Error())
		return nil, gerrors.ErrInitializeListenerFailed
	}

	return &Listener{
		addr:    fmt.Sprintf("%s:%s", "localhost", port),
		filters: filters,
		next:    next,
	}, nil
}

func (l *Listener) Process(conn net.Conn) {
	defer conn.Close()
	// filters 编排由配置顺序决定
	for _, f := range l.filters {
		err := f.HandleTCPConn(conn)
		if err != nil {
			log.Printf("[connector]failed to process, errmsg: %s", err.Error())
			return
		}
		if fs, ok := f.(tcpfilter.FilterWithStatus); ok {
			defer fs.OnCloseConn(conn)
		}
	}
	fmt.Println("[connector]pass connection to handler")
	l.next(conn)
}

func (l *Listener) Connect() error {
	listener, err := net.Listen("tcp", l.addr)
	if err != nil {
		// TODO
		return gerrors.ErrListenFailed
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
