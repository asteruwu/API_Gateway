package handler

import (
	"API_Gateway/builder"
	"API_Gateway/handler/message"
	gconst "API_Gateway/pkg/constant"
	gerrors "API_Gateway/pkg/errors"
	"bytes"
	"fmt"
	"net/http"
)

type Encoder struct {
	// 编码格式模版？编码模式选择？阈值设定？拆分报文...
}

func NewEncoder(cfg builder.EncoderConfig) *Encoder {
	return &Encoder{}
}

func (e *Encoder) Encode(resp *message.Response) ([]byte, error) {
	if resp == nil {
		return nil, gerrors.ErrEncodeResponse
	}

	if resp.StatusCode == 0 {
		resp.StatusCode = gconst.DefaultHTTPStatus
	}
	if resp.Proto == "" {
		resp.Proto = gconst.DefaultHTTPProto
	}

	var buf bytes.Buffer

	e.writeStatusLine(&buf, resp)
	e.writeHeaders(&buf, resp.Header)
	e.writeBody(&buf, resp.Body)

	return buf.Bytes(), nil
}

func (e *Encoder) writeStatusLine(buf *bytes.Buffer, resp *message.Response) {
	fmt.Fprintf(buf, "%s %d %s\r\n", resp.Proto, resp.StatusCode, http.StatusText(resp.StatusCode))
}

func (e *Encoder) writeHeaders(buf *bytes.Buffer, h http.Header) {
	for key, values := range h {
		for _, value := range values {
			fmt.Fprintf(buf, "%s: %s\r\n", key, value)
		}
	}
}

func (e *Encoder) writeBody(buf *bytes.Buffer, body []byte) {
	fmt.Fprintf(buf, "Content-Length: %d\r\n\r\n", len(body))
	buf.Write(body)
}
