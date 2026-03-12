package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config is the top-level application configuration.
type Config struct {
	App           AppConfig           `yaml:"app"`
	Postgres      PostgresConfig      `yaml:"postgres"`
	Elasticsearch ElasticsearchConfig `yaml:"elasticsearch"`
	Redis         RedisConfig         `yaml:"redis"`
	Kafka         KafkaConfig         `yaml:"kafka"`
	Search        SearchConfig        `yaml:"search"`
}

type AppConfig struct {
	Port           int    `yaml:"port"`
	Env            string `yaml:"env"`
	WarmupDatasets int    `yaml:"warmup_datasets"`
}

type PostgresConfig struct {
	Host           string `yaml:"host"`
	Port           int    `yaml:"port"`
	User           string `yaml:"user"`
	Password       string `yaml:"password"`
	DBName         string `yaml:"dbname"`
	MaxConnections int    `yaml:"max_connections"`
	MaxIdle        int    `yaml:"max_idle"`
}

// DSN returns a PostgreSQL connection string.
func (p PostgresConfig) DSN() string {
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		p.Host, p.Port, p.User, p.Password, p.DBName,
	)
}

type ElasticsearchConfig struct {
	Host            string        `yaml:"host"`
	IndexPrefix     string        `yaml:"index_prefix"`
	RefreshInterval time.Duration `yaml:"refresh_interval"`
}

type RedisConfig struct {
	Host           string `yaml:"host"`
	MaxMemory      string `yaml:"max_memory"`
	EvictionPolicy string `yaml:"eviction_policy"`
}

type KafkaConfig struct {
	Broker               string      `yaml:"broker"`
	Topics               KafkaTopics `yaml:"topics"`
	ConsumerLagThreshold int         `yaml:"consumer_lag_threshold"`
}

type KafkaTopics struct {
	Upserted string `yaml:"upserted"`
	Deleted  string `yaml:"deleted"`
	Changed  string `yaml:"changed"`
}

type SearchConfig struct {
	InMemoryLimit            int           `yaml:"in_memory_limit"`
	BleveFileLimit           int           `yaml:"bleve_file_limit"`
	StabilityThreshold       float64       `yaml:"stability_threshold"`
	StabilityTick            float64       `yaml:"stability_tick"`
	StabilityDecay           float64       `yaml:"stability_decay"`
	TierUpgradeConfirmations int           `yaml:"tier_upgrade_confirmations"`
	OutboxPollInterval       time.Duration `yaml:"outbox_poll_interval"`
	OutboxMaxAttempts        int           `yaml:"outbox_max_attempts"`
	BatchSize                int           `yaml:"batch_size"`
	WorkerCount              int           `yaml:"worker_count"`
}

// Load reads and parses the YAML config file at path.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return &cfg, nil
}

func (c *Config) validate() error {
	if c.App.Port == 0 {
		return fmt.Errorf("app.port is required")
	}
	if c.Postgres.Host == "" {
		return fmt.Errorf("postgres.host is required")
	}
	if c.Postgres.DBName == "" {
		return fmt.Errorf("postgres.dbname is required")
	}
	return nil
}
