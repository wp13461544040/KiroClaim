package handler

import (
	"log"
	"runtime/debug"
)

// 后台 goroutine 的 panic 兜底。
//
// gin 的 Recovery 中间件只覆盖 HTTP handler 所在的 goroutine，
// 我们自己 go 出来的任务（导入、定时巡检、并发健康检查）不在它的保护范围内。
// Go 里未捕获的 panic 会终止整个进程，所以这些任务必须自己 recover，
// 否则一个账号的异常数据就能让整个服务挂掉。

// goSafe 起一个带 panic 兜底的后台 goroutine。
func goSafe(name string, fn func()) {
	go runSafe(name, fn)
}

// runSafe 在当前 goroutine 内带 panic 兜底执行。
// 用于 worker 循环体：单个任务 panic 只跳过该任务，不会让整个 worker 退出。
func runSafe(name string, fn func()) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("后台任务 panic 已恢复 [%s]: %v\n%s", name, r, debug.Stack())
		}
	}()
	fn()
}
