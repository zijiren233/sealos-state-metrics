package event

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"
	"github.com/zijiren233/sealos-state-metric/pkg/collector/base"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

// Collector collects event metrics
type Collector struct {
	*base.BaseCollector

	client     kubernetes.Interface
	config     *Config
	informer   cache.SharedIndexInformer
	filter     *EventFilter
	aggregator *EventAggregator
	stopCh     chan struct{}
	logger     *log.Entry

	mu     sync.RWMutex
	events map[string]*corev1.Event // key: namespace/name

	// Metrics
	eventWarningTotal  *prometheus.Desc
	eventErrorTotal    *prometheus.Desc
	eventLastSeenTime  *prometheus.Desc
	eventUniqueObjects *prometheus.Desc
}

// initMetrics initializes Prometheus metric descriptors
func (c *Collector) initMetrics(namespace string) {
	c.eventWarningTotal = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "event", "warning_total"),
		"Total warning events (5-minute rolling window)",
		[]string{"namespace", "reason", "kind", "name"},
		nil,
	)
	c.eventErrorTotal = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "event", "error_total"),
		"Total error events",
		[]string{"namespace", "reason", "kind", "name"},
		nil,
	)
	c.eventLastSeenTime = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "event", "last_seen_timestamp"),
		"Last time the event was seen",
		[]string{"namespace", "reason", "kind"},
		nil,
	)
	c.eventUniqueObjects = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "event", "unique_objects"),
		"Number of unique objects affected by this event",
		[]string{"namespace", "reason"},
		nil,
	)

	// Register descriptors
	c.MustRegisterDesc(c.eventWarningTotal)
	c.MustRegisterDesc(c.eventErrorTotal)
	c.MustRegisterDesc(c.eventLastSeenTime)
	c.MustRegisterDesc(c.eventUniqueObjects)
}

// Start starts the collector
func (c *Collector) Start(ctx context.Context) error {
	if err := c.BaseCollector.Start(ctx); err != nil {
		return err
	}

	// Start aggregator if enabled
	if c.aggregator != nil {
		c.aggregator.Start()
	}

	// Create informer factory
	factory := informers.NewSharedInformerFactory(c.client, 10*time.Minute)

	// Create event informer
	c.informer = factory.Core().V1().Events().Informer()

	// Add event handlers
	//nolint:errcheck // AddEventHandler returns (registration, error) but error is always nil in client-go
	c.informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj any) {
			event := obj.(*corev1.Event) //nolint:errcheck // Type assertion is safe from informer
			if !c.filter.ShouldInclude(event) {
				return
			}

			key := eventKey(event)

			c.mu.Lock()
			c.events[key] = event.DeepCopy()
			c.mu.Unlock()

			if c.aggregator != nil {
				c.aggregator.AddEvent(event)
			}

			c.logger.WithFields(log.Fields{
				"event":  key,
				"reason": event.Reason,
				"type":   event.Type,
			}).Debug("Event added")
		},
		UpdateFunc: func(oldObj, newObj any) {
			event := newObj.(*corev1.Event) //nolint:errcheck // Type assertion is safe from informer
			if !c.filter.ShouldInclude(event) {
				return
			}

			key := eventKey(event)

			c.mu.Lock()
			c.events[key] = event.DeepCopy()
			c.mu.Unlock()

			if c.aggregator != nil {
				c.aggregator.AddEvent(event)
			}

			c.logger.WithFields(log.Fields{
				"event":  key,
				"reason": event.Reason,
				"count":  event.Count,
			}).Debug("Event updated")
		},
		DeleteFunc: func(obj any) {
			event := obj.(*corev1.Event) //nolint:errcheck // Type assertion is safe from informer
			key := eventKey(event)

			c.mu.Lock()
			delete(c.events, key)
			c.mu.Unlock()

			c.logger.WithField("event", key).Debug("Event deleted")
		},
	})

	// Start informer
	factory.Start(c.stopCh)

	// Wait for cache sync
	c.logger.Info("Waiting for event informer cache sync")

	if !cache.WaitForCacheSync(c.stopCh, c.informer.HasSynced) {
		return errors.New("failed to sync event informer cache")
	}

	c.logger.Info("Event collector started successfully")

	return nil
}

// Stop stops the collector
func (c *Collector) Stop() error {
	close(c.stopCh)

	if c.aggregator != nil {
		c.aggregator.Stop()
	}

	return c.BaseCollector.Stop()
}

// HasSynced returns true if the informer has synced
func (c *Collector) HasSynced() bool {
	return c.informer != nil && c.informer.HasSynced()
}

// collect collects metrics
func (c *Collector) collect(ch chan<- prometheus.Metric) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Collect individual event metrics
	for _, event := range c.events {
		if event.Type == corev1.EventTypeWarning {
			ch <- prometheus.MustNewConstMetric(
				c.eventWarningTotal,
				prometheus.CounterValue,
				float64(event.Count),
				event.Namespace,
				event.Reason,
				event.InvolvedObject.Kind,
				event.InvolvedObject.Name,
			)
		}

		// Track all non-normal events as potential errors
		if event.Type != corev1.EventTypeNormal {
			ch <- prometheus.MustNewConstMetric(
				c.eventErrorTotal,
				prometheus.CounterValue,
				float64(event.Count),
				event.Namespace,
				event.Reason,
				event.InvolvedObject.Kind,
				event.InvolvedObject.Name,
			)
		}
	}

	// Collect aggregated metrics
	if c.aggregator != nil {
		aggregated := c.aggregator.GetAggregated()
		for _, agg := range aggregated {
			// Last seen timestamp
			ch <- prometheus.MustNewConstMetric(
				c.eventLastSeenTime,
				prometheus.GaugeValue,
				float64(agg.LastSeen.Unix()),
				agg.Namespace,
				agg.Reason,
				agg.Kind,
			)

			// Unique objects count
			ch <- prometheus.MustNewConstMetric(
				c.eventUniqueObjects,
				prometheus.GaugeValue,
				float64(len(agg.UniqueObjects)),
				agg.Namespace,
				agg.Reason,
			)
		}
	}
}

// eventKey returns the key for an event
func eventKey(event *corev1.Event) string {
	return event.Namespace + "/" + event.Name
}
