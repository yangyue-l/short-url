package settings

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Cfg 全局配置，Init 后可读取
var Cfg *Config

type Config struct {
	Server     ServerConfig    `yaml:"server"`
	MySQL      MySQLConfig     `yaml:"mysql"`
	Redis      RedisConfig     `yaml:"redis"`
	RabbitMQ   RabbitMQConfig  `yaml:"rabbitmq"`
	Logger     LoggerConfig    `yaml:"logger"`
	AdminUsers []string        `yaml:"admin_users"`
	Snowflake  SnowflakeConfig `yaml:"snowflake"`
	JWT        JWTConfig       `yaml:"jwt"`
}

type SnowflakeConfig struct {
	StartTime string `yaml:"start_time"`
	MachineID int64  `yaml:"machine_id"`
}

type JWTConfig struct {
	Secret string `yaml:"secret"`
}

// BaseURL 返回不带尾部斜杠的服务地址
func (c *Config) BaseURL() string {
	return fmt.Sprintf("http://localhost:%d", c.Server.Port)
}

// IsAdmin 判断指定用户名是否为管理员
func (c *Config) IsAdmin(username string) bool {
	for _, u := range c.AdminUsers {
		if u == username {
			return true
		}
	}
	return false
}

type ServerConfig struct {
	Port int    `yaml:"port"`
	Mode string `yaml:"mode"`
}

type MySQLConfig struct {
	Host               string `yaml:"host"`
	Port               int    `yaml:"port"`
	User               string `yaml:"user"`
	Password           string `yaml:"password"`
	DB                 string `yaml:"db"`
	MaxOpenConns       int    `yaml:"max_open_conns"`
	MaxIdleConns       int    `yaml:"max_idle_conns"`
	ConnMaxLifetimeMin int    `yaml:"conn_max_lifetime_minutes"`
	ConnMaxIdleTimeMin int    `yaml:"conn_max_idle_time_minutes"`
}

type RedisConfig struct {
	Host               string `yaml:"host"`
	Port               int    `yaml:"port"`
	Password           string `yaml:"password"`
	DB                 int    `yaml:"db"`
	PoolSize           int    `yaml:"pool_size"`
	MinIdleConns       int    `yaml:"min_idle_conns"`
	PoolTimeoutSec     int    `yaml:"pool_timeout_seconds"`
	ReadTimeoutMillis  int    `yaml:"read_timeout_millis"`
	WriteTimeoutMillis int    `yaml:"write_timeout_millis"`
}

// ─── RabbitMQ 配置 ───

type RabbitMQConfig struct {
	Host      string           `yaml:"host"`
	Port      int              `yaml:"port"`
	User      string           `yaml:"user"`
	Password  string           `yaml:"password"`
	Vhost     string           `yaml:"vhost"`
	Click     ClickQueueConfig `yaml:"click"`
	Consumer  ConsumerConfig   `yaml:"consumer"`
	Reconnect ReconnectConfig  `yaml:"reconnect"`
}

// Addr 返回 RabbitMQ 连接地址
func (c *RabbitMQConfig) Addr() string {
	return fmt.Sprintf("amqp://%s:%s@%s:%d%s", c.User, c.Password, c.Host, c.Port, c.Vhost)
}

type ClickQueueConfig struct {
	Exchange     string `yaml:"exchange"`
	ExchangeType string `yaml:"exchange_type"`
	Queue        string `yaml:"queue"`
	RoutingKey   string `yaml:"routing_key"`
	Durable      bool   `yaml:"durable"`
	AutoDelete   bool   `yaml:"auto_delete"`
}

type ConsumerConfig struct {
	PrefetchCount int  `yaml:"prefetch_count"`
	AutoAck       bool `yaml:"auto_ack"`
	Workers       int  `yaml:"workers"`
}

type ReconnectConfig struct {
	Interval int `yaml:"interval"`
	MaxRetry int `yaml:"max_retry"`
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
