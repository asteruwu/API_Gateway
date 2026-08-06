package backend

import "net"

type ConnPool struct {
	addr string
	idle chan net.Conn
}

// 维护连接池状态
