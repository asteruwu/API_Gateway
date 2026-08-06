package backend

// 负责挑选
type LoadBalancer interface {
	Pick(candidates []*Instance) (*Instance, error)
}

// 轮询
// 最小连接
// 随机
// 权重
// ...
