package health

import (
	"slices"
	"sync"
	"testing"
)

func newTestInstanceHealth(t *testing.T, addrs ...string) *InstanceHealth {
	t.Helper()
	return NewInstanceHealth(addrs)
}

func TestNewInstanceHealthInitializesHealthy(t *testing.T) {
	addrs := []string{"127.0.0.1:1", "127.0.0.1:2", "127.0.0.1:3"}
	h := newTestInstanceHealth(t, addrs...)

	got := h.ReadByState(StateHealthy)
	slices.Sort(got)
	want := slices.Clone(addrs)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("ReadByState(StateHealthy) = %v, want %v", got, want)
	}
	if got := h.ReadByState(StateProbing); len(got) != 0 {
		t.Errorf("ReadByState(StateProbing) = %v, want empty", got)
	}
}

func TestInstanceHealthRead(t *testing.T) {
	h := newTestInstanceHealth(t, "127.0.0.1:1")

	cases := []struct {
		name      string
		addr      string
		wantState State
		wantOk    bool
	}{
		{"known addr returns healthy", "127.0.0.1:1", StateHealthy, true},
		{"unknown addr not found", "127.0.0.1:9", StateHealthy, false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := h.Read(c.addr)
			if ok != c.wantOk {
				t.Fatalf("ok = %v, want %v", ok, c.wantOk)
			}
			if ok && got.state != c.wantState {
				t.Errorf("state = %v, want %v", got.state, c.wantState)
			}
		})
	}
}

func TestInstanceHealthReadReturnsCopy(t *testing.T) {
	h := newTestInstanceHealth(t, "127.0.0.1:1")

	got, _ := h.Read("127.0.0.1:1")
	got.state = StateDead

	again, _ := h.Read("127.0.0.1:1")
	if again.state != StateHealthy {
		t.Errorf("state = %v, want %v (Read must return a copy)", again.state, StateHealthy)
	}
}

func TestInstanceHealthWrite(t *testing.T) {
	cases := []struct {
		name        string
		addr        string
		state       State
		wantHealthy []string
		wantProbing []string
	}{
		{
			name:        "move healthy to probing",
			addr:        "127.0.0.1:1",
			state:       StateProbing,
			wantHealthy: []string{"127.0.0.1:2"},
			wantProbing: []string{"127.0.0.1:1"},
		},
		{
			name:        "write same state is no-op",
			addr:        "127.0.0.1:1",
			state:       StateHealthy,
			wantHealthy: []string{"127.0.0.1:1", "127.0.0.1:2"},
			wantProbing: nil,
		},
		{
			name:        "unknown addr is ignored",
			addr:        "127.0.0.1:9",
			state:       StateDead,
			wantHealthy: []string{"127.0.0.1:1", "127.0.0.1:2"},
			wantProbing: nil,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newTestInstanceHealth(t, "127.0.0.1:1", "127.0.0.1:2")

			h.Write(c.addr, c.state)

			gotHealthy := h.ReadByState(StateHealthy)
			slices.Sort(gotHealthy)
			wantHealthy := slices.Clone(c.wantHealthy)
			slices.Sort(wantHealthy)
			if !slices.Equal(gotHealthy, wantHealthy) {
				t.Errorf("ReadByState(StateHealthy) = %v, want %v", gotHealthy, wantHealthy)
			}

			gotProbing := h.ReadByState(StateProbing)
			if !slices.Equal(gotProbing, c.wantProbing) {
				t.Errorf("ReadByState(StateProbing) = %v, want %v", gotProbing, c.wantProbing)
			}
		})
	}
}

func TestInstanceHealthWriteKeepsIndexConsistent(t *testing.T) {
	h := newTestInstanceHealth(t, "127.0.0.1:1", "127.0.0.1:2")

	h.Write("127.0.0.1:1", StateProbing)
	h.Write("127.0.0.1:1", StateDead)

	total := len(h.ReadByState(StateHealthy)) + len(h.ReadByState(StateProbing)) + len(h.ReadByState(StateDead))
	if total != 2 {
		t.Errorf("total indexed addrs = %d, want 2", total)
	}
	if got := h.ReadByState(StateProbing); len(got) != 0 {
		t.Errorf("ReadByState(StateProbing) = %v, want empty after moving to dead", got)
	}
}

func TestInstanceHealthConcurrentReadWrite(t *testing.T) {
	addrs := []string{"127.0.0.1:1", "127.0.0.1:2", "127.0.0.1:3", "127.0.0.1:4"}
	h := newTestInstanceHealth(t, addrs...)

	states := []State{StateHealthy, StateProbing, StateDead}
	const iterations = 200

	var wg sync.WaitGroup
	for _, addr := range addrs {
		wg.Add(1)
		go func(addr string) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				h.Write(addr, states[i%len(states)])
			}
		}(addr)
	}
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				_, _ = h.Read(addrs[j%len(addrs)])
				_ = h.ReadByState(states[j%len(states)])
			}
		}(i)
	}
	wg.Wait()

	seen := make(map[string]int)
	for _, state := range states {
		for _, addr := range h.ReadByState(state) {
			seen[addr]++
		}
	}
	if len(seen) != len(addrs) {
		t.Fatalf("indexed addrs = %d, want %d: %v", len(seen), len(addrs), seen)
	}
	for addr, count := range seen {
		if count != 1 {
			t.Errorf("addr %q appears in %d state buckets, want 1", addr, count)
		}
	}
}
