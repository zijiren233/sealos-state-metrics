package config

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/caarlos0/env/v9"
	log "github.com/sirupsen/logrus"
)

// EnvConfigLoader loads configuration from environment variables
// It automatically derives the env prefix from the moduleKey
// For example: "collectors.node" -> "COLLECTORS_NODE_"
// If options.Prefix is set, it's prepended to the auto-derived prefix
// Example: options.Prefix="APP_", moduleKey="collectors.node" -> "APP_COLLECTORS_NODE_"
type EnvConfigLoader struct {
	options env.Options
}

// EnvLoaderOption is a function that configures EnvConfigLoader
type EnvLoaderOption func(*EnvConfigLoader)

// WithPrefix sets a prefix to prepend to the auto-derived moduleKey prefix
// Example: WithPrefix("APP_") + moduleKey="collectors.node" -> "APP_COLLECTORS_NODE_"
func WithPrefix(prefix string) EnvLoaderOption {
	return func(l *EnvConfigLoader) {
		l.options.Prefix = prefix
	}
}

// WithTagName sets the struct tag name to use for environment variable mapping
// Default is "env". You can use other tags like "envconfig", "yaml", etc.
func WithTagName(tagName string) EnvLoaderOption {
	return func(l *EnvConfigLoader) {
		l.options.TagName = tagName
	}
}

// WithEnvironment provides a custom environment map instead of os.Environ()
// Useful for testing or when you want to provide env vars from non-standard sources
func WithEnvironment(environment map[string]string) EnvLoaderOption {
	return func(l *EnvConfigLoader) {
		l.options.Environment = environment
	}
}

// WithUseFieldNames makes the loader use struct field names instead of tag names
func WithUseFieldNames(use bool) EnvLoaderOption {
	return func(l *EnvConfigLoader) {
		l.options.UseFieldNameByDefault = use
	}
}

// WithOnSet sets a function to be called when a field is set
func WithOnSet(onSet env.OnSetFn) EnvLoaderOption {
	return func(l *EnvConfigLoader) {
		l.options.OnSet = onSet
	}
}

// WithFuncMap sets a custom function map for parsing
func WithFuncMap(funcMap map[reflect.Type]env.ParserFunc) EnvLoaderOption {
	return func(l *EnvConfigLoader) {
		l.options.FuncMap = funcMap
	}
}

// NewEnvConfigLoader creates a new environment variable config loader
func NewEnvConfigLoader(opts ...EnvLoaderOption) *EnvConfigLoader {
	loader := &EnvConfigLoader{
		options: env.Options{
			TagName: "env", // default tag name
		},
	}

	for _, opt := range opts {
		opt(loader)
	}

	return loader
}

// LoadModuleConfig loads configuration from environment variables
// It derives the prefix from moduleKey by replacing dots with underscores:
//   - moduleKey="collectors.node" -> "COLLECTORS_NODE_"
//   - options.Prefix="APP_" + moduleKey="collectors.node" -> "APP_COLLECTORS_NODE_"
func (l *EnvConfigLoader) LoadModuleConfig(moduleKey string, target any) error {
	// Copy options to avoid modifying the original
	opts := l.options

	// Auto-derive prefix from moduleKey
	derivedPrefix := strings.ToUpper(strings.ReplaceAll(moduleKey, ".", "_")) + "_"

	// Prepend custom prefix if provided
	if opts.Prefix != "" {
		opts.Prefix += derivedPrefix
	} else {
		opts.Prefix = derivedPrefix
	}

	if err := env.ParseWithOptions(target, opts); err != nil {
		return fmt.Errorf(
			"failed to parse environment variables with prefix %s: %w",
			opts.Prefix,
			err,
		)
	}

	log.WithFields(log.Fields{
		"module": moduleKey,
		"prefix": opts.Prefix,
	}).Debug("Module config loaded from environment variables")

	return nil
}
