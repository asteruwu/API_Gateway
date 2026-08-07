package handler

import (
	"API_Gateway/builder"
	"API_Gateway/handler/message"
)

type Encoder struct {
	// 编码格式模版？编码模式选择？阈值设定？拆分报文...
}

func NewEncoder(cfg builder.EncoderConfig) *Encoder {
	return &Encoder{}
}

func (e *Encoder) Encode(resp *message.Response) ([]byte, error) {
	return resp.Raw, nil
}
