package backend

import (
	"net"
	"sync/atomic"
)

type Backend interface {
	Call(service string) (net.Conn, error)
}

type Service struct {
	name      string
	instances []*Instance
	lb        LoadBalancer
}

type Instance struct {
	addr    string
	healthy atomic.Bool
	pool    *ConnPool
}
