package backend

import (
	"API_Gateway/builder"
	gerrors "API_Gateway/pkg/errors"
	"log"
	"net"
)

type BManager struct {
	service map[string]*Service
}

func NewBManager(cfg builder.BackendConfig) *BManager {
	svc := cfg.Service
	svcMap := make(map[string]*Service)
	return &BManager{
		service: buildService(svc, svcMap),
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

	conn, err := net.Dial("tcp", svc.instances[0].addr) // 先用单实例
	if err != nil {
		return nil, err
	}
	return conn, nil
}

func buildService(svc []builder.ServiceRef, svcMap map[string]*Service) map[string]*Service {
	if svc == nil {
		return nil
	}
	for _, s := range svc {
		instances := make([]*Instance, 0, len(s.Instances))
		for _, inst := range s.Instances {
			instances = append(instances, &Instance{
				addr: inst.Addr,
			})
		}
		svcMap[s.Name] = &Service{
			name:      s.Name,
			instances: instances,
		}
	}
	return svcMap
}
