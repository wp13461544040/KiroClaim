package handler

import (
	"sync"
	"testing"
)

// resetHealthScanForTest 把全局巡检状态清干净，避免用例之间互相影响。
func resetHealthScanForTest(t *testing.T) {
	t.Helper()
	clear := func() {
		healthScan.mu.Lock()
		healthScan.running = false
		healthScan.curEpoch = 0
		healthScan.curTotal = 0
		healthScan.curDone = 0
		healthScan.mu.Unlock()
	}
	clear()
	t.Cleanup(clear)
}

func readProgress(t *testing.T) (epoch uint64, total int, done int) {
	t.Helper()
	healthScan.mu.RLock()
	defer healthScan.mu.RUnlock()
	return healthScan.curEpoch, healthScan.curTotal, healthScan.curDone
}

func TestHealthScanProgressBeginAndBump(t *testing.T) {
	resetHealthScanForTest(t)

	beginHealthScanProgress(7, 3)
	if epoch, total, done := readProgress(t); epoch != 7 || total != 3 || done != 0 {
		t.Fatalf("初始化后 epoch=%d total=%d done=%d, want 7/3/0", epoch, total, done)
	}

	bumpHealthScanProgress(7)
	bumpHealthScanProgress(7)
	if _, _, done := readProgress(t); done != 2 {
		t.Fatalf("两次累加后 done=%d, want 2", done)
	}

	// done 不能超过 total，否则百分比会算出大于 100
	bumpHealthScanProgress(7)
	bumpHealthScanProgress(7)
	if _, _, done := readProgress(t); done != 3 {
		t.Fatalf("done=%d, want 3（不应超过 total）", done)
	}
}

// 被取代的旧 worker 不能把计数加到新一轮的进度上。
func TestHealthScanProgressIgnoresStaleEpoch(t *testing.T) {
	resetHealthScanForTest(t)

	beginHealthScanProgress(2, 5)
	bumpHealthScanProgress(1) // 上一轮的 worker
	bumpHealthScanProgress(9) // 不存在的轮次
	if _, _, done := readProgress(t); done != 0 {
		t.Fatalf("旧 epoch 的计数不应生效，done=%d, want 0", done)
	}

	bumpHealthScanProgress(2)
	if _, _, done := readProgress(t); done != 1 {
		t.Fatalf("当前 epoch 的计数应生效，done=%d, want 1", done)
	}
}

func TestHealthScanProgressClearedAfterRound(t *testing.T) {
	resetHealthScanForTest(t)

	beginHealthScanProgress(3, 10)
	bumpHealthScanProgress(3)
	clearHealthScanProgress()

	if epoch, total, done := readProgress(t); epoch != 0 || total != 0 || done != 0 {
		t.Fatalf("清理后 epoch=%d total=%d done=%d, want 全 0（否则 running=false 时仍会返回上一轮进度）",
			epoch, total, done)
	}

	// 清理后旧 epoch 再来计数也不该复活进度
	bumpHealthScanProgress(3)
	if _, _, done := readProgress(t); done != 0 {
		t.Fatalf("清理后不应再累加，done=%d, want 0", done)
	}
}

// 多个 worker 并发累加时计数不能丢。
func TestHealthScanProgressConcurrentBump(t *testing.T) {
	resetHealthScanForTest(t)

	const total = 200
	beginHealthScanProgress(5, total)

	var wg sync.WaitGroup
	for i := 0; i < total; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			bumpHealthScanProgress(5)
		}()
	}
	wg.Wait()

	if _, _, done := readProgress(t); done != total {
		t.Fatalf("并发累加后 done=%d, want %d", done, total)
	}
}

// 状态接口要把进度和百分比一起暴露出来，百分比在后端算好。
func TestHealthScanStatusExposesProgress(t *testing.T) {
	setupDispatchPolicyTestDB(t)
	resetHealthScanForTest(t)

	beginHealthScanProgress(4, 8)
	bumpHealthScanProgress(4)
	bumpHealthScanProgress(4)

	status := HealthScanStatus()
	if got := status["currentTotal"]; got != 8 {
		t.Fatalf("currentTotal = %v, want 8", got)
	}
	if got := status["currentDone"]; got != 2 {
		t.Fatalf("currentDone = %v, want 2", got)
	}
	if got := status["percent"]; got != 25 {
		t.Fatalf("percent = %v, want 25", got)
	}

	// 总数为 0 时百分比给 0，不能是 NaN 或除零 panic
	clearHealthScanProgress()
	status = HealthScanStatus()
	if got := status["percent"]; got != 0 {
		t.Fatalf("无进度时 percent = %v, want 0", got)
	}
}
