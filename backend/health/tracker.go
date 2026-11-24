package health

import (
	"API_Gateway/builder"
	gconst "API_Gateway/pkg/constant"
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
	cfg = withDefaults(cfg)
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

func withDefaults(cfg builder.HealthConfig) builder.HealthConfig {
	if !cfg.Enabled {
		return cfg
	}
	if cfg.Reporter.Window == 0 {
		cfg.Reporter.Window = gconst.DefaultHealthReporterWindow
	}
	if cfg.Reporter.ErrorRate == 0 {
		cfg.Reporter.ErrorRate = gconst.DefaultHealthReporterErrorRate
	}
	if cfg.Reporter.MinSamples == 0 {
		cfg.Reporter.MinSamples = gconst.DefaultHealthReporterMinSamples
	}
	if cfg.Probe.Type == "" {
		cfg.Probe.Type = gconst.DefaultHealthProbeType
	}
	if cfg.Probe.Timeout == 0 {
		cfg.Probe.Timeout = gconst.DefaultHealthProbeTimeout
	}
	if cfg.Probe.BaseInterval == 0 {
		cfg.Probe.BaseInterval = gconst.DefaultHealthProbeBaseInterval
	}
	if cfg.Probe.MaxInterval == 0 {
		cfg.Probe.MaxInterval = gconst.DefaultHealthProbeMaxInterval
	}
	if cfg.Probe.Jitter == 0 {
		cfg.Probe.Jitter = gconst.DefaultHealthProbeJitter
	}
	if cfg.Probe.SuccessThreshold == 0 {
		cfg.Probe.SuccessThreshold = gconst.DefaultHealthProbeSuccessThreshold
	}
	if cfg.Probe.MaxFailures == 0 {
		cfg.Probe.MaxFailures = gconst.DefaultHealthProbeMaxFailures
	}
	if cfg.Probe.MaxConcurrent == 0 {
		cfg.Probe.MaxConcurrent = gconst.DefaultHealthProbeMaxConcurrent
	}
	return cfg
}
