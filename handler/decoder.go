package handler

import (
	"API_Gateway/builder"
	"API_Gateway/handler/message"
	gconst "API_Gateway/pkg/constant"
	gerrors "API_Gateway/pkg/errors"
	"io"
	"net/http"
)

type Decoder struct {
	// 解析格式模版？解析模式选择？阈值设定？拆分报文...
	maxBody int64
}

func NewDecoder(cfg builder.DecoderConfig) *Decoder {
	max := cfg.MaxBody
	if max <= 0 {
		max = gconst.DefaultHandlerBodySize
	}
	return &Decoder{
		maxBody: max,
	}
}

func (d *Decoder) Decode(req *http.Request) (message.Request, error) {
	if req.ContentLength > d.maxBody {
		return message.Request{}, gerrors.ErrBodyTooLarge
	}

	var body []byte
	if req.Body != nil && req.Body != http.NoBody {
		defer req.Body.Close()

		reader := io.LimitReader(req.Body, d.maxBody+1)
		var err error
		body, err = io.ReadAll(reader)
		if int64(len(body)) > d.maxBody {
			return message.Request{}, gerrors.ErrBodyTooLarge
		}
		if err != nil {
			return message.Request{}, gerrors.ErrDecodeRequest
		}
	}

	return message.Request{
		Header:        req.Header.Clone(),
		Body:          body,
		Method:        req.Method,
		Proto:         req.Proto,
		Host:          req.Host,
		URL:           req.URL,
		ContentLength: req.ContentLength,
	}, nil
}
