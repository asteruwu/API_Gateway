package handler

import (
	"API_Gateway/builder"
	"API_Gateway/handler/message"
	"io"
	"net/http"
)

type Decoder struct {
	// 解析格式模版？解析模式选择？阈值设定？拆分报文...
}

func NewDecoder(cfg builder.DecoderConfig) *Decoder {
	return &Decoder{}
}

func (d *Decoder) Decode(req *http.Request) (message.Request, error) {
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return message.Request{}, err
	}
	return message.Request{
		Raw:   body,
		Proto: req.Proto,
	}, nil
}
