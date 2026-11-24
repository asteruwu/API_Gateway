package health

import (
	"context"
	"net"

	gconst "API_Gateway/pkg/constant"
)

type tcpProber struct{}

func newTCPProber() Prober {
	return &tcpProber{}
}

func (p *tcpProber) Type() string {
	return gconst.ProberTypeTCP
}

func (p *tcpProber) Probe(ctx context.Context, addr string) error {
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return err
	}
	return conn.Close()
}
