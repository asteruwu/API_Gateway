package pool

import (
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"API_Gateway/builder"
	gerrors "API_Gateway/pkg/errors"
)

// testBackend 一个只 accept 连接、不跑协议的假后端：记录已接受连接数，
// 支持批量关闭（模拟对端断开）与批量写入（模拟残留脏数据）。
type testBackend struct {
	ln net.Listener

	mu       sync.Mutex
	conns    []net.Conn
	accepted int
}

func newTestBackend(t *testing.T) *testBackend {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	b := &testBackend{ln: ln}
	go b.acceptLoop()
	t.Cleanup(func() {
		b.closeAll()
		_ = ln.Close()
	})
	return b
}

func (b *testBackend) addr() string { return b.ln.Addr().String() }

func (b *testBackend) acceptLoop() {
	for {
		conn, err := b.ln.Accept()
		if err != nil {
			return
		}
		b.mu.Lock()
		b.conns = append(b.conns, conn)
		b.accepted++
		b.mu.Unlock()
	}
}

func (b *testBackend) acceptedCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.accepted
}

func (b *testBackend) snapshot() []net.Conn {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]net.Conn(nil), b.conns...)
}

func (b *testBackend) closeAll() {
	for _, c := range b.snapshot() {
		_ = c.Close()
	}
}

func (b *testBackend) writeAll(data []byte) {
	for _, c := range b.snapshot() {
		_, _ = c.Write(data)
	}
}

// testPoolCfg 给出一个不触发预热的配置：borrow/idle 用默认值使 reaper 间隔为 5s，
// 测试在 1s 内结束，故 reaper 不会自行 dial，accepted 计数只反映 Get 的行为。
func testPoolCfg(maxConn int) builder.PoolConfig {
	return builder.PoolConfig{
		MaxConn:     maxConn,
		MinIdle:     1,
		DialTimeout: time.Second,
		WaitTimeout: 200 * time.Millisecond,
	}
}

func newPool(t *testing.T, addr string, cfg builder.PoolConfig) *ConnPool {
	t.Helper()
	p := NewConnPool(addr, cfg)
	t.Cleanup(func() { _ = p.Close() })
	return p
}

// waitAccepted 等待后端 accept 计数达到 want（Accept 与 dial 完成之间存在异步间隙）。
func waitAccepted(t *testing.T, b *testBackend, want int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if b.acceptedCount() >= want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("backend accepted %d conns, want >= %d", b.acceptedCount(), want)
}

// TestGetDialsThenReuses 空池 Get 新建连接，归还后再 Get 复用同一条（不新建）。
func TestGetDialsThenReuses(t *testing.T) {
	b := newTestBackend(t)
	p := newPool(t, b.addr(), testPoolCfg(1))

	pc1, err := p.Get()
	if err != nil {
		t.Fatalf("first Get: %v", err)
	}
	waitAccepted(t, b, 1)
	if got := b.acceptedCount(); got != 1 {
		t.Fatalf("accepted = %d, want 1 after first Get", got)
	}
	p.Put(pc1)

	pc2, err := p.Get()
	if err != nil {
		t.Fatalf("second Get: %v", err)
	}
	if pc2 != pc1 {
		t.Errorf("second Get returned a different conn, want the pooled one")
	}
	if got := b.acceptedCount(); got != 1 {
		t.Errorf("accepted = %d, want 1 (reused)", got)
	}
	p.Put(pc2)
}

// TestGetExhausted 池满时第二次 Get 阻塞到 WaitTimeout 后返回 ErrPoolExhausted。
func TestGetExhausted(t *testing.T) {
	b := newTestBackend(t)
	p := newPool(t, b.addr(), testPoolCfg(1))

	pc, err := p.Get()
	if err != nil {
		t.Fatalf("first Get: %v", err)
	}
	defer p.Put(pc) // 占住唯一名额

	start := time.Now()
	if _, err := p.Get(); !errors.Is(err, gerrors.ErrPoolExhausted) {
		t.Fatalf("second Get err = %v, want %v", err, gerrors.ErrPoolExhausted)
	}
	if elapsed := time.Since(start); elapsed < 100*time.Millisecond {
		t.Errorf("second Get returned in %v, want ~WaitTimeout (200ms)", elapsed)
	}
}

// TestGetDiscardsDeadIdleConn 后端关闭后，池在复用前探活判定死连接并丢弃、改拨新连接。
func TestGetDiscardsDeadIdleConn(t *testing.T) {
	b := newTestBackend(t)
	p := newPool(t, b.addr(), testPoolCfg(1))

	pc1, err := p.Get()
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	waitAccepted(t, b, 1) // 确保后端已记录该连接，closeAll 才有对象可关
	p.Put(pc1)

	b.closeAll()
	time.Sleep(50 * time.Millisecond) // 等 FIN 到达，使空闲连接变为死连接

	pc2, err := p.Get()
	if err != nil {
		t.Fatalf("Get after peer close: %v", err)
	}
	if pc2 == pc1 {
		t.Errorf("pool reused a dead conn")
	}
	waitAccepted(t, b, 2)
	if got := b.acceptedCount(); got != 2 {
		t.Errorf("accepted = %d, want 2 (dead discarded, new dialed)", got)
	}
	p.Put(pc2)
}

// TestGetDiscardsConnWithResidualData 空闲连接上出现残留字节时判为不可用并丢弃。
func TestGetDiscardsConnWithResidualData(t *testing.T) {
	b := newTestBackend(t)
	p := newPool(t, b.addr(), testPoolCfg(1))

	pc1, err := p.Get()
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	waitAccepted(t, b, 1) // 确保后端已记录该连接，writeAll 才写得进去
	p.Put(pc1)

	b.writeAll([]byte{0x01})          // 对端写入脏数据
	time.Sleep(50 * time.Millisecond) // 等数据到达

	pc2, err := p.Get()
	if err != nil {
		t.Fatalf("Get after residual data: %v", err)
	}
	if pc2 == pc1 {
		t.Errorf("pool reused a conn carrying residual data")
	}
	waitAccepted(t, b, 2)
	if got := b.acceptedCount(); got != 2 {
		t.Errorf("accepted = %d, want 2 (dirty discarded, new dialed)", got)
	}
	p.Put(pc2)
}

// TestCloseClosesIdleAndRejects Close 后空闲连接关闭，Get 返回 ErrPoolClosed，且 Close 幂等。
func TestCloseClosesIdleAndRejects(t *testing.T) {
	b := newTestBackend(t)
	p := NewConnPool(b.addr(), testPoolCfg(1))

	pc, err := p.Get()
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	p.Put(pc)

	if err := p.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := p.Get(); !errors.Is(err, gerrors.ErrPoolClosed) {
		t.Errorf("Get after Close err = %v, want %v", err, gerrors.ErrPoolClosed)
	}
	if err := p.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
}

// TestPoolConnCloseReleasesSlot 借出的连接被 Close（discard）后归还令牌，可再取新连接；Close 幂等。
func TestPoolConnCloseReleasesSlot(t *testing.T) {
	b := newTestBackend(t)
	p := newPool(t, b.addr(), testPoolCfg(1))

	pc1, err := p.Get()
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if err := pc1.Close(); err != nil {
		t.Fatalf("pc1.Close: %v", err)
	}
	if err := pc1.Close(); err != nil { // 幂等，不应 panic
		t.Errorf("pc1.Close second: %v", err)
	}

	pc2, err := p.Get()
	if err != nil {
		t.Fatalf("Get after discard: %v", err)
	}
	if pc2 == pc1 {
		t.Errorf("expected a fresh conn after discard")
	}
	p.Put(pc2)
}

// TestPutAfterCloseDiscards 池关闭后归还连接应被丢弃（关闭），而不是回填空闲集合。
func TestPutAfterCloseDiscards(t *testing.T) {
	b := newTestBackend(t)
	p := NewConnPool(b.addr(), testPoolCfg(1))

	pc, err := p.Get()
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if err := p.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	p.Put(pc)
	if !pc.closedFlag.Load() {
		t.Errorf("Put on closed pool should discard (close) the conn")
	}
}

// TestGetDialFailure 目标不可达时 Get 返回 ErrDialFailed。
func TestGetDialFailure(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()
	_ = ln.Close() // 关闭端口，后续 dial 必失败

	p := newPool(t, addr, testPoolCfg(1))
	if _, err := p.Get(); !errors.Is(err, gerrors.ErrDialFailed) {
		t.Errorf("Get err = %v, want %v", err, gerrors.ErrDialFailed)
	}
}

// TestConcurrentGetPut 并发 Get/Put 下池保持自洽（配合 -race 检测数据竞争）。
func TestConcurrentGetPut(t *testing.T) {
	b := newTestBackend(t)
	const workers = 8
	cfg := testPoolCfg(workers)
	cfg.WaitTimeout = 2 * time.Second
	p := newPool(t, b.addr(), cfg)

	const iters = 20
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iters; j++ {
				pc, err := p.Get()
				if err != nil {
					t.Errorf("Get: %v", err)
					return
				}
				p.Put(pc)
			}
		}()
	}
	wg.Wait()
}
