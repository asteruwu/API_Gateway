package pool

import (
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"API_Gateway/builder"
	gconst "API_Gateway/pkg/constant"
	gerrors "API_Gateway/pkg/errors"
)

// ConnPool 单个后端实例的连接池
type ConnPool struct {
	addr string

	maxConn       int
	minIdle       int
	dialTimeout   time.Duration
	borrowTimeout time.Duration
	idleTimeout   time.Duration
	waitTimeout   time.Duration
	reapInterval  time.Duration

	mu     sync.Mutex
	cond   *sync.Cond
	idle   []*PoolConn // LIFO
	active map[*PoolConn]struct{}
	slots  chan struct{} // 令牌 = 剩余可新建连接额度（容量 maxConn）
	closed bool
	done   chan struct{}
}

// PoolConn 池借出的连接，对外表现为标准 net.Conn
type PoolConn struct {
	net.Conn
	pool       *ConnPool
	idleSince  time.Time
	borrowedAt time.Time
	closeOnce  sync.Once
	closedFlag atomic.Bool
}

func NewConnPool(addr string, cfg builder.PoolConfig) *ConnPool {
	cfg = withPoolDefaults(cfg)
	p := &ConnPool{
		addr:          addr,
		maxConn:       cfg.MaxConn,
		minIdle:       cfg.MinIdle,
		dialTimeout:   cfg.DialTimeout,
		borrowTimeout: cfg.BorrowTimeout,
		idleTimeout:   cfg.IdleTimeout,
		waitTimeout:   cfg.WaitTimeout,
		reapInterval:  calcReapInterval(cfg.BorrowTimeout, cfg.IdleTimeout),
		idle:          make([]*PoolConn, 0, cfg.MinIdle),
		active:        make(map[*PoolConn]struct{}),
		slots:         make(chan struct{}, cfg.MaxConn),
		done:          make(chan struct{}),
	}
	p.cond = sync.NewCond(&p.mu)
	for i := 0; i < cfg.MaxConn; i++ {
		p.slots <- struct{}{}
	}
	go p.reapLoop()
	return p
}

// Get 借出连接：优先复用空闲连接（复用前做失效探测），否则取令牌新建；
// 池满时阻塞等待，超过 WaitTimeout 返回 ErrPoolExhausted。
func (p *ConnPool) Get() (*PoolConn, error) {
	deadline := time.Now().Add(p.waitTimeout)

	p.mu.Lock()
	for {
		if p.closed {
			p.mu.Unlock()
			return nil, gerrors.ErrPoolClosed
		}
		if n := len(p.idle); n > 0 {
			pc := p.idle[n-1]
			p.idle = p.idle[:n-1]
			pc.idleSince = time.Time{}
			p.mu.Unlock()

			if alive(pc.Conn) {
				p.mu.Lock()
				if p.closed {
					p.mu.Unlock()
					p.discard(pc)
					return nil, gerrors.ErrPoolClosed
				}
				pc.borrowedAt = time.Now()
				p.active[pc] = struct{}{}
				p.mu.Unlock()
				return pc, nil
			}
			p.discard(pc)
			p.mu.Lock()
			continue
		}

		// 尝试非阻塞取令牌新建
		if p.tryAcquireSlot() {
			p.mu.Unlock()
			conn, err := p.dial()
			if err != nil {
				p.releaseSlot()
				return nil, err
			}
			pc := &PoolConn{Conn: conn, pool: p}
			p.mu.Lock()
			if p.closed {
				p.mu.Unlock()
				p.discard(pc)
				return nil, gerrors.ErrPoolClosed
			}
			pc.borrowedAt = time.Now()
			p.active[pc] = struct{}{}
			p.mu.Unlock()
			return pc, nil
		}

		// 池满，等待归还或超时
		remaining := time.Until(deadline)
		if remaining <= 0 {
			p.mu.Unlock()
			return nil, gerrors.ErrPoolExhausted
		}
		timer := time.AfterFunc(remaining, func() {
			p.mu.Lock()
			p.cond.Broadcast()
			p.mu.Unlock()
		})
		p.cond.Wait()
		timer.Stop()
	}
}

// Put 归还连接：active → idle，广播唤醒等待中的 Get
func (p *ConnPool) Put(pc *PoolConn) {
	if pc == nil || pc.closedFlag.Load() {
		return
	}
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		p.discard(pc)
		return
	}
	if _, ok := p.active[pc]; !ok {
		p.mu.Unlock()
		return
	}
	delete(p.active, pc)
	pc.borrowedAt = time.Time{}
	pc.idleSince = time.Now()
	p.idle = append(p.idle, pc)
	p.mu.Unlock()
	p.cond.Broadcast()
}

// discard 池内唯一的关闭路径：幂等关 socket、摘除引用、归还令牌、广播通知
func (p *ConnPool) discard(pc *PoolConn) {
	if pc == nil {
		return
	}
	pc.closeOnce.Do(func() {
		pc.closedFlag.Store(true)
		_ = pc.Conn.Close()

		p.mu.Lock()
		delete(p.active, pc)
		p.removeIdleLocked(pc)
		p.mu.Unlock()

		p.releaseSlot()
	})
}

// Close 销毁连接：关 socket、摘除引用、归还令牌，幂等。
func (pc *PoolConn) Close() error {
	pc.pool.discard(pc)
	return nil
}

// CloseWrite 转发半关闭到内层 TCP 连接，供按 half-close 定界的 transformer 使用。
func (pc *PoolConn) CloseWrite() error {
	if tc, ok := pc.Conn.(*net.TCPConn); ok {
		return tc.CloseWrite()
	}
	return nil
}

// Close 关闭池：停 reaper，广播唤醒所有等待方，关闭全部空闲连接；
// 已借出的连接由持有者经 Put/discard 收敛
func (p *ConnPool) Close() error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	p.closed = true
	close(p.done)
	idle := p.idle
	p.idle = nil
	p.mu.Unlock()

	p.cond.Broadcast()
	for _, pc := range idle {
		p.discard(pc)
	}
	return nil
}

func (p *ConnPool) dial() (net.Conn, error) {
	conn, err := net.DialTimeout("tcp", p.addr, p.dialTimeout)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", gerrors.ErrDialFailed, err)
	}
	return conn, nil
}

func (p *ConnPool) removeIdleLocked(pc *PoolConn) {
	for i, c := range p.idle {
		if c == pc {
			p.idle = append(p.idle[:i], p.idle[i+1:]...)
			return
		}
	}
}

func (p *ConnPool) tryAcquireSlot() bool {
	select {
	case <-p.slots:
		return true
	default:
		return false
	}
}

func (p *ConnPool) releaseSlot() {
	select {
	case p.slots <- struct{}{}:
	default:
	}
	p.cond.Broadcast()
}

func withPoolDefaults(cfg builder.PoolConfig) builder.PoolConfig {
	if cfg.MaxConn <= 0 {
		cfg.MaxConn = gconst.DefaultPoolMaxConn
	}
	if cfg.MinIdle <= 0 {
		cfg.MinIdle = gconst.DefaultPoolMinIdle
	}
	if cfg.MinIdle > cfg.MaxConn {
		cfg.MinIdle = cfg.MaxConn
	}
	if cfg.DialTimeout <= 0 {
		cfg.DialTimeout = gconst.DefaultPoolDialTimeout
	}
	if cfg.BorrowTimeout <= 0 {
		cfg.BorrowTimeout = gconst.DefaultPoolBorrowTimeout
	}
	if cfg.IdleTimeout <= 0 {
		cfg.IdleTimeout = gconst.DefaultPoolIdleTimeout
	}
	if cfg.WaitTimeout <= 0 {
		cfg.WaitTimeout = gconst.DefaultPoolWaitTimeout
	}
	return cfg
}
