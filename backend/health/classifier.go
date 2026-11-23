package health

import "API_Gateway/handler/message"

type Classifier struct{}

func (c *Classifier) Classify(addr string, resp *message.Response, err error) Event {
	if err != nil {
		return Event{Result: false, Addr: addr}
	}
	return Event{Result: resp.StatusCode < 500, Addr: addr}
}
