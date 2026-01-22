package cloudbalance

import (
	"github.com/zijiren233/sealos-state-metric/pkg/collector"
	"github.com/zijiren233/sealos-state-metric/pkg/collector/base"
	"github.com/zijiren233/sealos-state-metric/pkg/registry"
)

const collectorName = "cloudbalance"

func init() {
	registry.MustRegister(collectorName, NewCollector)
}

// NewCollector creates a new CloudBalance collector
func NewCollector(factoryCtx *collector.FactoryContext) (collector.Collector, error) {
	// 1. Start with hard-coded defaults
	cfg := NewDefaultConfig()

	// 2. Load configuration from ConfigLoader pipe (file -> env)
	// ConfigLoader is never nil and handles priority: defaults < file < env
	if err := factoryCtx.ConfigLoader.LoadModuleConfig("collectors.cloudbalance", cfg); err != nil {
		factoryCtx.Logger.WithError(err).
			Debug("Failed to load cloudbalance collector config, using defaults")
	}

	c := &Collector{
		BaseCollector: base.NewBaseCollector(
			collectorName,
			collector.TypePolling,
			factoryCtx.Logger,
		),
		config:   cfg,
		balances: make(map[string]float64),
		logger:   factoryCtx.Logger,
	}

	c.initMetrics(factoryCtx.MetricsNamespace)
	c.SetCollectFunc(c.collect)

	return c, nil
}
