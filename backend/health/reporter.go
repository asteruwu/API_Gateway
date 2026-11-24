package health

import (
	"sync"
	"sync/atomic"
	"time"

	"API_Gateway/builder"
	gconst "API_Gateway/pkg/constant"
)

type Reporter struct {
	table      map[string]*instanceError
	window     time.Duration
	errorRate  float64
	minSamples int
	tasks      chan string
	done       chan struct{}
	closed     atomic.Bool
	loopWG     sync.WaitGroup
}

type instanceError struct {
	totalTimes atomic.Int64
	errTimes   atomic.Int64
}

func NewReporter(addrs []string, cfg builder.ReporterConfig) *Reporter {
	table := make(map[string]*instanceError, len(addrs))
	for _, addr := range addrs {
		table[addr] = &instanceError{}
	}
	return &Reporter{
		table:      table,
		window:     cfg.Window,
		errorRate:  cfg.ErrorRate,
		minSamples: cfg.MinSamples,
		tasks:      make(chan string, gconst.DefaultReporterTaskQueueSize),
		done:       make(chan struct{}),
	}
}

func (r *Reporter) Tasks() <-chan string {
	return r.tasks
}

func (r *Reporter) Start() {
	if r.closed.Load() {
		return
	}
	r.loopWG.Add(1)
	go r.loop()
}

func (r *Reporter) Close() {
	if r.closed.CompareAndSwap(false, true) {
		close(r.done)
	}
	r.loopWG.Wait()
}

func (r *Reporter) loop() {
	defer r.loopWG.Done()
	ticker := time.NewTicker(r.window)
	defer ticker.Stop()
	for !r.closed.Load() {
		select {
		case <-r.done:
			return
		case <-ticker.C:
		}
		for _, addr := range r.tick() {
			r.pushTask(addr)
		}
	}
}

func (r *Reporter) pushTask(addr string) {
	if r.closed.Load() {
		return
	}
	select {
	case r.tasks <- addr:
	default:
	}
}

func (r *Reporter) Report(e Event) {
	if r.closed.Load() {
		return
	}
	ie, ok := r.table[e.Addr]
	if !ok {
		return
	}
	ie.totalTimes.Add(1)
	if !e.Result {
		ie.errTimes.Add(1)
	}
}

func (r *Reporter) tick() []string {
	var triggered []string
	for addr, ie := range r.table {
		total := ie.totalTimes.Swap(0)
		errs := ie.errTimes.Swap(0)
		if total == 0 || total < int64(r.minSamples) {
			continue
		}
		if float64(errs)/float64(total) >= r.errorRate {
			triggered = append(triggered, addr)
		}
	}
	return triggered
}
