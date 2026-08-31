package httpfilter

import "API_Gateway/handler/message"

type TestFilter struct {
	Name string
}

func (t *TestFilter) HandleHTTPFilt(req *message.Request) (*message.Response, error) {
	return &message.Response{
		Body: req.Body,
	}, nil
}
