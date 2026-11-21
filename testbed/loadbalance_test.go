package testbed

import (
	"net"
	"testing"

	"API_Gateway/builder"
)

func requestFixed(t *testing.T, addr string, body []byte) []byte {
	t.Helper()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial gateway: %v", err)
	}
	defer conn.Close()
	return echoViaHalfClose(t, conn, body)
}

func TestRoundRobinDistributesAcrossInstances(t *testing.T) {
	backendA := startFixedBackend(t, []byte("a"))
	backendB := startFixedBackend(t, []byte("b"))

	const gatewayAddr = "127.0.0.1:9997"
	router := routerConfig(nil, map[string][]string{"testBackend": {"/"}})
	cfg := &builder.Config{
		Connector: builder.ConnectorConfig{
			Port: "9997",
		},
		Handler: builder.HandlerConfig{
			Filter: builder.HTTPFilterConfig{
				Router:    router,
				Pipelines: pipelinesFor(router),
			},
		},
		Backend: backendConfig(map[string][]string{
			"testBackend": {backendA, backendB},
		}),
	}
	startGateway(t, cfg)

	want := []string{"a", "b", "a", "b"}
	for i, expected := range want {
		got := requestFixed(t, gatewayAddr, []byte("ping"))
		if string(got) != expected {
			t.Errorf("request %d response = %q, want %q", i, got, expected)
		}
	}
}
