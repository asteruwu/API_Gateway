package backend

import (
	"API_Gateway/builder"
)

type BManager struct {
	service map[string]*Service
}

func NewBManager(cfg builder.BackendConfig) *BManager {
	return &BManager{}
}

func (b *BManager) Call(service string, payload []byte) ([]byte, error) {
	// 0. 查表
	// 1. lb 挑选
	// 2. 转发
	// 3. 返回响应
	return []byte{}, nil
}
