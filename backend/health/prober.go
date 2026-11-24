package health

import (
	"context"
	"fmt"

	gconst "API_Gateway/pkg/constant"
	gerrors "API_Gateway/pkg/errors"
)

type Prober interface {
	Type() string
	Probe(ctx context.Context, addr string) error
}

type proberFactory func() Prober

var proberFactories = map[string]proberFactory{
	gconst.ProberTypeTCP: newTCPProber,
}

func NewProber(typ string) (Prober, error) {
	factory, ok := proberFactories[typ]
	if !ok {
		return nil, fmt.Errorf("%w: unsupported prober type %q", gerrors.ErrInvalidConfig, typ)
	}
	return factory(), nil
}
