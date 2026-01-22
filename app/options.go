package app

import (
	"flag"
	"os"
	"strings"
)

// Options contains command-line options for the application
type Options struct {
	ConfigFile         string
	EnvFile            string
	KubeconfigFile     string
	EnabledCollectors  string
	LeaderElectionMode bool
	LogLevel           string
	MetricsAddress     string
}

// NewOptions creates a new Options instance with default values
func NewOptions() *Options {
	return &Options{
		ConfigFile:         "",
		EnvFile:            ".env",
		KubeconfigFile:     "",
		EnabledCollectors:  "cert,domain,node,pod,event,imagepull",
		LeaderElectionMode: true,
		LogLevel:           "info",
		MetricsAddress:     ":9090",
	}
}

// AddFlags adds flags to the flag set
func (o *Options) AddFlags(fs *flag.FlagSet) {
	fs.StringVar(&o.ConfigFile, "config", o.ConfigFile,
		"Path to configuration file (YAML format). If not specified, defaults will be used.")

	fs.StringVar(&o.EnvFile, "env-file", o.EnvFile,
		"Path to .env file for environment variables. Defaults to .env in current directory.")

	fs.StringVar(&o.KubeconfigFile, "kubeconfig", o.KubeconfigFile,
		"Path to kubeconfig file. Leave empty to use in-cluster configuration.")

	fs.StringVar(&o.EnabledCollectors, "collectors", o.EnabledCollectors,
		"Comma-separated list of enabled collectors (cert,domain,node,pod,event,imagepull).")

	fs.BoolVar(&o.LeaderElectionMode, "leader-election", o.LeaderElectionMode,
		"Enable leader election for high availability deployment.")

	fs.StringVar(&o.LogLevel, "log-level", o.LogLevel,
		"Log level (debug, info, warn, error).")

	fs.StringVar(&o.MetricsAddress, "metrics-address", o.MetricsAddress,
		"Address to listen on for metrics endpoint.")
}

// ParsedEnabledCollectors returns the list of enabled collectors
func (o *Options) ParsedEnabledCollectors() []string {
	if o.EnabledCollectors == "" {
		return []string{}
	}

	collectors := strings.Split(o.EnabledCollectors, ",")

	result := make([]string, 0, len(collectors))
	for _, c := range collectors {
		c = strings.TrimSpace(c)
		if c != "" {
			result = append(result, c)
		}
	}

	return result
}

// GetPodName returns the pod name from the environment variable
func GetPodName() string {
	return os.Getenv("POD_NAME")
}

// GetPodNamespace returns the pod namespace from the environment variable
func GetPodNamespace() string {
	ns := os.Getenv("POD_NAMESPACE")
	if ns == "" {
		ns = "default"
	}

	return ns
}
