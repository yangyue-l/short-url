package mysql

import (
	"fmt"
	"short-url/models"
	"short-url/settings"

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
	var err error
	db, err = gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	})
	if err != nil {
		return fmt.Errorf("connect mysql failed: %w", err)
	}

	// 自动迁移
	if err := db.AutoMigrate(&models.URL{}, &models.User{}); err != nil {
		return fmt.Errorf("auto migrate failed: %w", err)
	}

	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)

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
