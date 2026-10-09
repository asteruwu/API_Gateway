package pool

import (
	"net"
	"syscall"
)

// alive 非阻塞读探测：EAGAIN = 连接正常，EOF/RST/残留数据 = 不可用
func alive(conn net.Conn) bool {
	type syscallConner interface {
		SyscallConn() (syscall.RawConn, error)
	}
	sc, ok := conn.(syscallConner)
	if !ok {
		return false
	}
	raw, err := sc.SyscallConn()
	if err != nil {
		return false
	}

	isAlive := true
	rerr := raw.Read(func(fd uintptr) bool {
		var buf [1]byte
		n, err := syscall.Read(int(fd), buf[:])
		if n > 0 {
			isAlive = false // 残留脏数据
		} else if err == syscall.EAGAIN || err == syscall.EWOULDBLOCK {
			isAlive = true
		} else {
			isAlive = false // EOF 或其他错误
		}
		return true // 不等待，立即返回
	})
	if rerr != nil {
		return false
	}
	return isAlive
}
