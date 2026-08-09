package backend

import (
	"API_Gateway/builder"
	"fmt"
	"io"
	"log"
	"net"
)

type BManager struct {
	service map[string]*Service
}

const defaultReadBufferSize = 4096

func NewBManager(cfg builder.BackendConfig) *BManager {
	svc := cfg.Service
	svcMap := make(map[string]*Service)
	return &BManager{
		service: buildService(svc, svcMap),
	}
}

func (b *BManager) Call(service string, payload []byte) ([]byte, error) {
	// 0. 查表
	// 1. lb 挑选
	// 2. 转发
	// 3. 返回响应
	svc := b.service[service]
	if svc == nil {
		log.Println("[backend]service not exists")
		return nil, fmt.Errorf("[backend]service not exists")
	}
	conn, err := net.Dial("tcp", svc.instances[0].addr) // 先用单实例
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	_, err = conn.Write(payload)
	if err != nil {
		return nil, err
	}
	conn.(*net.TCPConn).CloseWrite()
	resp, err := io.ReadAll(conn)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func buildService(svc []builder.ServiceConfig, svcMap map[string]*Service) map[string]*Service {
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
