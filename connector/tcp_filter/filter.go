package tcpfilter

import "net"

type TCPFilter interface {
	HandleTCPConn(conn net.Conn) error
}
