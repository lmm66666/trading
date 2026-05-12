package indicator

import "context"

// Limiter 基于 channel 的并发限流器
type Limiter struct {
	ch chan struct{}
}

// NewLimiter 创建限流器，maxConcurrent 为最大并发数
func NewLimiter(maxConcurrent int) *Limiter {
	return &Limiter{ch: make(chan struct{}, maxConcurrent)}
}

// Acquire 获取一个执行槽位，阻塞直到获取成功或 context 取消
func (l *Limiter) Acquire(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case l.ch <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Release 释放一个执行槽位。必须与成功的 Acquire 配对调用；
// 若在没有未配对 Acquire 的情况下被调用，会 panic 以暴露调用方 bug，避免计数错乱后续 Acquire 永久阻塞。
func (l *Limiter) Release() {
	select {
	case <-l.ch:
	default:
		panic("indicator: Limiter.Release called without paired Acquire")
	}
}
