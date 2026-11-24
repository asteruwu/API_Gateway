package health

import (
	"context"
	"math"
	"math/rand/v2"
	"sync"
	"sync/atomic"
	"time"

	"API_Gateway/builder"
	gconst "API_Gateway/pkg/constant"
)

// ProbeScheduler 统一处理初次确认与 probing 恢复：接收 Reporter 推送的探测任务、
// 扫描到期的 probing 实例、执行探测并把结果写回 InstanceHealth
type ProbeScheduler struct {
	health *InstanceHealth
	prober Prober

	timeout          time.Duration
	baseInterval     time.Duration
	maxInterval      time.Duration
	jitter           float64
	successThreshold int
	maxFailures      int

	tasks        <-chan string
	results      chan probeResult
	sem          chan struct{}
	randFloat    func() float64
	scanInterval time.Duration

	table map[string]*instanceProbe

	done    chan struct{}
	closed  atomic.Bool
	loopWG  sync.WaitGroup
	probeWG sync.WaitGroup
	ctx     context.Context
	cancel  context.CancelFunc
}

// instanceProbe 单个实例的探测进度
type instanceProbe struct {
	successes   int
	failures    int
	backoff     int
	nextProbeAt time.Time
	inFlight    bool
}

type probeResult struct {
	addr string
	err  error
}

func NewProbeScheduler(addrs []string, health *InstanceHealth, prober Prober, tasks <-chan string, cfg builder.ProbeConfig) *ProbeScheduler {
	table := make(map[string]*instanceProbe, len(addrs))
	for _, addr := range addrs {
		table[addr] = &instanceProbe{}
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &ProbeScheduler{
		ctx:              ctx,
		cancel:           cancel,
		health:           health,
		prober:           prober,
		timeout:          cfg.Timeout,
		baseInterval:     cfg.BaseInterval,
		maxInterval:      cfg.MaxInterval,
		jitter:           cfg.Jitter,
		successThreshold: cfg.SuccessThreshold,
		maxFailures:      cfg.MaxFailures,
		tasks:            tasks,
		results:          make(chan probeResult, cfg.MaxConcurrent),
		sem:              make(chan struct{}, cfg.MaxConcurrent),
		randFloat:        rand.Float64,
		scanInterval:     gconst.DefaultProbeScanInterval,
		table:            table,
		done:             make(chan struct{}),
	}
}

func (s *ProbeScheduler) Start() {
	if s.closed.Load() {
		return
	}
	s.loopWG.Add(1)
	go s.loop()
}

func (s *ProbeScheduler) Close() {
	if s.closed.CompareAndSwap(false, true) {
		s.cancel()
		close(s.done)
	}
	// 先停调度，避免等待期间新增探测
	s.loopWG.Wait()
	s.probeWG.Wait()
}

func (s *ProbeScheduler) loop() {
	defer s.loopWG.Done()
	ticker := time.NewTicker(s.scanInterval)
	defer ticker.Stop()
	for !s.closed.Load() {
		select {
		case <-s.done:
			return
		case addr := <-s.tasks:
			s.onReporterTask(addr)
		case <-ticker.C:
			s.scanProbing()
		case res := <-s.results:
			s.onProbeResult(res)
		}
	}
}

func (s *ProbeScheduler) onReporterTask(addr string) {
	status, ok := s.health.Read(addr)
	if !ok || status.state != StateHealthy {
		return
	}
	s.startProbe(addr)
}

func (s *ProbeScheduler) scanProbing() {
	now := time.Now()
	for _, addr := range s.health.ReadByState(StateProbing) {
		ip, ok := s.table[addr]
		if !ok || ip.inFlight || now.Before(ip.nextProbeAt) {
			continue
		}
		s.startProbe(addr)
	}
}

func (s *ProbeScheduler) startProbe(addr string) {
	if s.closed.Load() {
		return
	}
	ip, ok := s.table[addr]
	if !ok || ip.inFlight {
		return
	}
	select {
	case s.sem <- struct{}{}:
	default:
		return
	}
	ip.inFlight = true
	if status, ok := s.health.Read(addr); ok && status.state != StateProbing {
		s.health.Write(addr, StateProbing)
	}
	s.probeWG.Add(1)
	go s.runProbe(addr)
}

func (s *ProbeScheduler) runProbe(addr string) {
	defer s.probeWG.Done()
	ctx, cancel := context.WithTimeout(s.ctx, s.timeout)
	defer cancel()
	err := s.prober.Probe(ctx, addr)
	select {
	case s.results <- probeResult{addr: addr, err: err}:
	case <-s.done:
	}
}

func (s *ProbeScheduler) onProbeResult(res probeResult) {
	if s.closed.Load() {
		return
	}
	ip, ok := s.table[res.addr]
	if !ok {
		<-s.sem
		return
	}
	ip.inFlight = false
	<-s.sem
	if res.err == nil {
		s.onProbeSuccess(res.addr, ip)
		return
	}
	s.onProbeFailure(res.addr, ip)
}

func (s *ProbeScheduler) onProbeSuccess(addr string, ip *instanceProbe) {
	ip.successes++
	ip.failures = 0
	ip.backoff = 0
	if ip.successes < s.successThreshold {
		ip.nextProbeAt = time.Now().Add(s.baseInterval)
		return
	}
	s.health.Write(addr, StateHealthy)
	ip.successes = 0
	ip.nextProbeAt = time.Time{}
}

func (s *ProbeScheduler) onProbeFailure(addr string, ip *instanceProbe) {
	ip.successes = 0
	ip.failures++
	ip.backoff = ip.failures - 1
	if ip.failures < s.maxFailures {
		ip.nextProbeAt = time.Now().Add(s.nextBackoff(ip.backoff))
		return
	}
	s.health.Write(addr, StateDead)
	ip.nextProbeAt = time.Time{}
}

func (s *ProbeScheduler) nextBackoff(backoff int) time.Duration {
	interval := backoffInterval(s.baseInterval, s.maxInterval, backoff)
	return applyJitter(interval, s.jitter, s.randFloat)
}

func backoffInterval(base, max time.Duration, backoff int) time.Duration {
	scaled := float64(base) * math.Pow(2, float64(backoff))
	if scaled > float64(max) {
		return max
	}
	return time.Duration(scaled)
}

func applyJitter(interval time.Duration, jitter float64, randFloat func() float64) time.Duration {
	offset := (randFloat()*2 - 1) * jitter * float64(interval)
	jittered := time.Duration(float64(interval) + offset)
	if jittered < 0 {
		return 0
	}
	return jittered
}
