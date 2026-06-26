package logic

import (
	"short-url/dao/mysql"

	"go.uber.org/zap"
)

const clickQueueSize = 1000

var clickQueue = make(chan string, clickQueueSize)

// InitClickWorkers 启动点击计数后台 worker（建议 workerNum=2~4）
func InitClickWorkers(workerNum int) {
	for i := 0; i < workerNum; i++ {
		go func() {
			for shortCode := range clickQueue {
				if err := mysql.IncrementClickCnt(shortCode); err != nil {
					zap.L().Warn("increment click count failed",
						zap.String("shortCode", shortCode), zap.Error(err))
				}
			}
		}()
	}
}

// recordClick 异步记录一次点击；队列满时同步执行保数据不丢
func recordClick(shortCode string) {
	select {
	case clickQueue <- shortCode:
	default:
		// 队列满则同步执行，不丢数据
		if err := mysql.IncrementClickCnt(shortCode); err != nil {
			zap.L().Warn("increment click count failed",
				zap.String("shortCode", shortCode), zap.Error(err))
		}
	}
}
