package handler

import (
	"API_Gateway/builder"
	"API_Gateway/handler/message"
)

type Decoder struct {
	// 解析格式模版？解析模式选择？阈值设定？拆分报文...
}

func NewDecoder(cfg builder.DecoderConfig) *Decoder {
	return &Decoder{}
}

func (d *Decoder) Decode(raw []byte) message.Request {
	return message.Request{
		Raw: raw,
	}
}
