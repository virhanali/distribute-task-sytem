package config

import (
	"fmt"
	"strings"

	"time"

	"github.com/spf13/viper"
)

type Config struct {
	Server   ServerConfig
	Database DatabaseConfig
	Redis    RedisConfig
	RabbitMQ RabbitMQConfig
	Worker   WorkerConfig
	Log      LogConfig
}

type ServerConfig struct {
	Port int    `mapstructure:"port"`
	Host string `mapstructure:"host"`
}

type DatabaseConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
	DBName   string `mapstructure:"dbname"`
	SSLMode  string `mapstructure:"sslmode"`

	MaxOpenConns    int           `mapstructure:"max_open_conns"`
	MaxIdleConns    int           `mapstructure:"max_idle_conns"`
	ConnMaxLifetime time.Duration `mapstructure:"conn_max_lifetime"`
}

func (c DatabaseConfig) FormatDSN() string {
	return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
		c.User, c.Password, c.Host, c.Port, c.DBName, c.SSLMode)
}

type RedisConfig struct {
	Addr     string `mapstructure:"addr"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

type RabbitMQConfig struct {
	URL          string `mapstructure:"url"`
	ExchangeName string `mapstructure:"exchange_name"`
}

type WorkerConfig struct {
	NumWorkers int      `mapstructure:"num_workers"`
	Queues     []string `mapstructure:"queues"`
}

type LogConfig struct {
	Level string `mapstructure:"level"`
}

func LoadConfig() (*Config, error) {
	v := viper.New()

	setDefaults(v)

	v.SetConfigName(".env")
	v.SetConfigType("env")
	v.AddConfigPath(".")
	v.AutomaticEnv()

	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("error reading config file: %w", err)
		}
	}

	bindFlatKeysToNested(v)

	var config Config
	if err := v.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("unable to decode into struct: %w", err)
	}

	if err := validateConfig(&config); err != nil {
		return nil, err
	}

	return &config, nil
}

func setDefaults(v *viper.Viper) {
	v.SetDefault("server.port", 8080)
	v.SetDefault("server.host", "0.0.0.0")

	v.SetDefault("database.host", "localhost")
	v.SetDefault("database.port", 5432)
	v.SetDefault("database.sslmode", "disable")
	v.SetDefault("database.max_open_conns", 25)
	v.SetDefault("database.max_idle_conns", 25)
	v.SetDefault("database.conn_max_lifetime", "15m")

	v.SetDefault("redis.addr", "localhost:6379")
	v.SetDefault("redis.db", 0)

	v.SetDefault("rabbitmq.exchange_name", "tasks")

	v.SetDefault("worker.num_workers", 10)
	v.SetDefault("worker.queues", []string{"tasks.high", "tasks.medium", "tasks.low"})

	v.SetDefault("log.level", "info")
}

func validateConfig(config *Config) error {
	if config.Database.User == "" {
		return fmt.Errorf("database.user is required")
	}
	if config.Database.Password == "" {
		return fmt.Errorf("database.password is required")
	}
	if config.Database.DBName == "" {
		return fmt.Errorf("database.dbname is required")
	}
	if config.RabbitMQ.URL == "" {
		return fmt.Errorf("rabbitmq.url is required")
	}
	return nil
}

func bindFlatKeysToNested(v *viper.Viper) {
	if v.IsSet("database_host") {
		v.Set("database.host", v.GetString("database_host"))
	}
	if v.IsSet("database_port") {
		v.Set("database.port", v.GetInt("database_port"))
	}
	if v.IsSet("database_user") {
		v.Set("database.user", v.GetString("database_user"))
	}
	if v.IsSet("database_password") {
		v.Set("database.password", v.GetString("database_password"))
	}
	if v.IsSet("database_dbname") {
		v.Set("database.dbname", v.GetString("database_dbname"))
	}
	if v.IsSet("database_sslmode") {
		v.Set("database.sslmode", v.GetString("database_sslmode"))
	}

	if v.IsSet("redis_addr") {
		v.Set("redis.addr", v.GetString("redis_addr"))
	}
	if v.IsSet("redis_password") {
		v.Set("redis.password", v.GetString("redis_password"))
	}
	if v.IsSet("redis_db") {
		v.Set("redis.db", v.GetInt("redis_db"))
	}

	if v.IsSet("rabbitmq_url") {
		v.Set("rabbitmq.url", v.GetString("rabbitmq_url"))
	}
	if v.IsSet("rabbitmq_exchange_name") {
		v.Set("rabbitmq.exchange_name", v.GetString("rabbitmq_exchange_name"))
	}

	if v.IsSet("server_port") {
		v.Set("server.port", v.GetInt("server_port"))
	}
	if v.IsSet("server_host") {
		v.Set("server.host", v.GetString("server_host"))
	}

	if v.IsSet("worker_num_workers") {
		v.Set("worker.num_workers", v.GetInt("worker_num_workers"))
	}
	if v.IsSet("worker_queues") {
		val := v.Get("worker_queues")
		if strVal, ok := val.(string); ok {
			v.Set("worker.queues", strings.Split(strVal, ","))
		} else {
			v.Set("worker.queues", val)
		}
	}
}
