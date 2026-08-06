package handler

import (
	"API_Gateway/builder"
	"API_Gateway/handler/message"
	"net"
)

type Decoder struct {
	// 解析格式模版？解析模式选择？阈值设定？拆分报文...
}

func NewDecoder(cfg builder.DecoderConfig) *Decoder {
	return &Decoder{}
}

// 直接包装成 filter 请求方便主流程处理？
func (d *Decoder) Decode(conn net.Conn) message.Request {
	return message.Request{}
}
