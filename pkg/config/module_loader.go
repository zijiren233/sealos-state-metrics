package config

import (
	"fmt"

	"github.com/mitchellh/mapstructure"
	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
)

// ModuleConfigLoader loads module-specific configuration from YAML content
type ModuleConfigLoader struct {
	configContent []byte
	tagName       string
	decodeHook    mapstructure.DecodeHookFunc
}

// ModuleLoaderOption is a function that configures ModuleConfigLoader
type ModuleLoaderOption func(*ModuleConfigLoader)

// WithModuleTagName sets the struct tag name to use for decoding
// Default is "yaml"
func WithModuleTagName(tagName string) ModuleLoaderOption {
	return func(l *ModuleConfigLoader) {
		l.tagName = tagName
	}
}

// WithModuleDecodeHook sets a custom decode hook function for type conversions
// Example: WithModuleDecodeHook(mapstructure.StringToTimeDurationHookFunc())
// For multiple hooks, use: WithModuleDecodeHook(mapstructure.ComposeDecodeHookFunc(hook1, hook2, ...))
func WithModuleDecodeHook(hook mapstructure.DecodeHookFunc) ModuleLoaderOption {
	return func(l *ModuleConfigLoader) {
		l.decodeHook = hook
	}
}

// NewModuleConfigLoader creates a new module config loader from content
func NewModuleConfigLoader(configContent []byte, opts ...ModuleLoaderOption) *ModuleConfigLoader {
	loader := &ModuleConfigLoader{
		configContent: configContent,
		tagName:       "yaml",
		decodeHook:    nil, // No default decode hook
	}

	for _, opt := range opts {
		opt(loader)
	}

	return loader
}

// LoadModuleConfig loads module-specific configuration from YAML content
// moduleKey is the path in YAML like "collectors.node"
// target is the struct to decode into
func (l *ModuleConfigLoader) LoadModuleConfig(moduleKey string, target any) error {
	if len(l.configContent) == 0 {
		log.WithField("module", moduleKey).
			Debug("No config content provided, skipping module config load")
		return nil
	}

	// Parse full config
	var fullConfig map[string]any
	if err := yaml.Unmarshal(l.configContent, &fullConfig); err != nil {
		return fmt.Errorf("failed to unmarshal YAML: %w", err)
	}

	// Navigate to module section (e.g., "collectors.node")
	moduleData, ok := navigateToKey(fullConfig, moduleKey)
	if !ok {
		log.WithField("module", moduleKey).Debug("Config key not found")
		return nil
	}

	// Decode using mapstructure
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		TagName:          l.tagName,
		Result:           target,
		DecodeHook:       l.decodeHook, // Apply decode hook for custom types (e.g., time.Duration)
		WeaklyTypedInput: true,         // Allow flexible basic type conversions
	})
	if err != nil {
		return fmt.Errorf("failed to create decoder: %w", err)
	}

	if err := decoder.Decode(moduleData); err != nil {
		return fmt.Errorf("failed to decode module config: %w", err)
	}

	log.WithFields(log.Fields{
		"module": moduleKey,
	}).Debug("Module config loaded")

	return nil
}

// navigateToKey navigates through nested map to find the value at the given key path
func navigateToKey(data map[string]any, key string) (map[string]any, bool) {
	keys := splitKey(key)
	current := data

	for _, k := range keys {
		next, exists := current[k]
		if !exists {
			return nil, false
		}

		// Navigate deeper
		m, ok := next.(map[string]any)
		if !ok {
			return nil, false
		}

		current = m
	}

	return current, true
}

// splitKey splits a dot-separated key like "collectors.node" into ["collectors", "node"]
func splitKey(key string) []string {
	if key == "" {
		return []string{}
	}

	result := []string{}

	current := ""
	for _, c := range key {
		if c == '.' {
			if current != "" {
				result = append(result, current)
				current = ""
			}
		} else {
			current += string(c)
		}
	}

	if current != "" {
		result = append(result, current)
	}

	return result
}
