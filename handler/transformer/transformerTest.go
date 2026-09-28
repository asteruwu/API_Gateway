package transformer

import (
	"API_Gateway/handler/message"
	gconst "API_Gateway/pkg/constant"
	"io"
	"net"
	"net/http"
)

type TestTransformer struct {
	Name string
}

func (t *TestTransformer) Transform(req *message.Request, conn net.Conn) (*message.Response, error) {
	defer conn.Close()

	if _, err := conn.Write(req.Body); err != nil {
		return nil, err
	}
	if tc, ok := conn.(*net.TCPConn); ok {
		if err := tc.CloseWrite(); err != nil {
			return nil, err
		}
	}

	body, err := io.ReadAll(conn)
	if err != nil {
		return nil, err
	}

	return &message.Response{
		StatusCode:    http.StatusOK,
		Proto:         gconst.DefaultHTTPProto,
		Header:        http.Header{},
		Body:          body,
		ContentLength: int64(len(body)),
	}, nil
}
