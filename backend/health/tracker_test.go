package health

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"API_Gateway/builder"
	gconst "API_Gateway/pkg/constant"
	gerrors "API_Gateway/pkg/errors"
)

func trackerTestConfig() builder.HealthConfig {
	return builder.HealthConfig{
		Enabled: true,
		Reporter: builder.ReporterConfig{
			Window:     time.Second,
			ErrorRate:  0.5,
			MinSamples: 1,
		},
		Probe: builder.ProbeConfig{
			Type:             gconst.ProberTypeTCP,
			Timeout:          time.Hour,
			BaseInterval:     time.Second,
			MaxInterval:      time.Minute,
			SuccessThreshold: 2,
			MaxFailures:      5,
			MaxConcurrent:    1,
		},
	}
}

func newTestTracker(t *testing.T, addrs []string) *HealthTracker {
	t.Helper()
	h, err := New(trackerTestConfig(), addrs)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := h.Close(); err != nil {
			t.Error(err)
		}
	})
	return h
}

func TestHealthTrackerStart(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newTestTracker(t, []string{"a"})
		h.scheduler.scanInterval = time.Second
		h.scheduler.prober = &fakeProber{probeFunc: func(context.Context, string) error { return nil }}
		h.Start()
		synctest.Wait()
		h.reporter.Report(Event{Addr: "a", Result: false})

		time.Sleep(time.Second)
		synctest.Wait()
		if got := h.Snapshot(); len(got) != 0 {
			t.Fatalf("snapshot after first probe = %v, want empty", got)
		}
		if status, _ := h.health.Read("a"); status.state != StateProbing {
			t.Fatalf("state = %v, want probing", status.state)
		}

		time.Sleep(time.Second)
		synctest.Wait()
		if got := h.Snapshot(); !slices.Equal(got, []string{"a"}) {
			t.Errorf("snapshot after recovery = %v, want [a]", got)
		}
	})
}

func TestHealthTrackerCloseIdempotent(t *testing.T) {
	cases := []struct {
		name  string
		start bool
	}{
		{"before start", false},
		{"after start", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				h := newTestTracker(t, []string{"a"})
				if c.start {
					h.Start()
					synctest.Wait()
				}
				for range 2 {
					if err := h.Close(); err != nil {
						t.Fatal(err)
					}
				}
				if !h.reporter.closed.Load() || !h.scheduler.closed.Load() {
					t.Fatal("components are not closed")
				}
				if err := h.scheduler.ctx.Err(); !errors.Is(err, context.Canceled) {
					t.Errorf("scheduler context error = %v, want canceled", err)
				}
				h.reporter.Report(Event{Addr: "a", Result: false})
				time.Sleep(2 * time.Second)
				synctest.Wait()
				if got := h.reporter.table["a"].totalTimes.Load(); got != 0 {
					t.Errorf("reports after close = %d, want 0", got)
				}
				if got := h.Snapshot(); !slices.Equal(got, []string{"a"}) {
					t.Errorf("snapshot after close = %v, want [a]", got)
				}
			})
		})
	}
}

func TestHealthTrackerCloseWaitsForProbe(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newTestTracker(t, []string{"a"})
		entered, cancelled, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
		var releaseOnce sync.Once
		t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })
		h.scheduler.prober = &fakeProber{probeFunc: func(ctx context.Context, addr string) error {
			close(entered)
			<-ctx.Done()
			close(cancelled)
			<-release
			return ctx.Err()
		}}
		h.Start()
		h.reporter.Report(Event{Addr: "a", Result: false})
		<-entered
		before, _ := h.health.Read("a")
		returned := make(chan error, 1)
		go func() { returned <- h.Close() }()
		<-cancelled
		synctest.Wait()
		if !h.reporter.closed.Load() {
			t.Error("reporter must close before probe cancellation")
		}
		if len(returned) != 0 {
			t.Error("Close returned before the probe exited")
		}
		releaseOnce.Do(func() { close(release) })
		if err := <-returned; err != nil {
			t.Fatal(err)
		}
		time.Sleep(2 * time.Second)
		synctest.Wait()
		if after, _ := h.health.Read("a"); after != before {
			t.Error("shutdown changed instance status")
		}
	})
}

func TestHealthTrackerCloseWithFullResults(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newTestTracker(t, []string{"a"})
		s := h.scheduler
		s.prober = &fakeProber{probeFunc: func(context.Context, string) error { return nil }}
		s.results <- probeResult{addr: "filled"}
		s.startProbe("a")
		synctest.Wait()
		if err := h.Close(); err != nil {
			t.Fatal(err)
		}
		if got := len(s.results); got != cap(s.results) {
			t.Errorf("result queue length = %d, want %d", got, cap(s.results))
		}
	})
}

func TestHealthTrackerSnapshot(t *testing.T) {
	cases := []struct {
		name   string
		addrs  []string
		states map[string]State
		want   []string
	}{
		{name: "empty"},
		{name: "all healthy", addrs: []string{"a", "b", "c"}, want: []string{"a", "b", "c"}},
		{name: "mixed", addrs: []string{"a", "b", "c"}, states: map[string]State{"b": StateProbing, "c": StateDead}, want: []string{"a"}},
		{name: "none healthy", addrs: []string{"a", "b"}, states: map[string]State{"a": StateProbing, "b": StateDead}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newTestTracker(t, c.addrs)
			for addr, state := range c.states {
				h.health.Write(addr, state)
			}
			got := h.Snapshot()
			slices.Sort(got)
			if !slices.Equal(got, c.want) {
				t.Fatalf("snapshot = %v, want %v", got, c.want)
			}
			if len(got) > 0 {
				got[0] = "modified"
				next := h.Snapshot()
				slices.Sort(next)
				if !slices.Equal(next, c.want) {
					t.Errorf("snapshot shares storage: %v", next)
				}
			}
		})
	}
}

func TestHealthTrackerDisabled(t *testing.T) {
	cases := []struct {
		name string
		cfg  builder.HealthConfig
	}{
		{name: "zero config"},
		{name: "invalid unused probe", cfg: builder.HealthConfig{Probe: builder.ProbeConfig{Type: "unknown", MaxConcurrent: -1}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h, err := New(c.cfg, []string{"a", "b"})
			if err != nil || h != nil {
				t.Fatalf("disabled New = (%v, %v), want (nil, nil)", h, err)
			}
		})
	}
}

func TestHealthTrackerUnknownProber(t *testing.T) {
	cfg := trackerTestConfig()
	cfg.Probe.Type = "unknown"
	h, err := New(cfg, []string{"a"})
	if h != nil || !errors.Is(err, gerrors.ErrInvalidConfig) {
		t.Fatalf("New = (%v, %v), want (nil, ErrInvalidConfig)", h, err)
	}
}
