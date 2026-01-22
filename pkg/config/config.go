package config

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/caarlos0/env/v9"
	"github.com/joho/godotenv"
	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
)

// GlobalConfig contains only top-level application configuration
// Module-specific configs are managed by each module independently
type GlobalConfig struct {
	// Server configuration
	Server ServerConfig `yaml:"server" envPrefix:"SERVER_"`

	// Kubernetes client configuration
	Kubernetes KubernetesConfig `yaml:"kubernetes" envPrefix:"KUBERNETES_"`

	// Metrics configuration
	Metrics MetricsConfig `yaml:"metrics" envPrefix:"METRICS_"`

	// Leader election configuration
	LeaderElection LeaderElectionConfig `yaml:"leaderElection" envPrefix:"LEADER_ELECTION_"`

	// Logging configuration
	Logging LoggingConfig `yaml:"logging" envPrefix:"LOGGING_"`

	// Performance tuning
	Performance PerformanceConfig `yaml:"performance" envPrefix:"PERFORMANCE_"`

	// Enabled collectors (list of collector names)
	EnabledCollectors []string `yaml:"enabledCollectors" env:"ENABLED_COLLECTORS" envSeparator:","`
}

// ServerConfig contains HTTP server configuration
type ServerConfig struct {
	Address     string `yaml:"address"     env:"ADDRESS"      envDefault:":9090"`
	MetricsPath string `yaml:"metricsPath" env:"METRICS_PATH" envDefault:"/metrics"`
	HealthPath  string `yaml:"healthPath"  env:"HEALTH_PATH"  envDefault:"/health"`
}

// KubernetesConfig contains Kubernetes client configuration
type KubernetesConfig struct {
	Kubeconfig string  `yaml:"kubeconfig" env:"KUBECONFIG"`
	QPS        float32 `yaml:"qps"        env:"QPS"        envDefault:"50"`
	Burst      int     `yaml:"burst"      env:"BURST"      envDefault:"100"`
}

// MetricsConfig contains Prometheus metrics configuration
type MetricsConfig struct {
	Namespace string `yaml:"namespace" env:"NAMESPACE" envDefault:"sealos"`
}

// LeaderElectionConfig contains leader election configuration
type LeaderElectionConfig struct {
	Enabled       bool          `yaml:"enabled"       env:"ENABLED"        envDefault:"true"`
	Namespace     string        `yaml:"namespace"     env:"NAMESPACE"      envDefault:"sealos-system"`
	LeaseName     string        `yaml:"leaseName"     env:"LEASE_NAME"     envDefault:"sealos-state-metric"`
	LeaseDuration time.Duration `yaml:"leaseDuration" env:"LEASE_DURATION" envDefault:"15s"`
	RenewDeadline time.Duration `yaml:"renewDeadline" env:"RENEW_DEADLINE" envDefault:"10s"`
	RetryPeriod   time.Duration `yaml:"retryPeriod"   env:"RETRY_PERIOD"   envDefault:"2s"`
}

// LoggingConfig contains logging configuration
type LoggingConfig struct {
	Level  string `yaml:"level"  env:"LEVEL"  envDefault:"info"`
	Format string `yaml:"format" env:"FORMAT" envDefault:"json"`
	Debug  bool   `yaml:"debug"  env:"DEBUG"  envDefault:"false"`
}

// ToLoggerOptions converts LoggingConfig to logger initialization options
func (c *LoggingConfig) ToLoggerOptions() (debug bool, level, format string) {
	return c.Debug, c.Level, c.Format
}

// PerformanceConfig contains performance tuning configuration
type PerformanceConfig struct {
	InformerResyncPeriod  time.Duration `yaml:"informerResyncPeriod"  env:"INFORMER_RESYNC_PERIOD"  envDefault:"10m"`
	Workers               int           `yaml:"workers"               env:"WORKERS"                 envDefault:"10"`
	MetricsUpdateInterval time.Duration `yaml:"metricsUpdateInterval" env:"METRICS_UPDATE_INTERVAL" envDefault:"1m"`
}

// ConfigLoader loads configuration from multiple sources with proper precedence
type ConfigLoader struct {
	configFile string
	envFile    string
}

// NewConfigLoader creates a new configuration loader
func NewConfigLoader(configFile, envFile string) *ConfigLoader {
	return &ConfigLoader{
		configFile: configFile,
		envFile:    envFile,
	}
}

// Load loads configuration with precedence: CLI flags > ENV vars > Config file > Defaults
func (l *ConfigLoader) Load() (*GlobalConfig, error) {
	// Step 1: Load .env file if specified (lowest priority, before system env)
	if l.envFile != "" {
		if err := godotenv.Load(l.envFile); err != nil {
			log.WithFields(log.Fields{
				"file":  l.envFile,
				"error": err,
			}).Debug("No .env file loaded")
		} else {
			log.WithField("file", l.envFile).Info("Loaded environment from .env file")
		}
	}

	// Step 2: Start with defaults (built into struct tags via envDefault)
	cfg := &GlobalConfig{
		EnabledCollectors: []string{"cert", "domain", "node", "pod", "event", "imagepull"},
	}

	// Step 3: Load from YAML config file if specified
	if l.configFile != "" {
		if err := l.loadFromYAML(cfg); err != nil {
			return nil, fmt.Errorf("failed to load config from YAML: %w", err)
		}
	}

	// Step 4: Override with environment variables (highest priority)
	if err := env.Parse(cfg); err != nil {
		return nil, fmt.Errorf("failed to parse environment variables: %w", err)
	}

	log.WithFields(log.Fields{
		"server":         cfg.Server.Address,
		"collectors":     cfg.EnabledCollectors,
		"leaderElection": cfg.LeaderElection.Enabled,
	}).Info("Configuration loaded")

	return cfg, nil
}

// loadFromYAML loads configuration from YAML file
func (l *ConfigLoader) loadFromYAML(cfg *GlobalConfig) error {
	data, err := os.ReadFile(l.configFile)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return fmt.Errorf("failed to unmarshal YAML: %w", err)
	}

	log.WithField("file", l.configFile).Info("Configuration loaded from YAML")

	return nil
}

// Validate validates the global configuration
func (c *GlobalConfig) Validate() error {
	if c.Server.Address == "" {
		return errors.New("server.address cannot be empty")
	}

	if c.Metrics.Namespace == "" {
		return errors.New("metrics.namespace cannot be empty")
	}

	if c.LeaderElection.Enabled {
		if c.LeaderElection.Namespace == "" {
			return errors.New("leaderElection.namespace required when enabled")
		}

		if c.LeaderElection.RenewDeadline >= c.LeaderElection.LeaseDuration {
			return errors.New("leaderElection.renewDeadline must be less than leaseDuration")
		}
	}

	validLevels := map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
	if !validLevels[c.Logging.Level] {
		return fmt.Errorf("invalid logging.level: %s", c.Logging.Level)
	}

	return nil
}
