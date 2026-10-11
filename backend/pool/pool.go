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

	mu      sync.Mutex
	idle    []*PoolConn
	active  map[*PoolConn]struct{}
	numOpen int
	waiters []*waiter
	closed  bool
	done    chan struct{}
}

type waiter struct {
	ch     chan struct{}
	queued bool
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
		done:          make(chan struct{}),
	}
	go p.reapLoop()
	return p
}

// Get 借出连接：优先复用空闲连接（复用前做失效探测），否则有空额则新建；
// 池满时登记等待者阻塞，超过 WaitTimeout 返回 ErrPoolExhausted。
func (p *ConnPool) Get() (*PoolConn, error) {
	deadline := time.Now().Add(p.waitTimeout)

	for {
		p.mu.Lock()
		if p.closed {
			p.mu.Unlock()
			return nil, gerrors.ErrPoolClosed
		}

		// 1. 优先复用空闲连接
		if n := len(p.idle); n > 0 {
			pc := p.idle[n-1]
			p.idle = p.idle[:n-1]
			p.mu.Unlock()

			if !alive(pc.Conn) {
				p.discard(pc)
				continue
			}
			p.mu.Lock()
			if p.closed {
				p.mu.Unlock()
				p.discard(pc)
				return nil, gerrors.ErrPoolClosed
			}
			pc.idleSince = time.Time{}
			pc.borrowedAt = time.Now()
			p.active[pc] = struct{}{}
			p.mu.Unlock()
			return pc, nil
		}

		// 2. 有空额则新建
		if p.numOpen < p.maxConn {
			p.numOpen++
			p.mu.Unlock()

			conn, err := p.dial()
			if err != nil {
				p.mu.Lock()
				p.numOpen--
				p.wakeOneLocked()
				p.mu.Unlock()
				return nil, err
			}
			pc := &PoolConn{
				Conn: conn,
				pool: p,
			}
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

		// 3. 池满：登记等待者，本地 timer 超时
		w := &waiter{
			ch:     make(chan struct{}, 1),
			queued: true,
		}
		p.waiters = append(p.waiters, w)
		p.mu.Unlock()

		remaining := time.Until(deadline)
		if remaining <= 0 {
			p.removeWaiter(w)
			return nil, gerrors.ErrPoolExhausted
		}
		timer := time.NewTimer(remaining)
		select {
		case <-w.ch:
			timer.Stop()
			continue
		case <-timer.C:
			p.mu.Lock()
			if w.queued {
				p.removeWaiterLocked(w)
				p.mu.Unlock()
				return nil, gerrors.ErrPoolExhausted
			}
			p.mu.Unlock()
			continue
		case <-p.done:
			timer.Stop()
			p.removeWaiter(w)
			return nil, gerrors.ErrPoolClosed
		}
	}
}

// Put 归还连接：active → idle，并只唤醒一个等待者
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
	p.wakeOneLocked()
	p.mu.Unlock()
}

// discard 池内唯一关闭路径的加锁包装：幂等关 socket、摘除引用、腾出配额、唤醒一个等待者
func (p *ConnPool) discard(pc *PoolConn) {
	if pc == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.discardLocked(pc)
}

// discardLocked discard 的核心，调用方必须已持有 p.mu
func (p *ConnPool) discardLocked(pc *PoolConn) {
	if pc == nil {
		return
	}
	pc.closeOnce.Do(func() {
		pc.closedFlag.Store(true)
		_ = pc.Conn.Close()

		delete(p.active, pc)
		p.removeIdleLocked(pc)
		p.numOpen--
		p.wakeOneLocked()
	})
}

// Close 销毁连接：关 socket、摘除引用、腾出配额，幂等。
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

// Close 关闭池：停 reaper、唤醒全部等待者、关闭全部空闲连接；
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
	p.waiters = nil
	p.mu.Unlock()

	for _, pc := range idle {
		p.discard(pc)
	}
	return nil
}

// wakeOneLocked 出队队首有效等待者并投递唤醒信号，调用方必须已持有 p.mu
func (p *ConnPool) wakeOneLocked() {
	for len(p.waiters) > 0 {
		w := p.waiters[0]
		p.waiters = p.waiters[1:]
		if w.queued {
			w.queued = false
			w.ch <- struct{}{}
			return
		}
	}
}

// removeWaiter 从等待队列摘除一个 waiter（超时/关闭路径）
func (p *ConnPool) removeWaiter(w *waiter) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.removeWaiterLocked(w)
}

func (p *ConnPool) removeWaiterLocked(w *waiter) {
	if w == nil || !w.queued {
		return
	}
	for i, x := range p.waiters {
		if x == w {
			p.waiters = append(p.waiters[:i], p.waiters[i+1:]...)
			break
		}
	}
	w.queued = false
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
