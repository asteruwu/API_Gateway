package ratelimit

import (
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"API_Gateway/builder"
	"API_Gateway/handler/http_filter/ratelimit/limiter"
	"API_Gateway/handler/message"
	gconst "API_Gateway/pkg/constant"
	gerrors "API_Gateway/pkg/errors"
)

type stubLimiter struct {
	allow      bool
	retryAfter time.Duration
	called     bool
}

func (s *stubLimiter) Allow(*message.Request) (bool, time.Duration) {
	s.called = true
	return s.allow, s.retryAfter
}

func TestRateLimiterHandleHTTPFilt(t *testing.T) {
	t.Run("allow", func(t *testing.T) {
		stub := &stubLimiter{allow: true}
		r := &RateLimiter{limiters: map[string]limiter.Limiter{"route#0": stub}}

		resp, err := r.HandleHTTPFilt(&message.Request{RouteID: "route#0"})
		if err != nil || resp != nil {
			t.Errorf("HandleHTTPFilt = (%v, %v), want (nil, nil)", resp, err)
		}
		if !stub.called {
			t.Error("limiter not called")
		}
	})

	t.Run("deny", func(t *testing.T) {
		stub := &stubLimiter{retryAfter: 1500 * time.Millisecond}
		r := &RateLimiter{limiters: map[string]limiter.Limiter{"route#0": stub}}

		resp, err := r.HandleHTTPFilt(&message.Request{RouteID: "route#0"})
		if !errors.Is(err, gerrors.ErrRateLimited) {
			t.Fatalf("err = %v, want %v", err, gerrors.ErrRateLimited)
		}
		if resp == nil {
			t.Fatal("response = nil, want Retry-After response")
		}
		if got := resp.Header.Get("Retry-After"); got != "2" {
			t.Errorf("Retry-After = %q, want 2", got)
		}
		if !stub.called {
			t.Error("limiter not called")
		}
	})

	t.Run("unknown route", func(t *testing.T) {
		stub := &stubLimiter{allow: true}
		r := &RateLimiter{limiters: map[string]limiter.Limiter{"route#0": stub}}

		resp, err := r.HandleHTTPFilt(&message.Request{RouteID: "route#missing"})
		if err != nil || resp != nil {
			t.Errorf("HandleHTTPFilt = (%v, %v), want (nil, nil)", resp, err)
		}
		if stub.called {
			t.Error("limiter called for unknown route")
		}
	})
}

func TestRetryResponse(t *testing.T) {
	cases := map[string]struct {
		retryAfter time.Duration
		want       string
	}{
		"zero":         {0, "1"},
		"sub-second":   {500 * time.Millisecond, "1"},
		"whole second": {time.Second, "1"},
		"round up":     {1500 * time.Millisecond, "2"},
		"max":          {gconst.RateLimitMaxRetryAfter, "9223372037"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			resp := retryResponse(c.retryAfter)
			if got := resp.Header.Get("Retry-After"); got != c.want {
				t.Errorf("Retry-After = %q, want %q", got, c.want)
			}
		})
	}
}

func TestNewRateLimiterBuildsRouteLimiters(t *testing.T) {
	cfg := builder.RateLimitFilterConfig{
		Limiters: []gconst.RateLimitDimension{gconst.RateLimitByRoute},
		Rules: map[string]builder.RateLimitRule{
			"route#a": {Dimension: gconst.RateLimitByRoute, Params: builder.RateLimitParams{QPS: 0.001, Burst: 1}},
			"route#b": {Dimension: gconst.RateLimitByRoute, Params: builder.RateLimitParams{QPS: 0.001, Burst: 1}},
		},
	}

	r, err := NewRateLimiter(cfg)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(r.limiters) != 2 {
		t.Fatalf("limiters = %d, want 2", len(r.limiters))
	}
	if r.limiters["route#a"] != r.limiters["route#b"] {
		t.Error("routes in the same dimension should share one limiter instance")
	}

	reqA := &message.Request{RouteID: "route#a"}
	if resp, err := r.HandleHTTPFilt(reqA); resp != nil || err != nil {
		t.Fatalf("route#a first request = (%v, %v), want (nil, nil)", resp, err)
	}
	resp, err := r.HandleHTTPFilt(reqA)
	if resp == nil || !errors.Is(err, gerrors.ErrRateLimited) {
		t.Fatalf("route#a second request = (%v, %v), want rate limited", resp, err)
	}
	if got := resp.Header.Get("Retry-After"); got == "" {
		t.Error("missing Retry-After header")
	}
	if resp, err := r.HandleHTTPFilt(&message.Request{RouteID: "route#b"}); resp != nil || err != nil {
		t.Fatalf("route#b request = (%v, %v), want (nil, nil)", resp, err)
	}
}

func TestNewRateLimiterUnsupportedLimiter(t *testing.T) {
	cfg := builder.RateLimitFilterConfig{
		Limiters: []gconst.RateLimitDimension{gconst.RateLimitDimension("ip")},
	}

	_, err := NewRateLimiter(cfg)
	if err == nil {
		t.Fatal("err = nil, want unsupported limiter error")
	}
	if !strings.Contains(err.Error(), "ip") {
		t.Errorf("err = %v, want mention ip", err)
	}
}

func TestNewRateLimiterEmptyRules(t *testing.T) {
	r, err := NewRateLimiter(builder.RateLimitFilterConfig{})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(r.limiters) != 0 {
		t.Errorf("limiters = %d, want 0", len(r.limiters))
	}
	if resp, err := r.HandleHTTPFilt(&message.Request{RouteID: "route#0"}); resp != nil || err != nil {
		t.Errorf("HandleHTTPFilt = (%v, %v), want (nil, nil)", resp, err)
	}
}

func TestRateLimiterRouteWithoutRulePasses(t *testing.T) {
	cfg := builder.RateLimitFilterConfig{
		Limiters: []gconst.RateLimitDimension{gconst.RateLimitByRoute},
		Rules: map[string]builder.RateLimitRule{
			"route#a": {Dimension: gconst.RateLimitByRoute, Params: builder.RateLimitParams{QPS: 0.001, Burst: 1}},
		},
	}

	r, err := NewRateLimiter(cfg)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if resp, err := r.HandleHTTPFilt(&message.Request{RouteID: "route#b"}); resp != nil || err != nil {
		t.Errorf("route without rule = (%v, %v), want (nil, nil)", resp, err)
	}
}

func TestRateLimiterConcurrent(t *testing.T) {
	const burst = 100
	cfg := builder.RateLimitFilterConfig{
		Limiters: []gconst.RateLimitDimension{gconst.RateLimitByRoute},
		Rules: map[string]builder.RateLimitRule{
			"route#0": {
				Dimension: gconst.RateLimitByRoute,
				Params:    builder.RateLimitParams{QPS: 0.001, Burst: burst},
			},
		},
	}

	r, err := NewRateLimiter(cfg)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	var allowed atomic.Int64
	var denied atomic.Int64
	var wg sync.WaitGroup
	start := make(chan struct{})

	for i := 0; i < burst*2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start

			resp, err := r.HandleHTTPFilt(&message.Request{RouteID: "route#0"})
			switch {
			case resp == nil && err == nil:
				allowed.Add(1)
			case resp != nil && errors.Is(err, gerrors.ErrRateLimited):
				denied.Add(1)
			default:
				t.Errorf("unexpected result: resp=%v err=%v", resp, err)
			}
		}()
	}

	close(start)
	wg.Wait()

	if got := allowed.Load(); got != burst {
		t.Errorf("allowed = %d, want %d", got, burst)
	}
	if got := denied.Load(); got != burst {
		t.Errorf("denied = %d, want %d", got, burst)
	}
}
