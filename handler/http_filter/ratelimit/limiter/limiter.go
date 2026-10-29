package limiter

import (
	"time"

	"API_Gateway/handler/message"
)

type Limiter interface {
	Allow(req *message.Request) (bool, time.Duration)
}
