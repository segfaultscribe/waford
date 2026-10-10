package internal

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/viper"
)

func LoadConfig(path string) (*Config, error) {
	var config Config

	// 1. Define the full file path using the passed directory path
	filename := filepath.Join(path, "waford.yaml")

	// 2. Set default values for all nested config fields
	viper.SetDefault("server.port", 4242)
	viper.SetDefault("database.path", "waford_001.db")

	viper.SetDefault("worker.count", 10)
	viper.SetDefault("worker.poll_interval", 5*time.Second)
	viper.SetDefault("worker.stale_recovery_timeout", 30*time.Second)

	viper.SetDefault("delivery.timeout", 10*time.Second)
	viper.SetDefault("delivery.pending_pull_limit", 100)
	viper.SetDefault("delivery.retry.base_delay", 1*time.Second)
	viper.SetDefault("delivery.retry.max_delay", 30*time.Second)
	viper.SetDefault("delivery.retry.max_retries", 3)
	viper.SetDefault("delivery.retry.retry_pull_limit", 50)

	viper.SetDefault("retry.base_delay", 1*time.Second)
	viper.SetDefault("retry.max_delay", 30*time.Second)
	viper.SetDefault("retry.max_retries", 3)
	viper.SetDefault("retry.max_pull_limit", 50)

	if _, err := os.Stat(filename); os.IsNotExist(err) {
		if mkErr := os.MkdirAll(path, 0o755); mkErr != nil {
			return &Config{}, mkErr
		}
		if writeErr := viper.WriteConfigAs(filename); writeErr != nil {
			return &Config{}, writeErr
		}
	}

	viper.SetConfigFile(filename)
	viper.SetConfigType("yaml")

	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	if err := viper.ReadInConfig(); err != nil {
		return &Config{}, err
	}

	if err := viper.Unmarshal(&config); err != nil {
		return &Config{}, err
	}

	return &config, nil
}

type Config struct {
	Server   ServerConfig
	Database DatabaseConfig
	Worker   WorkerConfig
	Delivery DeliveryConfig
	Retry    RetryConfig
}

type ServerConfig struct {
	Port int `mapstructure:"port"`
}

type DatabaseConfig struct {
	Path string `mapstructure:"path"`
}

type WorkerConfig struct {
	Count                int           `mapstructure:"count"`
	PollInterval         time.Duration `mapstructure:"poll_interval"`
	StaleRecoverytimeout time.Duration `mapstructure:"stale_recovery_timeout"`
}

type DeliveryConfig struct {
	Timeout          time.Duration `mapstructure:"timeout"`
	PendingPullLimit int           `mapstructure:"pending_pull_limit"`
	Retry            RetryConfig   `mapstructure:"retry"`
}

type RetryConfig struct {
	BaseDelay      time.Duration `mapstructure:"base_delay"`
	MaxDelay       time.Duration `mapstructure:"max_delay"`
	MaxRetries     int           `mapstructure:"max_retries"`
	RetryPullLimit int           `mapstructure:"retry_pull_limit"`
}
