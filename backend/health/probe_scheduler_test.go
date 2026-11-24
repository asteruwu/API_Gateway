package health

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"API_Gateway/builder"
)

type fakeProber struct {
	probeFunc func(ctx context.Context, addr string) error
}

func (f *fakeProber) Type() string {
	return "fake"
}

func (f *fakeProber) Probe(ctx context.Context, addr string) error {
	return f.probeFunc(ctx, addr)
}

func TestProbeSchedulerOnProbeSuccess(t *testing.T) {
	cases := []struct {
		name               string
		successThreshold   int
		priorSuccesses     int
		wantState          State
		wantSuccessesAfter int
	}{
		{"below threshold stays probing", 2, 0, StateProbing, 1},
		{"reaches threshold becomes healthy", 2, 1, StateHealthy, 0},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			addr := "127.0.0.1:1"
			health := NewInstanceHealth([]string{addr})
			health.Write(addr, StateProbing)
			s := NewProbeScheduler([]string{addr}, health, &fakeProber{}, make(chan string),
				builder.ProbeConfig{SuccessThreshold: c.successThreshold, BaseInterval: time.Second})
			ip := s.table[addr]
			ip.successes = c.priorSuccesses

			s.onProbeSuccess(addr, ip)

			status, _ := health.Read(addr)
			if status.state != c.wantState {
				t.Errorf("state = %v, want %v", status.state, c.wantState)
			}
			if ip.successes != c.wantSuccessesAfter {
				t.Errorf("successes = %d, want %d", ip.successes, c.wantSuccessesAfter)
			}
		})
	}
}

func TestProbeSchedulerOnProbeFailure(t *testing.T) {
	cases := []struct {
		name          string
		maxFailures   int
		priorFailures int
		wantState     State
	}{
		{"below max stays probing", 3, 0, StateProbing},
		{"reaches max becomes dead", 3, 2, StateDead},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			addr := "127.0.0.1:1"
			health := NewInstanceHealth([]string{addr})
			health.Write(addr, StateProbing)
			s := NewProbeScheduler([]string{addr}, health, &fakeProber{}, make(chan string),
				builder.ProbeConfig{MaxFailures: c.maxFailures, BaseInterval: time.Millisecond, MaxInterval: time.Second})
			ip := s.table[addr]
			ip.failures = c.priorFailures

			s.onProbeFailure(addr, ip)

			status, _ := health.Read(addr)
			if status.state != c.wantState {
				t.Errorf("state = %v, want %v", status.state, c.wantState)
			}
		})
	}
}

func TestBackoffInterval(t *testing.T) {
	base := 100 * time.Millisecond
	max := time.Second
	cases := []struct {
		backoff int
		want    time.Duration
	}{
		{0, 100 * time.Millisecond},
		{1, 200 * time.Millisecond},
		{2, 400 * time.Millisecond},
		{3, 800 * time.Millisecond},
		{4, time.Second},
		{10, time.Second},
	}

	for _, c := range cases {
		t.Run(fmt.Sprintf("backoff=%d", c.backoff), func(t *testing.T) {
			got := backoffInterval(base, max, c.backoff)
			if got != c.want {
				t.Errorf("backoffInterval(%v, %v, %d) = %v, want %v", base, max, c.backoff, got, c.want)
			}
		})
	}
}

func TestApplyJitter(t *testing.T) {
	interval := 100 * time.Millisecond
	cases := []struct {
		name    string
		randVal float64
		jitter  float64
		want    time.Duration
	}{
		{"zero jitter unchanged", 0.5, 0, interval},
		{"max positive jitter", 1.0, 0.2, interval + 20*time.Millisecond},
		{"max negative jitter", 0.0, 0.2, interval - 20*time.Millisecond},
		{"negative result floors to zero", 0.0, 2.0, 0},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := applyJitter(interval, c.jitter, func() float64 { return c.randVal })
			if got != c.want {
				t.Errorf("applyJitter(%v, %v, %v) = %v, want %v", interval, c.jitter, c.randVal, got, c.want)
			}
		})
	}
}

func TestProbeSchedulerOnReporterTaskOnlyAcceptsHealthy(t *testing.T) {
	cases := []struct {
		name           string
		state          State
		wantProbeCalls int32
	}{
		{"healthy triggers probe", StateHealthy, 1},
		{"probing ignored", StateProbing, 0},
		{"dead ignored", StateDead, 0},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			addr := "127.0.0.1:1"
			health := NewInstanceHealth([]string{addr})
			if c.state != StateHealthy {
				health.Write(addr, c.state)
			}
			var probeCalls atomic.Int32
			probed := make(chan struct{})
			prober := &fakeProber{probeFunc: func(ctx context.Context, addr string) error {
				probeCalls.Add(1)
				close(probed)
				return nil
			}}
			s := NewProbeScheduler([]string{addr}, health, prober, make(chan string),
				builder.ProbeConfig{Timeout: time.Second, MaxConcurrent: 4})

			s.onReporterTask(addr)

			if c.wantProbeCalls == 1 {
				select {
				case <-probed:
				case <-time.After(time.Second):
					t.Fatal("timed out waiting for probe")
				}
			}
			if got := probeCalls.Load(); got != c.wantProbeCalls {
				t.Errorf("probeCalls = %d, want %d", got, c.wantProbeCalls)
			}
		})
	}
}

func TestProbeSchedulerStartProbeDedupesInFlight(t *testing.T) {
	addr := "127.0.0.1:1"
	health := NewInstanceHealth([]string{addr})
	var probeCalls atomic.Int32
	block := make(chan struct{})
	prober := &fakeProber{probeFunc: func(ctx context.Context, addr string) error {
		probeCalls.Add(1)
		<-block
		return nil
	}}
	s := NewProbeScheduler([]string{addr}, health, prober, make(chan string),
		builder.ProbeConfig{Timeout: time.Second, MaxConcurrent: 4})

	s.startProbe(addr)
	s.startProbe(addr)
	close(block)

	select {
	case res := <-s.results:
		if res.addr != addr {
			t.Fatalf("result addr = %q, want %q", res.addr, addr)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for probe result")
	}
	if got := probeCalls.Load(); got != 1 {
		t.Errorf("probeCalls = %d, want 1 (second startProbe must be deduped by in_flight)", got)
	}
}

func TestProbeSchedulerScanProbingSkipsDead(t *testing.T) {
	addr := "127.0.0.1:1"
	health := NewInstanceHealth([]string{addr})
	health.Write(addr, StateDead)
	var probeCalls atomic.Int32
	prober := &fakeProber{probeFunc: func(ctx context.Context, addr string) error {
		probeCalls.Add(1)
		return nil
	}}
	s := NewProbeScheduler([]string{addr}, health, prober, make(chan string),
		builder.ProbeConfig{Timeout: time.Second, MaxConcurrent: 4})

	s.scanProbing()

	if got := probeCalls.Load(); got != 0 {
		t.Errorf("probeCalls = %d, want 0 (dead instances must never be scanned)", got)
	}
}

func TestProbeSchedulerConcurrentReporterTasks(t *testing.T) {
	addrs := make([]string, 8)
	for i := range addrs {
		addrs[i] = fmt.Sprintf("127.0.0.1:%d", i+1)
	}
	health := NewInstanceHealth(addrs)
	tasks := make(chan string, 64)
	prober := &fakeProber{probeFunc: func(ctx context.Context, addr string) error {
		return nil
	}}
	s := NewProbeScheduler(addrs, health, prober, tasks, builder.ProbeConfig{
		Timeout:          time.Second,
		BaseInterval:     time.Millisecond,
		MaxInterval:      10 * time.Millisecond,
		SuccessThreshold: 2,
		MaxFailures:      3,
		MaxConcurrent:    8,
	})
	s.scanInterval = 5 * time.Millisecond
	s.Start()
	defer s.Close()

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for _, addr := range addrs {
				tasks <- addr
			}
		}()
	}
	wg.Wait()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && len(health.ReadByState(StateHealthy)) != len(addrs) {
		time.Sleep(10 * time.Millisecond)
	}
	if got := len(health.ReadByState(StateHealthy)); got != len(addrs) {
		t.Fatalf("healthy count = %d, want %d", got, len(addrs))
	}
}
