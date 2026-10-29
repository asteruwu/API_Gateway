package route

import (
	"time"

	"API_Gateway/builder"
	"API_Gateway/handler/http_filter/ratelimit/bucket"
	"API_Gateway/handler/message"
	gconst "API_Gateway/pkg/constant"
)

type RouteLimiter struct {
	buckets map[string]*bucket.TokenBucket
	now     func() time.Time
}

func NewRouteLimiter(params map[string]builder.RateLimitParams) *RouteLimiter {
	return newRouteLimiter(params, time.Now)
}

func PreparationForRouteLimiter(raw map[string]builder.RateLimitRule) (map[string]builder.RateLimitParams, []string) {
	params := make(map[string]builder.RateLimitParams)
	routes := []string{}
	for route, rule := range raw {
		if rule.Dimension == gconst.RateLimitByRoute {
			params[route] = rule.Params
			routes = append(routes, route)
		}
	}
	return params, routes
}

func newRouteLimiter(params map[string]builder.RateLimitParams, now func() time.Time) *RouteLimiter {
	base := now()
	buckets := make(map[string]*bucket.TokenBucket, len(params))
	for routeID, p := range params {
		buckets[routeID] = bucket.NewTokenBucket(p.QPS, p.Burst, base)
	}
	return &RouteLimiter{
		buckets: buckets,
		now:     now,
	}
}

func (l *RouteLimiter) Allow(req *message.Request) (bool, time.Duration) {
	bucket, ok := l.buckets[req.RouteID]
	if !ok {
		return true, 0
	}
	return bucket.Allow(l.now())
}
