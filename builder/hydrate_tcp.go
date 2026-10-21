package builder

import (
	"fmt"

	"API_Gateway/pkg/tools"
)

var tcpFilterHydrators = map[string]pluginHydrator{
	"tcp_limit": hydrateTCPLimit,
}

func hydrateTCPLimit(raw any) (any, error) {
	m, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("expected map, got %T", raw)
	}
	maxConn, ok := tools.ToInt(m["max_conn"])
	if !ok {
		return nil, fmt.Errorf("max_conn: expected integer")
	}
	return TCPLimiterFilterConfig{MaxConn: maxConn}, nil
}
