package pod

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

// Collector collects pod metrics
type Collector struct {
	*base.BaseCollector

	client     kubernetes.Interface
	config     *Config
	informer   cache.SharedIndexInformer
	aggregator *PodAggregator
	stopCh     chan struct{}
	logger     *log.Entry

	mu   sync.RWMutex
	pods map[string]*corev1.Pod // key: namespace/name

	// Metrics
	podStatusPhase      *prometheus.Desc
	podRestartTotal     *prometheus.Desc
	podCondition        *prometheus.Desc
	podOOMKilledTotal   *prometheus.Desc
	podAbnormalDuration *prometheus.Desc
	podAggregatedCount  *prometheus.Desc
}

// initMetrics initializes Prometheus metric descriptors
func (c *Collector) initMetrics(namespace string) {
	c.podStatusPhase = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "pod", "status_phase"),
		"Pod phase",
		[]string{"namespace", "pod", "phase"},
		nil,
	)
	c.podRestartTotal = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "pod", "restart_total"),
		"Total container restarts",
		[]string{"namespace", "pod", "container"},
		nil,
	)
	c.podCondition = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "pod", "condition"),
		"Pod condition status",
		[]string{"namespace", "pod", "condition", "status"},
		nil,
	)
	c.podOOMKilledTotal = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "pod", "oom_killed_total"),
		"Total OOM killed containers",
		[]string{"namespace", "pod", "container"},
		nil,
	)
	c.podAbnormalDuration = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "pod", "abnormal_duration_seconds"),
		"Duration the pod has been abnormal",
		[]string{"namespace", "pod"},
		nil,
	)
	c.podAggregatedCount = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "pod", "aggregated_count"),
		"Number of pods in this phase (aggregated)",
		[]string{"namespace", "phase"},
		nil,
	)

	// Register descriptors
	c.MustRegisterDesc(c.podStatusPhase)
	c.MustRegisterDesc(c.podRestartTotal)
	c.MustRegisterDesc(c.podCondition)
	c.MustRegisterDesc(c.podOOMKilledTotal)
	c.MustRegisterDesc(c.podAbnormalDuration)
	c.MustRegisterDesc(c.podAggregatedCount)
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
	// TODO: Support filtering by namespaces
	factory := informers.NewSharedInformerFactory(c.client, 10*time.Minute)

	// Create pod informer
	c.informer = factory.Core().V1().Pods().Informer()

	// Add event handlers
	//nolint:errcheck // AddEventHandler returns (registration, error) but error is always nil in client-go
	c.informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj any) {
			pod := obj.(*corev1.Pod) //nolint:errcheck // Type assertion is safe from informer
			key := podKey(pod)

			c.mu.Lock()
			c.pods[key] = pod.DeepCopy()
			c.mu.Unlock()

			if c.aggregator != nil {
				c.aggregator.AddPod(pod)
			}

			c.logger.WithFields(log.Fields{
				"pod":   key,
				"phase": pod.Status.Phase,
			}).Debug("Pod added")
		},
		UpdateFunc: func(oldObj, newObj any) {
			pod := newObj.(*corev1.Pod) //nolint:errcheck // Type assertion is safe from informer
			key := podKey(pod)

			c.mu.Lock()
			c.pods[key] = pod.DeepCopy()
			c.mu.Unlock()

			if c.aggregator != nil {
				c.aggregator.AddPod(pod)
			}

			c.logger.WithFields(log.Fields{
				"pod":   key,
				"phase": pod.Status.Phase,
			}).Debug("Pod updated")
		},
		DeleteFunc: func(obj any) {
			pod := obj.(*corev1.Pod) //nolint:errcheck // Type assertion is safe from informer
			key := podKey(pod)

			c.mu.Lock()
			delete(c.pods, key)
			c.mu.Unlock()

			if c.aggregator != nil {
				c.aggregator.RemovePod(pod)
			}

			c.logger.WithField("pod", key).Debug("Pod deleted")
		},
	})

	// Start informer
	factory.Start(c.stopCh)

	// Wait for cache sync
	c.logger.Info("Waiting for pod informer cache sync")

	if !cache.WaitForCacheSync(c.stopCh, c.informer.HasSynced) {
		return errors.New("failed to sync pod informer cache")
	}

	c.logger.Info("Pod collector started successfully")

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

	// Collect detailed metrics for each pod
	for _, pod := range c.pods {
		// Pod phase
		ch <- prometheus.MustNewConstMetric(
			c.podStatusPhase,
			prometheus.GaugeValue,
			1,
			pod.Namespace,
			pod.Name,
			string(pod.Status.Phase),
		)

		// Pod conditions
		for _, condition := range pod.Status.Conditions {
			status := "unknown"
			switch condition.Status {
			case corev1.ConditionTrue:
				status = "true"
			case corev1.ConditionFalse:
				status = "false"
			}

			ch <- prometheus.MustNewConstMetric(
				c.podCondition,
				prometheus.GaugeValue,
				boolToFloat64(condition.Status == corev1.ConditionTrue),
				pod.Namespace,
				pod.Name,
				string(condition.Type),
				status,
			)
		}

		// Container restarts and OOM kills
		for _, containerStatus := range pod.Status.ContainerStatuses {
			// Restart count
			if int(containerStatus.RestartCount) >= c.config.RestartThreshold {
				ch <- prometheus.MustNewConstMetric(
					c.podRestartTotal,
					prometheus.CounterValue,
					float64(containerStatus.RestartCount),
					pod.Namespace,
					pod.Name,
					containerStatus.Name,
				)
			}

			// OOM kills
			if containerStatus.LastTerminationState.Terminated != nil {
				if containerStatus.LastTerminationState.Terminated.Reason == "OOMKilled" {
					ch <- prometheus.MustNewConstMetric(
						c.podOOMKilledTotal,
						prometheus.CounterValue,
						1,
						pod.Namespace,
						pod.Name,
						containerStatus.Name,
					)
				}
			}
		}

		// Abnormal duration
		if c.aggregator != nil {
			duration := c.aggregator.GetAbnormalDuration(pod.Namespace, pod.Name)
			if duration > c.config.AbnormalThreshold {
				ch <- prometheus.MustNewConstMetric(
					c.podAbnormalDuration,
					prometheus.GaugeValue,
					duration.Seconds(),
					pod.Namespace,
					pod.Name,
				)
			}
		}
	}

	// Collect aggregated metrics
	if c.aggregator != nil {
		aggregated := c.aggregator.GetAggregated()
		for _, agg := range aggregated {
			ch <- prometheus.MustNewConstMetric(
				c.podAggregatedCount,
				prometheus.GaugeValue,
				float64(agg.Count),
				agg.Namespace,
				string(agg.Phase),
			)
		}
	}
}

// podKey returns the key for a pod
func podKey(pod *corev1.Pod) string {
	return pod.Namespace + "/" + pod.Name
}

// boolToFloat64 converts a boolean to a float64
func boolToFloat64(b bool) float64 {
	if b {
		return 1.0
	}
	return 0.0
}
