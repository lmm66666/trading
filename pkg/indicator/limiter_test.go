package indicator

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestLimiterAcquireRelease(t *testing.T) {
	l := NewLimiter(2)
	ctx := context.Background()

	if err := l.Acquire(ctx); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if err := l.Acquire(ctx); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}

	// 第三个应该阻塞，用带 timeout 的 context 测试
	ctxTimeout, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()
	if err := l.Acquire(ctxTimeout); err != context.DeadlineExceeded {
		t.Fatalf("expected DeadlineExceeded, got %v", err)
	}

	l.Release()
	if err := l.Acquire(ctx); err != nil {
		t.Fatalf("expected nil after release, got %v", err)
	}
}

func TestLimiterConcurrency(t *testing.T) {
	l := NewLimiter(3)
	ctx := context.Background()

	var maxConcurrent int
	var current int
	var mu sync.Mutex
	var wg sync.WaitGroup

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := l.Acquire(ctx); err != nil {
				return
			}
			defer l.Release()

			mu.Lock()
			current++
			if current > maxConcurrent {
				maxConcurrent = current
			}
			mu.Unlock()

			time.Sleep(20 * time.Millisecond)

			mu.Lock()
			current--
			mu.Unlock()
		}()
	}

	wg.Wait()
	if maxConcurrent > 3 {
		t.Fatalf("expected max concurrent <= 3, got %d", maxConcurrent)
	}
}

func TestLimiterContextCancel(t *testing.T) {
	l := NewLimiter(1)
	ctx := context.Background()

	// 占满槽位
	if err := l.Acquire(ctx); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}

	ctxCancel, cancel := context.WithCancel(ctx)
	go func() {
		time.Sleep(30 * time.Millisecond)
		cancel()
	}()

	if err := l.Acquire(ctxCancel); err != context.Canceled {
		t.Fatalf("expected Canceled, got %v", err)
	}
}

func TestLimiterReleaseWithoutAcquirePanics(t *testing.T) {
	l := NewLimiter(2)
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on unpaired Release, got none")
		}
	}()
	l.Release()
}
