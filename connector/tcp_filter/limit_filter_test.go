package tcpfilter

import (
	"errors"
	"net"
	"testing"

	gerrors "API_Gateway/pkg/errors"
)

// testFilterConn 返回一对直连的 pipe 连接的一端供 filter 使用，
// t.Cleanup 负责关闭两端，避免测试结束后泄漏。
func testFilterConn(t *testing.T) net.Conn {
	t.Helper()
	c1, c2 := net.Pipe()
	t.Cleanup(func() {
		_ = c1.Close()
		_ = c2.Close()
	})
	return c1
}

// TestLimitFilterAcceptUpToMax 未达上限时全部准入，计数正确累加。
func TestLimitFilterAcceptUpToMax(t *testing.T) {
	lf := NewLimitFilter(2)

	if err := lf.HandleTCPConn(testFilterConn(t)); err != nil {
		t.Fatalf("conn1 should pass, got err: %v", err)
	}
	if err := lf.HandleTCPConn(testFilterConn(t)); err != nil {
		t.Fatalf("conn2 should pass, got err: %v", err)
	}
	if got := lf.currConn; got != 2 {
		t.Fatalf("currConn = %d, want 2", got)
	}
}

// TestLimitFilterRejectOverMax 超过上限被拒，且返回约定的哨兵错误。
func TestLimitFilterRejectOverMax(t *testing.T) {
	lf := NewLimitFilter(2)

	if err := lf.HandleTCPConn(testFilterConn(t)); err != nil {
		t.Fatalf("conn1 should pass, got err: %v", err)
	}
	if err := lf.HandleTCPConn(testFilterConn(t)); err != nil {
		t.Fatalf("conn2 should pass, got err: %v", err)
	}

	err := lf.HandleTCPConn(testFilterConn(t))
	if err == nil {
		t.Fatal("conn3 should be rejected, got nil")
	}
	if !errors.Is(err, gerrors.ErrTCPConnLimitExceeded) {
		t.Fatalf("conn3 err = %v, want %v", err, gerrors.ErrTCPConnLimitExceeded)
	}
	// 拒绝路径必须回滚刚才 +1 的计数
	if got := lf.currConn; got != 2 {
		t.Fatalf("currConn after reject = %d, want 2", got)
	}
}

// TestLimitFilterReleaseOnClose 连接关闭归还计数后，名额可再次被占用。
func TestLimitFilterReleaseOnClose(t *testing.T) {
	lf := NewLimitFilter(2)
	connA := testFilterConn(t)
	connB := testFilterConn(t)

	if err := lf.HandleTCPConn(connA); err != nil {
		t.Fatalf("connA should pass, got err: %v", err)
	}
	if err := lf.HandleTCPConn(connB); err != nil {
		t.Fatalf("connB should pass, got err: %v", err)
	}

	if err := lf.OnCloseConn(connA); err != nil {
		t.Fatalf("OnCloseConn(connA) err: %v", err)
	}
	if got := lf.currConn; got != 1 {
		t.Fatalf("currConn after release A = %d, want 1", got)
	}
	if err := lf.OnCloseConn(connB); err != nil {
		t.Fatalf("OnCloseConn(connB) err: %v", err)
	}
	if got := lf.currConn; got != 0 {
		t.Fatalf("currConn after release B = %d, want 0", got)
	}

	// 全部归还后，新连接应能再次准入
	if err := lf.HandleTCPConn(testFilterConn(t)); err != nil {
		t.Fatalf("conn after full release should pass, got err: %v", err)
	}
}
