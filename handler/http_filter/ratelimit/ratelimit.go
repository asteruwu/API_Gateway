package ratelimit

import (
	"fmt"
	"net/http"
	"slices"
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

type RouteParamsTable struct {
	table map[string]builder.RateLimitParams
}

func NewRateLimiter(cfg builder.RateLimitFilterConfig) (*RateLimiter, error) {
	byDim, err := groupRulesByDimension(cfg)
	if err != nil {
		return nil, err
	}

	limiters := make(map[string]limiter.Limiter)
	for _, dim := range cfg.Limiters {
		group := byDim[dim]
		switch dim {
		case gconst.RateLimitByRoute:
			if len(group.table) == 0 {
				continue
			}
			attachToLimiters(group.table, route.NewRouteLimiter(group.table), limiters)
		default:
			return nil, fmt.Errorf("unsupported rate limit dimension %q", dim)
		}
	}
	return &RateLimiter{limiters: limiters}, nil
}

func groupRulesByDimension(cfg builder.RateLimitFilterConfig) (map[gconst.RateLimitDimension]RouteParamsTable, error) {
	byDim := make(map[gconst.RateLimitDimension]RouteParamsTable)
	for routeID, rule := range cfg.Rules {
		if !slices.Contains(cfg.Limiters, rule.Dimension) {
			return nil, fmt.Errorf("route %q uses rate limit dimension %q which is not enabled in limiters %v",
				routeID, rule.Dimension, cfg.Limiters)
		}
		t := byDim[rule.Dimension]
		if t.table == nil {
			t.table = make(map[string]builder.RateLimitParams)
			byDim[rule.Dimension] = t
		}
		t.table[routeID] = rule.Params
	}
	return byDim, nil
}

func attachToLimiters(table map[string]builder.RateLimitParams, l limiter.Limiter, limiters map[string]limiter.Limiter) {
	for routeID := range table {
		limiters[routeID] = l
	}
}

func (r *RateLimiter) HandleHTTPFilt(req *message.Request) (*message.Response, error) {
	l, ok := r.limiters[req.RouteID]
	if !ok {
		return nil, nil
	}
	ok, retryAfter := l.Allow(req)
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
