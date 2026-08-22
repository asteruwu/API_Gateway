package tcpfilter

import (
	gerrors "API_Gateway/pkg/errors"
	"log"
	"net"
	"sync/atomic"
)

type LimitFilter struct {
	name     string
	maxConn  int64
	currConn int64
}

func NewLimitFilter(maxConn int64) *LimitFilter {
	return &LimitFilter{
		name:    "tcp_limit_filter",
		maxConn: maxConn,
	}
}

func (l *LimitFilter) HandleTCPConn(conn net.Conn) error {
	curr := atomic.AddInt64(&l.currConn, 1)
	if curr > l.maxConn {
		atomic.AddInt64(&l.currConn, -1)
		log.Printf("[connector]failed to pass filter: %s, closed conn", l.name)
		return gerrors.ErrTCPConnLimitExceeded
	}
	return nil
}

func (l *LimitFilter) OnCloseConn(conn net.Conn) error {
	atomic.AddInt64(&l.currConn, -1)
	return nil
}
