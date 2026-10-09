package pool

import (
	"time"

	gconst "API_Gateway/pkg/constant"
)

// reapLoop 后台回收协程，生命周期随池创建/关闭
func (p *ConnPool) reapLoop() {
	ticker := time.NewTicker(p.reapInterval)
	defer ticker.Stop()
	for {
		select {
		case <-p.done:
			return
		case <-ticker.C:
		}
		p.reapOnce(time.Now())
	}
}

// reapOnce 一轮回收：摘除超时活跃 → 收敛过期空闲 → 统一关闭 → 按余量预热
func (p *ConnPool) reapOnce(now time.Time) {
	expiredActive := p.reapExpiredActive(now)
	expiredIdle := p.reapExpiredIdle(now)

	for _, pc := range expiredActive {
		p.discard(pc)
	}
	for _, pc := range expiredIdle {
		p.discard(pc)
	}
	for i := 0; i < p.warmDeficit(); i++ {
		if !p.dialIdle() {
			break
		}
	}
}

// reapExpiredActive 锁内摘除借出超时的活跃连接，返回后由调用方在锁外 discard
func (p *ConnPool) reapExpiredActive(now time.Time) []*PoolConn {
	p.mu.Lock()
	defer p.mu.Unlock()
	var expired []*PoolConn
	for pc := range p.active {
		if now.Sub(pc.borrowedAt) > p.borrowTimeout {
			delete(p.active, pc)
			expired = append(expired, pc)
		}
	}
	return expired
}

// reapExpiredIdle 锁内摘除空闲过期连接（不低于 MinIdle），返回后由调用方在锁外 discard
func (p *ConnPool) reapExpiredIdle(now time.Time) []*PoolConn {
	p.mu.Lock()
	defer p.mu.Unlock()
	removable := len(p.idle) - p.minIdle
	if removable <= 0 {
		return nil
	}
	var count int
	for count < removable && count < len(p.idle) {
		if now.Sub(p.idle[count].idleSince) <= p.idleTimeout {
			break
		}
		count++
	}
	if count == 0 {
		return nil
	}
	expired := make([]*PoolConn, count)
	copy(expired, p.idle[:count])
	p.idle = append(p.idle[:0], p.idle[count:]...)
	return expired
}

// warmDeficit 距 MinIdle 还差几个空闲连接
func (p *ConnPool) warmDeficit() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return 0
	}
	if deficit := p.minIdle - len(p.idle); deficit > 0 {
		return deficit
	}
	return 0
}

// dialIdle 预热一条空闲连接
func (p *ConnPool) dialIdle() bool {
	if !p.tryAcquireSlot() {
		return false
	}
	p.mu.Lock()
	if p.closed || len(p.idle) >= p.minIdle {
		p.mu.Unlock()
		p.releaseSlot()
		return false
	}
	p.mu.Unlock()

	conn, err := p.dial()
	if err != nil {
		p.releaseSlot()
		return false
	}
	pc := &PoolConn{Conn: conn, pool: p}
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		p.discard(pc)
		return false
	}
	pc.idleSince = time.Now()
	p.idle = append(p.idle, pc)
	p.mu.Unlock()
	return true
}

func calcReapInterval(borrow, idle time.Duration) time.Duration {
	shortest := borrow
	if idle < shortest {
		shortest = idle
	}
	interval := shortest / 2
	switch {
	case interval < gconst.DefaultPoolReapIntervalMin:
		return gconst.DefaultPoolReapIntervalMin
	case interval > gconst.DefaultPoolReapIntervalMax:
		return gconst.DefaultPoolReapIntervalMax
	default:
		return interval
	}
}
