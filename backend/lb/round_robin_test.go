package lb

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	gerrors "API_Gateway/pkg/errors"
)

type testCandidate struct {
	addr string
}

func (c testCandidate) Addr() string {
	return c.addr
}

func TestRoundRobinPick(t *testing.T) {
	candidates := []Candidate{
		testCandidate{addr: "127.0.0.1:8081"},
		testCandidate{addr: "127.0.0.1:8082"},
		testCandidate{addr: "127.0.0.1:8083"},
	}
	cases := []struct {
		name       string
		candidates []Candidate
		want       []int
		wantErr    error
	}{
		{name: "cycles", candidates: candidates, want: []int{0, 1, 2, 0}},
		{name: "single", candidates: candidates[:1], want: []int{0, 0, 0}},
		{name: "empty", candidates: nil, wantErr: gerrors.ErrNoInstance},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lb := newRoundRobin()
			if tc.wantErr != nil {
				_, err := lb.Pick(tc.candidates)
				if !errors.Is(err, tc.wantErr) {
					t.Errorf("err = %v, want %v", err, tc.wantErr)
				}
				return
			}
			for i, want := range tc.want {
				got, err := lb.Pick(tc.candidates)
				if err != nil {
					t.Fatalf("pick %d: %v", i, err)
				}
				if got != want {
					t.Errorf("pick %d = %d, want %d", i, got, want)
				}
			}
		})
	}
}

func TestRoundRobinPickConcurrent(t *testing.T) {
	candidates := []Candidate{
		testCandidate{addr: "127.0.0.1:8081"},
		testCandidate{addr: "127.0.0.1:8082"},
		testCandidate{addr: "127.0.0.1:8083"},
	}
	const (
		goroutines        = 9
		picksPerGoroutine = 100
	)

	lb := newRoundRobin()
	counts := make([]atomic.Int64, len(candidates))
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < picksPerGoroutine; j++ {
				index, err := lb.Pick(candidates)
				if err != nil {
					t.Errorf("pick: %v", err)
					return
				}
				counts[index].Add(1)
			}
		}()
	}
	wg.Wait()

	wantPerCandidate := int64(goroutines * picksPerGoroutine / len(candidates))
	var total int64
	for i := range counts {
		got := counts[i].Load()
		if got != wantPerCandidate {
			t.Errorf("candidate %d picks = %d, want %d", i, got, wantPerCandidate)
		}
		total += got
	}
	if wantTotal := int64(goroutines * picksPerGoroutine); total != wantTotal {
		t.Errorf("total picks = %d, want %d", total, wantTotal)
	}
}
