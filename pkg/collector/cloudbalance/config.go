package cloudbalance

import (
	"time"
)

// CloudProvider defines supported cloud providers
type CloudProvider string

const (
	AliCloud     CloudProvider = "alicloud"
	VolcEngine   CloudProvider = "volcengine"
	TencentCloud CloudProvider = "tencentcloud"
)

// AccountConfig holds configuration for a single cloud account
type AccountConfig struct {
	Provider        CloudProvider `yaml:"provider"        json:"provider"`
	AccountID       string        `yaml:"accountId"       json:"account_id"`
	AccessKeyID     string        `yaml:"accessKeyId"     json:"access_key_id"`
	AccessKeySecret string        `yaml:"accessKeySecret" json:"access_key_secret"`
	RegionID        string        `yaml:"regionId"        json:"region_id"`
}

// Config contains configuration for the CloudBalance collector
type Config struct {
	Accounts      []AccountConfig `yaml:"accounts"      env:"ACCOUNTS"       json:"accounts"`
	CheckInterval time.Duration   `yaml:"checkInterval" env:"CHECK_INTERVAL" json:"check_interval"`
}

// NewDefaultConfig returns the default configuration for CloudBalance collector
func NewDefaultConfig() *Config {
	return &Config{
		Accounts:      []AccountConfig{},
		CheckInterval: 5 * time.Minute,
	}
}
