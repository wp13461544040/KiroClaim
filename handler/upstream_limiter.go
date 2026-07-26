package handler

import "sync"

// upstreamCheckLimiter is a fair process-wide semaphore for account checks that
// call Kiro/AWS upstream APIs. Import jobs, client redemption and admin refresh
// all share the same queue.
type upstreamCheckLimiter struct {
	mu      sync.Mutex
	cond    *sync.Cond
	limit   int
	inUse   int
	next    uint64
	serving uint64
}

func newUpstreamCheckLimiter(limit int) *upstreamCheckLimiter {
	if limit <= 0 {
		limit = 6
	}
	l := &upstreamCheckLimiter{limit: limit}
	l.cond = sync.NewCond(&l.mu)
	return l
}

func (l *upstreamCheckLimiter) SetLimit(limit int) {
	if limit <= 0 {
		limit = 6
	}
	l.mu.Lock()
	l.limit = limit
	l.cond.Broadcast()
	l.mu.Unlock()
}

func (l *upstreamCheckLimiter) Limit() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.limit
}

func (l *upstreamCheckLimiter) Acquire() func() {
	l.mu.Lock()
	ticket := l.next
	l.next++
	for ticket != l.serving || l.inUse >= l.limit {
		l.cond.Wait()
	}
	l.serving++
	l.inUse++
	l.cond.Broadcast()
	l.mu.Unlock()

	return func() {
		l.mu.Lock()
		if l.inUse > 0 {
			l.inUse--
		}
		l.cond.Broadcast()
		l.mu.Unlock()
	}
}

var globalUpstreamCheckLimiter = newUpstreamCheckLimiter(6)

func acquireUpstreamCheckSlot() func() {
	return globalUpstreamCheckLimiter.Acquire()
}

func updateUpstreamCheckConcurrency(limit int) {
	globalUpstreamCheckLimiter.SetLimit(limit)
}

func currentUpstreamCheckConcurrency() int {
	return globalUpstreamCheckLimiter.Limit()
}
