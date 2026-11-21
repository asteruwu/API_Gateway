package lb

import (
	"math/rand/v2"

	gerrors "API_Gateway/pkg/errors"
)

type random struct {
	intN func(int) int
}

func newRandom() LoadBalancer {
	return &random{intN: rand.IntN}
}

func (r *random) Pick(candidates []Candidate) (int, error) {
	if len(candidates) == 0 {
		return 0, gerrors.ErrNoInstance
	}
	return r.intN(len(candidates)), nil
}
