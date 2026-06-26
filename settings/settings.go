package settings

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Cfg 全局配置，Init 后可用
var Cfg *Config

type Config struct {
	Server ServerConfig `yaml:"server"`
	MySQL  MySQLConfig  `yaml:"mysql"`
	Redis  RedisConfig  `yaml:"redis"`
	Logger LoggerConfig `yaml:"logger"`
}

// BaseURL 返回不带尾部斜杠的服务地址
func (c *Config) BaseURL() string {
	return fmt.Sprintf("http://localhost:%d", c.Server.Port)
}

type ServerConfig struct {
	Port int    `yaml:"port"`
	Mode string `yaml:"mode"`
}

type MySQLConfig struct {
	Host         string `yaml:"host"`
	Port         int    `yaml:"port"`
	User         string `yaml:"user"`
	Password     string `yaml:"password"`
	DB           string `yaml:"db"`
	MaxOpenConns int    `yaml:"max_open_conns"`
	MaxIdleConns int    `yaml:"max_idle_conns"`
}

type RedisConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Password string `yaml:"password"`
	DB       int    `yaml:"db"`
	PoolSize int    `yaml:"pool_size"`
}

type LoggerConfig struct {
	Level      string `yaml:"level"`
	Filename   string `yaml:"filename"`
	MaxSize    int    `yaml:"max_size"`
	MaxBackups int    `yaml:"max_backups"`
	MaxAge     int    `yaml:"max_age"`
}

func Init(configPath string) (*Config, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("read config file failed: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config failed: %w", err)
	}
	Cfg = &cfg
	return &cfg, nil
}
