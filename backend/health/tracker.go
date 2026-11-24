package health

import (
	"API_Gateway/builder"
)

type HealthTracker struct {
	health    *InstanceHealth
	reporter  *Reporter
	scheduler *ProbeScheduler
}

func New(cfg builder.HealthConfig, addrs []string) (*HealthTracker, error) {
	if !cfg.Enabled {
		return nil, nil
	}
	prober, err := NewProber(cfg.Probe.Type)
	if err != nil {
		return nil, err
	}
	health := NewInstanceHealth(addrs)
	reporter := NewReporter(addrs, cfg.Reporter)
	scheduler := NewProbeScheduler(addrs, health, prober, reporter.Tasks(), cfg.Probe)
	return &HealthTracker{
		health:    health,
		reporter:  reporter,
		scheduler: scheduler,
	}, nil
}

func (h *HealthTracker) Start() {
	h.scheduler.Start()
	h.reporter.Start()
}

func (h *HealthTracker) Close() error {
	h.reporter.Close()
	h.scheduler.Close()
	return nil
}

func (h *HealthTracker) Snapshot() []string {
	return h.health.ReadByState(StateHealthy)
}
