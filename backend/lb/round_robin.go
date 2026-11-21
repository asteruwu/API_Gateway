package lb

import (
	"sync/atomic"

	gerrors "API_Gateway/pkg/errors"
)

type roundRobin struct {
	next atomic.Uint64
}

func newRoundRobin() LoadBalancer {
	return &roundRobin{}
}

func (r *roundRobin) Pick(candidates []Candidate) (int, error) {
	if len(candidates) == 0 {
		return 0, gerrors.ErrNoInstance
	}
	index := (r.next.Add(1) - 1) % uint64(len(candidates))
	return int(index), nil
}
