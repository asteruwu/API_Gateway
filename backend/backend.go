package backend

import (
	"log"
	"net"

	"API_Gateway/builder"
	gerrors "API_Gateway/pkg/errors"
)

type BManager struct {
	service map[string]*Service
}

func NewBManager(cfg builder.BackendConfig) (*BManager, error) {
	services, err := buildServices(cfg.Service, cfg.Pool)
	if err != nil {
		return nil, err
	}
	return &BManager{service: services}, nil
}

func (b *BManager) Call(service string) (net.Conn, func(error), error) {
	svc, ok := b.service[service]
	if !ok {
		log.Println("[backend]service not exists")
		return nil, nil, gerrors.ErrServiceNotFound
	}

	inst, err := svc.pick()
	if err != nil {
		return nil, nil, err
	}

	pc, err := inst.pool.Get()
	if err != nil {
		return nil, nil, err
	}

	release := func(err error) {
		if err != nil {
			_ = pc.Close()
			return
		}
		inst.pool.Put(pc)
	}
	return pc, release, nil
}

func (b *BManager) Close() error {
	return closeServices(b.service)
}
