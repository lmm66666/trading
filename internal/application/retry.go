package application

import (
	"context"
	"database/sql/driver"
	"errors"
	"net"
	"syscall"
	"time"
	"trading/internal/port"
)

// retryDelays 按尝试次数索引重试延迟，长度必须恰为 port.MaxRunAttempts-1
//（除首次尝试外每档一条），由 TestRetryScheduleMatchesAttemptLimit 保证。
var retryDelays = []time.Duration{250 * time.Millisecond, time.Second, 4 * time.Second}

func retryAt(now time.Time, attempt int) (time.Time, bool) {
	if attempt < 1 || attempt >= port.MaxRunAttempts {
		return time.Time{}, false
	}
	return now.UTC().Add(retryDelays[attempt-1]), true
}
func classifyFailure(err error) port.Failure {
	f := port.Failure{Code: "COMPUTE_FAILED", Message: "计算失败"}
	switch {
	case errors.Is(err, context.Canceled):
		f.Code = "CANCELLED"
		f.Message = "任务已取消"
	case errors.Is(err, port.ErrLeaseLost):
		f.Code = "LEASE_LOST"
		f.Message = "执行租约失效"
	case errors.Is(err, ErrIncompleteMarketData):
		f.Code = "INCOMPLETE_DATA"
		f.Message = "行情数据不完整"
	case errors.Is(err, port.ErrTemporary):
		f.Code = "TEMPORARY"
		f.Message = "基础设施暂时不可用"
		f.Retryable = true
	case errors.Is(err, context.DeadlineExceeded):
		f.Code = "TIMEOUT"
		f.Message = "操作超时"
		f.Retryable = true
	case errors.Is(err, driver.ErrBadConn) || errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.ECONNREFUSED) || errors.Is(err, syscall.EPIPE) || errors.Is(err, syscall.ETIMEDOUT):
		f.Code = "CONNECTION"
		f.Message = "连接暂时不可用"
		f.Retryable = true
	default:
		var n net.Error
		if errors.As(err, &n) && n.Timeout() {
			f.Code = "TIMEOUT"
			f.Message = "操作超时"
			f.Retryable = true
		}
	}
	return f
}
