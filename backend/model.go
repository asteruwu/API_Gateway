package backend

import (
	"net"
	"sync/atomic"

	"API_Gateway/backend/lb"
	cp "API_Gateway/backend/pool"
)

type Backend interface {
	Call(service string) (net.Conn, func(error), error)
}

type Service struct {
	name      string
	instances []*Instance
	balancer  lb.LoadBalancer
}

type Instance struct {
	addr    string
	healthy atomic.Bool
	pool    *cp.ConnPool
}

func (i *Instance) Addr() string {
	return i.addr
}

var _ lb.Candidate = (*Instance)(nil)
