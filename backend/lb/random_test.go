package lb

import (
	"errors"
	"fmt"
	"sync"
	"testing"

	gconst "API_Gateway/pkg/constant"
	gerrors "API_Gateway/pkg/errors"
)

func TestRandomPick(t *testing.T) {
	candidates := []Candidate{
		testCandidate{addr: "127.0.0.1:8081"},
		testCandidate{addr: "127.0.0.1:8082"},
		testCandidate{addr: "127.0.0.1:8083"},
	}
	cases := []struct {
		name       string
		candidates []Candidate
		wantN      int
		want       int
		wantErr    error
	}{
		{name: "single", candidates: candidates[:1], wantN: 1, want: 0},
		{name: "injected", candidates: candidates, wantN: 3, want: 2},
		{name: "empty", wantErr: gerrors.ErrNoInstance},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			lb := &random{intN: func(n int) int {
				called = true
				if n != tc.wantN {
					t.Errorf("intN(%d), want %d", n, tc.wantN)
				}
				return tc.want
			}}

			got, err := lb.Pick(tc.candidates)
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("err = %v, want %v", err, tc.wantErr)
			}
			if err == nil {
				if !called {
					t.Error("intN was not called")
				}
				if got != tc.want {
					t.Errorf("index = %d, want %d", got, tc.want)
				}
				return
			}
			if called {
				t.Error("intN was called for empty candidates")
			}
		})
	}
}

func TestRandomPickSingleRepeated(t *testing.T) {
	candidates := []Candidate{testCandidate{addr: "127.0.0.1:8081"}}
	lb := newRandom()

	for i := 0; i < 10; i++ {
		got, err := lb.Pick(candidates)
		if err != nil {
			t.Fatalf("pick %d: %v", i, err)
		}
		if got != 0 {
			t.Errorf("pick %d = %d, want 0", i, got)
		}
	}
}

func TestRandomPickConcurrent(t *testing.T) {
	candidates := []Candidate{
		testCandidate{addr: "127.0.0.1:8081"},
		testCandidate{addr: "127.0.0.1:8082"},
		testCandidate{addr: "127.0.0.1:8083"},
	}
	const (
		goroutines = 16
		picks      = 100
	)

	lb := newRandom()
	errCh := make(chan error, goroutines)
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < picks; j++ {
				index, err := lb.Pick(candidates)
				if err != nil {
					errCh <- err
					return
				}
				if index < 0 || index >= len(candidates) {
					errCh <- fmt.Errorf("index %d out of range", index)
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Error(err)
	}
}

func TestNewRandom(t *testing.T) {
	balancer, err := New(gconst.LBPolicyRandom)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if balancer == nil {
		t.Fatal("balancer = nil, want random")
	}
}
