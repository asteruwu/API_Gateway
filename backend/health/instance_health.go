package health

import (
	"slices"
	"sync"
	"time"
)

type InstanceHealth struct {
	mu    sync.RWMutex
	table map[string]*instanceStatus
	stat  map[State][]*instanceStatus
}

func NewInstanceHealth(addrs []string) *InstanceHealth {
	h := &InstanceHealth{
		table: make(map[string]*instanceStatus, len(addrs)),
		stat:  make(map[State][]*instanceStatus),
	}
	now := time.Now()
	for _, addr := range addrs {
		s := &instanceStatus{addr: addr, state: StateHealthy, updatedAt: now}
		h.table[addr] = s
		h.stat[StateHealthy] = append(h.stat[StateHealthy], s)
	}
	return h
}

func (h *InstanceHealth) Read(addr string) (instanceStatus, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	s, ok := h.table[addr]
	if !ok {
		return instanceStatus{}, false
	}
	return *s, true
}

func (h *InstanceHealth) ReadByState(state State) []string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	bucket := h.stat[state]
	addrs := make([]string, len(bucket))
	for i, s := range bucket {
		addrs[i] = s.addr
	}
	return addrs
}

func (h *InstanceHealth) Write(addr string, state State) {
	h.mu.Lock()
	defer h.mu.Unlock()
	s, ok := h.table[addr]
	if !ok {
		return
	}
	if s.state == state {
		s.updatedAt = time.Now()
		return
	}
	h.moveToState(s, state)
	s.updatedAt = time.Now()
}

func (h *InstanceHealth) moveToState(s *instanceStatus, state State) {
	bucket := h.stat[s.state]
	if idx := slices.Index(bucket, s); idx >= 0 {
		h.stat[s.state] = slices.Delete(bucket, idx, idx+1)
	}
	s.state = state
	h.stat[state] = append(h.stat[state], s)
}
