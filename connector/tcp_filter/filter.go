package tcpfilter

import (
	"API_Gateway/builder"
	gerrors "API_Gateway/pkg/errors"
	"log"
	"net"
)

type TCPFilter interface {
	HandleTCPConn(conn net.Conn) error
}

func BuildTCPFilters(cfgs []any) ([]TCPFilter, error) {
	filters := []TCPFilter{}

	for i, cfg := range cfgs {
		switch filter := cfg.(type) {
		case builder.TCPLimiterFilterConfig:
			if !filter.Enable {
				continue
			}
			lf := NewLimitFilter(int64(filter.MaxConn))
			filters = append(filters, lf)
		default:
			log.Printf("[connector]failed to initialize tcp filter %d, unsupported config type %T", i, cfg)
			return nil, gerrors.ErrInitializeTCPFiltersFailed
		}
	}

	return filters, nil
}

type FilterWithStatus interface {
	OnCloseConn(conn net.Conn) error
}
