package handler

import "net"

type Handler interface {
	HandleHTTPConn(conn net.Conn) error
}
