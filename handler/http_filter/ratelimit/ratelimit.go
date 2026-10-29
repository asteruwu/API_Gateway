package ratelimit

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"API_Gateway/builder"
	"API_Gateway/handler/http_filter/ratelimit/limiter"
	"API_Gateway/handler/http_filter/ratelimit/limiter/route"
	"API_Gateway/handler/message"
	gconst "API_Gateway/pkg/constant"
	gerrors "API_Gateway/pkg/errors"
)

type RateLimiter struct {
	limiters map[string]limiter.Limiter
}

func NewRateLimiter(cfg builder.RateLimitFilterConfig) (*RateLimiter, error) {
	limiters := make(map[string]limiter.Limiter)

	for _, l := range cfg.Limiters {
		switch l {
		case gconst.RateLimitByRoute:
			params, routes := route.PreparationForRouteLimiter(cfg.Rules)
			routeLimiter := route.NewRouteLimiter(params)
			attachToLimiters(routes, routeLimiter, limiters)
		default:
			return nil, fmt.Errorf("unsupported rate limit dimension %q", l)
		}
	}
	return &RateLimiter{limiters: limiters}, nil
}

func attachToLimiters(routes []string, l limiter.Limiter, limiters map[string]limiter.Limiter) {
	for _, route := range routes {
		limiters[route] = l
	}
}

func (r *RateLimiter) HandleHTTPFilt(req *message.Request) (*message.Response, error) {
	limiter, ok := r.limiters[req.RouteID]
	if !ok {
		return nil, nil
	}
	ok, retryAfter := limiter.Allow(req)
	if ok {
		return nil, nil
	}
	return retryResponse(retryAfter), gerrors.ErrRateLimited
}

func retryResponse(retryAfter time.Duration) *message.Response {
	seconds := int64(retryAfter / time.Second)
	if retryAfter%time.Second != 0 {
		seconds++
	}
	if seconds < 1 {
		seconds = 1
	}
	header := http.Header{}
	header.Set("Retry-After", strconv.FormatInt(seconds, 10))
	return &message.Response{Header: header}
}
