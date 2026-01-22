package node

import (
	"time"
)

// Config contains configuration for the Node collector
type Config struct {
	Enabled               bool          `yaml:"enabled"               env:"ENABLED"`
	IncludeLabels         bool          `yaml:"includeLabels"         env:"INCLUDE_LABELS"`
	IncludeTaints         bool          `yaml:"includeTaints"         env:"INCLUDE_TAINTS"`
	IgnoreNewNodeDuration time.Duration `yaml:"ignoreNewNodeDuration" env:"IGNORE_NEW_NODE_DURATION"`
}

// NewDefaultConfig returns the default configuration for Node collector
// This function only returns hard-coded defaults without any env parsing
func NewDefaultConfig() *Config {
	return &Config{
		Enabled:               true,
		IncludeLabels:         true,
		IncludeTaints:         true,
		IgnoreNewNodeDuration: 30 * time.Minute,
	}
}

// Validate validates the configuration
func (c *Config) Validate() error {
	return nil
}
