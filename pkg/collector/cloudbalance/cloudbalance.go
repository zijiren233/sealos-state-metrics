package cloudbalance

import (
	"context"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"
	"github.com/zijiren233/sealos-state-metric/pkg/collector/base"
)

// Collector implements cloud balance monitoring
type Collector struct {
	*base.BaseCollector
	config *Config
	logger *log.Entry

	// Prometheus metrics
	balanceGauge *prometheus.Desc

	// Internal state
	mu       sync.RWMutex
	balances map[string]float64 // key: provider:accountID
}

// initMetrics initializes Prometheus metric descriptors
func (c *Collector) initMetrics(namespace string) {
	c.balanceGauge = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "cloudbalance", "balance"),
		"Current balance for each cloud account",
		[]string{"provider", "account_id"},
		nil,
	)

	// Register descriptor
	c.MustRegisterDesc(c.balanceGauge)
}

// Start starts the collector
func (c *Collector) Start(ctx context.Context) error {
	if err := c.BaseCollector.Start(ctx); err != nil {
		return err
	}

	// Start background polling
	go c.pollLoop()

	c.logger.Info("CloudBalance collector started successfully")

	return nil
}

// Stop stops the collector
func (c *Collector) Stop() error {
	// BaseCollector.Stop() will cancel the context,
	// which will cause pollLoop to exit via Context().Done()
	return c.BaseCollector.Stop()
}

// HasSynced returns true (polling collector is always synced)
func (c *Collector) HasSynced() bool {
	return true
}

// Interval returns the polling interval
func (c *Collector) Interval() time.Duration {
	return c.config.CheckInterval
}

// pollLoop periodically queries cloud balances
func (c *Collector) pollLoop() {
	// Initial poll
	_ = c.Poll(c.Context())
	c.SetReady(true)

	ticker := time.NewTicker(c.config.CheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := c.Poll(c.Context()); err != nil {
				c.logger.WithError(err).Error("Failed to poll cloud balances")
			}
		case <-c.Context().Done():
			c.logger.Info("Context cancelled, stopping cloud balance poll loop")
			return
		}
	}
}

// Poll queries all configured cloud accounts
func (c *Collector) Poll(ctx context.Context) error {
	if len(c.config.Accounts) == 0 {
		c.logger.Debug("No cloud accounts configured for monitoring")
		return nil
	}

	c.logger.WithField("count", len(c.config.Accounts)).Info("Starting cloud balance checks")

	c.mu.Lock()
	defer c.mu.Unlock()

	for _, account := range c.config.Accounts {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		balance, err := QueryBalance(account)
		if err != nil {
			c.logger.WithFields(log.Fields{
				"provider":   account.Provider,
				"account_id": account.AccountID,
			}).WithError(err).Error("Failed to query cloud balance")
			continue
		}

		key := string(account.Provider) + ":" + account.AccountID
		c.balances[key] = balance

		c.logger.WithFields(log.Fields{
			"provider":   account.Provider,
			"account_id": account.AccountID,
			"balance":    balance,
		}).Debug("Cloud balance updated")
	}

	return nil
}

// collect implements the collect method for Prometheus
func (c *Collector) collect(ch chan<- prometheus.Metric) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for _, account := range c.config.Accounts {
		key := string(account.Provider) + ":" + account.AccountID
		balance, exists := c.balances[key]
		if !exists {
			continue
		}

		ch <- prometheus.MustNewConstMetric(
			c.balanceGauge,
			prometheus.GaugeValue,
			balance,
			string(account.Provider),
			account.AccountID,
		)
	}
}
