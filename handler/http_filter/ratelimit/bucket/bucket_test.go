package bucket

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestTokenBucketExhaustsBurst(t *testing.T) {
	now := time.Unix(0, 0)
	b := NewTokenBucket(2, 2, now)

	for i := 0; i < 2; i++ {
		if ok, _ := b.Allow(now); !ok {
			t.Fatalf("request %d denied, want allowed", i)
		}
	}

	ok, retryAfter := b.Allow(now)
	if ok {
		t.Fatal("request after burst allowed, want denied")
	}
	if want := 500 * time.Millisecond; retryAfter != want {
		t.Errorf("retryAfter = %v, want %v", retryAfter, want)
	}
}

func TestTokenBucketRefillsOverTime(t *testing.T) {
	base := time.Unix(0, 0)
	b := NewTokenBucket(2, 1, base)

	if ok, _ := b.Allow(base); !ok {
		t.Fatal("initial request denied, want allowed")
	}

	half := base.Add(250 * time.Millisecond)
	ok, retryAfter := b.Allow(half)
	if ok {
		t.Fatal("request at half token allowed, want denied")
	}
	if want := 250 * time.Millisecond; retryAfter != want {
		t.Errorf("retryAfter = %v, want %v", retryAfter, want)
	}

	full := base.Add(500 * time.Millisecond)
	if ok, _ := b.Allow(full); !ok {
		t.Fatal("request after one token refilled denied, want allowed")
	}
	if ok, _ := b.Allow(full); ok {
		t.Fatal("request after consuming refilled token allowed, want denied")
	}
}

func TestTokenBucketCapsAtBurst(t *testing.T) {
	base := time.Unix(0, 0)
	b := NewTokenBucket(2, 2, base)
	later := base.Add(time.Hour)

	for i := 0; i < 2; i++ {
		if ok, _ := b.Allow(later); !ok {
			t.Fatalf("request %d denied, want allowed", i)
		}
	}
	if ok, _ := b.Allow(later); ok {
		t.Fatal("request beyond burst allowed, want denied")
	}
}

func TestTokenBucketIgnoresBackwardClock(t *testing.T) {
	base := time.Unix(0, 0)
	b := NewTokenBucket(2, 1, base)

	if ok, _ := b.Allow(base); !ok {
		t.Fatal("initial request denied, want allowed")
	}
	if ok, _ := b.Allow(base.Add(-time.Second)); ok {
		t.Fatal("request with backward clock allowed, want denied")
	}
	if ok, _ := b.Allow(base.Add(500 * time.Millisecond)); !ok {
		t.Fatal("request after clock catches up denied, want allowed")
	}
}

func TestTokenBucketConcurrent(t *testing.T) {
	const burst = 100
	base := time.Unix(0, 0)
	b := NewTokenBucket(1, burst, base)

	var allowed atomic.Int64
	var wg sync.WaitGroup
	start := make(chan struct{})

	for i := 0; i < burst*2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if ok, _ := b.Allow(base); ok {
				allowed.Add(1)
			}
		}()
	}

	close(start)
	wg.Wait()

	if got := allowed.Load(); got != burst {
		t.Errorf("allowed = %d, want %d", got, burst)
	}
}
