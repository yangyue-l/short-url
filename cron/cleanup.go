package cron

import (
	"context"
	"time"

	"short-url/dao/mysql"

	"go.uber.org/zap"
)

// StartCleanupJob 启动日志清理任务（每天凌晨 3 点清理 90 天前的 click_logs）
func StartCleanupJob(ctx context.Context) {
	zap.L().Info("log cleanup job started (90 days retention)")

	go func() {
		for {
			// 计算到凌晨 3 点的等待时间
			now := time.Now()
			next := time.Date(now.Year(), now.Month(), now.Day()+1, 3, 0, 0, 0, now.Location())
			wait := next.Sub(now)

			select {
			case <-ctx.Done():
				return
			case <-time.After(wait):
				cleanOnce()
			}
		}
	}()
}

func cleanOnce() {
	rows, err := mysql.CleanOldClickLogs(90)
	if err != nil {
		zap.L().Error("clean old click logs failed", zap.Error(err))
		return
	}
	if rows > 0 {
		zap.L().Info("cleaned old click logs", zap.Int64("rows", rows))
	}
}
