package pod

import (
	"time"
)

// Config contains configuration for the Pod collector
type Config struct {
	Enabled           bool             `yaml:"enabled"           env:"ENABLED"`
	Namespaces        []string         `yaml:"namespaces"        env:"NAMESPACES"         envSeparator:","`
	AbnormalThreshold time.Duration    `yaml:"abnormalThreshold" env:"ABNORMAL_THRESHOLD"`
	RestartThreshold  int              `yaml:"restartThreshold"  env:"RESTART_THRESHOLD"`
	Aggregator        AggregatorConfig `yaml:"aggregator"                                                  envPrefix:"AGGREGATOR_"`
}

// AggregatorConfig contains configuration for pod aggregation
type AggregatorConfig struct {
	Enabled    bool          `yaml:"enabled"    env:"ENABLED"`
	WindowSize time.Duration `yaml:"windowSize" env:"WINDOW_SIZE"`
	MaxEvents  int           `yaml:"maxEvents"  env:"MAX_EVENTS"`
}

// NewDefaultConfig returns the default configuration for Pod collector
// This function only returns hard-coded defaults without any env parsing
func NewDefaultConfig() *Config {
	return &Config{
		Enabled:           true,
		Namespaces:        []string{},
		AbnormalThreshold: 5 * time.Minute,
		RestartThreshold:  5,
		Aggregator: AggregatorConfig{
			Enabled:    true,
			WindowSize: 10 * time.Second,
			MaxEvents:  1000,
		},
	}
}
