package event

import (
	"errors"
	"time"

	"github.com/zijiren233/sealos-state-metric/pkg/collector"
	"github.com/zijiren233/sealos-state-metric/pkg/collector/base"
	"github.com/zijiren233/sealos-state-metric/pkg/registry"
	corev1 "k8s.io/api/core/v1"
)

const collectorName = "event"

func init() {
	registry.MustRegister(collectorName, NewCollector)
}

// NewCollector creates a new Event collector
func NewCollector(factoryCtx *collector.FactoryContext) (collector.Collector, error) {
	// 1. Start with hard-coded defaults
	cfg := NewDefaultConfig()

	// 2. Load configuration from ConfigLoader pipe (file -> env)
	// ConfigLoader is never nil and handles priority: defaults < file < env
	if err := factoryCtx.ConfigLoader.LoadModuleConfig("collectors.event", cfg); err != nil {
		factoryCtx.Logger.WithError(err).
			Debug("Failed to load event collector config, using defaults")
	}

	if !cfg.Enabled {
		return nil, errors.New("event collector is not enabled")
	}

	c := &Collector{
		BaseCollector: base.NewBaseCollector(
			collectorName,
			collector.TypeInformer,
			factoryCtx.Logger,
		),
		client: factoryCtx.Client,
		config: cfg,
		filter: NewEventFilter(3 * time.Minute), // Ignore events older than 3 minutes
		events: make(map[string]*corev1.Event),
		stopCh: make(chan struct{}),
		logger: factoryCtx.Logger,
	}

	// Initialize aggregator if enabled
	if cfg.Aggregator.Enabled {
		c.aggregator = NewEventAggregator(
			cfg.Aggregator.WindowSize,
			cfg.Aggregator.MaxEvents,
			factoryCtx.Logger,
		)
	}

	c.initMetrics(factoryCtx.MetricsNamespace)
	c.SetCollectFunc(c.collect)

	return c, nil
}
