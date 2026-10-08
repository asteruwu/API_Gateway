package backend

import (
	"API_Gateway/builder"
	gerrors "API_Gateway/pkg/errors"
	"errors"
	"log"
	"net"
)

type BManager struct {
	service map[string]*Service
}

func NewBManager(cfg builder.BackendConfig) *BManager {
	svcMap := make(map[string]*Service)
	return &BManager{
		service: buildService(cfg.Service, cfg.Pool, svcMap),
	}
}

func (b *BManager) Call(service string) (net.Conn, error) {
	// 0. 查表
	// 1. lb 挑选实例
	// 2. 连接池选取连接
	// 3. 返回
	var svc *Service
	if s, ok := b.service[service]; !ok {
		log.Println("[backend]service not exists")
		return nil, gerrors.ErrServiceNotFound
	} else {
		svc = s
	}

	if len(svc.instances) == 0 {
		return nil, gerrors.ErrNoInstance
	}

	conn, err := svc.instances[0].pool.Get() // 先用单实例
	if err != nil {
		return nil, err
	}
	return conn, nil
}

func (b *BManager) Close() error {
	var errs []error
	for _, svc := range b.service {
		for _, inst := range svc.instances {
			if inst.pool != nil {
				errs = append(errs, inst.pool.Close())
			}
		}
	}
	return errors.Join(errs...)
}

func buildService(svc []builder.ServiceRef, poolCfg builder.PoolConfig, svcMap map[string]*Service) map[string]*Service {
	if svc == nil {
		return nil
	}
	for _, s := range svc {
		instances := make([]*Instance, 0, len(s.Instances))
		for _, inst := range s.Instances {
			instances = append(instances, &Instance{
				addr: inst.Addr,
				pool: NewConnPool(inst.Addr, poolCfg),
			})
		}
		svcMap[s.Name] = &Service{
			name:      s.Name,
			instances: instances,
		}
	}
	return svcMap
}
