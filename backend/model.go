package backend

import "sync/atomic"

type Backend interface {
	Call(service string, payload []byte) ([]byte, error)
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
