package backend

import (
	"errors"
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
	probeTimeout  time.Duration
	reapInterval  time.Duration

	mu     sync.Mutex
	idle   []*pooledConn // LIFO
	active map[*pooledConn]struct{}
	slots  chan struct{} // 令牌 = 剩余可新建连接额度（容量 maxConn）
	closed bool
	done   chan struct{} // 通知 reaper 退出
}

// pooledConn 池内连接
type pooledConn struct {
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
		probeTimeout:  gconst.DefaultPoolProbeTimeout,
		reapInterval:  calcReapInterval(cfg.BorrowTimeout, cfg.IdleTimeout),
		idle:          make([]*pooledConn, 0, cfg.MinIdle),
		active:        make(map[*pooledConn]struct{}),
		slots:         make(chan struct{}, cfg.MaxConn),
		done:          make(chan struct{}),
	}
	for i := 0; i < cfg.MaxConn; i++ {
		p.slots <- struct{}{}
	}
	go p.reapLoop()
	return p
}

// Get 借出连接：优先复用空闲连接（复用前做失效探测），否则取令牌新建；
// 池满时阻塞等待，超过 WaitTimeout 返回 ErrPoolExhausted。
func (p *ConnPool) Get() (*pooledConn, error) {
	for {
		p.mu.Lock()
		if p.closed {
			p.mu.Unlock()
			return nil, gerrors.ErrPoolClosed
		}
		if n := len(p.idle); n > 0 {
			pc := p.idle[n-1]
			p.idle = p.idle[:n-1]
			pc.idleSince = time.Time{}
			p.mu.Unlock()

			if p.alive(pc.Conn) {
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
			p.discard(pc) // 坏连接：归还令牌，继续取下一条
			continue
		}
		p.mu.Unlock()

		select {
		case <-p.slots:
			if p.isClosed() {
				p.releaseSlot()
				return nil, gerrors.ErrPoolClosed
			}
			conn, err := p.dial()
			if err != nil {
				p.releaseSlot()
				return nil, err
			}
			pc := &pooledConn{Conn: conn, pool: p}
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
		case <-time.After(p.waitTimeout):
			return nil, gerrors.ErrPoolExhausted
		case <-p.done:
			return nil, gerrors.ErrPoolClosed
		}
	}
}

// Put 归还连接：active → idle；池已关闭或已被 reaper 摘除则交由 discard 收尾。
func (p *ConnPool) Put(pc *pooledConn) {
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
	pc.idleSince = time.Now()
	p.idle = append(p.idle, pc)
	p.mu.Unlock()
}

// discard 池内唯一的关闭路径：幂等地关 socket、摘除引用、归还令牌。
func (p *ConnPool) discard(pc *pooledConn) {
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

// Close 关闭连接：交由池统一收尾（摘除引用、幂等关闭、归还令牌）。
func (pc *pooledConn) Close() error {
	pc.pool.discard(pc)
	return nil
}

// CloseWrite 转发半关闭到内层 TCP 连接，供按 half-close 定界的 transformer 使用。
func (pc *pooledConn) CloseWrite() error {
	if tc, ok := pc.Conn.(*net.TCPConn); ok {
		return tc.CloseWrite()
	}
	return nil
}

// Close 关闭池：停止 reaper，并关闭全部空闲连接；
// 已借出的连接由持有者在 release 时经 Put/discard 收敛。
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

	for _, pc := range idle {
		p.discard(pc)
	}
	return nil
}

// reapLoop 池级后台回收协程：定时扫描，生命周期与池绑定。
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

// reapOnce 一轮回收：先回收借出超时的活跃连接，再收敛空闲，最后按余量预热。
func (p *ConnPool) reapOnce(now time.Time) {
	for _, pc := range p.snapshotExpiredActive(now) {
		p.discard(pc)
	}
	for _, pc := range p.snapshotExpiredIdle(now) {
		p.discard(pc)
	}
	for i := 0; i < p.warmDeficit(); i++ {
		if !p.dialIdle() {
			break
		}
	}
}

// snapshotExpiredActive 只读快照：借出时间超过 BorrowTimeout 的活跃连接。
func (p *ConnPool) snapshotExpiredActive(now time.Time) []*pooledConn {
	p.mu.Lock()
	defer p.mu.Unlock()
	var expired []*pooledConn
	for pc := range p.active {
		if now.Sub(pc.borrowedAt) > p.borrowTimeout {
			expired = append(expired, pc)
		}
	}
	return expired
}

// snapshotExpiredIdle 只读快照：空闲超过 IdleTimeout 的连接，且驱逐后不低于 MinIdle。
func (p *ConnPool) snapshotExpiredIdle(now time.Time) []*pooledConn {
	p.mu.Lock()
	defer p.mu.Unlock()
	removable := len(p.idle) - p.minIdle
	var expired []*pooledConn
	for i := 0; i < len(p.idle) && i < removable; i++ {
		pc := p.idle[i]
		if now.Sub(pc.idleSince) <= p.idleTimeout {
			break
		}
		expired = append(expired, pc)
	}
	return expired
}

// warmDeficit 只读：距离 MinIdle 还差几个空闲连接。
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

// dialIdle 预热一条空闲连接：非阻塞取令牌 → 拨号 → 入 idle；无额度或已关闭返回 false。
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
	pc := &pooledConn{Conn: conn, pool: p}
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

// alive 借出前失效探测：零字节读 + 极短 deadline，
// 超时表示对端存活且无数据；EOF/RST/残留字节均判定为坏连接。
func (p *ConnPool) alive(conn net.Conn) bool {
	if err := conn.SetReadDeadline(time.Now().Add(p.probeTimeout)); err != nil {
		return false
	}
	defer conn.SetReadDeadline(time.Time{})

	var b [1]byte
	n, err := conn.Read(b[:])
	if n > 0 {
		return false
	}
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return true
	}
	return false
}

func (p *ConnPool) dial() (net.Conn, error) {
	conn, err := net.DialTimeout("tcp", p.addr, p.dialTimeout)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", gerrors.ErrDialFailed, err)
	}
	return conn, nil
}

func (p *ConnPool) removeIdleLocked(pc *pooledConn) {
	for i, c := range p.idle {
		if c == pc {
			p.idle = append(p.idle[:i], p.idle[i+1:]...)
			return
		}
	}
}

func (p *ConnPool) isClosed() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.closed
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
		// 容量已满说明重复归还；丢弃以免阻塞，且不会放大池容量
	}
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

func withPoolDefaults(cfg builder.PoolConfig) builder.PoolConfig {
	if cfg.MaxConn <= 0 {
		cfg.MaxConn = gconst.DefaultPoolMaxConn
	}
	if cfg.MinIdle < 0 {
		cfg.MinIdle = 0
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
