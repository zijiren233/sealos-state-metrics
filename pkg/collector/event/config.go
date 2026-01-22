package event

import (
	"time"

	"github.com/caarlos0/env/v9"
)

// Config contains configuration for the Event collector
type Config struct {
	Enabled    bool             `yaml:"enabled"    env:"ENABLED"     envDefault:"true"`
	Namespaces []string         `yaml:"namespaces" env:"NAMESPACES"                             envSeparator:","`
	EventTypes []string         `yaml:"eventTypes" env:"EVENT_TYPES" envDefault:"Warning,Error" envSeparator:","`
	Aggregator AggregatorConfig `yaml:"aggregator"                                                               envPrefix:"AGGREGATOR_"`
}

// AggregatorConfig contains configuration for event aggregation
type AggregatorConfig struct {
	Enabled    bool          `yaml:"enabled"    env:"ENABLED"     envDefault:"true"`
	WindowSize time.Duration `yaml:"windowSize" env:"WINDOW_SIZE" envDefault:"10s"`
	MaxEvents  int           `yaml:"maxEvents"  env:"MAX_EVENTS"  envDefault:"1000"`
}

// NewDefaultConfig returns the default configuration for Event collector
func NewDefaultConfig() *Config {
	cfg := &Config{}
	_ = env.ParseWithOptions(cfg, env.Options{Prefix: "EVENT_COLLECTOR_"})
	return cfg
}
