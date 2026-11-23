package health

import (
	"testing"
	"time"

	"API_Gateway/builder"
)

func reportN(r *Reporter, addr string, fails, successes int) {
	for i := 0; i < fails; i++ {
		r.Report(Event{Result: false, Addr: addr})
	}
	for i := 0; i < successes; i++ {
		r.Report(Event{Result: true, Addr: addr})
	}
}

func TestReporterTick(t *testing.T) {
	cases := []struct {
		name          string
		fails         int
		successes     int
		errorRate     float64
		minSamples    int
		wantTriggered bool
	}{
		{"below min samples not triggered", 1, 0, 0.5, 5, false},
		{"error rate reaches threshold triggers", 3, 2, 0.5, 5, true},
		{"success dilutes error rate", 3, 7, 0.5, 5, false},
		{"no requests not triggered", 0, 0, 0.5, 1, false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := NewReporter([]string{"a"}, builder.ReporterConfig{
				Window:     time.Hour,
				ErrorRate:  c.errorRate,
				MinSamples: c.minSamples,
			})
			reportN(r, "a", c.fails, c.successes)

			triggered := r.tick()
			got := len(triggered) == 1 && triggered[0] == "a"
			if got != c.wantTriggered {
				t.Errorf("tick() = %v, want triggered=%v", triggered, c.wantTriggered)
			}
		})
	}
}

func TestReporterTickClearsCounters(t *testing.T) {
	r := NewReporter([]string{"a"}, builder.ReporterConfig{
		Window:     time.Hour,
		ErrorRate:  0.5,
		MinSamples: 1,
	})
	reportN(r, "a", 3, 2)

	if triggered := r.tick(); len(triggered) != 1 {
		t.Fatalf("first tick() = %v, want 1 triggered addr", triggered)
	}

	if triggered := r.tick(); len(triggered) != 0 {
		t.Errorf("second tick() without new events = %v, want empty (counters must be cleared)", triggered)
	}
}

func TestReporterPushTaskDropsWhenQueueFull(t *testing.T) {
	r := NewReporter(nil, builder.ReporterConfig{Window: time.Hour, ErrorRate: 0.5, MinSamples: 1})

	cap := cap(r.tasks)
	for i := 0; i < cap; i++ {
		r.pushTask("filler")
	}
	r.pushTask("overflow")

	if got := len(r.tasks); got != cap {
		t.Fatalf("queue length = %d, want %d (overflow must be dropped)", got, cap)
	}
	for i := 0; i < cap; i++ {
		if addr := <-r.tasks; addr != "filler" {
			t.Errorf("drained addr = %q, want %q", addr, "filler")
		}
	}
}

func TestReporterStartPushesTaskOnWindow(t *testing.T) {
	r := NewReporter([]string{"a"}, builder.ReporterConfig{
		Window:     5 * time.Millisecond,
		ErrorRate:  0.5,
		MinSamples: 1,
	})
	r.Start()
	defer r.Close()

	r.Report(Event{Result: false, Addr: "a"})

	select {
	case addr := <-r.Tasks():
		if addr != "a" {
			t.Errorf("got %q, want %q", addr, "a")
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timed out waiting for probe task")
	}
}

func TestReporterCloseIdempotent(t *testing.T) {
	r := NewReporter([]string{"a"}, builder.ReporterConfig{
		Window:     time.Hour,
		ErrorRate:  0.5,
		MinSamples: 1,
	})
	r.Start()
	r.Close()
	r.Close()
}
