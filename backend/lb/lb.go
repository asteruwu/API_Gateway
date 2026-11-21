package lb

import (
	"fmt"

	gconst "API_Gateway/pkg/constant"
	gerrors "API_Gateway/pkg/errors"
)

type Candidate interface {
	Addr() string
}

type LoadBalancer interface {
	Pick(candidates []Candidate) (int, error)
}

type loadBalancerFactory func() LoadBalancer

var lbFactories = map[string]loadBalancerFactory{
	gconst.LBPolicyRoundRobin: newRoundRobin,
	gconst.LBPolicyRandom:     newRandom,
}

func New(policy string) (LoadBalancer, error) {
	factory, ok := lbFactories[policy]
	if !ok {
		return nil, fmt.Errorf("%w: unsupported lb policy %q", gerrors.ErrInvalidConfig, policy)
	}
	return factory(), nil
}
