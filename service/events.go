package service

import "context"

// eventEmitter 前端事件推送实现（wails EventsEmit）。
// 由应用层在启动时注入；测试环境保持 nil，推送静默跳过
// （wails EventsEmit 在非 wails 上下文会 log.Fatal，必须避免直接调用）。
var eventEmitter func(ctx context.Context, eventName string, data ...any)

// SetEventEmitter 注入事件推送实现（main 启动时调用一次）
func SetEventEmitter(fn func(ctx context.Context, eventName string, data ...any)) {
	eventEmitter = fn
}

// emit 向所有监听者推送事件；未注入实现时忽略
func emit(ctx context.Context, eventName string, data ...any) {
	if eventEmitter != nil {
		eventEmitter(ctx, eventName, data...)
	}
}
