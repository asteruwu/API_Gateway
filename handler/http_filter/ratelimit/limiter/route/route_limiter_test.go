package route

import (
	"testing"
	"time"

	"API_Gateway/builder"
	"API_Gateway/handler/message"
)

func TestRouteLimiterAllowAndReject(t *testing.T) {
	now := time.Unix(0, 0)
	limiter := newRouteLimiter(map[string]builder.RateLimitParams{
		"route#0": {QPS: 2, Burst: 2},
	}, func() time.Time { return now })

	req := &message.Request{RouteID: "route#0"}
	for i := 0; i < 2; i++ {
		if ok, _ := limiter.Allow(req); !ok {
			t.Fatalf("request %d denied, want allowed", i)
		}
	}

	ok, retryAfter := limiter.Allow(req)
	if ok {
		t.Fatal("request after burst allowed, want denied")
	}
	if want := 500 * time.Millisecond; retryAfter != want {
		t.Errorf("retryAfter = %v, want %v", retryAfter, want)
	}

	now = now.Add(500 * time.Millisecond)
	if ok, _ := limiter.Allow(req); !ok {
		t.Fatal("request after refill denied, want allowed")
	}
	if ok, _ := limiter.Allow(req); ok {
		t.Fatal("request after consuming refilled token allowed, want denied")
	}
}

func TestRouteLimiterIsolatesRoutes(t *testing.T) {
	now := time.Unix(0, 0)
	limiter := newRouteLimiter(map[string]builder.RateLimitParams{
		"route#a": {QPS: 1, Burst: 1},
		"route#b": {QPS: 1, Burst: 1},
	}, func() time.Time { return now })

	reqA := &message.Request{RouteID: "route#a"}
	reqB := &message.Request{RouteID: "route#b"}

	if ok, _ := limiter.Allow(reqA); !ok {
		t.Fatal("route#a first request denied, want allowed")
	}
	if ok, _ := limiter.Allow(reqA); ok {
		t.Fatal("route#a second request allowed, want denied")
	}
	if ok, _ := limiter.Allow(reqB); !ok {
		t.Fatal("route#b request denied after route#a exhausted, want allowed")
	}
	if ok, _ := limiter.Allow(reqB); ok {
		t.Fatal("route#b second request allowed, want denied")
	}
}

func TestRouteLimiterUnknownRoutePasses(t *testing.T) {
	limiter := newRouteLimiter(map[string]builder.RateLimitParams{
		"route#a": {QPS: 1, Burst: 1},
	}, func() time.Time { return time.Unix(0, 0) })

	if ok, retryAfter := limiter.Allow(&message.Request{RouteID: "route#missing"}); !ok || retryAfter != 0 {
		t.Errorf("unknown route = (%v, %v), want (true, 0)", ok, retryAfter)
	}
}
