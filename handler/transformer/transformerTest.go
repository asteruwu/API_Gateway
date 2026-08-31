package transformer

import "API_Gateway/handler/message"

type TestTransformer struct {
	Name string
}

func (t *TestTransformer) Transform(req *message.Request) ([]byte, error) {
	return req.Body, nil
}

func (t *TestTransformer) Restore(resp []byte) (*message.Response, error) {
	return &message.Response{
		Body: resp,
	}, nil
}
