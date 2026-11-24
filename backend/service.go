package backend

import (
	"API_Gateway/backend/health"
	"API_Gateway/backend/lb"
	cp "API_Gateway/backend/pool"
	"API_Gateway/builder"
	gerrors "API_Gateway/pkg/errors"
	"errors"
	"fmt"
)

func buildServices(refs []builder.ServiceRef, poolCfg builder.PoolConfig) (map[string]*Service, error) {
	if refs == nil {
		return nil, nil
	}
	services := make(map[string]*Service, len(refs))
	for _, ref := range refs {
		svc, err := buildService(ref, poolCfg)
		if err != nil {
			return nil, errors.Join(err, closeServices(services))
		}
		services[svc.name] = svc
	}
	return services, nil
}

func buildService(ref builder.ServiceRef, poolCfg builder.PoolConfig) (*Service, error) {
	balancer, err := lb.New(ref.LB)
	if err != nil {
		return nil, fmt.Errorf("service %q: %w", ref.Name, err)
	}
	instances := buildInstances(ref.Instances, poolCfg)
	tracker, err := buildHealthTracker(ref.Name, instances, ref.Health)
	if err != nil {
		return nil, errors.Join(err, closeInstances(instances))
	}
	if tracker != nil {
		tracker.Start()
	}
	return &Service{
		name:      ref.Name,
		instances: instances,
		balancer:  balancer,
		tracker:   tracker,
	}, nil
}

func buildInstances(refs []builder.InstanceConfig, poolCfg builder.PoolConfig) []*Instance {
	instances := make([]*Instance, 0, len(refs))
	for _, ref := range refs {
		instances = append(instances, &Instance{
			addr: ref.Addr,
			pool: cp.NewConnPool(ref.Addr, poolCfg),
		})
	}
	return instances
}

func buildHealthTracker(service string, ins []*Instance, healthCfg builder.HealthConfig) (*health.HealthTracker, error) {
	addrs := make([]string, 0, len(ins))
	for _, i := range ins {
		addrs = append(addrs, i.addr)
	}
	tracker, err := health.New(healthCfg, addrs)
	if err != nil {
		return nil, fmt.Errorf("service %q: %w", service, err)
	}
	return tracker, nil
}

func closeServices(services map[string]*Service) error {
	var errs []error
	for _, svc := range services {
		if svc.tracker != nil {
			errs = append(errs, svc.tracker.Close())
		}
		errs = append(errs, closeInstances(svc.instances))
	}
	return errors.Join(errs...)
}

func closeInstances(instances []*Instance) error {
	var errs []error
	for _, inst := range instances {
		if inst.pool != nil {
			errs = append(errs, inst.pool.Close())
		}
	}
	return errors.Join(errs...)
}

func (s *Service) pick() (*Instance, error) {
	if len(s.instances) == 0 {
		return nil, gerrors.ErrNoInstance
	}

	candidates := make([]lb.Candidate, len(s.instances))
	for i, inst := range s.instances {
		candidates[i] = inst
	}

	index, err := s.balancer.Pick(candidates)
	if err != nil {
		return nil, err
	}
	if index < 0 || index >= len(s.instances) {
		return nil, fmt.Errorf("%w: lb returned invalid index %d", gerrors.ErrInternal, index)
	}
	return s.instances[index], nil
}
