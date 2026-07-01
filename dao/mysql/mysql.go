package mysql

import (
	"fmt"
	"short-url/models"
	"short-url/settings"
	"time"

	"go.uber.org/zap"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var db *gorm.DB

func Init(cfg *settings.MySQLConfig) error {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		cfg.User, cfg.Password, cfg.Host, cfg.Port, cfg.DB,
	)

	// 生产环境只记录慢查询和错误，开发环境记录所有 SQL
	logLevel := logger.Info
	if settings.Cfg != nil && settings.Cfg.Server.Mode == "release" {
		logLevel = logger.Warn
	}

	var err error
	db, err = gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logLevel),
	})
	if err != nil {
		return fmt.Errorf("connect mysql failed: %w", err)
	}

	// 自动迁移（含 ClickStatsDaily 预聚合表）
	if err := db.AutoMigrate(&models.URL{}, &models.User{}, &models.ClickLog{}, &models.ClickStatsDaily{}); err != nil {
		return fmt.Errorf("auto migrate failed: %w", err)
	}

	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	if cfg.ConnMaxLifetimeMin > 0 {
		sqlDB.SetConnMaxLifetime(time.Duration(cfg.ConnMaxLifetimeMin) * time.Minute)
	}
	if cfg.ConnMaxIdleTimeMin > 0 {
		sqlDB.SetConnMaxIdleTime(time.Duration(cfg.ConnMaxIdleTimeMin) * time.Minute)
	}

	zap.L().Info("mysql init success")
	return nil
}

func Close() {
	if db != nil {
		sqlDB, _ := db.DB()
		sqlDB.Close()
	}
}

func GetDB() *gorm.DB {
	return db
}
