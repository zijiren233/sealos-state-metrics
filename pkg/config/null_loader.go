package config

// NullConfigLoader is a no-op implementation of ConfigLoader
// Used as a fallback when no config file is provided
type NullConfigLoader struct{}

// NewNullConfigLoader creates a new null config loader
func NewNullConfigLoader() *NullConfigLoader {
	return &NullConfigLoader{}
}

// LoadModuleConfig does nothing and returns nil
func (l *NullConfigLoader) LoadModuleConfig(moduleKey string, target any) error {
	return nil
}
