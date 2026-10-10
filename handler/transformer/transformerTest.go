package transformer

import (
	"API_Gateway/handler/message"
	gconst "API_Gateway/pkg/constant"
	gerrors "API_Gateway/pkg/errors"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
)

const maxFrameSize = 16 << 20 // 16 MiB

type TestTransformer struct {
	Name string
}

func (t *TestTransformer) Transform(req *message.Request, conn net.Conn) (*message.Response, error) {
	if err := writeFrame(conn, req.Body); err != nil {
		return nil, fmt.Errorf("%w: %v", gerrors.ErrTransform, err)
	}
	body, err := readFrame(conn)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", gerrors.ErrTransform, err)
	}

	return &message.Response{
		StatusCode:    http.StatusOK,
		Proto:         gconst.DefaultHTTPProto,
		Header:        http.Header{},
		Body:          body,
		ContentLength: int64(len(body)),
	}, nil
}

// writeFrame 写入一帧
func writeFrame(w io.Writer, payload []byte) error {
	var hdr [4]byte
	binary.BigEndian.PutUint32(hdr[:], uint32(len(payload)))
	if _, err := w.Write(hdr[:]); err != nil {
		return err
	}
	_, err := w.Write(payload)
	return err
}

// readFrame 读取一帧
func readFrame(r io.Reader) ([]byte, error) {
	var hdr [4]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return nil, err
	}
	n := binary.BigEndian.Uint32(hdr[:])
	if n > maxFrameSize {
		return nil, fmt.Errorf("frame too large: %d", n)
	}
	payload := make([]byte, n)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, err
	}
	return payload, nil
}
