package httpfilter

import "API_Gateway/handler/message"

type HTTPFilter interface {
	HandleHTTPFilt(req *message.Request) (*message.Response, error)
}
